package main

import (
    "context"
    "errors"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"

    "awesomeProject/internal/gui/config"
    "awesomeProject/internal/obsws"
    "awesomeProject/internal/midi"

    "github.com/andreykaipov/goobs"
    "github.com/andreykaipov/goobs/api/requests/scenes"
    "github.com/wailsapp/wails/v2/pkg/runtime"
    "sync"
)

type App struct {
    ctx context.Context
    cfg *config.Config
    // MIDI runtime
    midiCancel context.CancelFunc
    midiDrv    midi.Input
    // OBS connection cache
    cacheMu       sync.Mutex
    cacheClients  map[string]*goobs.Client // key: addr (host:port)
    cachePassword string
}

func NewApp() *App {
    cfg, _ := config.Load()
    if cfg == nil { cfg = config.Default() }
    // 互換性: 古い設定（enabled未導入）では全て有効にしておく
    allDisabled := true
    for i := range cfg.Connections { if cfg.Connections[i].Enabled { allDisabled = false; break } }
    if len(cfg.Connections) > 0 && allDisabled {
        for i := range cfg.Connections { cfg.Connections[i].Enabled = true }
    }
    return &App{cfg: cfg, cacheClients: map[string]*goobs.Client{}}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx; _ = a.emitLog("info", "GUI 起動") }

func (a *App) shutdown(ctx context.Context) {
    _ = a.MidiStop()
    a.cacheMu.Lock()
    for _, c := range a.cacheClients { _ = c.Disconnect() }
    a.cacheClients = map[string]*goobs.Client{}
    a.cacheMu.Unlock()
}

// 設定API
func (a *App) GetConfig() (*config.Config, error) {
    if a.cfg == nil { return config.Default(), nil }
    return a.cfg, nil
}
func (a *App) SaveConfig(c *config.Config) error {
    if c == nil { return errors.New("config is nil") }
    normalizeUniqueConnectionNames(c)
    a.cfg = c
    if err := config.Save(c); err != nil { return err }
    return a.emitLog("info", "設定を保存しました")
}

// normalizeUniqueConnectionNames は接続名の重複を解消する（GUI保存時）。
func normalizeUniqueConnectionNames(c *config.Config) {
    if c == nil || len(c.Connections) == 0 { return }
    used := map[string]struct{}{}
    for i := range c.Connections {
        name := strings.TrimSpace(c.Connections[i].Name)
        if name == "" { name = fmt.Sprintf("OBS %d", i+1) }
        base := name
        try := base
        suffix := 2
        for {
            key := strings.ToLower(strings.TrimSpace(try))
            if _, ok := used[key]; !ok {
                c.Connections[i].Name = try
                used[key] = struct{}{}
                break
            }
            try = fmt.Sprintf("%s (%d)", base, suffix)
            suffix++
        }
    }
}

// 接続テスト
func (a *App) TestConnections() (map[string]string, error) {
    if len(a.cfg.Connections) == 0 { return nil, errors.New("接続先がありません") }
    pw := strings.TrimSpace(a.cfg.CommonPassword)
    out := map[string]string{}
    for _, c := range a.cfg.Connections {
        if !c.Enabled { out[c.Name] = "SKIP: disabled"; continue }
        addr := obsws.NormalizeObsAddr(c.Addr)
        cli, err := openObs(addr, pw)
        if err != nil { out[c.Name] = fmt.Sprintf("NG: %v", err); continue }
        if _, err := cli.Scenes.GetSceneList(nil); err != nil { out[c.Name] = fmt.Sprintf("NG: %v", err) } else { out[c.Name] = "OK" }
        _ = cli.Disconnect()
    }
    return out, nil
}

// 共通シーン一覧
func (a *App) ListScenes() ([]string, error) {
    if len(a.cfg.Connections) == 0 { return nil, errors.New("接続先がありません") }
    pw := strings.TrimSpace(a.cfg.CommonPassword)
    var inter map[string]struct{}
    first := true
    for _, c := range a.cfg.Connections {
        if !c.Enabled { continue }
        addr := obsws.NormalizeObsAddr(c.Addr)
        cli, err := a.getClientCached(addr, pw)
        if err != nil { return nil, fmt.Errorf("%s への接続に失敗: %w", c.Name, err) }
        lst, err := cli.Scenes.GetSceneList(nil); if err != nil { return nil, fmt.Errorf("%s のシーン一覧取得に失敗: %w", c.Name, err) }
        set := map[string]struct{}{}; for _, s := range lst.Scenes { set[s.SceneName] = struct{}{} }
        if first { inter = set; first = false } else { for k := range inter { if _, ok := set[k]; !ok { delete(inter, k) } } }
    }
    if first { return nil, errors.New("有効な接続がありません") }
    out := make([]string, 0, len(inter)); for k := range inter { out = append(out, k) }
    if len(out) > 1 { sortStrings(out) }
    return out, nil
}

