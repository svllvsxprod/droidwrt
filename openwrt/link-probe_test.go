package main

import (
	"errors"
	"testing"
)

func TestTraceRetriesFailedEndpoint(t *testing.T) {
	calls := 0
	r := probeLinkTrace(4, "profile-a", func(target string) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("timeout")
		}
		if target != "https://1.0.0.1/cdn-cgi/trace" {
			t.Fatal(target)
		}
		return "ip=192.0.2.42\nloc=DE\ndwr_ttfb=1.25\n", nil
	})
	if calls != 2 || r["state"] != "verified" || r["latency_ms"] != 1250 || r["error"] != nil {
		t.Fatal(r)
	}
}

func TestTraceDoesNotClaimSuccessOnOutageOrWrongFamily(t *testing.T) {
	for _, fail := range []bool{true, false} {
		for _, family := range []int{4, 6} {
			r := probeLinkTrace(family, "profile-b", func(string) (string, error) {
				if fail {
					return "ip=192.0.2.42", errors.New("partial response")
				}
				if family == 4 {
					return "ip=2001:db8::1", nil
				}
				return "ip=192.0.2.42", nil
			})
			if r["state"] != "unavailable" || r["ip"] != nil || r["attempts"] != 2 {
				t.Fatal(r)
			}
		}
	}
}

func TestIPv6TraceNeedsNoRetryOnSuccess(t *testing.T) {
	calls := 0
	r := probeLinkTrace(6, "profile-c", func(string) (string, error) { calls++; return "ip=2001:db8::1\nloc=US", nil })
	if calls != 1 || r["state"] != "verified" || r["country"] != "US" {
		t.Fatal(r)
	}
}
