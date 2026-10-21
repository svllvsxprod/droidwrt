package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func stalledTransportSample() awgTransportSample {
	age := int64(900)
	return awgTransportSample{Local: true, Alternative: true, Inactive: true, RX: 100, TX: 100, HandshakeAge: &age}
}

func TestAWGTransportRecoversStallOnlyOnceAndPersistsLatch(t *testing.T) {
	now := time.Now()
	r := awgTransportRecovery{}
	s := stalledTransportSample()
	if r.check(now, "network-a", time.Minute, s) {
		t.Fatal("first observation repaired immediately")
	}
	for i := 1; i < 4; i++ {
		s.TX += 100
		if r.check(now.Add(time.Duration(i)*10*time.Second), "network-a", time.Minute, s) {
			t.Fatal("native handshake retry window not respected")
		}
	}
	if !r.check(now.Add(time.Minute), "network-a", time.Minute, s) {
		t.Fatal("live core with stale receive/handshake and working reserve was abandoned")
	}
	for i := 2; i < 20; i++ {
		s.TX += 100
		if r.check(now.Add(time.Duration(i)*time.Minute), "network-a", time.Minute, s) {
			t.Fatal("refresh loop during remote outage")
		}
	}
	b, _ := json.Marshal(r)
	var restarted awgTransportRecovery
	json.Unmarshal(b, &restarted)
	for i := 0; i < 10; i++ {
		s.TX += 100
		if restarted.check(now.Add(time.Duration(i+30)*time.Minute), "network-a", time.Minute, s) {
			t.Fatal("controller restart bypassed persisted latch")
		}
	}
	// One transient successful probe does not re-arm repair; five do.
	s.Ready = true
	for i := 0; i < 4; i++ {
		restarted.check(now.Add(time.Hour), "network-a", time.Minute, s)
	}
	if !restarted.Attempted {
		t.Fatal("short recovery re-armed repair")
	}
	restarted.check(now.Add(time.Hour), "network-a", time.Minute, s)
	if restarted.Attempted {
		t.Fatal("stable recovery did not re-arm repair")
	}
}

func TestAWGTransportLeavesHealthyActivePartialAndOfflinePathsAlone(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*awgTransportSample, int)
	}{
		{"working", func(s *awgTransportSample, _ int) { s.Ready = true }},
		{"active_route", func(s *awgTransportSample, _ int) { s.Inactive = false }},
		{"no_verified_alternative", func(s *awgTransportSample, _ int) { s.Alternative = false }},
		{"local_core_dead", func(s *awgTransportSample, _ int) { s.Local = false }},
		{"receive_progress_even_if_probe_fails", func(s *awgTransportSample, i int) { s.RX += uint64(i * 100) }},
		{"fresh_handshake", func(s *awgTransportSample, _ int) { age := int64(10); s.HandshakeAge = &age }},
		{"no_send_attempts", func(s *awgTransportSample, _ int) { s.TX = 100 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := awgTransportRecovery{}
			now := time.Now()
			for i := 0; i < 30; i++ {
				s := stalledTransportSample()
				s.TX += uint64(i * 100)
				tc.modify(&s, i)
				if r.check(now.Add(time.Duration(i)*time.Minute), "same", time.Minute, s) {
					t.Fatal("unsafe socket refresh")
				}
			}
		})
	}
}

func TestAWGTransportNoHandshakeAndNetworkChange(t *testing.T) {
	r := awgTransportRecovery{Identity: "old", Attempted: true}
	s := stalledTransportSample()
	s.HandshakeAge = nil
	now := time.Now()
	for i := 0; i < 4; i++ {
		s.TX += 100
		got := r.check(now.Add(time.Duration(i)*20*time.Second), "new", time.Minute, s)
		if got != (i == 3) {
			t.Fatalf("missing handshake recovery at sample %d = %v", i, got)
		}
	}
}

func TestAWGTransportBudgetUsesProfileAndKeepsExplicitPort(t *testing.T) {
	w, p := awgTransportOptions("RejectAfterTime = 150-180\nRekeyTimeout = 3-7\nMaxHandshakeAttempts = 15-20\n")
	if w != 342*time.Second || p != 0 {
		t.Fatal("profile retry budget", w, p)
	}
	w, p = awgTransportOptions("ListenPort = 51820\nRejectAfterTime = 4294967295\n")
	if p != 51820 || w <= 0 || w > 15*time.Minute {
		t.Fatal("fixed port/invalid timing", w, p)
	}
}

func TestAWGSessionRecoveryPlanPreservesProfile(t *testing.T) {
	for _, raw := range []string{awgFixture, strings.Replace(awgFixture, "[Interface]", "[Interface]\nListenPort = 51820", 1)} {
		plan, err := awgSessionRecoveryPlan(raw)
		if err != nil {
			t.Fatal(err)
		}
		original, _, _, err := awgPlan(raw)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, "ListenPort") {
			if plan != original {
				t.Fatal("explicit port/profile altered")
			}
		} else if strings.Replace(plan, "listen_port=0\n", "", 1) != original {
			t.Fatal("credentials or AWG parameters changed")
		}
		if !strings.Contains(plan, "replace_peers=true\n") {
			t.Fatal("failed session not replaced")
		}
		if strings.Index(plan, "listen_port=") > strings.Index(plan, "public_key=") {
			t.Fatal("device field placed after peer")
		}
	}
	if _, err := awgSessionRecoveryPlan("invalid"); err == nil {
		t.Fatal("invalid profile accepted")
	}
}
