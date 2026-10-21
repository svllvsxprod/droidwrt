package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const profileDir = "/data/local/Droidspaces/Containers/DroidWRT/etc/droidwrt/profiles"

var profileLock sync.Mutex
var profileRoot = profileDir

type Profile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	State string `json:"state"`
}

func jsonReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
func validKey(s string) bool {
	b, e := base64.StdEncoding.DecodeString(s)
	return e == nil && len(b) == 32
}
func parseAWG(raw string) error {
	section := ""
	peer := 0
	private := false
	address := false
	pubs := 0
	endpoints := 0
	allowed := 0
	ifaceKeys := "PrivateKey Address DNS MTU ListenPort Jc Jmin Jmax S1 S2 S3 S4 H1 H2 H3 H4 I1 I2 I3 I4 I5 HeaderProtectionKey ContentPaddingAddition RekeyAfterTime RekeyTimeout RejectAfterTime KeepaliveTimeout MaxHandshakeAttempts RandomTrailers DisableCookies"
	peerKeys := "PublicKey PresharedKey Endpoint AllowedIPs PersistentKeepalive"
	seen := map[string]bool{}
	scan := bufio.NewScanner(strings.NewReader(raw))
	scan.Buffer(make([]byte, 4096), 262144)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			switch line {
			case "[Interface]":
				if private || section != "" {
					return errors.New("Секция Interface должна быть первой и единственной")
				}
				section = "Interface"
			case "[Peer]":
				section = "Peer"
				peer++
			default:
				return errors.New("Неизвестная секция AWG")
			}
			continue
		}
		pair := strings.SplitN(line, "=", 2)
		if len(pair) != 2 || section == "" {
			return errors.New("Ожидается конфиг AWG с секциями Interface и Peer")
		}
		key := strings.TrimSpace(pair[0])
		value := strings.TrimSpace(pair[1])
		keys := ifaceKeys
		if section == "Peer" {
			keys = peerKeys
		}
		if !strings.Contains(" "+keys+" ", " "+key+" ") {
			return fmt.Errorf("Параметр %s пока не поддерживается. Команды PreUp/PostUp не выполняются", key)
		}
		id := fmt.Sprintf("%s:%d:%s", section, peer, key)
		if seen[id] {
			return fmt.Errorf("Параметр %s повторяется", key)
		}
		seen[id] = true
		if value == "" {
			return fmt.Errorf("Параметр %s не заполнен", key)
		}
		if strings.Contains(" Jc Jmin Jmax MTU ListenPort S1 S2 S3 S4 ", " "+key+" ") {
			bits := 32
			if key == "ListenPort" || strings.HasPrefix(key, "S") {
				bits = 16
			}
			n, e := strconv.ParseUint(value, 10, bits)
			if e != nil || (key == "MTU" && (n < 576 || n > 65535)) {
				return fmt.Errorf("Некорректное число в %s", key)
			}
		}
		if strings.Contains(" H1 H2 H3 H4 ContentPaddingAddition RekeyAfterTime RekeyTimeout RejectAfterTime KeepaliveTimeout MaxHandshakeAttempts PersistentKeepalive ", " "+key+" ") {
			if value != "(off)" {
				parts := strings.Split(value, "-")
				if len(parts) > 2 {
					return fmt.Errorf("Некорректный диапазон в %s", key)
				}
				var last uint64
				for i, v := range parts {
					n, e := strconv.ParseUint(v, 10, 32)
					if e != nil || (i > 0 && n < last) {
						return fmt.Errorf("Некорректный диапазон в %s", key)
					}
					last = n
				}
			}
		}
		if key == "RandomTrailers" || key == "DisableCookies" {
			if _, e := strconv.ParseBool(value); e != nil && value != "on" && value != "off" {
				return fmt.Errorf("Некорректное значение в %s", key)
			}
		}
		switch key {
		case "PrivateKey", "PublicKey", "PresharedKey", "HeaderProtectionKey":
			if !validKey(value) {
				return fmt.Errorf("Некорректный ключ в %s", key)
			}
			if key == "PrivateKey" {
				private = true
			}
			if key == "PublicKey" {
				pubs++
			}
		case "Address", "AllowedIPs":
			for _, v := range strings.Split(value, ",") {
				if _, _, e := net.ParseCIDR(strings.TrimSpace(v)); e != nil {
					return fmt.Errorf("Некорректная сеть в %s", key)
				}
			}
			if key == "Address" {
				address = true
			} else {
				allowed++
			}
		case "Endpoint":
			host, port, e := net.SplitHostPort(value)
			n, ne := strconv.Atoi(port)
			if e != nil || host == "" || ne != nil || n < 1 || n > 65535 {
				return errors.New("Endpoint должен содержать адрес сервера и порт")
			}
			endpoints++
		}
	}
	if scan.Err() != nil {
		return errors.New("Конфиг слишком большой")
	}
	if !private || !address || peer == 0 || pubs != peer || allowed != peer || endpoints != peer {
		return errors.New("В AWG нужны PrivateKey, Address и для каждого Peer — PublicKey, Endpoint, AllowedIPs")
	}
	return nil
}
func vlessLink(raw string) (map[string]any, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "vless" || u.User == nil || u.User.Username() == "" || u.Hostname() == "" {
		return nil, errors.New("Некорректная ссылка VLESS")
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return nil, errors.New("В ссылке VLESS нужен порт сервера")
	}
	q := u.Query()
	allowed := "type security sni fp pbk sid spx pqv encryption flow host path mode extra alpn allowInsecure"
	for k, vs := range q {
		if !strings.Contains(" "+allowed+" ", " "+k+" ") {
			return nil, fmt.Errorf("Параметр ссылки %s пока не поддерживается: используйте JSON Xray", k)
		}
		if len(vs) != 1 {
			return nil, fmt.Errorf("Параметр %s повторяется", k)
		}
	}
	network := q.Get("type")
	if network == "" {
		network = "tcp"
	}
	if network != "xhttp" && network != "tcp" && network != "raw" {
		return nil, errors.New("Ссылка поддерживает XHTTP и RAW/TCP; для другого транспорта используйте JSON Xray")
	}
	security := q.Get("security")
	if security != "reality" && security != "tls" {
		return nil, errors.New("Нужна ссылка VLESS с REALITY или TLS")
	}
	enc := q.Get("encryption")
	if enc == "" {
		enc = "none"
	}
	user := map[string]any{"id": u.User.Username(), "encryption": enc}
	if q.Get("flow") != "" {
		user["flow"] = q.Get("flow")
	}
	stream := map[string]any{"network": network, "security": security}
	if security == "reality" {
		if q.Get("pbk") == "" || q.Get("sni") == "" {
			return nil, errors.New("В REALITY нужны pbk и sni")
		}
		fp := q.Get("fp")
		if fp == "" {
			fp = "chrome"
		}
		r := map[string]any{"serverName": q.Get("sni"), "fingerprint": fp, "publicKey": q.Get("pbk"), "shortId": q.Get("sid")}
		if q.Get("spx") != "" {
			r["spiderX"] = q.Get("spx")
		}
		if q.Get("pqv") != "" {
			r["mldsa65Verify"] = q.Get("pqv")
		}
		stream["realitySettings"] = r
	} else {
		t := map[string]any{"serverName": q.Get("sni")}
		if q.Get("fp") != "" {
			t["fingerprint"] = q.Get("fp")
		}
		if q.Get("alpn") != "" {
			t["alpn"] = strings.Split(q.Get("alpn"), ",")
		}
		if q.Get("allowInsecure") == "1" || q.Get("allowInsecure") == "true" {
			return nil, errors.New("Проверка сертификата TLS должна быть включена")
		}
		stream["tlsSettings"] = t
	}
	if network == "xhttp" {
		mode := q.Get("mode")
		if mode == "" {
			mode = "auto"
		}
		path := q.Get("path")
		if path == "" {
			path = "/"
		}
		x := map[string]any{"mode": mode, "host": q.Get("host"), "path": path}
		if q.Get("extra") != "" {
			var extra map[string]any
			if json.Unmarshal([]byte(q.Get("extra")), &extra) != nil {
				return nil, errors.New("Некорректный JSON в XHTTP extra")
			}
			x["extra"] = extra
		}
		stream["xhttpSettings"] = x
	}
	return map[string]any{"tag": "vpn", "protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": u.Hostname(), "port": port, "users": []any{user}}}}, "streamSettings": stream}, nil
}
func normalizeProfile(raw string) (string, []byte, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
	if strings.HasPrefix(raw, "[Interface]") || strings.HasPrefix(raw, "#") {
		if e := parseAWG(raw); e != nil {
			return "", nil, e
		}
		return "AWG", []byte(raw + "\n"), nil
	}
	var outbound map[string]any
	if strings.HasPrefix(raw, "vless://") {
		o, e := vlessLink(raw)
		if e != nil {
			return "", nil, e
		}
		outbound = o
	} else if strings.HasPrefix(raw, "{") {
		var cfg map[string]any
		if json.Unmarshal([]byte(raw), &cfg) != nil {
			return "", nil, errors.New("Некорректный JSON")
		}
		if cfg["protocol"] == "vless" {
			outbound = cfg
		} else {
			arr, ok := cfg["outbounds"].([]any)
			if !ok {
				return "", nil, errors.New("В JSON не найден исходящий сервер VLESS")
			}
			for _, v := range arr {
				if o, ok := v.(map[string]any); ok && o["protocol"] == "vless" {
					if outbound != nil {
						return "", nil, errors.New("В JSON несколько серверов VLESS. Экспортируйте один профиль")
					}
					outbound = o
				}
			}
		}
		if outbound == nil {
			return "", nil, errors.New("В JSON не найден сервер VLESS")
		}
		clean := map[string]any{"tag": "vpn", "protocol": "vless"}
		for _, k := range []string{"settings", "streamSettings", "mux"} {
			if v, ok := outbound[k]; ok {
				clean[k] = v
			}
		}
		outbound = clean
	} else {
		return "", nil, errors.New("Нужен AWG .conf, ссылка vless:// или JSON Xray. Для vpn:// экспортируйте конфиг AWG")
	}
	b, e := json.MarshalIndent(map[string]any{"log": map[string]any{"loglevel": "warning"}, "outbounds": []any{outbound}}, "", "  ")
	return "VLESS", b, e
}
func profiles() []Profile {
	result := []Profile{}
	files, _ := filepath.Glob(filepath.Join(profileRoot, "*.meta.json"))
	for _, f := range files {
		var p Profile
		if b, e := os.ReadFile(f); e == nil && json.Unmarshal(b, &p) == nil {
			result = append(result, p)
		}
	}
	result = append(result, readSubscription().Profiles...)
	return result
}
func installProfile(name, raw string) (Profile, error) {
	profileLock.Lock()
	defer profileLock.Unlock()
	var p Profile
	if len(raw) > 262144 {
		return p, errors.New("Конфиг больше 256 КБ")
	}
	if len(profiles()) >= 32 {
		return p, errors.New("Сохранено уже 32 профиля")
	}
	kind, content, e := normalizeProfile(raw)
	if e != nil {
		return p, e
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = kind
	}
	if len([]rune(name)) > 64 {
		return p, errors.New("Название должно быть короче 64 символов")
	}
	id := make([]byte, 16)
	if _, e = rand.Read(id); e != nil {
		return p, e
	}
	p = Profile{hex.EncodeToString(id), name, kind, "saved"}
	if e = os.MkdirAll(profileRoot, 0700); e != nil {
		return p, e
	}
	ext := ".conf"
	if kind == "VLESS" {
		ext = ".json"
	}
	f := filepath.Join(profileRoot, p.ID+ext)
	if e = atomicPrivate(f, content); e != nil {
		return p, e
	}
	if kind == "VLESS" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/data/local/Droidspaces/bin/droidspaces", "--name=DroidWRT", "run", "/usr/bin/xray", "run", "-test", "-c", "/etc/droidwrt/profiles/"+p.ID+ext)
		if cmd.Run() != nil {
			os.Remove(f)
			return p, errors.New("Xray отклонил параметры профиля. Проверьте экспорт и версию сервера")
		}
	}
	b, _ := json.Marshal(p)
	if e = atomicPrivate(filepath.Join(profileRoot, p.ID+".meta.json"), b); e != nil {
		os.Remove(f)
		return p, e
	}
	return p, nil
}
func replaceAWGProfile(id, raw string) (Profile, error) {
	profileLock.Lock()
	defer profileLock.Unlock()
	var p Profile
	if !profileIDPattern.MatchString(id) || len(raw) > 262144 {
		return p, errors.New("Некорректное обновление профиля")
	}
	b, e := os.ReadFile(filepath.Join(profileRoot, id+".meta.json"))
	if e != nil || json.Unmarshal(b, &p) != nil || p.ID != id || p.Kind != "AWG" {
		return p, errors.New("Профиль AWG не найден")
	}
	kind, content, e := normalizeProfile(raw)
	if e != nil {
		return p, e
	}
	if kind != "AWG" {
		return p, errors.New("Ожидается конфиг AWG")
	}
	path := filepath.Join(profileRoot, id+".conf")
	previous, e := os.ReadFile(path)
	if e != nil {
		return p, e
	}
	if e = atomicPrivate(path+".previous", previous); e != nil {
		return p, e
	}
	if e = atomicPrivate(path, content); e != nil {
		return p, e
	}
	return p, nil
}
func profileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		jsonReply(w, profiles())
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var v struct {
		Name      string `json:"name"`
		Content   string `json:"content"`
		ReplaceID string `json:"replace_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 300000)).Decode(&v) != nil {
		http.Error(w, "Не удалось прочитать конфиг", 400)
		return
	}
	var p Profile
	var e error
	if v.ReplaceID != "" {
		routerLock.Lock()
		p, e = replaceAWGProfile(v.ReplaceID, v.Content)
		if e == nil {
			linkMu.Lock()
			delete(linkData, "awg")
			delete(linkData, "awg_ipv6")
			linkMu.Unlock()
		}
		routerLock.Unlock()
	} else {
		p, e = installProfile(v.Name, v.Content)
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	jsonReply(w, p)
}
