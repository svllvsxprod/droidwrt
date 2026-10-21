package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type DNSObservation struct {
	Country  string `json:"country"`
	IP       string `json:"ip"`
	Provider string `json:"provider"`
	Location string `json:"location"`
}
type LeakProbe struct {
	IPv4      string              `json:"ipv4"`
	IPv6      string              `json:"ipv6"`
	DNS       map[string][]string `json:"dns_observed_source"`
	Resolvers []DNSObservation    `json:"resolvers"`
	Completed int                 `json:"completed"`
	Requested int                 `json:"requested"`
}

func dnsSkipName(b []byte, o int) (int, bool) {
	for o < len(b) {
		n := int(b[o])
		o++
		if n == 0 {
			return o, true
		}
		if n&192 == 192 {
			return o + 1, o < len(b)
		}
		if n > 63 || o+n > len(b) {
			return 0, false
		}
		o += n
	}
	return 0, false
}
func dnsTXTAddresses(b []byte, id uint16) []string {
	if len(b) < 12 || binary.BigEndian.Uint16(b) != id || b[2]&128 == 0 || b[3]&15 != 0 || binary.BigEndian.Uint16(b[4:]) != 1 {
		return nil
	}
	o, ok := dnsSkipName(b, 12)
	if !ok || o+4 > len(b) {
		return nil
	}
	o += 4
	out := []string{}
	for j := 0; j < int(binary.BigEndian.Uint16(b[6:8])); j++ {
		o, ok = dnsSkipName(b, o)
		if !ok || o+10 > len(b) {
			return nil
		}
		typ := binary.BigEndian.Uint16(b[o:])
		size := int(binary.BigEndian.Uint16(b[o+8:]))
		o += 10
		end := o + size
		if end > len(b) {
			return nil
		}
		if typ == 16 {
			var txt strings.Builder
			for p := o; p < end; {
				length := int(b[p])
				p++
				if p+length > end {
					return nil
				}
				txt.Write(b[p : p+length])
				p += length
			}
			if ip := net.ParseIP(txt.String()); ip != nil {
				out = append(out, ip.String())
			}
		}
		o = end
	}
	return out
}
func dnsObservedSource(ctx context.Context, server, proto string) []string {
	d := net.Dialer{Timeout: 4 * time.Second}
	c, e := d.DialContext(ctx, proto, net.JoinHostPort(server, "53"))
	if e != nil {
		return nil
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(4 * time.Second))
	q := []byte{0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 6, 'w', 'h', 'o', 'a', 'm', 'i', 10, 'c', 'l', 'o', 'u', 'd', 'f', 'l', 'a', 'r', 'e', 0, 0, 16, 0, 3}
	if _, e = rand.Read(q[:2]); e != nil {
		return nil
	}
	id := binary.BigEndian.Uint16(q)
	if proto == "tcp" {
		q = append([]byte{0, byte(len(q))}, q...)
	}
	if _, e = c.Write(q); e != nil {
		return nil
	}
	b := make([]byte, 4096)
	var n int
	if proto == "tcp" {
		if _, e = io.ReadFull(c, b[:2]); e != nil {
			return nil
		}
		n = int(binary.BigEndian.Uint16(b[:2]))
		if n > len(b) {
			return nil
		}
		_, e = io.ReadFull(c, b[:n])
	} else {
		n, e = c.Read(b)
	}
	if e != nil {
		return nil
	}
	return dnsTXTAddresses(b[:n], id)
}
func leakHTTP(family string) *http.Client {
	resolverIP := "192.168.8.1"
	if family == "6" {
		resolverIP = "fd57:6477:7274:1::1"
	}
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 4 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(resolverIP, "53"))
	}}
	d := net.Dialer{Timeout: 5 * time.Second, Resolver: resolver}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return d.DialContext(ctx, "tcp"+family, address)
	}, TLSHandshakeTimeout: 5 * time.Second, MaxIdleConns: 4, IdleConnTimeout: time.Second}
	return &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
}
func fetchProbe(ctx context.Context, c *http.Client, url string) ([]byte, error) {
	b, e := fetchProbeOnce(ctx, c, url)
	if e == nil || ctx.Err() != nil {
		return b, e
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	return fetchProbeOnce(ctx, c, url)
}
func fetchProbeOnce(ctx context.Context, c *http.Client, url string) ([]byte, error) {
	r, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return nil, e
	}
	r.Header.Set("User-Agent", "DroidWRT diagnostics")
	resp, e := c.Do(r)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 65537))
}
func traceAddress(b []byte, family string) string {
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "ip=") {
			ip := net.ParseIP(strings.TrimPrefix(line, "ip="))
			if ip != nil && (ip.To4() == nil) == (family == "6") {
				return ip.String()
			}
		}
	}
	return ""
}
func runLeakProbe() {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	out := LeakProbe{DNS: map[string][]string{}, Resolvers: []DNSObservation{}, Requested: 50}
	var mu sync.Mutex
	var wg sync.WaitGroup
	c4, c6 := leakHTTP("4"), leakHTTP("6")
	defer c4.CloseIdleConnections()
	defer c6.CloseIdleConnections()
	for _, family := range []string{"4", "6"} {
		family := family
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, url := c4, "https://1.1.1.1/cdn-cgi/trace"
			if family == "6" {
				client, url = c6, "https://[2606:4700:4700::1111]/cdn-cgi/trace"
			}
			b, e := fetchProbe(ctx, client, url)
			if e == nil {
				mu.Lock()
				if family == "4" {
					out.IPv4 = traceAddress(b, family)
				} else {
					out.IPv6 = traceAddress(b, family)
				}
				mu.Unlock()
			}
		}()
		for _, proto := range []string{"udp", "tcp"} {
			proto := proto
			wg.Add(1)
			go func() {
				defer wg.Done()
				server := "192.168.8.1"
				if family == "6" {
					server = "fd57:6477:7274:1::1"
				}
				ips := dnsObservedSource(ctx, server, proto)
				if len(ips) == 0 && ctx.Err() == nil {
					ips = dnsObservedSource(ctx, server, proto)
				}
				mu.Lock()
				out.DNS[proto+family] = ips
				mu.Unlock()
			}()
		}
	}
	// dns4/dns6 select authoritative DNS transport, not the HTTP address family.
	// The result endpoints are fetched over IPv4; IPv6 egress is checked separately.
	// No cookies, subscriptions, VPN config, or credentials are sent to this service.
	jobs := make(chan int, 50)
	for i := 0; i < 50; i++ {
		jobs <- i
	}
	close(jobs)
	observed := map[string]DNSObservation{}
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				token := make([]byte, 6) // BrowserLeaks requires a 12-character label.
				if _, e := rand.Read(token); e != nil {
					return
				}
				family, tld := "4", "org"
				if i%2 == 1 {
					family = "6"
				}
				if i < 4 {
					tld = "net"
				}
				host := hex.EncodeToString(token) + ".dns" + family + ".browserleaks." + tld
				b, e := fetchProbe(ctx, c4, "https://"+host)
				if e != nil || len(b) > 65536 {
					continue
				}
				var rows map[string]json.RawMessage
				if json.Unmarshal(b, &rows) != nil {
					continue
				}
				valid := false
				entries := []DNSObservation{}
				for address, raw := range rows {
					ip := net.ParseIP(address)
					if ip == nil {
						continue
					}
					var values []string
					if json.Unmarshal(raw, &values) != nil || len(values) < 3 || len(values[0]) != 2 || len(values[1]) > 160 || len(values[2]) > 160 {
						continue
					}
					entries = append(entries, DNSObservation{strings.ToUpper(values[0]), ip.String(), values[2], values[1]})
					valid = true
				}
				mu.Lock()
				if valid {
					out.Completed++
					for _, entry := range entries {
						observed[entry.IP] = entry
					}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for _, entry := range observed {
		out.Resolvers = append(out.Resolvers, entry)
	}
	sort.Slice(out.Resolvers, func(i, j int) bool { return out.Resolvers[i].IP < out.Resolvers[j].IP })
	json.NewEncoder(os.Stdout).Encode(out)
}
