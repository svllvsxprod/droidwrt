package main

// Reuse established Reality/H2 sessions instead of paying for a new mobile TCP
// and TLS handshake on every request. Keep imported overrides and other XHTTP
// modes intact: only the single-stream Reality mode was verified on this router.
func mihomoTransportDefaults(cfg map[string]any) {
	proxies, _ := cfg["proxies"].([]any)
	for _, item := range proxies {
		p, ok := item.(map[string]any)
		if !ok || p["network"] != "xhttp" {
			continue
		}
		if reality, ok := p["reality-opts"].(map[string]any); !ok || len(reality) == 0 {
			continue
		}
		x, ok := p["xhttp-opts"].(map[string]any)
		if !ok {
			continue
		}
		if _, explicit := x["reuse-settings"]; explicit {
			continue
		}
		if _, split := x["download-settings"]; split {
			continue
		}
		mode, _ := x["mode"].(string)
		if mode != "" && mode != "auto" && mode != "stream-one" {
			continue
		}
		x["reuse-settings"] = map[string]any{
			"max-concurrency":     "8-16",
			"h-max-request-times": "600-900",
			"h-max-reusable-secs": "600-900",
		}
	}
}
