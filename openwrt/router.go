package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type RouterConfig struct {
	Primary   string `json:"primary"`
	Preferred string `json:"preferred"`
	Mihomo    string `json:"mihomo"`
	AWG       string `json:"awg"`
}

var routerLock sync.Mutex
var routerHeartbeat atomic.Int64
var routerTelemetryLock sync.RWMutex
var routerTelemetry = map[string]any{"primary": "blocked", "mihomo_state": "unknown", "awg_state": "unknown", "route_ready": false}

func readRouter() RouterConfig {
	var c RouterConfig
	b, _ := os.ReadFile(moduleDir + "/router.json")
	json.Unmarshal(b, &c)
	if c.Primary == "" {
		c.Primary = "blocked"
	}
	if c.Preferred == "" {
		c.Preferred = c.Primary
	}
	return c
}
func saveRouter(c RouterConfig) error {
	b, _ := json.Marshal(c)
	return atomicPrivate(moduleDir+"/router.json", b)
}
func controlSecret() string {
	b, _ := os.ReadFile(moduleDir + "/control-token")
	return strings.TrimSpace(string(b))
}
func mihomoHealthy() bool {
	// A single remote site's timeout must not stand in for core availability.
	for _, target := range []string{"https://www.gstatic.com/generate_204", "https://1.1.1.1/cdn-cgi/trace"} {
		if mihomoProbe(target) {
			return true
		}
	}
	return false
}
func mihomoProbe(target string) bool {
	client := http.Client{Timeout: 9 * time.Second}
	req, _ := http.NewRequest("GET", "http://172.29.255.2:9090/proxies/VPN/delay?timeout=8000&url="+url.QueryEscape(target), nil)
	req.Header.Set("Authorization", "Bearer "+controlSecret())
	r, e := client.Do(req)
	if e != nil {
		return false
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return false
	}
	var data struct {
		Delay *int `json:"delay"`
	}
	return json.NewDecoder(r.Body).Decode(&data) == nil && data.Delay != nil && *data.Delay >= 0
}

