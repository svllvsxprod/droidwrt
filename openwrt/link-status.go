package main

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var linkMu sync.RWMutex
var linkData = map[string]any{}

func linkSnapshot() map[string]any {
	linkMu.RLock()
	defer linkMu.RUnlock()
	out := map[string]any{}
	for k, v := range linkData {
		out[k] = v
	}
	return out
}

// Read only active managers: the graveyard contains old AP states.
func parseHotspotStatus(raw string) string {
	start := strings.Index(raw, "Dump of WifiActiveModeWarden\n")
	if start < 0 {
		return "unknown"
	}
	raw = raw[start:]
	end := strings.Index(raw, "Dump of ActiveModeWarden.Graveyard")
	if end < 0 {
		return "unknown"
	}
	raw = raw[:end]
	if !strings.Contains(raw, "NumActiveModeManagers:") {
		return "unknown"
	}
	i := strings.Index(raw, "Dump of SoftApManager id=")
	if i < 0 {
		return "disabled"
	}
	ap := raw[i:]
	if j := strings.Index(ap[1:], "\nDump of "); j >= 0 {
		ap = ap[:j+1]
	}
	if strings.Contains(ap, "current StateMachine mode: StartedState") && strings.Contains(ap, "mIfaceIsUp: true") {
		return "enabled"
	}
	return "transitioning"
}

var apFrequencyPattern = regexp.MustCompile(`frequency=\s*([0-9]+)`)

func hotspotFrequency(raw string) int {
	if parseHotspotStatus(raw) != "enabled" {
		return 0
	}
	start := strings.Index(raw, "Dump of WifiActiveModeWarden\n")
	end := strings.Index(raw[start:], "Dump of ActiveModeWarden.Graveyard")
	active := raw[start : start+end]
	i := strings.Index(active, "mCurrentSoftApInfoMap")
	if i < 0 {
		return 0
	}
	line := strings.SplitN(active[i:], "\n", 2)[0]
	m := apFrequencyPattern.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0
	}
	value, _ := strconv.Atoi(m[1])
	return value
}
func hotspotMonitor() {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		b, e := exec.CommandContext(ctx, "/system/bin/dumpsys", "wifi").Output()
		cancel()
		ap := "unknown"
		if e == nil {
			ap = parseHotspotStatus(string(b))
		}
		linkMu.Lock()
		linkData["hotspot_state"] = ap
		linkData["hotspot_frequency_mhz"] = hotspotFrequency(string(b))
		linkData["hotspot_checked_at"] = time.Now().Unix()
		linkMu.Unlock()
		time.Sleep(5 * time.Second)
	}
}
func linkMonitor() {
	go hotspotMonitor()
	next := map[string]time.Time{}
	keys := map[string]string{}
	for {

		c := readRouter()
		uplink, _ := os.ReadFile(moduleDir + "/uplink-state")
		for _, engine := range []string{"mihomo", "awg", "direct"} {
			profile := c.Mihomo
			if engine == "awg" {
				profile = c.AWG
			}
			if engine == "direct" {
				profile = "direct"
			}
			key := profile + ":" + string(uplink)
			if profile == "" {
				continue
			}
			if keys[engine] == key && time.Now().Before(next[engine]) {
				continue
			}
			keys[engine] = key
			next[engine] = time.Now().Add(60 * time.Second)
			result := probeLinkTrace(4, profile, func(target string) (string, error) {
				args := []string{"curl", "-4", "--silent", "--fail", "--max-time", "10", "--connect-timeout", "8", "--max-filesize", "4096", "--write-out", "\ndwr_ttfb=%{time_starttransfer}\n"}
				if engine == "mihomo" {
					args = append(args, "--proxy", "http://127.0.0.1:7890")
				} else if engine == "awg" {
					args = append(args, "--interface", "awg0")
				} else {
					args = append(args, "--interface", "dwr-wan")
				}
				return containerCommand(append(args, target)...)
			})
			// Schedule from completion, retry failures promptly without retaining
			// a stale address as proof of a working connection.
			delay := 60 * time.Second
			if result["state"] != "verified" {
				delay = 15 * time.Second
			}
			next[engine] = time.Now().Add(delay)

			after := readRouter()
			current := after.Mihomo
			if engine == "awg" {
				current = after.AWG
			}
			if engine == "direct" {
				current = "direct"
			}
			if profile != current {
				continue
			}
			linkMu.Lock()
			linkData[engine] = result
			linkMu.Unlock()
		}
		time.Sleep(5 * time.Second)
	}
}
