package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharingRejectsUnsupportedAndUnexpectedInput(t *testing.T) {
	for _, body := range []string{`{"usb":"rndis"}`, `{"usb":"ncm; reboot"}`, `{"usb":"ncm","command":"reboot"}`, `{"usb_debug":"false"}`, `{"ethernet_lan":"true"}`, `{}`, `null`} {
		w := httptest.NewRecorder()
		sharingHandler(w, httptest.NewRequest("POST", "/droidwrt/sharing", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("bad request accepted: %s", body)
		}
	}
}

func TestSharingDebugMigrationAndPersistence(t *testing.T) {
	legacy := decodeSharing([]byte(`{"usb":"ecm","hotspot":true}`))
	if !legacy.USBDebug || legacy.USB != "ecm" || !legacy.Hotspot {
		t.Fatal("legacy settings changed", legacy)
	}
	for _, mode := range []string{"off", "ncm", "ecm"} {
		for _, debug := range []bool{false, true} {
			want := SharingConfig{USB: mode, USBDebug: debug, Hotspot: true}
			b, _ := json.Marshal(want)
			if got := decodeSharing(b); got != want {
				t.Fatal("explicit preference not retained", got, want)
			}
		}
	}
	if got := decodeSharing([]byte(`{"usb_debug":false,broken`)); !got.USBDebug {
		t.Fatal("corrupt configuration disabled recovery access")
	}
}

func TestEthernetRoleMigrationAndPersistence(t *testing.T) {
	for _, raw := range []string{`{}`, `{"usb":"ecm","usb_debug":false,"hotspot":true}`} {
		if decodeSharing([]byte(raw)).EthernetLAN {
			t.Fatal("legacy WAN role was changed")
		}
	}
	for _, lan := range []bool{false, true} {
		want := SharingConfig{USB: "ncm", USBDebug: false, Hotspot: true, EthernetLAN: lan}
		raw, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		if got := decodeSharing(raw); got != want {
			t.Fatal(got, want)
		}
	}
}