// ListScenesFor は指定接続名のシーン一覧（表示名配列）を返す。
func (a *App) ListScenesFor(connectionName string) ([]string, error) {
    if strings.TrimSpace(connectionName) == "" { return nil, errors.New("接続名を指定してください") }
    pw := strings.TrimSpace(a.cfg.CommonPassword)
    for _, c := range a.cfg.Connections {
        if !c.Enabled { continue }
        if c.Name == connectionName {
            addr := obsws.NormalizeObsAddr(c.Addr)
            cli, err := a.getClientCached(addr, pw)
            if err != nil { return nil, fmt.Errorf("%s への接続に失敗: %w", c.Name, err) }
            lst, err := cli.Scenes.GetSceneList(nil)
            if err != nil { return nil, fmt.Errorf("%s のシーン一覧取得に失敗: %w", c.Name, err) }
            names := make([]string, 0, len(lst.Scenes))
            for _, s := range lst.Scenes { names = append(names, s.SceneName) }
            if len(names) > 1 { sortStrings(names) }
            return names, nil
        }
    }
    return nil, fmt.Errorf("接続が見つかりません: %s", connectionName)
}

// シーン切替
func (a *App) TriggerScene(scene string) error {
    scene = strings.TrimSpace(scene)
    if scene == "" { return errors.New("シーン名が空です") }
    if len(a.cfg.Connections) == 0 { return errors.New("接続先がありません") }
    pw := strings.TrimSpace(a.cfg.CommonPassword)
    addrs := make([]string, 0, len(a.cfg.Connections)); for _, c := range a.cfg.Connections { if c.Enabled { addrs = append(addrs, c.Addr) } }
    if len(addrs) == 0 { return errors.New("有効な接続がありません") }
    go func(){ _ = a.emitLog("info", fmt.Sprintf("シーン切替: %s", scene)); if err := a.setSceneCached(addrs, pw, scene); err != nil { _ = a.emitLog("error", fmt.Sprintf("切替失敗: %v", err)) } else { _ = a.emitLog("info", "切替完了") } }()
    return nil
}

// インポート
func (a *App) ImportFromDir(connectionName, dir string, loop bool, activate bool, transition string, monitoring string, debug bool) error {
    var target *config.Connection
    for i := range a.cfg.Connections { if a.cfg.Connections[i].Name == connectionName && a.cfg.Connections[i].Enabled { target = &a.cfg.Connections[i]; break } }
    if target == nil { return fmt.Errorf("接続が見つかりません: %s", connectionName) }
    if strings.TrimSpace(dir) == "" { return errors.New("ディレクトリを指定してください") }
    pw := strings.TrimSpace(a.cfg.CommonPassword)
    abs := dir; if !filepath.IsAbs(dir) { if wd, err := os.Getwd(); err == nil { abs = filepath.Join(wd, dir) } }
    opts := obsws.ImportOptions{ Addr: target.Addr, Password: pw, Dir: abs, Loop: loop, Activate: activate, Transition: transition, Monitoring: monitoring, Debug: debug }
    go func(){ _ = a.emitLog("info", fmt.Sprintf("インポート開始: %s -> %s", connectionName, abs)); if err := obsws.ImportScenes(opts); err != nil { _ = a.emitLog("error", fmt.Sprintf("インポート失敗: %v", err)) } else { _ = a.emitLog("info", "インポート完了") } }()
    return nil
}

// ログイベント
func (a *App) emitLog(level, msg string) error { log.Printf("[%s] %s", level, msg); if a.ctx != nil { runtime.EventsEmit(a.ctx, "log", map[string]any{"level": level, "msg": msg, "time": time.Now().Format(time.RFC3339)}) } ; return nil }

// ソート
func sortStrings(s []string) { n := len(s); for i := 0; i < n; i++ { for j := 0; j < n-1-i; j++ { if strings.ToLower(s[j]) > strings.ToLower(s[j+1]) { s[j], s[j+1] = s[j+1], s[j] } } } }

