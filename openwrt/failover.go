package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Preferred is user intent; Primary is the route actually installed. Automatic
// decisions never select direct and never overwrite the user's preference.
type failoverState struct {
	key        string
	good, bad  map[string]int
	lastSwitch time.Time
}

func (s *failoverState) choose(now time.Time, c RouterConfig, healthy map[string]bool) (string, string) {
	key := c.Preferred + ":" + c.AWG + ":" + c.Mihomo
	if s.key != key {
		*s = failoverState{key: key, good: map[string]int{}, bad: map[string]int{}}
	}
	if c.Preferred == "direct" {
		return "direct", "manual_direct"
	}
	if c.Preferred != "awg" && c.Preferred != "mihomo" {
		return "blocked", "no_vpn_selected"
	}
	for _, e := range []string{"awg", "mihomo"} {
		if healthy[e] {
			s.good[e]++
			s.bad[e] = 0
		} else {
			s.good[e] = 0
			s.bad[e]++
		}
	}
	current := c.Primary
	if current == "awg" || current == "mihomo" {
		if s.bad[current] < 3 {
			if current != c.Preferred && s.good[c.Preferred] >= 5 && now.Sub(s.lastSwitch) >= 60*time.Second {
				return c.Preferred, "preferred_recovered"
			}
			return current, ""
		}
		// Once the active path has failed three times, a fresh successful dual-stack
		// probe is enough to use the other VPN. Waiting for a second success here
		// unnecessarily blocked clients even though a working VPN was available.
		if reserve := otherVPN(current); healthy[reserve] {
			return reserve, "verified_reserve"
		}
	}
	for _, e := range []string{c.Preferred, otherVPN(c.Preferred)} {
		if s.good[e] >= 2 {
			return e, "healthy_reserve"
		}
	}
	return "blocked", "vpn_unavailable"
}

func otherVPN(e string) string {
	if e == "awg" {
		return "mihomo"
	}
	return "awg"
}

// These requests are bound to the assigned VPN, not the Android default route.
// A recent handshake alone cannot prove a working data path. Literal addresses
// also avoid making external bootstrap DNS a failover dependency.
type tunnelFamilyProbe struct {
	Ready     bool     `json:"ready"`
	Millis    int64    `json:"millis"`
	Attempts  int      `json:"attempts"`
	ExitCodes []int    `json:"exit_codes,omitempty"`
	HTTP      []string `json:"http,omitempty"`
	Budgets   []int64  `json:"budgets_ms,omitempty"`
}

func engineFamilyProbe(engine string, ipv6 bool, timing *probeTiming) tunnelFamilyProbe {
	started := time.Now()
	result := tunnelFamilyProbe{}
	// Pin the fallback to a separate provider without bootstrap DNS. A failed
	// Cloudflare endpoint alone must not be mistaken for a failed VPN.
	targets := []string{"https://1.1.1.1/cdn-cgi/trace", "https://dns.google/resolve?name=example.com&type=A"}
	google := "dns.google:443:8.8.8.8"
	if ipv6 {
		targets[0] = "https://[2606:4700:4700::1111]/cdn-cgi/trace"
		google = "dns.google:443:[2001:4860:4860::8888]"
	}
	for i, target := range targets {
		result.Attempts++
		budget := timing.budget()
		result.Budgets = append(result.Budgets, budget.Milliseconds())
		args := []string{"curl", "--silent", "--fail", "--max-time", strconv.FormatFloat(budget.Seconds(), 'f', 3, 64), "--connect-timeout", strconv.FormatFloat(budget.Seconds(), 'f', 3, 64), "--max-filesize", "4096", "--output", "/dev/null", "--write-out", "%{http_code}|%{time_total}"}
		device := "awg0"
		if engine == "mihomo" {
			device = "dwr-vpn"
		}
		family := "-4"
		if ipv6 {
			family = "-6"
		}
		args = append(args, family, "--interface", device)
		if i == 1 {
			args = append(args, "--resolve", google)
		}
		out, err := containerCommand(append(args, target)...)
		fields := strings.Split(out, "|")
		httpCode := fields[0]
		elapsed := 0.0
		if len(fields) == 2 {
			elapsed, _ = strconv.ParseFloat(fields[1], 64)
		}
		result.Millis = time.Since(started).Milliseconds()
		result.HTTP = append(result.HTTP, httpCode)
		success := err == nil && httpCode == "200"
		code := 0
		if err != nil {
			code = -1
			if e, ok := err.(*exec.ExitError); ok {
				code = e.ExitCode()
			}
			result.ExitCodes = append(result.ExitCodes, code)
		}
		timing.observe(time.Duration(elapsed*float64(time.Second)), success && elapsed > 0, code == 28)
		if success {
			result.Ready = true
			return result
		}
	}
	return result
}

