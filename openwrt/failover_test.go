package main

import (
	"testing"
	"time"
)

func TestFailoverLossReserveRecoveryAndFailClosed(t *testing.T) {
	c := RouterConfig{Primary: "awg", Preferred: "awg", AWG: "a", Mihomo: "m"}
	s := failoverState{}
	now := time.Unix(1000, 0)
	step := func(a, m bool, seconds int, want string) {
		t.Helper()
		now = now.Add(time.Duration(seconds) * time.Second)
		target, _ := s.choose(now, c, map[string]bool{"awg": a, "mihomo": m})
		if target != want {
			t.Fatalf("got %s want %s", target, want)
		}
		if target != c.Primary {
			s.lastSwitch = now
			c.Primary = target
		}
	}
	step(true, true, 0, "awg")
	step(false, true, 15, "awg") // One timeout does not change the route.
	step(false, true, 15, "awg") // Repeated loss is confirmed before switching.
	step(false, true, 15, "mihomo")
	step(true, true, 15, "mihomo")
	step(false, true, 15, "mihomo") // Flapping preferred resets its good streak.
	step(true, true, 15, "mihomo")
	step(true, true, 15, "mihomo")
	step(true, true, 15, "mihomo")
	step(true, true, 15, "mihomo")
	step(true, true, 30, "awg")
	step(false, false, 15, "awg")
	step(false, false, 15, "awg")
	step(false, false, 15, "blocked")
	step(false, true, 15, "blocked") // A reserve must be verified twice.
	step(false, true, 15, "mihomo")
	if c.Preferred != "awg" {
		t.Fatal("user preference lost")
	}
}

func TestFailoverHonorsDirectAndManualSelection(t *testing.T) {
	s := failoverState{}
	now := time.Now()
	c := RouterConfig{Primary: "direct", Preferred: "direct", AWG: "a", Mihomo: "m"}
	h := map[string]bool{"awg": true, "mihomo": true}
	if e, _ := s.choose(now, c, h); e != "direct" {
		t.Fatal(e)
	}
	c.Primary = "mihomo"
	c.Preferred = "mihomo"
	if e, _ := s.choose(now, c, h); e != "mihomo" {
		t.Fatal(e)
	}
	c.Preferred = "blocked"
	if e, _ := s.choose(now, c, h); e != "blocked" {
		t.Fatal(e)
	}
}

func TestFailoverRestartAndProfileChangeDoNotTrustOldHealth(t *testing.T) {
	s := failoverState{}
	now := time.Now()
	c := RouterConfig{Primary: "blocked", Preferred: "awg", AWG: "a", Mihomo: "m"}
	h := map[string]bool{"awg": false, "mihomo": true}
	if e, _ := s.choose(now, c, h); e != "blocked" {
		t.Fatal(e)
	}
	c.Mihomo = "replacement"
	if e, _ := s.choose(now.Add(time.Second), c, h); e != "blocked" {
		t.Fatal("old profile health reused")
	}
	if e, _ := s.choose(now.Add(2*time.Second), c, h); e != "mihomo" {
		t.Fatal(e)
	}
}

func TestFailoverDoesNotBlockFreshlyVerifiedReserve(t *testing.T) {
	s := failoverState{}
	c := RouterConfig{Primary: "mihomo", Preferred: "awg", AWG: "a", Mihomo: "m"}
	now := time.Unix(1000, 0)
	if e, _ := s.choose(now, c, map[string]bool{"awg": false, "mihomo": false}); e != "mihomo" {
		t.Fatal("one failed observation must not switch", e)
	}
	if e, _ := s.choose(now.Add(3*time.Second), c, map[string]bool{"awg": false, "mihomo": false}); e != "mihomo" {
		t.Fatal("second transient failure switched too soon")
	}
	if e, _ := s.choose(now.Add(5*time.Second), c, map[string]bool{"awg": true, "mihomo": false}); e != "awg" {
		t.Fatal("fresh working reserve must not be blocked", e)
	}
	// A dead reserve is never a fallback, regardless of a previous success.
	if e, _ := s.choose(now.Add(10*time.Second), c, map[string]bool{"awg": false, "mihomo": false}); e != "blocked" {
		t.Fatal("both unavailable must remain fail-closed", e)
	}
}
