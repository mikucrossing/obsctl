package config

import (
	"os"
	"path/filepath"
	"testing"
)

func useTempUserConfigDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	oldAppData := os.Getenv("APPDATA")

	_ = os.Setenv("HOME", tmp)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	_ = os.Setenv("APPDATA", filepath.Join(tmp, "AppData", "Roaming"))

	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
		_ = os.Setenv("APPDATA", oldAppData)
	})

	return tmp
}

func TestSaveLoadWithBluetoothDefaults(t *testing.T) {
	useTempUserConfigDir(t)

	c := Default()
	c.Connections = []Connection{{Name: "OBS 1", Addr: "127.0.0.1:4455", Enabled: true}}

	if err := Save(c); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.Bluetooth.Role == "" {
		t.Fatalf("Bluetooth role should be normalized")
	}
	if got.Bluetooth.LeadTimeMs <= 0 || got.Bluetooth.PairingCodeTTLSec <= 0 || got.Bluetooth.AcceptLateMs <= 0 {
		t.Fatalf("Bluetooth defaults should be positive: %+v", got.Bluetooth)
	}
	if got.Bluetooth.MaxNodes <= 0 {
		t.Fatalf("Bluetooth max_nodes should be positive")
	}
}

func TestLoadLegacyConfigWithoutBluetooth(t *testing.T) {
	useTempUserConfigDir(t)

	p, err := path()
	if err != nil {
		t.Fatalf("path failed: %v", err)
	}

	legacy := `{
  "connections": [{"name":"OBS 1","addr":"127.0.0.1:4455","enabled":true,"password":""}],
  "common_password":"",
  "import_defaults":{"loop":false,"activate":false,"transition":"fade","monitoring":"off"},
  "midi":{"enabled":false,"device":"","channel":"","debounce":"30ms","rate_limit":"50ms","mappings":[]}
}`

	if err := os.WriteFile(p, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy config failed: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got.Bluetooth.Role != "off" {
		t.Fatalf("expected bluetooth role off, got %q", got.Bluetooth.Role)
	}
	if got.Bluetooth.LeadTimeMs != 300 {
		t.Fatalf("expected default lead_time_ms=300, got %d", got.Bluetooth.LeadTimeMs)
	}
	if got.Bluetooth.PairingCodeTTLSec != 60 {
		t.Fatalf("expected default pairing_code_ttl_sec=60, got %d", got.Bluetooth.PairingCodeTTLSec)
	}
	if got.Bluetooth.AcceptLateMs != 500 {
		t.Fatalf("expected default accept_late_ms=500, got %d", got.Bluetooth.AcceptLateMs)
	}
	if got.Bluetooth.MaxNodes != 4 {
		t.Fatalf("expected default max_nodes=4, got %d", got.Bluetooth.MaxNodes)
	}
	if !got.Bluetooth.AutoReconnect || !got.Bluetooth.DropMissedEvents {
		t.Fatalf("expected default reconnect/drop flags true")
	}
}