// Called under routerLock, including manual selection. Workers own distinct
// timing objects and join before the assessment is read by telemetry.
var timingIdentity string
var tunnelTimings = map[string]*probeTiming{}
var tunnelAssessments = map[string]*pathAssessment{}

func tunnelHealth(c RouterConfig) map[string]bool {
	uplink, _ := os.ReadFile(moduleDir + "/uplink-state")
	identity := c.AWG + ":" + c.Mihomo + ":" + strings.TrimSpace(string(uplink))
	if identity != timingIdentity {
		timingIdentity = identity
		tunnelTimings = map[string]*probeTiming{}
		tunnelAssessments = map[string]*pathAssessment{}
	}
	for _, key := range []string{"awg", "awg6", "mihomo", "mihomo6"} {
		if tunnelTimings[key] == nil {
			tunnelTimings[key] = &probeTiming{}
		}
	}
	local := map[string]bool{}

	var wg sync.WaitGroup
	var mu sync.Mutex
	result := map[string]tunnelFamilyProbe{}
	for _, engine := range []string{"awg", "mihomo"} {
		configured := (engine == "awg" && c.AWG != "") || (engine == "mihomo" && c.Mihomo != "")
		if configured && engine == "awg" {
			h := awgHealth()
			configured = h["running"] == true && h["configured"] == true
		}
		if configured && engine == "mihomo" {
			configured = mihomoControlReady()
		}
		local[engine] = configured
		if !configured {
			continue
		}
		for _, ipv6 := range []bool{false, true} {
			wg.Add(1)
			go func(e string, v6 bool) {
				defer wg.Done()
				key := e
				if v6 {
					key += "6"
				}
				probe := engineFamilyProbe(e, v6, tunnelTimings[key])
				mu.Lock()
				result[key] = probe
				mu.Unlock()
			}(engine, ipv6)
		}
	}
	wg.Wait()
	for _, e := range []string{"awg", "mihomo"} {
		if tunnelAssessments[e] == nil {
			tunnelAssessments[e] = &pathAssessment{}
		}
		tunnelAssessments[e].observe(local[e], result[e].Ready, result[e+"6"].Ready, result[e].Attempts > 1 || result[e+"6"].Attempts > 1)
	}
	recordTunnelSample(c, local, result)

	// Bounded diagnostics distinguish IPv4/IPv6 timeouts, TLS errors and local
	// core loss. No profile contents, keys or subscription URLs enter this file.
	degraded := (c.AWG != "" && (!result["awg"].Ready || !result["awg6"].Ready)) || (c.Mihomo != "" && (!result["mihomo"].Ready || !result["mihomo6"].Ready))
	for _, p := range result {
		degraded = degraded || p.Attempts > 1
	}
	if degraded {
		var history []json.RawMessage
		path := moduleDir + "/tunnel-failures.json"
		if b, err := os.ReadFile(path); err == nil && len(b) < 65536 {
			json.Unmarshal(b, &history)
		}
		entry, _ := json.Marshal(map[string]any{"at": time.Now().Unix(), "primary": c.Primary, "probes": result})
		history = append(history, entry)
		if len(history) > 24 {
			history = history[len(history)-24:]
		}
		b, _ := json.Marshal(history)
		atomicPrivate(path, b)
	}
	return map[string]bool{"awg": result["awg"].Ready && result["awg6"].Ready, "mihomo": result["mihomo"].Ready && result["mihomo6"].Ready}
}
