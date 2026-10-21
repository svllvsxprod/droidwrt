package main

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHotspotValidation(t *testing.T) {
	good := HotspotConfig{"Моя сеть", "5", "wpa2", "long password!"}
	if e := validateHotspot(good); e != nil {
		t.Fatal(e)
	}
	for _, security := range []string{"wpa2", "wpa2-wpa3", "wpa3", "open"} {
		c := good
		c.Security = security
		if security == "open" {
			c.Password = ""
		}
		if e := validateHotspot(c); e != nil {
			t.Fatal(security, e)
		}
	}
	cases := []HotspotConfig{}
	for _, ssid := range []string{"", "  ", strings.Repeat("я", 17), "wifi\nname", string([]byte{255})} {
		c := good
		c.SSID = ssid
		cases = append(cases, c)
	}
	for _, pass := range []string{"short", strings.Repeat("x", 64), "пароль12345", "abcdefgh\n"} {
		c := good
		c.Password = pass
		cases = append(cases, c)
	}
	c := good
	c.Band = "6"
	cases = append(cases, c)
	c = good
	c.Security = "wep"
	cases = append(cases, c)
	c = good
	c.Security = "open"
	cases = append(cases, c)
	for _, c := range cases {
		if validateHotspot(c) == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
func TestHotspotCapabilities(t *testing.T) {
	caps := parseAPCapabilities("mCurrentSoftApCapability: SupportedFeatures=111 MaximumSupportedClientNumber=16")
	if !caps.Available || !caps.Band24 || !caps.Band5 || !caps.WPA3 {
		t.Fatal(caps)
	}
	c := HotspotConfig{"test", "5", "wpa3", "abcdefgh"}
	if checkHotspotCapabilities(c, caps) != nil {
		t.Fatal("supported configuration rejected")
	}
	for _, limited := range []HotspotCapabilities{{}, {true, true, false, true}, {true, true, true, false}} {
		if checkHotspotCapabilities(c, limited) == nil {
			t.Fatal("unsupported accepted")
		}
	}
	if parseAPCapabilities("WIFI_FEATURE_WPA3_SAE").Available {
		t.Fatal("STA capability incorrectly used as AP capability")
	}
}
func TestHotspotCommitRollback(t *testing.T) {
	old := HotspotConfig{"original", "5", "wpa2", "abcdefgh"}
	next := old
	next.SSID = "new"
	disk := old
	applied := old
	calls := 0
	e := commitHotspot(old, next, func(c HotspotConfig) error { disk = c; return nil }, func() error {
		calls++
		if disk == next {
			return errors.New("driver rejected")
		}
		applied = disk
		return nil
	})
	if e == nil || disk != old || applied != old || calls != 2 {
		t.Fatal("rollback did not restore original")
	}
	calls = 0
	if e = commitHotspot(old, old, func(c HotspotConfig) error { t.Fatal("unchanged config written"); return nil }, func() error { calls++; return nil }); e != nil || calls != 0 {
		t.Fatal("unchanged config restarted radio")
	}
	if e = commitHotspot(old, next, func(c HotspotConfig) error { disk = c; return nil }, func() error { applied = disk; return nil }); e != nil || disk != next || applied != next {
		t.Fatal("valid config not committed")
	}
}
func TestHotspotInvalidRequestsDoNotTouchRadio(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"ssid":"x","band":"6","security":"open","password":""}`, `{"ssid":"x","band":"5","security":"open","password":"","command":"reboot"}`, `{"ssid":"x","band":"5","security":"open","password":""} {}`} {
		w := httptest.NewRecorder()
		hotspotAccessHandler(w, httptest.NewRequest("POST", "/droidwrt/hotspot-access", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal("invalid request accepted", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("credentials could be cached")
		}
	}
}
