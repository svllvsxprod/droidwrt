package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type SharingConfig struct {
	USB         string `json:"usb"`
	USBDebug    bool   `json:"usb_debug"`
	Hotspot     bool   `json:"hotspot"`
	EthernetLAN bool   `json:"ethernet_lan"`
}
type SharingStatus struct {
	USB               string `json:"usb"`
	USBDebug          bool   `json:"usb_debug"`
	Hotspot           bool   `json:"hotspot"`
	SSID              string `json:"ssid"`
	Band              string `json:"band"`
	Security          string `json:"security"`
	HotspotState      string `json:"hotspot_state"`
	State             string `json:"state"`
	Carrier           bool   `json:"carrier"`
	CheckedAt         int64  `json:"checked_at"`
	EthernetLAN       bool   `json:"ethernet_lan"`
	EthernetState     string `json:"ethernet_state"`
	EthernetInterface string `json:"ethernet_interface"`
	EthernetCarrier   bool   `json:"ethernet_carrier"`
}

var sharingMu sync.RWMutex
var sharingData = SharingStatus{USB: "off", State: "pending"}

func validUSB(mode string) bool { return mode == "off" || mode == "ncm" || mode == "ecm" }
func readSharing() SharingConfig {
	b, _ := os.ReadFile(moduleDir + "/sharing.json")
	return decodeSharing(b)
}
func decodeSharing(b []byte) SharingConfig {
	c := SharingConfig{USB: "off", USBDebug: true}
	if len(b) > 0 && json.Unmarshal(b, &c) != nil {
		return SharingConfig{USB: "off", USBDebug: true}
	}
	if !validUSB(c.USB) {
		c.USB = "off"
	}
	return c
}
func sharingSnapshot() SharingStatus {
	sharingMu.RLock()
	defer sharingMu.RUnlock()
	return sharingData
}

// Called with routerLock held so USB cannot be attached in the middle of a policy update.
func reconcileSharing(c SharingConfig) error {
	result := SharingStatus{USB: c.USB, USBDebug: c.USBDebug, Hotspot: c.Hotspot, EthernetLAN: c.EthernetLAN, EthernetState: "unavailable", HotspotState: "unavailable", State: "unavailable", CheckedAt: time.Now().Unix()}
	defer func() { sharingMu.Lock(); sharingData = result; sharingMu.Unlock() }()
	if c.USB != "off" || c.Hotspot || c.EthernetLAN {
		if _, e := containerCommand("/usr/sbin/droidwrt-lan-sharing"); e != nil {
			return e
		}
	}
	if e := reconcileEthernet(c, &result); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	debug := "off"
	if c.USBDebug {
		debug = "on"
	}
	b, e := exec.CommandContext(ctx, "/system/bin/sh", moduleDir+"/usb-sharing.sh", c.USB, debug).Output()
	if e != nil {
		return errors.New("Не удалось настроить USB. Проверьте кабель и режим USB в настройках Android.")
	}
	if c.USB == "off" {
		result.State = "disabled"
		return reconcileHotspot(c, &result)
	}
	iface := strings.TrimSpace(string(b))
	if iface != "usb0" && iface != "usb1" && iface != "usb2" {
		return errors.New("USB-интерфейс не найден")
	}
	state, e := containerCommand("ip", "-j", "link", "show", "dev", iface)
	if e != nil {
		return e
	}
	var links []struct {
		Flags []string `json:"flags"`
	}
	if json.Unmarshal([]byte(state), &links) != nil || len(links) != 1 {
		return errors.New("Нет состояния USB")
	}
	result.State = "ready"
	for _, flag := range links[0].Flags {
		if flag == "LOWER_UP" {
			result.Carrier = true
		}
	}
	return reconcileHotspot(c, &result)
}

func reconcileEthernet(c SharingConfig, result *SharingStatus) error {
	mode := "off"
	if c.EthernetLAN {
		mode = "on"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/system/bin/sh", moduleDir+"/ethernet-sharing.sh", mode).Output()
	if err != nil {
		return errors.New("Не удалось изменить режим RJ45. Переподключите адаптер и повторите.")
	}
	var state struct {
		State     string `json:"state"`
		Interface string `json:"interface"`
		Carrier   bool   `json:"carrier"`
	}
	if json.Unmarshal(b, &state) != nil {
		return errors.New("Нет состояния RJ45")
	}
	result.EthernetState, result.EthernetInterface, result.EthernetCarrier = state.State, state.Interface, state.Carrier
	return nil
}
func reconcileHotspot(c SharingConfig, result *SharingStatus) error {
	cfg, e := readHotspotConfig()
	if e != nil {
		return e
	}
	result.SSID = cfg.SSID
	result.Band = cfg.Band
	result.Security = cfg.Security
	result.HotspotState = "unavailable"
	mode := "off"
	if c.Hotspot {
		mode = "on"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := exec.CommandContext(ctx, "/system/bin/sh", moduleDir+"/hotspot.sh", mode).Run(); e != nil {
		return errors.New("Не удалось настроить точку доступа OpenWrt")
	}
	result.HotspotState = "disabled"
	if c.Hotspot {
		result.HotspotState = "ready"
	}
	return nil
}
func sharingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		jsonReply(w, sharingSnapshot())
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var request struct {
		USB         *string `json:"usb"`
		USBDebug    *bool   `json:"usb_debug"`
		Hotspot     *bool   `json:"hotspot"`
		EthernetLAN *bool   `json:"ethernet_lan"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
	dec.DisallowUnknownFields()
	if dec.Decode(&request) != nil || (request.USB == nil && request.Hotspot == nil && request.USBDebug == nil && request.EthernetLAN == nil) || (request.USB != nil && !validUSB(*request.USB)) {
		http.Error(w, "Выберите NCM, ECM или выключите USB-раздачу", 400)
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
	c := readSharing()
	before := c
	if request.USB != nil {
		c.USB = *request.USB
	}
	if request.USBDebug != nil {
		c.USBDebug = *request.USBDebug
	}
	if request.Hotspot != nil {
		c.Hotspot = *request.Hotspot
	}
	if request.EthernetLAN != nil {
		c.EthernetLAN = *request.EthernetLAN
	}
	if c.USB != "off" || c.Hotspot || c.EthernetLAN {
		if e := applyPrimary(readRouter().Primary); e != nil {
			http.Error(w, e.Error(), 503)
			return
		}
	}
	if e := reconcileSharing(c); e != nil {
		if c.EthernetLAN != before.EthernetLAN {
			reconcileEthernet(before, &SharingStatus{})
		}
		http.Error(w, e.Error(), 503)
		return
	}
	b, _ := json.Marshal(c)
	if e := atomicPrivate(moduleDir+"/sharing.json", b); e != nil {
		if c.EthernetLAN != before.EthernetLAN {
			reconcileEthernet(before, &SharingStatus{})
		}
		http.Error(w, "Не удалось сохранить настройку", 500)
		return
	}
	jsonReply(w, sharingSnapshot())
}

func hotspotPassword() (string, error) {
	path := moduleDir + "/hotspot-pass"
	if b, e := os.ReadFile(path); e == nil {
		value := strings.TrimSpace(string(b))
		if len(value) >= 8 && len(value) <= 63 {
			return value, nil
		}
		return "", errors.New("Invalid hotspot password")
	}
	b := make([]byte, 18)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	value := base64.RawURLEncoding.EncodeToString(b)
	return value, atomicPrivate(path, []byte(value))
}
