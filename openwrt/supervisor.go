package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const moduleDir = "/data/adb/modules/droidwrt_router"

func controlSystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var p struct {
		Action  string `json:"action"`
		Enabled *bool  `json:"enabled"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 512)).Decode(&p) != nil || p.Enabled == nil || p.Action != "wifi" {
		http.Error(w, "Invalid action", 400)
		return
	}
	value := "disabled"
	if *p.Enabled {
		value = "enabled"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if exec.CommandContext(ctx, "/system/bin/cmd", "wifi", "set-wifi-enabled", value).Run() != nil {
		http.Error(w, "Не удалось изменить состояние Wi-Fi", 503)
		return
	}
	jsonReply(w, map[string]any{"accepted": true})
}
func probe(path string) bool {
	c := http.Client{Timeout: 3 * time.Second}
	r, e := c.Get("http://127.0.0.1:8080" + path)
	if e != nil {
		return false
	}
	defer r.Body.Close()
	return r.StatusCode == 204
}
func networkCheck() string {
	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/system/bin/sh", moduleDir+"/network-watch.sh", "check").Output()
	if err != nil {
		return "init"
	}
	reason := strings.TrimSpace(string(b))
	switch reason {
	case "healthy", "missing", "init", "netifd", "dnsmasq", "odhcpd", "uhttpd", "policy":
		return reason
	}
	return "init"
}
func repairContainer(reason string) bool {
	// Give recovery priority over the frequent routing loop, otherwise their
	// periodic schedules can repeatedly miss the short unlocked interval.
	atomicPrivate(moduleDir+"/network-repair-request", []byte(reason))
	lock, ok := recoveryLock()
	if !ok {
		return false
	}
	defer lock.Close()
	defer os.Remove(moduleDir + "/network-repair-request")
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, "/data/local/Droidspaces/bin/busybox", "timeout", "65", "/system/bin/sh", moduleDir+"/network-watch.sh", "repair", reason).Run()
	b, _ := json.Marshal(map[string]any{"reason": reason, "at": time.Now().Unix(), "command_ok": err == nil})
	atomicPrivate(moduleDir+"/network-recovery.json", b)
	if err != nil {
		log.Print("Network recovery incomplete")
	}
	return true
}
func supervise() {
	f, e := os.OpenFile(moduleDir+"/supervisor.lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		log.Fatal(e)
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return
	}
	exe, _ := os.Executable()
	os.Remove(moduleDir + "/network-repair-request")
	// If only the previous supervisor died, its API child may still own :8080.
	// Match the exact executable and empty argument list, never probe children.
	entries, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, path := range entries {
		b, _ := os.ReadFile(path)
		if string(b) == exe+"\x00" {
			pid, _ := strconv.Atoi(filepath.Base(filepath.Dir(path)))
			if pid > 1 && pid != os.Getpid() {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	}
	var child *exec.Cmd
	var done chan error
	apiFailures, containerFailures := 0, 0
	delay := 5 * time.Second
	nextRepair := time.Time{}
	started := time.Time{}
	nextNetworkCheck := time.Time{}
	networkReason := "unknown"
	previousReason := ""
	for {
		if _, e := os.Stat(moduleDir + "/maintenance"); e == nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if child == nil {
			child = exec.Command(exe)
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			if e := child.Start(); e != nil {
				child = nil
				time.Sleep(delay)
				continue
			}
			done = make(chan error, 1)
			go func(cmd *exec.Cmd) { done <- cmd.Wait() }(child)
			started = time.Now()
			log.Print("Control service started")
		}
		select {
		case <-done:
			child = nil
			apiFailures = 0
			log.Print("Control service exited; restarting")
			time.Sleep(delay)
			if time.Since(started) < time.Minute && delay < time.Minute {
				delay *= 2
			}
			continue
		default:
		}
		if probe("/droidwrt/healthz") {
			apiFailures = 0
			if time.Since(started) > time.Minute {
				delay = 5 * time.Second
			}
		} else {
			apiFailures++
		}
		if apiFailures >= 3 {
			log.Print("Control service stalled; restarting")
			child.Process.Kill()
			<-done
			child = nil
			apiFailures = 0
			time.Sleep(delay)
			continue
		}
		if time.Now().After(nextNetworkCheck) {
			networkReason = networkCheck()
			nextNetworkCheck = time.Now().Add(3 * time.Second)
			if networkReason == "healthy" {
				containerFailures = 0
				previousReason = ""
				// A transient failure can clear before recovery acquires the lock.
				// Do not leave the routing loop paused for an obsolete request.
				os.Remove(moduleDir + "/network-repair-request")
			} else {
				if previousReason != networkReason {
					containerFailures = 0
				}
				containerFailures++
				previousReason = networkReason
			}
			if containerFailures >= 2 && time.Now().After(nextRepair) {
				log.Print("Recovering network: ", networkReason)
				if repairContainer(networkReason) {
					nextRepair = time.Now().Add(20 * time.Second)
					containerFailures = 0
				}
			}
		}
		// Tiny bounded status file: no configs, credentials or remote addresses.
		payload, _ := json.Marshal(map[string]any{"checked_at": time.Now().Unix(), "control_failures": apiFailures, "container_failures": containerFailures, "network_state": networkReason})
		temporary := filepath.Join(moduleDir, "health.json.tmp")
		if os.WriteFile(temporary, payload, 0600) == nil {
			os.Rename(temporary, filepath.Join(moduleDir, "health.json"))
		}
		time.Sleep(2 * time.Second)
	}
}
