package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config は GUI の永続設定です。
// 初期段階では単純化のため、全接続で共通パスワードを採用します。
type Config struct {
	// OBS 接続リスト
	Connections []Connection `json:"connections"`
	// 共通パスワード（後方互換のために残します。未使用）
	CommonPassword string `json:"common_password"`
	// Import の既定値
	ImportDefaults ImportDefaults `json:"import_defaults"`
	// MIDI 設定
	MIDI MidiConfig `json:"midi"`
	// Bluetooth同期設定
	Bluetooth BluetoothSyncConfig `json:"bluetooth"`
}

type Connection struct {
	Name     string `json:"name"`
	Addr     string `json:"addr"` // host:port （ws:// 不要）
	Enabled  bool   `json:"enabled"`
	Password string `json:"password"` // 個別パスワード（空なら無認証）
}

type ImportDefaults struct {
	Loop       bool   `json:"loop"`
	Activate   bool   `json:"activate"`
	Transition string `json:"transition"` // fade|cut
	Monitoring string `json:"monitoring"` // off|monitor-only|monitor-and-output
}

// MidiConfig は GUI 用の簡易MIDI設定。
// mappings は "ch:note=Scene Name" 形式の文字列配列。
type MidiConfig struct {
	Enabled   bool     `json:"enabled"`
	Device    string   `json:"device"`
	Channel   string   `json:"channel"`    // 例: "1,2"（空=全）
	Debounce  string   `json:"debounce"`   // 例: "30ms"
	RateLimit string   `json:"rate_limit"` // 例: "50ms"
	Mappings  []string `json:"mappings"`
}

// BluetoothSyncConfig はGUI用 Bluetooth 同期設定。
type BluetoothSyncConfig struct {
	Enabled           bool          `json:"enabled"`
	Role              string        `json:"role"` // off|parent|child
	DeviceName        string        `json:"device_name"`
	LeadTimeMs        int           `json:"lead_time_ms"`
	PairingCodeTTLSec int           `json:"pairing_code_ttl_sec"`
	AcceptLateMs      int           `json:"accept_late_ms"`
	MaxNodes          int           `json:"max_nodes"`
	AutoReconnect     bool          `json:"auto_reconnect"`
	DropMissedEvents  bool          `json:"drop_missed_events"`
	TrustedPeers      []TrustedPeer `json:"trusted_peers"`
}

type TrustedPeer struct {
	PeerID   string `json:"peer_id"`
	Name     string `json:"name"`
	Secret   string `json:"secret"`
	LastSeen string `json:"last_seen"`
	Platform string `json:"platform"`
}

func Default() *Config {
	return &Config{
		Connections:    []Connection{},
		CommonPassword: "",
		ImportDefaults: ImportDefaults{Loop: false, Activate: false, Transition: "fade", Monitoring: "off"},
		MIDI:           MidiConfig{Enabled: false, Device: "", Channel: "", Debounce: "30ms", RateLimit: "50ms", Mappings: []string{}},
		Bluetooth: BluetoothSyncConfig{
			Enabled:           false,
			Role:              "off",
			DeviceName:        "",
			LeadTimeMs:        300,
			PairingCodeTTLSec: 60,
			AcceptLateMs:      500,
			MaxNodes:          4,
			AutoReconnect:     true,
			DropMissedEvents:  true,
			TrustedPeers:      []TrustedPeer{},
		},
	}
}

// 保存先パス（OS毎の規定の設定ディレクトリ配下）
func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(dir, "obsctl-gui")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Load は設定を読み込みます。無い場合は (nil, os.ErrNotExist) を返します。
func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	bt, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(bt, &c); err != nil {
		return nil, err
	}
	normalizeConfig(&c)
	return &c, nil
}

// Save は設定を保存します。
func Save(c *Config) error {
	if c == nil {
		return errors.New("nil config")
	}
	normalizeConfig(c)
	p, err := path()
	if err != nil {
		return err
	}
	bt, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, bt, 0o600)
}

func normalizeConfig(c *Config) {
	if c == nil {
		return
	}
	if c.Connections == nil {
		c.Connections = []Connection{}
	}
	if c.MIDI.Mappings == nil {
		c.MIDI.Mappings = []string{}
	}
	b := &c.Bluetooth
	legacyUnset := !b.Enabled &&
		strings.TrimSpace(b.Role) == "" &&
		strings.TrimSpace(b.DeviceName) == "" &&
		b.LeadTimeMs == 0 &&
		b.PairingCodeTTLSec == 0 &&
		b.AcceptLateMs == 0 &&
		b.MaxNodes == 0 &&
		!b.AutoReconnect &&
		!b.DropMissedEvents &&
		len(b.TrustedPeers) == 0
	switch b.Role {
	case "off", "parent", "child":
	default:
		b.Role = "off"
	}
	if b.LeadTimeMs <= 0 {
		b.LeadTimeMs = 300
	}
	if b.PairingCodeTTLSec <= 0 {
		b.PairingCodeTTLSec = 60
	}
	if b.AcceptLateMs <= 0 {
		b.AcceptLateMs = 500
	}
	if b.MaxNodes <= 0 {
		b.MaxNodes = 4
	}
	if legacyUnset {
		b.AutoReconnect = true
		b.DropMissedEvents = true
	}
	if b.TrustedPeers == nil {
		b.TrustedPeers = []TrustedPeer{}
	}
}
