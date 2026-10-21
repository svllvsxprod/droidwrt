package main

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWatchdogDoesNotCountRecordedSuspendAsStall(t *testing.T) {
	// 15 Oct: 17m08s between samples, of which ~16m10s was Android suspend.
	// The old Unix-time heartbeat incorrectly reported a dead controller.
	start := int64(time.Second)
	awake := 58 * time.Second
	if heartbeatStalled(start, start+int64(awake)) {
		t.Fatal("healthy loop restarted after system suspend")
	}
	if !heartbeatStalled(start, start+int64(181*time.Second)) {
		t.Fatal("a genuinely stuck awake loop must still be recovered")
	}
	if heartbeatStalled(0, int64(time.Hour)) {
		t.Fatal("uninitialized heartbeat treated as stuck")
	}
}

func TestSharingKeepsRouterAwakeWithExpiringLease(t *testing.T) {
	for _, c := range []SharingConfig{{Hotspot: true}, {EthernetLAN: true}, {USB: "ncm"}, {USB: "ecm"}} {
		if !needsRouterWake(c) {
			t.Fatal("active sharing can suspend", c)
		}
	}
	if needsRouterWake(SharingConfig{USB: "off", USBDebug: true}) || needsRouterWake(SharingConfig{}) {
		t.Fatal("debugging alone must not keep a disabled router awake")
	}
	var paths, values []string
	write := func(path string, b []byte) error {
		paths = append(paths, path)
		values = append(values, string(b))
		return nil
	}
	if err := setRouterWake(true, write); err != nil {
		t.Fatal(err)
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || parts[0] != routerWakeName || paths[0] != "/sys/power/wake_lock" {
		t.Fatal(paths, values)
	}
	ns, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || ns <= int64(5*time.Second) || ns > int64(time.Minute) {
		t.Fatal("lease must survive renewal interval and expire after a controller crash", values[0])
	}
	if err := setRouterWake(false, write); err != nil {
		t.Fatal(err)
	}
	if paths[1] != "/sys/power/wake_unlock" || values[1] != routerWakeName {
		t.Fatal("unrelated wake sources affected")
	}
	failure := errors.New("permission denied")
	if err := setRouterWake(true, func(string, []byte) error { return failure }); !errors.Is(err, failure) {
		t.Fatal("kernel failure hidden")
	}
}

func TestMissingHandshakeIsUnknownNotUnixEpochAge(t *testing.T) {
	if handshakeAge(0, 1792060311) != nil || handshakeAge(200, 100) != nil {
		t.Fatal("absent/future handshake has fabricated age")
	}
	if handshakeAge(100, 123) != int64(23) {
		t.Fatal("valid age lost")
	}
}
