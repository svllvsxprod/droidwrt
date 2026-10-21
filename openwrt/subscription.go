package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const subscriptionFile = moduleDir + "/subscription.json"

var subscriptionLock sync.Mutex
var subscriptionPath = subscriptionFile

// The URL and profile contents are private. Only summary metadata is returned by GET.
type Subscription struct {
	URL      string    `json:"url"`
	Updated  int64     `json:"updated"`
	Profiles []Profile `json:"profiles"`
}

func atomicPrivate(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".pending-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(name, path); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func readSubscription() Subscription {
	var s Subscription
	b, _ := os.ReadFile(subscriptionPath)
	json.Unmarshal(b, &s)
	return s
}
func subscriptionSummary(s Subscription) map[string]any {
	return map[string]any{"configured": s.URL != "", "updated": s.Updated, "count": len(s.Profiles)}
}
func mihomoProxy(p map[string]any) ([]byte, error) {
	if p["type"] != "vless" {
		return nil, errors.New("Подписка содержит неподдерживаемый тип VPN")
	}
	allowed := " name type server port uuid udp tls servername client-fingerprint fingerprint reality-opts network xhttp-opts packet-encoding flow encryption alpn skip-cert-verify smux ws-opts h2-opts http-opts grpc-opts "
	clean := map[string]any{}
	for k, v := range p {
		if !strings.Contains(allowed, " "+k+" ") {
			return nil, errors.New("Неподдерживаемое поле профиля Mihomo: " + k)
		}
		clean[k] = v
	}
	if clean["skip-cert-verify"] == true {
		return nil, errors.New("Проверка сертификата должна быть включена")
	}
	for _, key := range []string{"server", "uuid"} {
		if v, ok := clean[key].(string); !ok || v == "" {
			return nil, errors.New("Неполный профиль Mihomo")
		}
	}
	if clean["tls"] != true {
		return nil, errors.New("Для VLESS требуется TLS или Reality")
	}
	clean["name"] = "VPN"
	// Never import the subscription's controller, listeners, DNS, routing or file paths.
	return json.Marshal(map[string]any{"mode": "rule", "log-level": "warning", "ipv6": false, "proxies": []any{clean}, "rules": []string{"MATCH,VPN"}})
}
func subscriptionConfigs(raw []byte) ([]map[string]any, error) {
	var cfg map[string]any
	if yaml.Unmarshal(raw, &cfg) != nil {
		return nil, errors.New("Не удалось прочитать подписку Mihomo/Clash")
	}
	list, ok := cfg["proxies"].([]any)
	if !ok || len(list) == 0 || len(list) > 32 {
		return nil, errors.New("Подписка должна содержать от 1 до 32 профилей")
	}
	result := []map[string]any{}
	for _, v := range list {
		p, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("Некорректный профиль")
		}
		result = append(result, p)
	}
	return result, nil
}
func validateMihomo(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	containerPath := "/etc/droidwrt/profiles/" + filepath.Base(path)
	cmd := exec.CommandContext(ctx, "/data/local/Droidspaces/bin/droidspaces", "--name=DroidWRT", "run", "/usr/bin/mihomo", "-t", "-d", "/tmp/droidwrt-validation", "-f", containerPath)
	if cmd.Run() != nil {
		return errors.New("Mihomo отклонил профиль. Рабочая подписка сохранена")
	}
	return nil
}
func fetchSubscription(address string) ([]byte, error) {
	u, e := url.Parse(address)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return nil, errors.New("Нужна HTTPS-ссылка подписки")
	}
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if b, e := os.ReadFile("/data/local/Droidspaces/Containers/DroidWRT/etc/ssl/certs/ca-certificates.crt"); e == nil {
		pool.AppendCertsFromPEM(b)
	}
	c := http.Client{Timeout: 25 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || req.URL.Scheme != "https" || req.URL.Host != u.Host {
			return errors.New("redirect rejected")
		}
		return nil
	}}
	req, _ := http.NewRequest("GET", address, nil)
	req.Header.Set("User-Agent", "mihomo/1.19.32")
	req.Header.Set("Accept", "application/yaml,application/json,text/plain")
	r, e := c.Do(req)
	if e != nil {
		return nil, errors.New("Не удалось загрузить подписку. Предыдущая версия сохранена")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, errors.New("Сервер не отдал подписку. Проверьте ссылку")
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024+1))
	if e != nil || len(b) > 4*1024*1024 {
		return nil, errors.New("Не удалось прочитать подписку или превышен размер 4 МБ")
	}
	return b, nil
}
func updateSubscription(address string) (Subscription, error) {
	previous := readSubscription()
	if address == "" {
		address = previous.URL
	}
	b, e := fetchSubscription(address)
	if e != nil {
		return previous, e
	}
	entries, e := subscriptionConfigs(b)
	if e != nil {
		return previous, e
	}
	next := Subscription{URL: address, Updated: time.Now().Unix(), Profiles: []Profile{}}
	if e = os.MkdirAll(profileRoot, 0700); e != nil {
		return previous, e
	}
	for _, entry := range entries {
		content, e := mihomoProxy(entry)
		if e != nil {
			return previous, e
		}
		digest := sha256.Sum256(content)
		id := hex.EncodeToString(digest[:16])
		name, _ := entry["name"].(string)
		if name == "" {
			name = "Mihomo"
		}
		if len([]rune(name)) > 64 {
			name = string([]rune(name)[:64])
		}
		path := filepath.Join(profileRoot, id+".mihomo.json")
		if e = atomicPrivate(path, content); e != nil {
			return previous, e
		}
		if e = validateMihomo(path); e != nil {
			return previous, e
		}
		next.Profiles = append(next.Profiles, Profile{ID: id, Name: name, Kind: "Mihomo", State: "saved"})
	}
	// One durable pointer commits the whole validated set. Failures leave old entries selected.
	data, _ := json.Marshal(next)
	if e = atomicPrivate(subscriptionPath, data); e != nil {
		return previous, e
	}
	return next, nil
}
func subscriptionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		jsonReply(w, subscriptionSummary(readSubscription()))
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var p struct {
		Action string `json:"action"`
		URL    string `json:"url"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&p) != nil {
		http.Error(w, "Неверный запрос", 400)
		return
	}
	subscriptionLock.Lock()
	defer subscriptionLock.Unlock()
	if p.Action == "remove" {
		b, _ := json.Marshal(Subscription{})
		if atomicPrivate(subscriptionPath, b) != nil {
			http.Error(w, "Не удалось удалить подписку", 500)
			return
		}
		jsonReply(w, map[string]any{"removed": true})
		return
	}
	if p.Action != "update" {
		http.Error(w, "Неверное действие", 400)
		return
	}
	s, e := updateSubscription(strings.TrimSpace(p.URL))
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	jsonReply(w, subscriptionSummary(s))
}
