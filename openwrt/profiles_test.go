package main

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

const awgFixture = "[Interface]\nPrivateKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=\nAddress = 10.8.0.2/32\nJc = 4\nS1 = 16\nS2 = 16\nS3 = 16\nS4 = 16\nHeaderProtectionKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=\nContentPaddingAddition = 5-20\n[Peer]\nPublicKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=\nEndpoint = 192.0.2.1:51820\nAllowedIPs = 0.0.0.0/0, ::/0\nPersistentKeepalive = 20-30\n"

func TestAWG31(t *testing.T) {
	k, b, e := normalizeProfile(awgFixture)
	if e != nil || k != "AWG" || !strings.Contains(string(b), "HeaderProtectionKey") {
		t.Fatalf("AWG3 preservation failed: %v", e)
	}
}
func TestRejectHooksAndMalformedAWG(t *testing.T) {
	for _, bad := range []string{strings.Replace(awgFixture, "Jc = 4", "PostUp = touch /bad", 1), strings.Replace(awgFixture, "Jc = 4", "Jc = nope", 1), strings.Replace(awgFixture, "5-20", "20-5", 1), strings.Replace(awgFixture, "10.8.0.2/32", "garbage", 1), strings.Replace(awgFixture, "192.0.2.1:51820", "192.0.2.1", 1)} {
		if _, _, e := normalizeProfile(bad); e == nil {
			t.Fatal("Malformed config accepted")
		}
	}
}
func TestXHTTPExtraPreserved(t *testing.T) {
	raw := "vless://00000000-0000-4000-8000-000000000001@example.com:443?type=xhttp&security=reality&sni=example.com&pbk=example&sid=ab&path=%2Ftransport&extra=" + url.QueryEscape(`{"xmux":{"maxConcurrency":"16-32"},"noSSEHeader":true}`)
	k, b, e := normalizeProfile(raw)
	if e != nil || k != "VLESS" {
		t.Fatal(e)
	}
	var cfg map[string]any
	json.Unmarshal(b, &cfg)
	o := cfg["outbounds"].([]any)[0].(map[string]any)
	x := o["streamSettings"].(map[string]any)["xhttpSettings"].(map[string]any)
	if x["path"] != "/transport" || x["extra"].(map[string]any)["noSSEHeader"] != true {
		t.Fatal("XHTTP settings lost")
	}
}
func TestXrayJSONDoesNotImportListenersOrLogs(t *testing.T) {
	_, b, e := normalizeProfile(`{"log":{"access":"/private/file"},"inbounds":[{"port":22}],"outbounds":[{"protocol":"vless","settings":{"vnext":[]},"tag":"supplied"}]}`)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "/private/file") || strings.Contains(string(b), "inbounds") || strings.Contains(string(b), "supplied") {
		t.Fatal("Foreign router configuration imported")
	}
}
func TestRejectAmbiguousVLESS(t *testing.T) {
	for _, v := range []string{"vless://id@server:443?type=xhttp&security=reality&sni=server", `{"outbounds":[{"protocol":"vless"},{"protocol":"vless"}]}`, "vpn://unknown"} {
		if _, _, e := normalizeProfile(v); e == nil {
			t.Fatal("Invalid config accepted")
		}
	}
}
