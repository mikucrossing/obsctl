package config

import (
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
)

// Config は GUI の永続設定です。
// 初期段階では単純化のため、全接続で共通パスワードを採用します。
type Config struct {
    // OBS 接続リスト
    Connections []Connection `json:"connections"`
    // 全接続で使う共通パスワード（平文）
    CommonPassword string `json:"common_password"`
    // Import の既定値
    ImportDefaults ImportDefaults `json:"import_defaults"`
    // MIDI 設定
    MIDI MidiConfig `json:"midi"`
}

type Connection struct {
    Name string `json:"name"`
    Addr string `json:"addr"` // host:port （ws:// 不要）
    Enabled bool `json:"enabled"`
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
    Channel   string   `json:"channel"`     // 例: "1,2"（空=全）
    Debounce  string   `json:"debounce"`    // 例: "30ms"
    RateLimit string   `json:"rate_limit"`  // 例: "50ms"
    Mappings  []string `json:"mappings"`
}

func Default() *Config {
    return &Config{
        Connections:     []Connection{},
        CommonPassword:  "",
        ImportDefaults: ImportDefaults{Loop: false, Activate: false, Transition: "fade", Monitoring: "off"},
        MIDI: MidiConfig{Enabled: false, Device: "", Channel: "", Debounce: "30ms", RateLimit: "50ms", Mappings: []string{}},
    }
}

// 保存先パス（OS毎の規定の設定ディレクトリ配下）
func path() (string, error) {
    dir, err := os.UserConfigDir()
    if err != nil { return "", err }
    d := filepath.Join(dir, "obsctl-gui")
    if err := os.MkdirAll(d, 0o755); err != nil { return "", err }
    return filepath.Join(d, "config.json"), nil
}

// Load は設定を読み込みます。無い場合は (nil, os.ErrNotExist) を返します。
func Load() (*Config, error) {
    p, err := path()
    if err != nil { return nil, err }
    bt, err := os.ReadFile(p)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) { return nil, os.ErrNotExist }
        return nil, err
    }
    var c Config
    if err := json.Unmarshal(bt, &c); err != nil { return nil, err }
    return &c, nil
}

// Save は設定を保存します。
func Save(c *Config) error {
    if c == nil { return errors.New("nil config") }
    p, err := path()
    if err != nil { return err }
    bt, err := json.MarshalIndent(c, "", "  ")
    if err != nil { return err }
    return os.WriteFile(p, bt, 0o600)
}
