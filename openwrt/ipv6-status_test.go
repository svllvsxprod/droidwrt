package main

import "testing"

func TestIPv6AddressRequiresAssignedAddress(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"Address = 10.0.0.2/32\nAllowedIPs = 0.0.0.0/0, ::/0", false},
		{"Address = 10.0.0.2/32, fd00::2/128", true},
		{"Address = ::/128", false},
		{"Address = ::ffff:10.0.0.2/128", false},
		{"# Address = fd00::2/128\nAddress = 10.0.0.2/32", false},
	} {
		if got := hasIPv6Address(tc.raw); got != tc.want {
			t.Errorf("hasIPv6Address(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