// OpenDirectoryDialog を表示し、選択されたパスを返す（キャンセル時は空文字）。
func (a *App) OpenDirectoryDialog(defaultDir string, title string) (string, error) {
    if a.ctx == nil { return "", errors.New("no context") }
    opts := runtime.OpenDialogOptions{
        Title:            title,
        DefaultDirectory: defaultDir,
        CanCreateDirectories: true,
        ShowHiddenFiles:  false,
        TreatPackagesAsDirectories: true,
    }
    return runtime.OpenDirectoryDialog(a.ctx, opts)
}

// openObs はパスワードが空の場合に無認証で接続を試みる。
func openObs(addr, password string) (*goobs.Client, error) {
    if strings.TrimSpace(password) == "" {
        return goobs.New(addr)
    }
    return goobs.New(addr, goobs.WithPassword(password))
}

// OpenExternalURL は既定ブラウザでURLを開く。
func (a *App) OpenExternalURL(url string) error {
    if a.ctx == nil { return errors.New("no context") }
    runtime.BrowserOpenURL(a.ctx, url)
    return nil
}

// --- MIDI Support ---

// MidiListDevices はMIDI入力デバイスの一覧を返す。
func (a *App) MidiListDevices() ([]string, error) {
    return midi.ListInputs()
}

// MidiGetConfig は現在のMIDI設定を返す。
func (a *App) MidiGetConfig() (config.MidiConfig, error) {
    return a.cfg.MIDI, nil
}

// MidiSaveConfig はMIDI設定を保存する。
func (a *App) MidiSaveConfig(mc config.MidiConfig) error {
    a.cfg.MIDI = mc
    if err := config.Save(a.cfg); err != nil { return err }
    return a.emitLog("info", "MIDI設定を保存しました")
}

// MidiStart は現在の設定でMIDIリスニングを開始する。
func (a *App) MidiStart() error {
    mc := a.cfg.MIDI
    if strings.TrimSpace(mc.Device) == "" {
        return errors.New("MIDIデバイスを選択してください")
    }
    // Stop existing
    _ = a.MidiStop()

    chs := parseChannels(mc.Channel)
    debounce := mustParseDurationDefault(mc.Debounce, 30*time.Millisecond)
    ratelimit := mustParseDurationDefault(mc.RateLimit, 50*time.Millisecond)
    noteMap := parseNoteMaps(mc.Mappings)

    drv, events, err := midi.OpenInput(mc.Device)
    if err != nil { return err }
    a.midiDrv = drv
    ctx, cancel := context.WithCancel(context.Background())
    a.midiCancel = cancel
    _ = a.emitLog("info", fmt.Sprintf("MIDI開始: device=%s ch=%s", mc.Device, mc.Channel))

    // Rate limit & debounce control
    lastAt := map[string]time.Time{}
    var lastEvent time.Time

    go func(){
        defer func(){ _ = a.emitLog("info", "MIDI停止") }()
        for {
            select {
            case <-ctx.Done():
                return
            case ev, ok := <-events:
                if !ok { return }
                // Debounce (any event)
                if !lastEvent.IsZero() && time.Since(lastEvent) < debounce { continue }
                lastEvent = time.Now()
                if ev.Type != midi.NoteOn { continue }
                if len(chs) > 0 && !containsChannel(chs, int(ev.Channel)) { continue }
                key := fmt.Sprintf("%d:%d", ev.Channel, ev.Data1)
                scene, ok := noteMap[key]
                if !ok { continue }
                if t, ok2 := lastAt[key]; ok2 {
                    if since := time.Since(t); since < ratelimit { continue }
                }
                lastAt[key] = time.Now()
                // Trigger scene to enabled connections (cached clients)
                addrs, pw, err := a.getEnabledAddrsAndPassword()
                if err != nil || len(addrs) == 0 { _ = a.emitLog("error", "有効な接続がありません"); continue }
                if err := a.setSceneCached(addrs, pw, scene); err != nil { _ = a.emitLog("error", fmt.Sprintf("MIDI切替失敗: %v", err)) } else { _ = a.emitLog("info", fmt.Sprintf("MIDI切替: %s (CH%d Note%d)", scene, ev.Channel, ev.Data1)) }
            }
        }
    }()
    return nil
}

