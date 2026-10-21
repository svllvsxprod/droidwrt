package main

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var dnsLock sync.Mutex
var dnsCache []string
var dnsUntil time.Time
var defaultNetworkPattern = regexp.MustCompile(`(?m)^Active default network: ([0-9]+)`)
var dnsPattern = regexp.MustCompile(`DnsAddresses: \[([^\]]*)\]`)

func androidDNSServers() ([]string, error) {
	dnsLock.Lock()
	defer dnsLock.Unlock()
	if time.Now().Before(dnsUntil) && len(dnsCache) > 0 {
		return append([]string{}, dnsCache...), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/system/bin/dumpsys", "connectivity").Output()
	if e != nil {
		return nil, errors.New("Android network unavailable")
	}
	text := string(b)
	id := defaultNetworkPattern.FindStringSubmatch(text)
	if len(id) != 2 {
		return nil, errors.New("No default Android network")
	}
	servers := []string{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "NetworkAgentInfo{network{"+id[1]+"}") {
			continue
		}
		m := dnsPattern.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		for _, v := range strings.Split(m[1], ",") {
			ip := strings.TrimPrefix(strings.TrimSpace(v), "/")
			if net.ParseIP(ip) != nil {
				servers = append(servers, ip)
			}
		}
		break
	}
	if len(servers) == 0 {
		return nil, errors.New("No Android DNS servers")
	}
	dnsCache = servers
	dnsUntil = time.Now().Add(10 * time.Second)
	return append([]string{}, servers...), nil
}
func useAndroidResolver() {
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		servers, e := androidDNSServers()
		if e != nil {
			return nil, e
		}
		d := net.Dialer{Timeout: 3 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(servers[0], "53"))
	}}
}
