package main

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDNSProbeRejectsMalformedAndUnrelatedReplies(t *testing.T) {
	id := uint16(0x5117)
	header := []byte{0x51, 0x17, 0x81, 0x80, 0, 1, 0, 1, 0, 0, 0, 0}
	question := []byte{1, 'x', 0, 0, 16, 0, 3}
	ip := "2001:db8::42"
	rr := []byte{0xc0, 0x0c, 0, 16, 0, 3, 0, 0, 0, 0, 0, byte(len(ip) + 1), byte(len(ip))}
	b := append(append(append(header, question...), rr...), []byte(ip)...)
	if got := dnsTXTAddresses(b, id); len(got) != 1 || got[0] != ip {
		t.Fatal("valid compressed answer rejected")
	}
	for n := 0; n < len(b); n++ {
		if got := dnsTXTAddresses(b[:n], id); len(got) != 0 {
			t.Fatalf("truncated reply accepted at %d", n)
		}
	}
	if got := dnsTXTAddresses(b, id+1); len(got) != 0 {
		t.Fatal("wrong transaction accepted")
	}
	bad := append([]byte{}, b...)
	binary.BigEndian.PutUint16(bad[4:6], 0)
	if got := dnsTXTAddresses(bad, id); len(got) != 0 {
		t.Fatal("missing question accepted")
	}
}
func TestLeakClassificationRequiresCompleteEvidence(t *testing.T) {
	good := LeakProbe{IPv4: "192.0.2.4", IPv6: "2001:db8::4", Requested: 50, Completed: 50, Resolvers: []DNSObservation{{IP: "192.0.2.53", Country: "US"}}, DNS: map[string][]string{"udp4": {"192.0.2.4"}, "tcp4": {"192.0.2.4"}, "udp6": {"2001:db8::4"}, "tcp6": {"192.0.2.4"}}}
	if state, _ := classifyLeak(good, good.IPv4, good.IPv6); state != "passed" {
		t.Fatal(state)
	}
	if state, _ := classifyLeak(good, "", good.IPv6); state != "incomplete" {
		t.Fatal("missing baseline treated as leak or success", state)
	}
	good.Completed = 49
	if state, _ := classifyLeak(good, good.IPv4, good.IPv6); state != "incomplete" {
		t.Fatal("partial resolver test passed")
	}
	good.Completed = 50
	good.DNS["udp4"] = []string{"198.51.100.9"}
	if state, _ := classifyLeak(good, good.IPv4, good.IPv6); state != "mismatch" {
		t.Fatal("DNS bypass undetected")
	}
}

func TestLeakFetchTransientFailureAndCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporary", 503)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()
	b, e := fetchProbe(context.Background(), server.Client(), server.URL)
	if e != nil || string(b) != "ok" || calls.Load() != 2 {
		t.Fatal("transient request did not recover")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = fetchProbe(ctx, server.Client(), server.URL); e == nil || calls.Load() != 2 {
		t.Fatal("cancelled request continued")
	}
}
