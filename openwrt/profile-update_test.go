package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAWGReplacementPreservesIdentityAndRejectsInvalid(t *testing.T) {
	oldRoot := profileRoot
	profileRoot = t.TempDir()
	defer func() { profileRoot = oldRoot }()
	p, e := installProfile("test", awgFixture)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(profileRoot, p.ID+".conf")
	before, _ := os.ReadFile(path)
	meta, _ := os.ReadFile(filepath.Join(profileRoot, p.ID+".meta.json"))
	if _, e = replaceAWGProfile(p.ID, "invalid"); e == nil {
		t.Fatal("invalid accepted")
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != string(before) {
		t.Fatal("invalid update changed profile")
	}
	updated := strings.Replace(awgFixture, "10.8.0.2/32", "10.8.0.2/32, fd00::2/128", 1)
	q, e := replaceAWGProfile(p.ID, updated)
	if e != nil || q != p {
		t.Fatal("identity changed or replacement failed", e)
	}
	content, _ := os.ReadFile(path)
	backup, _ := os.ReadFile(path + ".previous")
	afterMeta, _ := os.ReadFile(filepath.Join(profileRoot, p.ID+".meta.json"))
	if !hasIPv6Address(string(content)) || string(backup) != string(before) || string(meta) != string(afterMeta) {
		t.Fatal("replacement or backup incorrect")
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("private file permissions lost")
	}
	if _, e = replaceAWGProfile("../bad", updated); e == nil {
		t.Fatal("unsafe id accepted")
	}
}
