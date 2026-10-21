package main

import (
	"testing"
	"time"
)

func TestAdaptiveSlowNetworkAndRecovery(t *testing.T) {
	p := probeTiming{}
	initial := p.budget()
	p.observe(initial, false, true)
	if p.budget() <= initial {
		t.Fatal("timeout did not expand allowance")
	}
	p.observe(5*time.Second, true, false)
	if p.budget() < 5*time.Second {
		t.Fatal("slow success classified against fast network budget")
	}
	for i := 0; i < 20; i++ {
		p.observe(p.budget(), false, true)
	}
	if p.budget() != 12*time.Second {
		t.Fatal("failed network not bounded")
	}
	for i := 0; i < 100; i++ {
		p.observe(200*time.Millisecond, true, false)
	}
	if p.budget() != 3*time.Second {
		t.Fatal("budget did not recover", p.budget())
	}
}
func TestPathAssessmentTransientLossAndFamilyIsolation(t *testing.T) {
	p := pathAssessment{}
	p.observe(true, true, true, false)
	if p.State != "connected" {
		t.Fatal(p)
	}
	p.observe(true, false, true, false)
	if p.State != "degraded" {
		t.Fatal("one family timeout is not a dead core", p)
	}
	p.observe(true, true, true, true)
	if p.State != "degraded" || p.Failures != 0 {
		t.Fatal("fallback success must remain usable", p)
	}
	for i := 0; i < 3; i++ {
		p.observe(true, false, true, false)
	}
	if p.State != "unresponsive" {
		t.Fatal("persistent IPv4 failure was hidden", p)
	}
	p.observe(true, true, true, false)
	if p.State != "connected" {
		t.Fatal(p)
	}
}
func TestAdaptiveIndependentPaths(t *testing.T) {
	fast, slow := probeTiming{}, probeTiming{}
	fast.observe(100*time.Millisecond, true, false)
	slow.observe(6*time.Second, true, false)
	if fast.budget() >= slow.budget() {
		t.Fatal("slow reserve distorted fast path")
	}
}
