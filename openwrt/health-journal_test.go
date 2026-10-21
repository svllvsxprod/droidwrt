package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthJournalRetentionAndAppend(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, n := range []string{"2026-10-09.jsonl", "2026-10-10.jsonl", "unrelated.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("keep"), 0600)
	}
	for i := 0; i < 2; i++ {
		if e := appendHealthJournal(dir, now, map[string]any{"event": i}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := os.Stat(filepath.Join(dir, "2026-10-09.jsonl")); !os.IsNotExist(e) {
		t.Fatal("expired history retained")
	}
	for _, n := range []string{"2026-10-10.jsonl", "unrelated.txt"} {
		if _, e := os.Stat(filepath.Join(dir, n)); e != nil {
			t.Fatal("retention deleted unrelated/current file")
		}
	}
	f, e := os.Open(filepath.Join(dir, "2026-10-13.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for i := 0; i < 2; i++ {
		var v map[string]int
		if dec.Decode(&v) != nil || v["event"] != i {
			t.Fatal("history not appended")
		}
	}
}
