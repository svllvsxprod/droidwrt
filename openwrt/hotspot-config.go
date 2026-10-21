package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type HotspotConfig struct {
	SSID     string `json:"ssid"`
	Band     string `json:"band"`
	Security string `json:"security"`
	Password string `json:"password"`
}
type HotspotCapabilities struct {
	Available bool `json:"available"`
	Band24    bool `json:"band24"`
	Band5     bool `json:"band5"`
	WPA3      bool `json:"wpa3"`
}

var apCapabilityPattern = regexp.MustCompile(`mCurrentSoftApCapability: SupportedFeatures=([0-9]+)`)

func parseAPCapabilities(raw string) HotspotCapabilities {
	m := apCapabilityPattern.FindStringSubmatch(raw)
	if len(m) != 2 {
		return HotspotCapabilities{}
	}
	bits, e := strconv.ParseUint(m[1], 10, 64)
	if e != nil {
		return HotspotCapabilities{}
	}
	return HotspotCapabilities{true, bits&32 != 0, bits&64 != 0, bits&4 != 0}
}
func hotspotCapabilities() HotspotCapabilities {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/system/bin/dumpsys", "wifi").Output()
	if e != nil {
		return HotspotCapabilities{}
	}
	return parseAPCapabilities(string(b))
}
func validateHotspot(c HotspotConfig) error {
	if !utf8.ValidString(c.SSID) || len(c.SSID) < 1 || len(c.SSID) > 32 || strings.TrimSpace(c.SSID) == "" {
		return errors.New("Имя сети: от 1 до 32 байт UTF-8")
	}
	for _, r := range c.SSID {
		if unicode.IsControl(r) {
			return errors.New("В имени сети недопустимы управляющие символы")
		}
	}
	if c.Band != "2.4" && c.Band != "5" {
		return errors.New("Выберите диапазон 2,4 или 5 ГГц")
	}
	switch c.Security {
	case "open":
		if c.Password != "" {
			return errors.New("Открытая сеть не использует пароль")
		}
		return nil
	case "wpa2", "wpa2-wpa3", "wpa3":
	default:
		return errors.New("Выберите поддерживаемый протокол защиты")
	}
	// Portable passphrases across PSK, SAE and client QR scanners.
	if len(c.Password) < 8 || len(c.Password) > 63 {
		return errors.New("Пароль: от 8 до 63 символов")
	}
	for _, r := range c.Password {
		if r < 32 || r > 126 {
			return errors.New("Для пароля используйте латинские буквы, цифры и знаки ASCII")
		}
	}
	return nil
}
func checkHotspotCapabilities(c HotspotConfig, caps HotspotCapabilities) error {
	if !caps.Available {
		return errors.New("Не удалось проверить возможности Wi-Fi. Повторите попытку")
	}
	if c.Band == "5" && !caps.Band5 || c.Band == "2.4" && !caps.Band24 {
		return errors.New("Этот диапазон не поддерживается точкой доступа")
	}
	if (c.Security == "wpa3" || c.Security == "wpa2-wpa3") && !caps.WPA3 {
		return errors.New("WPA3 не поддерживается точкой доступа")
	}
	return nil
}

// All callers hold routerLock. Migrate the existing password without changing the radio.
func readHotspotConfig() (HotspotConfig, error) {
	var c HotspotConfig
	b, e := os.ReadFile(moduleDir + "/hotspot-config.json")
	if e == nil {
		if json.Unmarshal(b, &c) != nil {
			return c, errors.New("Повреждены настройки Wi-Fi")
		}
		return c, validateHotspot(c)
	}
	if !os.IsNotExist(e) {
		return c, e
	}
	pass, e := hotspotPassword()
	if e != nil {
		return c, e
	}
	c = HotspotConfig{"DroidWRT", "5", "wpa2", pass}
	b, _ = json.Marshal(c)
	if e = atomicPrivate(moduleDir+"/hotspot-config.json", b); e != nil {
		return c, e
	}
	// An already owned radio was started with these exact legacy settings.
	if _, e = os.Stat(moduleDir + "/hotspot-owned"); e == nil {
		if e = atomicPrivate(moduleDir+"/hotspot-applied.json", b); e != nil {
			return c, e
		}
	}
	return c, nil
}
func storeHotspot(c HotspotConfig) error {
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	return atomicPrivate(moduleDir+"/hotspot-config.json", b)
}

// Roll back both desired and radio configuration if applying a new configuration fails.
func commitHotspot(old, next HotspotConfig, save func(HotspotConfig) error, apply func() error) error {
	if old == next {
		return nil
	}
	if e := save(next); e != nil {
		return errors.New("Не удалось сохранить настройки Wi-Fi")
	}
	if e := apply(); e != nil {
		if save(old) != nil {
			return errors.New("Не удалось применить и восстановить настройки Wi-Fi")
		}
		if apply() != nil {
			return errors.New("Прежние настройки сохранены; точка доступа ожидает восстановления")
		}
		return errors.New("Телефон не запустил выбранную конфигурацию. Прежние настройки восстановлены")
	}
	return nil
}
func hotspotAccessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var next HotspotConfig
	if r.Method == "POST" {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		dec.DisallowUnknownFields()
		if dec.Decode(&next) != nil || dec.Decode(new(any)) != io.EOF {
			http.Error(w, "Некорректные параметры Wi-Fi", 400)
			return
		}
		if e := validateHotspot(next); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
	}
	routerLock.Lock()
	defer routerLock.Unlock()
	old, e := readHotspotConfig()
	if e != nil {
		http.Error(w, "Не удалось прочитать настройки Wi-Fi", 500)
		return
	}
	caps := hotspotCapabilities()
	if r.Method == "POST" {
		if e = checkHotspotCapabilities(next, caps); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		c := readSharing()
		e = commitHotspot(old, next, storeHotspot, func() error {
			if !c.Hotspot {
				return nil
			}
			result := sharingSnapshot()
			e := reconcileHotspot(c, &result)
			sharingMu.Lock()
			sharingData = result
			sharingMu.Unlock()
			return e
		})
		if e != nil {
			http.Error(w, e.Error(), 503)
			return
		}
		old = next
		sharingMu.Lock()
		sharingData.SSID = next.SSID
		sharingData.Band = next.Band
		sharingData.Security = next.Security
		sharingMu.Unlock()
	}
	jsonReply(w, map[string]any{"ssid": old.SSID, "band": old.Band, "security": old.Security, "password": old.Password, "capabilities": caps})
}
