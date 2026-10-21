package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var healthJournalLock sync.Mutex
var lastTunnelSample time.Time
var lastTunnelState string

// UTC day files, current day plus the previous three days. No profiles, keys,
// URLs, addresses or raw core responses are written. Size is bounded as well.
func appendHealthJournal(dir string, now time.Time, value any) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, now.UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < 16*1024*1024 {
		_, err = f.Write(append(data, '\n'))
	}
	// Only our dated files are eligible for retention cleanup.
	files, _ := filepath.Glob(filepath.Join(dir, "????-??-??.jsonl"))
	cutoff := now.UTC().AddDate(0, 0, -3).Format("2006-01-02")
	for _, name := range files {
		day := strings.TrimSuffix(filepath.Base(name), ".jsonl")
		if _, e := time.Parse("2006-01-02", day); e == nil && day < cutoff {
			os.Remove(name)
		}
	}
	return err
}
func recordHealthEvent(kind string, fields map[string]any) {
	healthJournalLock.Lock()
	defer healthJournalLock.Unlock()
	now := time.Now()
	appendHealthJournal(moduleDir+"/health-history", now, map[string]any{"at": now.Unix(), "type": kind, "details": fields})
}
func recordTunnelSample(c RouterConfig, local map[string]bool, probes map[string]tunnelFamilyProbe) {
	healthJournalLock.Lock()
	defer healthJournalLock.Unlock()
	now := time.Now()
	states := map[string]string{}
	for e, p := range tunnelAssessments {
		states[e] = p.State
	}
	state := c.Primary + ":" + states["awg"] + ":" + states["mihomo"]
	interval := time.Minute
	if states["awg"] != "connected" || states["mihomo"] != "connected" {
		interval = 15 * time.Second
	}
	if state == lastTunnelState && now.Sub(lastTunnelSample) < interval {
		return
	}
	lastTunnelState, lastTunnelSample = state, now
	uplink, _ := os.ReadFile(moduleDir + "/uplink-state")
	identity := sha256.Sum256(uplink)
	a := awgHealth()
	fields := map[string]any{"primary": c.Primary, "preferred": c.Preferred, "states": states, "local": local, "probes": probes,
		"uplink_id": hex.EncodeToString(identity[:8]), "awg_rx": a["rx_bytes"], "awg_tx": a["tx_bytes"], "awg_handshake_age": a["handshake_age_seconds"],
		"control_pid": os.Getpid(), "control_uptime_ms": controlUptime() / int64(time.Millisecond),
		"boot_uptime_ms": bootUptimeMillis(), "power_guard": routerWakeHeld.Load()}
	// Reuse the existing direct-path check, retaining its timestamp so stale
	// results cannot be mistaken for simultaneous proof of upstream availability.
	if direct, ok := linkSnapshot()["direct"].(map[string]any); ok {
		fields["direct_probe"] = map[string]any{"state": direct["state"], "checked_at": direct["checked_at"]}
	}
	appendHealthJournal(moduleDir+"/health-history", now, map[string]any{"at": now.Unix(), "type": "sample", "details": fields})
}
