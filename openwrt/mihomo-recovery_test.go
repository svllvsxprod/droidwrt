package main

import (
	"testing"
	"time"
)

func TestMihomoRecoveryUpstreamConfirmationAndCooldown(t *testing.T) {
	r := mihomoRecovery{uplink: "wlan0 old"}
	now := time.Unix(1000, 0)
	for i := 0; i < 10; i++ {
		if r.check(now, "wlan0 old", true, false) != "" {
			t.Fatal("remote loss restarted live core")
		}
	}
	// A new network must collect its own failures, not inherit the old outage.
	for i := 0; i < 2; i++ {
		if r.check(now, "eth0 new", true, false) != "" {
			t.Fatal("premature upstream repair")
		}
	}
	if r.check(now, "eth0 new", true, false) != "upstream_changed" {
		t.Fatal("stale upstream was not repaired")
	}
	for i := 0; i < 10; i++ {
		if r.check(now.Add(10*time.Second), "wlan0 newer", true, false) != "" {
			t.Fatal("cooldown bypassed")
		}
	}
	if r.check(now.Add(61*time.Second), "wlan0 newer", true, false) != "upstream_changed" {
		t.Fatal("pending transition lost")
	}
	if r.check(now.Add(125*time.Second), "wlan0 newer", true, false) != "" {
		t.Fatal("repeated remote repair")
	}
}
func TestMihomoHealthyTransitionAndLocalHang(t *testing.T) {
	r := mihomoRecovery{uplink: "wlan0 old"}
	now := time.Unix(1000, 0)
	if r.check(now, "eth0 new", true, true) != "" {
		t.Fatal("healthy transition interrupted")
	}
	for i := 0; i < 2; i++ {
		if r.check(now, "", false, false) != "" {
			t.Fatal("transient local timeout")
		}
	}
	if r.check(now, "", false, false) != "local_controller_unresponsive" {
		t.Fatal("local hang not recovered")
	}
	r.check(now.Add(time.Second), "eth0 new", true, true)
	if r.check(now.Add(65*time.Second), "eth0 new", false, false) != "" {
		t.Fatal("failure count not reset")
	}
}
func TestMihomoSameInterfaceAddressChange(t *testing.T) {
	r := mihomoRecovery{uplink: "wlan0 src=192.168.31.90 via=192.168.31.1"}
	now := time.Unix(1000, 0)
	for i := 0; i < 3; i++ {
		got := r.check(now, "wlan0 src=192.168.31.176 via=192.168.31.1", true, false)
		if (i == 2) != (got == "upstream_changed") {
			t.Fatal("DHCP change detection", i, got)
		}
	}
}
