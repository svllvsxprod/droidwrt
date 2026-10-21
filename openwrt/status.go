package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func containerReady() bool {
	client := http.Client{Timeout: 2 * time.Second}
	resp, e := client.Get("http://172.29.255.2/cgi-bin/luci/")
	ready := e == nil
	if resp != nil {
		ready = resp.StatusCode == 200 || resp.StatusCode == 403
		resp.Body.Close()
	}
	return ready
}

func systemStatus() map[string]any {
	data := map[string]any{"ready": containerReady(), "profiles": profiles(), "vpn_state": "not_configured", "vpn_route_ready": false}
	if len(profiles()) > 0 {
		data["vpn_state"] = "profiles_saved"
	}
	if b, e := os.ReadFile("/proc/uptime"); e == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			v, _ := strconv.ParseFloat(f[0], 64)
			data["uptime_seconds"] = int(v)
			data["sample_time_ms"] = int64(v * 1000)
		}
	}
	if b, e := os.ReadFile("/proc/meminfo"); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) > 1 && (f[0] == "MemAvailable:" || f[0] == "MemTotal:") {
				v, _ := strconv.ParseInt(f[1], 10, 64)
				data[strings.TrimSuffix(f[0], ":")] = v * 1024
			}
		}
	}
	// Sum user through steal; guest counters are already included in user/nice.
	if b, e := os.ReadFile("/proc/stat"); e == nil {
		f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
		if len(f) >= 9 && f[0] == "cpu" {
			var total, idle uint64
			for i := 1; i <= 8; i++ {
				v, err := strconv.ParseUint(f[i], 10, 64)
				if err != nil {
					break
				}
				total += v
				if i == 4 || i == 5 {
					idle += v
				}
			}
			data["cpu_total"] = total
			data["cpu_idle"] = idle
		}
	}
	usb := "adb"
	links, _ := filepath.Glob("/config/usb_gadget/g1/configs/b.1/f*")
	for _, link := range links {
		target, _ := os.Readlink(link)
		if strings.Contains(target, "ncm.") {
			usb = "ncm"
		}
		if strings.Contains(target, "ecm.") {
			usb = "ecm"
		}
	}
	data["usb_mode"] = usb
	interfaces := []map[string]any{}
	if b, e := os.ReadFile("/proc/net/dev"); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			pair := strings.SplitN(line, ":", 2)
			if len(pair) != 2 {
				continue
			}
			name := strings.TrimSpace(pair[0])
			if !strings.HasPrefix(name, "ccmni") && !strings.HasPrefix(name, "wlan") && !strings.HasPrefix(name, "eth") && !strings.HasPrefix(name, "usb") {
				continue
			}
			f := strings.Fields(pair[1])
			if len(f) < 16 {
				continue
			}
			rx, _ := strconv.ParseUint(f[0], 10, 64)
			tx, _ := strconv.ParseUint(f[8], 10, 64)
			interfaces = append(interfaces, map[string]any{"name": name, "rx": rx, "tx": tx})
		}
	}
	router := routerSnapshot()
	data["router"] = router
	data["links"] = linkSnapshot()
	data["leaks"] = leakSnapshot()
	data["sharing"] = sharingSnapshot()
	data["vpn_route_ready"] = router["route_ready"]
	if router["primary"] == "direct" {
		data["vpn_state"] = "direct"
	} else if router["route_ready"] == true {
		data["vpn_state"] = "connected"
	} else if router["primary"] == "mihomo" || router["primary"] == "awg" {
		data["vpn_state"] = "connecting"
	}
	data["subscription"] = subscriptionSummary(readSubscription())
	data["interfaces"] = interfaces
	return data
}
