package main

import (
	"testing"
	"time"
)

func TestAWGRemoteLossAndFlappingNeverRestartLiveCore(t *testing.T) {
	r := awgRecovery{}
	now := time.Unix(1000, 0)
	for i := 0; i < 300; i++ {
		if got := r.check(now.Add(time.Duration(i)*time.Minute), true, i%7 == 0); got != "" {
			t.Fatal("remote loss restarted healthy core", got)
		}
	}
}
func TestAWGLocalFailureConfirmationAndBackoff(t *testing.T) {
	r := awgRecovery{}
	now := time.Unix(1000, 0)
	for i := 0; i < 2; i++ {
		if r.check(now, false, false) != "" {
			t.Fatal("single local timeout caused restart")
		}
	}
	if r.check(now, false, false) != "local_core_unresponsive" {
		t.Fatal("dead core not recovered")
	}
	for i := 0; i < 10; i++ {
		if r.check(now.Add(10*time.Second), false, false) != "" {
			t.Fatal("cooldown bypassed")
		}
	}
	if r.check(now.Add(time.Minute), false, false) == "" || r.delay != 2*time.Minute {
		t.Fatal("backoff not increased")
	}
	// A remote success is not sufficient to reset the repair backoff.
	r.check(now.Add(2*time.Minute), true, true)
	if r.delay != 2*time.Minute {
		t.Fatal("one success reset recovery backoff")
	}
	for i := 0; i < 5; i++ {
		r.check(now.Add(3*time.Minute), true, true)
	}
	if r.delay != 0 {
		t.Fatal("sustained recovery did not reset backoff")
	}
}
