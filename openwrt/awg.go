package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const containerRoot = "/data/local/Droidspaces/Containers/DroidWRT"

var profileIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func containerCommand(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/data/local/Droidspaces/bin/droidspaces", append([]string{"--name=DroidWRT", "run"}, args...)...)
	b, e := cmd.Output()
	return strings.TrimSpace(string(b)), e
}
func containerPID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/data/local/Droidspaces/bin/droidspaces", "--name=DroidWRT", "pid").Output()
	s := strings.TrimSpace(string(b))
	if _, err := strconv.Atoi(s); e != nil || err != nil {
		return "", errors.New("Container unavailable")
	}
	return s, nil
}
func awgSocket() (string, error) {
	pid, e := containerPID()
	if e != nil {
		return "", e
	}
	return "/proc/" + pid + "/root/tmp/run/amneziawg/awg0.sock", nil
}
func awgUAPI(request string) (string, error) {
	path, e := awgSocket()
	if e != nil {
		return "", e
	}
	c, e := net.DialTimeout("unix", path, 700*time.Millisecond)
	if e != nil {
		return "", e
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	if _, e = c.Write([]byte(request + "\n\n")); e != nil {
		return "", e
	}
	s := bufio.NewScanner(c)
	var result strings.Builder
	ok := false
	for s.Scan() {
		line := s.Text()
		if line == "" {
			break
		}
		if line == "errno=0" {
			ok = true
		}
		result.WriteString(line + "\n")
	}
	if !ok {
		return "", errors.New("AWG rejected configuration")
	}
	return result.String(), s.Err()
}
func awgPlan(raw string) (string, []string, int, error) {
	if e := parseAWG(raw); e != nil {
		return "", nil, 0, e
	}
	var b strings.Builder
	b.WriteString("set=1\nreplace_peers=true\n")
	addresses := []string{}
	mtu := 1380
	mapping := map[string]string{"PrivateKey": "private_key", "PublicKey": "public_key", "PresharedKey": "preshared_key", "ListenPort": "listen_port", "Endpoint": "endpoint", "PersistentKeepalive": "persistent_keepalive_interval", "HeaderProtectionKey": "header_protection_key", "ContentPaddingAddition": "content_padding_addition", "RekeyAfterTime": "rekey_after_time", "RekeyTimeout": "rekey_timeout", "RejectAfterTime": "reject_after_time", "KeepaliveTimeout": "keepalive_timeout", "MaxHandshakeAttempts": "max_handshake_attempts", "RandomTrailers": "random_trailers", "DisableCookies": "disable_cookies"}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		p := strings.SplitN(line, "=", 2)
		k, v := strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
		switch k {
		case "Address":
			for _, a := range strings.Split(v, ",") {
				addresses = append(addresses, strings.TrimSpace(a))
			}
			continue
		case "DNS":
			continue
		case "MTU":
			mtu, _ = strconv.Atoi(v)
			continue
		case "PrivateKey", "PublicKey", "PresharedKey", "HeaderProtectionKey":
			key, _ := base64.StdEncoding.DecodeString(v)
			v = hex.EncodeToString(key)
		case "RandomTrailers", "DisableCookies":
			if v == "on" {
				v = "true"
			}
			if v == "off" {
				v = "false"
			}
		case "Endpoint":
			host, port, e := net.SplitHostPort(v)
			if e != nil {
				return "", nil, 0, e
			}
			if net.ParseIP(host) == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				ips, e := net.DefaultResolver.LookupIP(ctx, "ip4", host)
				cancel()
				if e != nil || len(ips) == 0 {
					return "", nil, 0, errors.New("Не удалось определить адрес AWG-сервера")
				}
				host = ips[0].String()
			}
			v = net.JoinHostPort(host, port)
		case "AllowedIPs":
			b.WriteString("replace_allowed_ips=true\n")
			for _, cidr := range strings.Split(v, ",") {
				b.WriteString("allowed_ip=" + strings.TrimSpace(cidr) + "\n")
			}
			continue
		}
		key, ok := mapping[k]
		if !ok {
			key = strings.ToLower(k)
		}
		b.WriteString(key + "=" + v + "\n")
	}
	return strings.TrimSpace(b.String()), addresses, mtu, nil
}
func startAWG(id string) error {
	if !profileIDPattern.MatchString(id) {
		return errors.New("Invalid profile")
	}
	raw, e := os.ReadFile(filepath.Join(profileRoot, id+".conf"))
	if e != nil {
		return errors.New("Профиль AWG не найден")
	}
	request, addresses, mtu, e := awgPlan(string(raw))
	if e != nil {
		return e
	}
	if e = atomicPrivate(containerRoot+"/etc/droidwrt/awg-enabled", []byte(id)); e != nil {
		return e
	}
	containerCommand("/etc/init.d/droidwrt-awg", "stop")
	if _, e = containerCommand("/etc/init.d/droidwrt-awg", "start"); e != nil {
		return errors.New("Не удалось запустить AWG")
	}
	available := false
	for i := 0; i < 25; i++ {
		if _, e = awgUAPI("get=1"); e == nil {
			available = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !available {
		return errors.New("AWG не создал управляющий сокет")
	}
	if _, e = awgUAPI(request); e != nil {
		return e
	}
	if _, e = containerCommand("sysctl", "-w", "net.ipv6.conf.awg0.disable_ipv6=0"); e != nil {
		return e
	}
	for _, a := range addresses {
		if _, e = containerCommand("ip", "address", "replace", a, "dev", "awg0"); e != nil {
			return errors.New("Не удалось назначить адрес AWG")
		}
	}
	if _, e = containerCommand("ip", "link", "set", "awg0", "mtu", strconv.Itoa(mtu), "up"); e != nil {
		return e
	}
	// Bound local probes use this table; the core's encrypted UDP uses ordinary WAN.
	if _, e = containerCommand("ip", "route", "replace", "default", "dev", "awg0", "table", "201"); e != nil {
		return e
	}
	containerCommand("ip", "rule", "del", "pref", "10201")
	if _, e = containerCommand("ip", "rule", "add", "pref", "10201", "oif", "awg0", "lookup", "201"); e != nil {
		return e
	}
	containerCommand("ip", "-6", "rule", "del", "pref", "10201")
	if _, e = containerCommand("ip", "-6", "rule", "add", "pref", "10201", "oif", "awg0", "lookup", "201"); e != nil {
		return e
	}
	return nil
}
func awgHealth() map[string]any {
	result := map[string]any{"running": false, "handshake_recent": false}
	raw, e := awgUAPI("get=1")
	if e != nil {
		return result
	}
	result["running"] = true
	result["configured"] = strings.Contains(raw, "private_key=")
	var last int64
	var rx, tx uint64
	for _, line := range strings.Split(raw, "\n") {
		p := strings.SplitN(line, "=", 2)
		if len(p) != 2 {
			continue
		}
		switch p[0] {
		case "last_handshake_time_sec":
			v, _ := strconv.ParseInt(p[1], 10, 64)
			if v > last {
				last = v
			}
		case "rx_bytes":
			v, _ := strconv.ParseUint(p[1], 10, 64)
			rx += v
		case "tx_bytes":
			v, _ := strconv.ParseUint(p[1], 10, 64)
			tx += v
		}
	}
	age := time.Now().Unix() - last
	result["handshake_age_seconds"] = handshakeAge(last, time.Now().Unix())
	result["handshake_recent"] = last > 0 && age >= 0 && age < 180
	result["rx_bytes"] = rx
	result["tx_bytes"] = tx
	return result
}
func handshakeAge(last, now int64) any {
	if last <= 0 || last > now {
		return nil
	}
	return now - last
}
func testAWG(id string) {
	if e := startAWG(id); e != nil {
		fmt.Println("AWG setup failed:", e)
		os.Exit(1)
	}
	out, e := containerCommand("curl", "-4", "--interface", "awg0", "--silent", "--output", "/dev/null", "--max-time", "25", "--write-out", "%{http_code}", "https://www.gstatic.com/generate_204")
	h := awgHealth()
	fmt.Printf("AWG HTTPS=%s successful=%t recent_handshake=%v\n", out, e == nil && out == "204", h["handshake_recent"])
}
