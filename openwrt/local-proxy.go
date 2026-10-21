// The Android UI reaches LuCI over loopback; the container stays isolated.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--leak-probe" {
		runLeakProbe()
		return
	}
	useAndroidResolver()
	if len(os.Args) == 3 && os.Args[1] == "--test-awg" {
		testAWG(os.Args[2])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--supervise" {
		supervise()
		return
	}
	target, _ := url.Parse("http://172.29.255.2")
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Host = target.Host
	}
	token, err := os.ReadFile("/data/adb/modules/droidwrt_router/control-token")
	if err != nil {
		log.Fatal("Control token unavailable")
	}
	secret := strings.TrimSpace(string(token))
	if len(secret) < 32 {
		log.Fatal("Invalid control token")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/droidwrt/healthz", func(w http.ResponseWriter, r *http.Request) {
		last := routerHeartbeat.Load()
		if heartbeatStalled(last, controlUptime()) {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("/droidwrt/readyz", func(w http.ResponseWriter, r *http.Request) {
		if containerReady() {
			w.WriteHeader(204)
		} else {
			w.WriteHeader(503)
		}
	})
	mux.HandleFunc("/droidwrt/power", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
			http.Error(w, "Unauthorized", 401)
			return
		}
		var request struct {
			Action string `json:"action"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&request) != nil ||
			(request.Action != "shutdown" && request.Action != "reboot") {
			http.Error(w, "Invalid action", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"accepted\":true}"))
		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := exec.Command("/system/bin/svc", "power", request.Action).Run(); err != nil {
				log.Print("Power action failed: ", err)
			}
		}()
	})
	authorize := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
				http.Error(w, "Unauthorized", 401)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/droidwrt/router", authorize(routerHandler))
	mux.HandleFunc("/droidwrt/subscription", authorize(subscriptionHandler))
	mux.HandleFunc("/droidwrt/system", authorize(controlSystem))
	mux.HandleFunc("/droidwrt/profiles", authorize(profileHandler))
	mux.HandleFunc("/droidwrt/leaks", authorize(leakHandler))
	mux.HandleFunc("/droidwrt/sharing", authorize(sharingHandler))
	mux.HandleFunc("/droidwrt/hotspot-access", authorize(hotspotAccessHandler))
	mux.HandleFunc("/droidwrt/status", authorize(func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, systemStatus())
	}))
	mux.Handle("/", proxy)
	server := &http.Server{
		Addr: "127.0.0.1:8080", Handler: mux,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}
	recordHealthEvent("control_started", map[string]any{"pid": os.Getpid(), "policy": "adaptive-v3-transport"})
	go routerPowerMonitor()
	go routerMonitor()
	go linkMonitor()
	go ipv6Monitor()
	go leakMonitor()
	log.Fatal(server.ListenAndServe())
}
