package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

type LeakReport struct {
	State     string            `json:"state"`
	Mode      string            `json:"mode"`
	Profile   string            `json:"profile"`
	CheckedAt int64             `json:"checked_at"`
	Running   bool              `json:"running"`
	Checks    map[string]string `json:"checks"`
	Probe     *LeakProbe        `json:"probe,omitempty"`
}

var leakMu sync.RWMutex
var leakData = LeakReport{State: "pending"}
var leakWake = make(chan struct{}, 1)

func leakSnapshot() LeakReport { leakMu.RLock(); defer leakMu.RUnlock(); return leakData }
func leakHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		jsonReply(w, leakSnapshot())
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	select {
	case leakWake <- struct{}{}:
	default:
	}
	jsonReply(w, map[string]bool{"accepted": true})
}
func currentLeakKey() (RouterConfig, string, string) {
	c := readRouter()
	profile := c.AWG
	if c.Primary == "mihomo" {
		profile = c.Mihomo
	}
	if c.Primary == "direct" {
		profile = "direct"
	}
	uplink, _ := os.ReadFile(moduleDir + "/uplink-state")
	path := profileRoot + "/" + profile + ".conf"
	if c.Primary == "mihomo" {
		path = profileRoot + "/" + profile + ".mihomo.json"
	}
	revision := ""
	if st, e := os.Stat(path); e == nil {
		revision = st.ModTime().String()
	}
	return c, profile, c.Primary + ":" + profile + ":" + revision + ":" + string(uplink)
}
func expectedLeakIP(engine, profile string, v6 bool) string {
	name := engine
	if v6 {
		name += "_ipv6"
	}
	snapshot := linkSnapshot()
	entry, ok := snapshot[name].(map[string]any)
	if !ok || entry["state"] != "verified" || entry["profile"] != profile {
		return ""
	}
	checked, ok := entry["checked_at"].(int64)
	if !ok || time.Now().Unix()-checked > 90 {
		return ""
	}
	ip, _ := entry["ip"].(string)
	return ip
}
func classifyLeak(p LeakProbe, expected4, expected6 string) (string, map[string]string) {
	checks := map[string]string{"ipv4": "unknown", "ipv6": "unknown", "dns": "unknown", "resolvers": "unknown"}
	state := "passed"
	for _, v := range []struct{ name, actual, expected string }{{"ipv4", p.IPv4, expected4}, {"ipv6", p.IPv6, expected6}} {
		if v.actual == "" || v.expected == "" {
			checks[v.name] = "unknown"
			state = "incomplete"
		} else if v.actual != v.expected {
			checks[v.name] = "mismatch"
			state = "mismatch"
		} else {
			checks[v.name] = "passed"
		}
	}
	dns := "passed"
	for _, transport := range []string{"udp4", "tcp4", "udp6", "tcp6"} {
		ips := p.DNS[transport]
		if len(ips) == 0 {
			if dns != "mismatch" {
				dns = "unknown"
			}
			continue
		}
		for _, ip := range ips {
			if ip != expected4 && ip != expected6 {
				if expected4 == "" || expected6 == "" {
					if dns != "mismatch" {
						dns = "unknown"
					}
				} else {
					dns = "mismatch"
				}
			}
		}
	}
	checks["dns"] = dns
	if p.Completed == p.Requested && p.Requested == 50 && len(p.Resolvers) > 0 {
		checks["resolvers"] = "passed"
	}
	for _, v := range checks {
		if v == "mismatch" {
			return "mismatch", checks
		}
		if v != "passed" {
			state = "incomplete"
		}
	}
	return state, checks
}

type leakClient struct {
	cmd       *exec.Cmd
	done      chan error
	container string
}

func (c *leakClient) stop() {
	if c.cmd != nil {
		c.cmd.Process.Kill()
		<-c.done
		c.cmd = nil
	}
}
func (c *leakClient) ensure() error {
	p, e := containerPID()
	if e != nil {
		return e
	}
	if c.cmd != nil {
		select {
		case <-c.done:
			c.cmd = nil
		default:
			if c.container == p {
				return nil
			}
			c.stop()
		}
	}
	cmd := exec.Command("/data/local/Droidspaces/bin/busybox", "unshare", "-n", "/system/bin/sleep", "2147483647")
	configureLeakChild(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	c.cmd = cmd
	c.done = make(chan error, 1)
	go func() { c.done <- cmd.Wait() }()
	c.container = p
	// unshare happens after Start; wait until the network namespace is distinct.
	host, _ := os.Readlink("/proc/self/ns/net")
	ready := false
	for i := 0; i < 40; i++ {
		ns, _ := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", cmd.Process.Pid))
		if ns != "" && ns != host {
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		c.stop()
		return fmt.Errorf("probe namespace not ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if e = exec.CommandContext(ctx, "/system/bin/sh", moduleDir+"/leak-client.sh", p, strconv.Itoa(cmd.Process.Pid)).Run(); e != nil {
		c.stop()
		return e
	}
	return nil
}
func leakMonitor() {
	var client leakClient
	defer client.stop()
	lastKey := ""
	next := time.Time{}
	for {
		forced := false
		select {
		case <-leakWake:
			forced = true
		default:
		}
		c, profile, key := currentLeakKey()
		if c.Primary == "direct" {
			leakMu.Lock()
			leakData = LeakReport{State: "direct", Mode: c.Primary, Profile: profile}
			leakMu.Unlock()
			lastKey = ""
			time.Sleep(time.Second)
			continue
		}
		if c.Primary != "awg" && c.Primary != "mihomo" {
			time.Sleep(time.Second)
			continue
		}
		if !forced && key == lastKey && time.Now().Before(next) {
			time.Sleep(time.Second)
			continue
		}
		lastKey = key
		next = time.Now().Add(5 * time.Minute)
		leakMu.Lock()
		leakData = LeakReport{State: "checking", Mode: c.Primary, Profile: profile, Running: true}
		leakMu.Unlock()
		report := LeakReport{State: "incomplete", Mode: c.Primary, Profile: profile}
		if e := client.ensure(); e == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
			cmd := exec.CommandContext(ctx, "/data/local/Droidspaces/bin/busybox", "nsenter", "-t", strconv.Itoa(client.cmd.Process.Pid), "-n", moduleDir+"/local-proxy", "--leak-probe")
			cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+containerRoot+"/etc/ssl/certs/ca-certificates.crt")
			b, e := cmd.Output()
			cancel()
			var p LeakProbe
			if e == nil && json.Unmarshal(b, &p) == nil {
				report.Probe = &p
				report.State, report.Checks = classifyLeak(p, expectedLeakIP(c.Primary, profile, false), expectedLeakIP(c.Primary, profile, true))
			}
		}
		_, _, after := currentLeakKey()
		if after != key {
			lastKey = ""
			continue
		}
		report.CheckedAt = time.Now().Unix()
		if report.State != "passed" {
			next = time.Now().Add(time.Minute)
		}
		leakMu.Lock()
		leakData = report
		leakMu.Unlock()
	}
}
