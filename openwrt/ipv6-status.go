package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func hasIPv6Address(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		p := strings.SplitN(line, "=", 2)
		if len(p) != 2 || strings.TrimSpace(p[0]) != "Address" {
			continue
		}
		for _, address := range strings.Split(p[1], ",") {
			ip, _, e := net.ParseCIDR(strings.TrimSpace(address))
			if e == nil && ip.To4() == nil && !ip.IsUnspecified() && !ip.IsLoopback() {
				return true
			}
		}
	}
	return false
}

// A literal IPv6 destination prevents a proxy from resolving the test to IPv4.
// This verifies each engine's IPv6 exit; it is not a client-side leak certificate.
func ipv6Monitor() {
	for {
		for _, engine := range []string{"mihomo", "awg", "direct"} {
			c := readRouter()
			profile := c.Mihomo
			if engine == "awg" {
				profile = c.AWG
			}
			if engine == "direct" {
				profile = "direct"
			}
			if profile == "" {
				continue
			}
			result := map[string]any{"state": "unavailable", "profile": profile}
			raw, _ := os.ReadFile(filepath.Join(profileRoot, profile+".conf"))
			if engine == "awg" && !hasIPv6Address(string(raw)) {
				result["state"] = "not_configured"
			} else {
				result = probeLinkTrace(6, profile, func(target string) (string, error) {
					args := []string{"curl", "--silent", "--fail", "--max-time", "10", "--connect-timeout", "8", "--max-filesize", "4096"}
					if engine == "mihomo" {
						args = append(args, "--proxy", "http://127.0.0.1:7890")
					} else {
						device := "awg0"
						if engine == "direct" {
							device = "dwr-wan"
						}
						args = append(args, "-6", "--interface", device)
					}
					return containerCommand(append(args, target)...)
				})
			}
			result["checked_at"] = time.Now().Unix()
			after := readRouter()
			if (engine == "mihomo" && after.Mihomo != profile) || (engine == "awg" && after.AWG != profile) {
				continue
			}
			linkMu.Lock()
			linkData[engine+"_ipv6"] = result
			linkMu.Unlock()
		}
		time.Sleep(60 * time.Second)
	}
}
