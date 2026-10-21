package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMihomoTransportDefaultsPreserveProfileSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		enabled   bool
	}{
		{"reality-auto", `{"network":"xhttp","reality-opts":{"public-key":"test"},"xhttp-opts":{"mode":"auto","path":"/kept"}}`, true},
		{"reality-stream", `{"network":"xhttp","reality-opts":{"public-key":"test"},"xhttp-opts":{"mode":"stream-one"}}`, true},
		{"explicit-reuse", `{"network":"xhttp","reality-opts":{"public-key":"test"},"xhttp-opts":{"reuse-settings":{"max-connections":"2"}}}`, false},
		{"explicit-null", `{"network":"xhttp","reality-opts":{"public-key":"test"},"xhttp-opts":{"reuse-settings":null}}`, false},
		{"packet-up", `{"network":"xhttp","reality-opts":{"public-key":"test"},"xhttp-opts":{"mode":"packet-up"}}`, false},
		{"split-download", `{"network":"xhttp","reality-opts":{"public-key":"test"},"xhttp-opts":{"download-settings":{"host":"example.com"}}}`, false},
		{"tls-only", `{"network":"xhttp","tls":true,"xhttp-opts":{"mode":"auto"}}`, false},
		{"other-transport", `{"network":"grpc","reality-opts":{"public-key":"test"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p, original map[string]any
			json.Unmarshal([]byte(tc.raw), &p)
			json.Unmarshal([]byte(tc.raw), &original)
			cfg := map[string]any{"proxies": []any{p}, "rules": []string{"MATCH,VPN"}}
			mihomoTransportDefaults(cfg)
			once, _ := json.Marshal(cfg)
			mihomoTransportDefaults(cfg)
			twice, _ := json.Marshal(cfg)
			if string(once) != string(twice) {
				t.Fatal("defaults are not idempotent")
			}
			if tc.enabled {
				x := p["xhttp-opts"].(map[string]any)
				if _, ok := x["reuse-settings"].(map[string]any); !ok {
					t.Fatal("missing reuse settings")
				}
				delete(x, "reuse-settings")
			}
			if !reflect.DeepEqual(p, original) {
				t.Fatal("changed imported transport settings")
			}
		})
	}
}