// MidiStop はMIDI受信を停止する。
func (a *App) MidiStop() error {
    if a.midiCancel != nil { a.midiCancel(); a.midiCancel = nil }
    if a.midiDrv != nil { _ = a.midiDrv.Close(); a.midiDrv = nil }
    return nil
}

func (a *App) getEnabledAddrsAndPassword() ([]string, string, error) {
    if len(a.cfg.Connections) == 0 { return nil, "", errors.New("接続なし") }
    addrs := make([]string, 0, len(a.cfg.Connections))
    for _, c := range a.cfg.Connections { if c.Enabled { addrs = append(addrs, c.Addr) } }
    return addrs, strings.TrimSpace(a.cfg.CommonPassword), nil
}

// Helpers
func mustParseDurationDefault(s string, d time.Duration) time.Duration { if v, err := time.ParseDuration(strings.TrimSpace(s)); err == nil { return v }; return d }

// parseChannels: copy from CLI
func parseChannels(s string) []int {
    if strings.TrimSpace(s) == "" { return nil }
    var out []int
    for _, p := range strings.Split(s, ",") {
        p = strings.TrimSpace(p)
        if p == "" { continue }
        var v int
        _, err := fmt.Sscanf(p, "%d", &v)
        if err == nil && v >= 1 && v <= 16 { out = append(out, v) }
    }
    return out
}

// parseNoteMaps: from CLI (strings of "ch:note=Scene")
func parseNoteMaps(values []string) map[string]string {
    out := map[string]string{}
    for _, v := range values {
        v = strings.TrimSpace(v)
        if v == "" { continue }
        parts := strings.SplitN(v, "=", 2)
        if len(parts) != 2 { continue }
        left := strings.TrimSpace(parts[0])
        right := strings.TrimSpace(parts[1])
        if right == "" { continue }
        ln := strings.Split(left, ":")
        if len(ln) != 2 { continue }
        ch := strings.TrimSpace(ln[0])
        note := strings.TrimSpace(ln[1])
        var chv, nv int
        if _, err := fmt.Sscanf(ch, "%d", &chv); err != nil || chv < 1 || chv > 16 { continue }
        if _, err := fmt.Sscanf(note, "%d", &nv); err != nil || nv < 0 || nv > 127 { continue }
        key := fmt.Sprintf("%d:%d", chv, nv)
        out[key] = right
    }
    return out
}

func containsChannel(list []int, v int) bool {
    if len(list) == 0 { return true }
    for _, x := range list { if x == v { return true } }
    return false
}

// --- Cached OBS connections ---
func (a *App) getClientCached(addr, pw string) (*goobs.Client, error) {
    a.cacheMu.Lock()
    defer a.cacheMu.Unlock()
    if a.cachePassword != pw {
        for _, c := range a.cacheClients { _ = c.Disconnect() }
        a.cacheClients = map[string]*goobs.Client{}
        a.cachePassword = pw
    }
    if c, ok := a.cacheClients[addr]; ok { return c, nil }
    var c *goobs.Client
    var err error
    if strings.TrimSpace(pw) == "" { c, err = goobs.New(addr) } else { c, err = goobs.New(addr, goobs.WithPassword(pw)) }
    if err != nil { return nil, err }
    a.cacheClients[addr] = c
    return c, nil
}

func (a *App) setSceneCached(addrs []string, pw, scene string) error {
    for _, raw := range addrs {
        addr := obsws.NormalizeObsAddr(strings.TrimSpace(raw))
        if addr == "" { continue }
        cli, err := a.getClientCached(addr, pw)
        if err != nil { return fmt.Errorf("%s 接続失敗: %w", addr, err) }
        _, err = cli.Scenes.SetCurrentProgramScene((&scenes.SetCurrentProgramSceneParams{}).WithSceneName(scene))
        if err != nil {
            // recreate once on failure
            a.cacheMu.Lock(); if old, ok := a.cacheClients[addr]; ok { _ = old.Disconnect(); delete(a.cacheClients, addr) }; a.cacheMu.Unlock()
            cli2, err2 := a.getClientCached(addr, pw)
            if err2 != nil { return fmt.Errorf("%s 再接続失敗: %w", addr, err2) }
            if _, err3 := cli2.Scenes.SetCurrentProgramScene((&scenes.SetCurrentProgramSceneParams{}).WithSceneName(scene)); err3 != nil {
                return fmt.Errorf("%s 切替失敗: %w", addr, err3)
            }
        }
    }
    return nil
}