// Distinguish an unavailable core from a live core whose remote probe timed out.
func mihomoControlReady() bool {
	client := http.Client{Timeout: 700 * time.Millisecond}
	req, _ := http.NewRequest("GET", "http://172.29.255.2:9090/version", nil)
	req.Header.Set("Authorization", "Bearer "+controlSecret())
	r, err := client.Do(req)
	if err != nil {
		return false
	}
	defer r.Body.Close()
	return r.StatusCode == http.StatusOK
}
func startMihomo(id string) error {
	if !profileIDPattern.MatchString(id) {
		return errors.New("Неверный профиль")
	}
	b, e := os.ReadFile(filepath.Join(profileRoot, id+".mihomo.json"))
	if e != nil {
		return errors.New("Профиль Mihomo не найден")
	}
	var cfg map[string]any
	if json.Unmarshal(b, &cfg) != nil {
		return errors.New("Повреждённый профиль")
	}
	mihomoTransportDefaults(cfg)
	cfg["external-controller"] = "172.29.255.2:9090"
	cfg["secret"] = controlSecret()
	cfg["mixed-port"] = 7890
	cfg["allow-lan"] = false
	cfg["bind-address"] = "127.0.0.1"
	cfg["ipv6"] = true
	cfg["tun"] = map[string]any{"enable": true, "device": "dwr-vpn", "stack": "gvisor", "mtu": 1400, "auto-route": false, "auto-detect-interface": false, "dns-hijack": []string{"any:53", "tcp://any:53"}, "inet6-address": []string{"fdfe:dcba:9876::1/126"}}
	// Client DNS must use the assigned VPN, even when a server disallows port 53.
	// Only resolving the VPN server itself uses the independent bootstrap resolver.
	cfg["dns"] = map[string]any{
		"enable": true, "ipv6": true, "enhanced-mode": "redir-host",
		"nameserver":              []string{"https://1.1.1.1/dns-query#VPN"},
		"proxy-server-nameserver": []string{"https://1.1.1.1/dns-query"},
		"default-nameserver":      []string{"1.1.1.1"},
	}
	b, _ = json.Marshal(cfg)
	if e = atomicPrivate(containerRoot+"/etc/droidwrt/active-mihomo.json", b); e != nil {
		return e
	}
	if _, e = containerCommand("sysctl", "-w", "net.ipv6.conf.default.disable_ipv6=0"); e != nil {
		return e
	}
	containerCommand("/etc/init.d/droidwrt-mihomo", "enable")
	if _, e = containerCommand("/etc/init.d/droidwrt-mihomo", "restart"); e != nil {
		return errors.New("Не удалось запустить Mihomo")
	}
	for i := 0; i < 4; i++ {
		time.Sleep(700 * time.Millisecond)
		if mihomoHealthy() {
			return nil
		}
	}
	return errors.New("Mihomo запущен, но проверка VPN не прошла")
}
func configuredAWGDNS() (string, error) {
	id, e := os.ReadFile(containerRoot + "/etc/droidwrt/awg-enabled")
	if e != nil {
		return "1.1.1.1", nil
	}
	name := strings.TrimSpace(string(id))
	if !profileIDPattern.MatchString(name) {
		return "", errors.New("Некорректный профиль AWG")
	}
	raw, e := os.ReadFile(filepath.Join(profileRoot, name+".conf"))
	if e != nil {
		return "", e
	}
	for _, line := range strings.Split(string(raw), "\n") {
		p := strings.SplitN(line, "=", 2)
		if len(p) != 2 || strings.TrimSpace(p[0]) != "DNS" {
			continue
		}
		for _, value := range strings.Split(p[1], ",") {
			ip := net.ParseIP(strings.TrimSpace(value))
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				return ip.String(), nil
			}
		}
		return "", errors.New("Для DNS AWG требуется IPv4-адрес")
	}
	return "1.1.1.1", nil
}
func applyPrimary(engine string) error {
	dns, e := configuredAWGDNS()
	if e != nil {
		return e
	}
	if e = atomicPrivate(containerRoot+"/etc/droidwrt/awg-dns", []byte(dns)); e != nil {
		return e
	}

	if _, e := containerCommand("/usr/sbin/droidwrt-policy", engine); e != nil {
		return errors.New("Не удалось применить маршруты")
	}
	return nil
}
func routerSnapshot() map[string]any {
	routerTelemetryLock.RLock()
	defer routerTelemetryLock.RUnlock()
	copy := map[string]any{}
	for k, v := range routerTelemetry {
		copy[k] = v
	}
	return copy
}
func updateRouterTelemetry(c RouterConfig, mihomo bool, a map[string]any) {
	mstate, astate := "not_configured", "not_configured"
	if c.Mihomo != "" {
		mstate = "reconnecting"
		if mihomo {
			mstate = "connected"
		} else if mihomoControlReady() {
			mstate = "unresponsive"
		}
	}
	if c.AWG != "" {
		astate = "reconnecting"
		if a["tunnel_ready"] == true {
			astate = "connected"
		}
	}
	// Preserve API compatibility while distinguishing loss from a slow path.
	if c.AWG != "" && tunnelAssessments["awg"] != nil {
		astate = tunnelAssessments["awg"].State
	}
	if c.Mihomo != "" && tunnelAssessments["mihomo"] != nil {
		mstate = tunnelAssessments["mihomo"].State
	}
	ready := c.Primary == "direct" || (c.Primary == "mihomo" && mihomo) || (c.Primary == "awg" && a["tunnel_ready"] == true)
	routerTelemetryLock.Lock()
	routerTelemetry = map[string]any{"primary": c.Primary, "preferred": c.Preferred, "automatic_reserve": c.Primary != c.Preferred && (c.Primary == "awg" || c.Primary == "mihomo"), "mihomo_profile": c.Mihomo, "awg_profile": c.AWG, "mihomo_state": mstate, "awg_state": astate, "route_ready": ready, "awg_rx": a["rx_bytes"], "awg_tx": a["tx_bytes"], "checked_at": time.Now().Unix()}
	routerTelemetryLock.Unlock()
}
func routerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		jsonReply(w, routerSnapshot())
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var p struct {
		Action  string `json:"action"`
		Engine  string `json:"engine"`
		Profile string `json:"profile"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&p) != nil {
		http.Error(w, "Некорректный запрос", 400)
		return
	}
	routerLock.Lock()
	defer routerLock.Unlock()
	lock, available := recoveryLock()
	if !available {
		http.Error(w, "Восстанавливается сеть. Повторите через несколько секунд.", 503)
		return
	}
	defer lock.Close()
	c := readRouter()
	before := c
	var err error
	switch p.Action {
	case "assign":
		switch p.Engine {
		case "mihomo":
			err = startMihomo(p.Profile)
			if err == nil {
				c.Mihomo = p.Profile
			} else if before.Mihomo != "" {
				startMihomo(before.Mihomo)
			}
		case "awg":
			err = startAWG(p.Profile)
			if err == nil {
				out, e := containerCommand("curl", "-4", "--interface", "awg0", "--silent", "--output", "/dev/null", "--max-time", "20", "--write-out", "%{http_code}", "https://www.gstatic.com/generate_204")
				if e != nil || out != "204" {
					err = errors.New("AWG запущен, но проверка VPN не прошла")
				}
			}
			if err == nil {
				c.AWG = p.Profile
				containerCommand("/etc/init.d/droidwrt-awg", "enable")
			} else if before.AWG != "" {
				startAWG(before.AWG)
			}
		default:
			err = errors.New("Доступны Mihomo и AWG")
		}
	case "select":
		switch p.Engine {
		case "direct":
			c.Primary = "direct"
		case "mihomo":
			if c.Mihomo == "" || !tunnelHealth(c)["mihomo"] {
				err = errors.New("Сначала подключите профиль Mihomo")
			} else {
				c.Primary = "mihomo"
			}
		case "awg":
			if c.AWG == "" || !tunnelHealth(c)["awg"] {
				err = errors.New("Сначала подключите профиль AWG")
			} else {
				c.Primary = "awg"
			}
		default:
			err = errors.New("Неверный режим")
		}
	default:
		err = errors.New("Неверное действие")
	}
	if err == nil && p.Action == "select" {
		c.Preferred = c.Primary
	}
	if err == nil {
		err = applyPrimary(c.Primary)
	}
	if err == nil {
		err = saveRouter(c)
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	recordHealthEvent("manual_action", map[string]any{"action": p.Action, "engine": p.Engine, "primary": c.Primary})
	h := tunnelHealth(c)
	a := awgHealth()
	a["tunnel_ready"] = h["awg"]
	updateRouterTelemetry(c, h["mihomo"], a)
	jsonReply(w, routerSnapshot())
}
func recoveryLock() (*os.File, bool) {
	f, e := os.OpenFile(moduleDir+"/recovery.lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, false
	}
	return f, true
}
func routerMonitor() {
	routerHeartbeat.Store(controlUptime())
	initialUplink, _ := os.ReadFile(moduleDir + "/uplink-state")
	recovery := mihomoRecovery{uplink: strings.TrimSpace(string(initialUplink))}
	failover := failoverState{}
	awgRepair := awgRecovery{}
	awgTransport := loadAWGTransportRecovery()
	for {
		func() {
			defer func() { routerHeartbeat.Store(controlUptime()) }()
			routerLock.Lock()
			defer routerLock.Unlock()
			lock, ok := recoveryLock()
			if !ok {
				return
			}
			defer lock.Close()
			c := readRouter()
			if _, err := os.Stat(moduleDir + "/network-repair-request"); err == nil {
				updateRouterTelemetry(c, false, map[string]any{})
				return
			}
			if _, err := containerPID(); err != nil {
				updateRouterTelemetry(c, false, map[string]any{})
				return
			}
			// Reclaim a replugged LAN adapter before Android can select it as WAN.
			// On missing guard rules, normal policy recovery below runs first.
			reconcileEthernet(readSharing(), &SharingStatus{})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			uplinkErr := exec.CommandContext(ctx, "/system/bin/sh", moduleDir+"/uplink.sh").Run()
			cancel()
			// Repair routing before probing. Never open direct mode during recovery.
			if applyPrimary(c.Primary) != nil {
				updateRouterTelemetry(c, false, map[string]any{})
				return
			}
			healthy := tunnelHealth(c)
			a := awgHealth()
			target, reason := failover.choose(time.Now(), c, healthy)
			if target != c.Primary {
				if err := applyPrimary(target); err == nil {
					old := c.Primary
					c.Primary = target
					if saveRouter(c) == nil {
						failover.lastSwitch = time.Now()
						b, _ := json.Marshal(map[string]any{"from": old, "to": target, "preferred": c.Preferred, "reason": reason, "at": time.Now().Unix()})
						atomicPrivate(moduleDir+"/failover.json", b)
						recordHealthEvent("failover", map[string]any{"from": old, "to": target, "reason": reason})
					}
				}
			}
			// Publish the selected reserve BEFORE attempting to repair the old engine.
			a["tunnel_ready"] = healthy["awg"]
			updateRouterTelemetry(c, healthy["mihomo"], a)
			if c.AWG != "" {
				recoverAWGTransport(&awgTransport, c, healthy, a)
				if reason := awgRepair.check(time.Now(), a["running"] == true && a["configured"] == true, healthy["awg"]); reason != "" {
					err := startAWG(c.AWG)
					b, _ := json.Marshal(map[string]any{"at": time.Now().Unix(), "reason": reason, "restart_ok": err == nil})
					atomicPrivate(moduleDir+"/awg-recovery.json", b)
					recordHealthEvent("awg_recovery", map[string]any{"reason": reason, "restart_ok": err == nil})
				}
			}
			if c.Mihomo != "" {
				uplink := ""
				if uplinkErr == nil {
					b, _ := os.ReadFile(moduleDir + "/uplink-state")
					uplink = strings.TrimSpace(string(b))
				}
				if reason := recovery.check(time.Now(), uplink, mihomoControlReady(), healthy["mihomo"]); reason != "" {
					_, err := containerCommand("/etc/init.d/droidwrt-mihomo", "restart")
					b, _ := json.Marshal(map[string]any{"reason": reason, "at": time.Now().Unix(), "restart_ok": err == nil})
					atomicPrivate(moduleDir+"/mihomo-recovery.json", b)
					recordHealthEvent("mihomo_recovery", map[string]any{"reason": reason, "restart_ok": err == nil})
				}
			}
			reconcileSharing(readSharing())
		}()
		time.Sleep(3 * time.Second)
	}
}
