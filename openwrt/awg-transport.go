package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Persist the latch, not monotonic timestamps: one session refresh per failed
// episode/network. Restarting the UI/controller must not start a refresh loop.
type awgTransportRecovery struct {
	Identity        string `json:"identity"`
	Attempted       bool   `json:"attempted"`
	initialized     bool
	lastRX, firstTX uint64
	lastProgress    time.Time
	stagnant, good  int
}

type awgTransportSample struct {
	Local, Ready, Alternative, Inactive bool
	RX, TX                              uint64
	HandshakeAge                        *int64
}

func (r *awgTransportRecovery) check(now time.Time, identity string, window time.Duration, s awgTransportSample) bool {
	if r.Identity != identity {
		*r = awgTransportRecovery{Identity: identity}
	}
	if !r.initialized || s.RX != r.lastRX || !s.Local {
		r.initialized = true
		r.lastRX, r.firstTX, r.lastProgress, r.stagnant = s.RX, s.TX, now, 0
	} else {
		r.stagnant++
	}
	if s.Ready && s.Local {
		r.good++
		r.lastProgress, r.stagnant = now, 0
		if r.good >= 5 {
			r.Attempted = false
		}
		return false
	}
	r.good = 0
	if r.Attempted || !s.Local || !s.Inactive || !s.Alternative || r.stagnant < 3 ||
		s.TX <= r.firstTX || now.Sub(r.lastProgress) < window {
		return false
	}
	// A current handshake means the transport is still exchanging authenticated
	// packets. DNS/TLS/IPv6-only probe trouble must never reset its session.
	if s.HandshakeAge != nil && *s.HandshakeAge < int64(window/time.Second) {
		return false
	}
	r.Attempted = true
	return true
}

func awgTransportOptions(raw string) (time.Duration, uint16) {
	reject, retry, attempts := int64(180), int64(5), int64(18)
	var port uint16
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if key == "ListenPort" {
			n, err := strconv.ParseUint(value, 10, 16)
			if err == nil {
				port = uint16(n)
			}
		}
		rangeParts := strings.Split(value, "-")
		n, err := strconv.ParseInt(strings.TrimSpace(rangeParts[len(rangeParts)-1]), 10, 64)
		if err != nil || n <= 0 || n > 86400 {
			continue
		}
		switch key {
		case "RejectAfterTime":
			reject = n
		case "RekeyTimeout":
			retry = n
		case "MaxHandshakeAttempts":
			attempts = n
		}
	}
	// Wait for key expiry and the configured handshake retry budget, not a
	// single failed ping. Bounds only limit unreasonable/custom profiles.
	// AWG retries include up to 334 ms jitter and a final retry before expiry.
	seconds := reject + ((retry*1000+334)*(attempts+2)+999)/1000
	if seconds < 30 {
		seconds = 30
	}
	if seconds > 900 {
		seconds = 900
	}
	return time.Duration(seconds) * time.Second, port
}

func awgSessionRecoveryPlan(raw string) (string, error) {
	request, _, _, err := awgPlan(raw)
	if err != nil {
		return "", err
	}
	if !strings.Contains(request, "\nlisten_port=") {
		request = strings.Replace(request, "set=1\n", "set=1\nlisten_port=0\n", 1)
	}
	return request, nil
}

func loadAWGTransportRecovery() awgTransportRecovery {
	var r awgTransportRecovery
	b, _ := os.ReadFile(moduleDir + "/awg-transport-recovery.json")
	json.Unmarshal(b, &r)
	return r
}

func recoverAWGTransport(r *awgTransportRecovery, c RouterConfig, healthy map[string]bool, a map[string]any) {
	if !profileIDPattern.MatchString(c.AWG) {
		return
	}
	raw, err := os.ReadFile(filepath.Join(profileRoot, c.AWG+".conf"))
	if err != nil {
		return
	}
	window, port := awgTransportOptions(string(raw))
	uplink, err := os.ReadFile(moduleDir + "/uplink-state")
	if err != nil || len(strings.TrimSpace(string(uplink))) == 0 {
		return
	}
	hash := sha256.Sum256(append(append([]byte(c.AWG+":"), raw...), uplink...))
	identity := hex.EncodeToString(hash[:16])
	rx, rxOK := a["rx_bytes"].(uint64)
	tx, txOK := a["tx_bytes"].(uint64)
	var age *int64
	if n, ok := a["handshake_age_seconds"].(int64); ok {
		age = &n
	}
	alternative := healthy["mihomo"]
	if c.Primary == "direct" {
		if d, ok := linkSnapshot()["direct"].(map[string]any); ok {
			checked, ok := d["checked_at"].(int64)
			age := time.Now().Unix() - checked
			alternative = alternative || (ok && age >= 0 && age <= 90 && d["state"] == "verified")
		}
	}
	previousIdentity, previousAttempt := r.Identity, r.Attempted
	refresh := r.check(time.Now(), identity, window, awgTransportSample{
		Local: a["running"] == true && a["configured"] == true && rxOK && txOK,
		Ready: healthy["awg"], Alternative: alternative, Inactive: c.Primary != "awg",
		RX: rx, TX: tx, HandshakeAge: age,
	})
	if r.Identity != previousIdentity || r.Attempted != previousAttempt {
		b, _ := json.Marshal(r)
		if atomicPrivate(moduleDir+"/awg-transport-recovery.json", b) != nil {
			return
		}
	}
	if !refresh {
		return
	}
	// Recreate only the failed peer session with its existing credentials. A
	// port-only rebind did not recover the reproduced dead UDP data path.
	// UAPI preserves the core process, TUN addresses, routes and other engines.
	request, err := awgSessionRecoveryPlan(string(raw))
	if err == nil {
		_, err = awgUAPI(request)
	}
	recordHealthEvent("awg_session_recovery", map[string]any{
		"reason": "no_authenticated_progress", "method": "same_profile_new_session", "command_ok": err == nil,
		"window_seconds": int64(window / time.Second), "fixed_port": port != 0,
		"handshake_age": a["handshake_age_seconds"], "rx": rx, "tx": tx,
	})
}
