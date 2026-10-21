package main

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// Literal addresses remove local DNS from the exit-address measurement.
// A fresh successful response is required; no last-known IP is promoted to success.
func probeLinkTrace(family int, profile string, run func(string) (string, error)) map[string]any {
	targets := []string{"https://1.1.1.1/cdn-cgi/trace", "https://1.0.0.1/cdn-cgi/trace"}
	if family == 6 {
		targets = []string{"https://[2606:4700:4700::1111]/cdn-cgi/trace", "https://[2606:4700:4700::1001]/cdn-cgi/trace"}
	}
	result := map[string]any{"state": "unavailable", "profile": profile}
	for i, target := range targets {
		result["attempts"] = i + 1
		out, err := run(target)
		result["checked_at"] = time.Now().Unix()
		if err != nil {
			result["error"] = "probe_failed"
			continue
		}
		values := map[string]string{}
		for _, line := range strings.Split(out, "\n") {
			p := strings.SplitN(line, "=", 2)
			if len(p) == 2 {
				values[p[0]] = strings.TrimSpace(p[1])
			}
		}
		ip := net.ParseIP(values["ip"])
		if ip == nil || (family == 4) != (ip.To4() != nil) {
			result["error"] = "invalid_address_family"
			continue
		}
		result["state"], result["ip"] = "verified", ip.String()
		delete(result, "error")
		country := values["loc"]
		if len(country) == 2 && country[0] >= 'A' && country[0] <= 'Z' && country[1] >= 'A' && country[1] <= 'Z' {
			result["country"] = country
		}
		if n, err := strconv.ParseFloat(values["dwr_ttfb"], 64); err == nil && n > 0 {
			result["latency_ms"] = int(n * 1000)
		}
		return result
	}
	return result
}
