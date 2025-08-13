package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "encoding/json"
    "strings"
    "time"

    "awesomeProject/internal/midi"
    "awesomeProject/internal/obsws"
    "github.com/andreykaipov/goobs"
)

func runMidi(args []string) {
    if len(args) > 0 && (args[0] == "ls-devices" || args[0] == "list" || args[0] == "devices") {
        // デバイス一覧
        names, err := midi.ListInputs()
        if err != nil {
            log.Printf("MIDI デバイス一覧の取得に失敗: %v", err)
            log.Println("ネイティブMIDI機能はビルドタグ 'midi_native' が必要です。")
            os.Exit(1)
        }
        if len(names) == 0 {
            fmt.Println("(入力デバイスなし)")
            return
        }
        for _, n := range names {
            fmt.Println(n)
        }
        return
    } else if len(args) > 0 && (args[0] == "gen-json" || args[0] == "gen" ) {
        runMidiGenJSON(args[1:])
        return
    }

    fs := flag.NewFlagSet("midi", flag.ExitOnError)

    addrs := fs.String("addrs", "127.0.0.1:4455", "OBS WebSocket のアドレスをカンマ区切り（host:port）")
    password := fs.String("password", "", "OBS WebSocket のパスワード（共通）")
    device := fs.String("device", "", "監視する MIDI 入力デバイス名")
    channel := fs.String("channel", "", "受け付ける MIDI チャネル (1-16、カンマ区切り。未指定は全て)")
    debounce := fs.Duration("debounce", 30*time.Millisecond, "デバウンス間隔")
    ratelimit := fs.Duration("ratelimit", 50*time.Millisecond, "レート制限の最短間隔")
    timeout := fs.Duration("timeout", 5*time.Second, "OBS リクエストのタイムアウト")
    debug := fs.Bool("debug", false, "デバッグログを有効化")
    mapNotes := multiFlag{}
    fs.Var(&mapNotes, "map-note", "ノート→シーンの対応（複数可）。例: 1:36=028_エンドロール（ch:note=scene）")
    configPath := fs.String("config", "", "JSON設定ファイルへのパス（device/channel/debounce/rate_limit/mappings を読込）")

    fs.Usage = midiUsage
    _ = fs.Parse(args)

    // JSON設定の読み込み（存在すれば）
    // フラグの明示指定を検出（未指定なら JSON の既定値で上書き可）
    setFlags := map[string]bool{}
    fs.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
    cfgNoteMap := map[string]string{}
    if strings.TrimSpace(*configPath) != "" {
        // 未指定のときはゼロ値にして JSON を適用可能にする
        if !setFlags["debounce"] { *debounce = 0 }
        if !setFlags["ratelimit"] { *ratelimit = 0 }
        if err := loadJSONConfig(*configPath, device, channel, debounce, ratelimit, cfgNoteMap); err != nil {
            log.Fatalf("-config の読み込みに失敗しました: %v", err)
        }
    }

    if *device == "" {
        log.Println("-device を指定してください（JSONの device も利用可）。利用可能なデバイスは 'obsctl midi ls-devices' で確認できます。")
        os.Exit(2)
    }

    if *debug {
        log.Printf("Debug: addrs=%s device=%s channel=%s debounce=%s ratelimit=%s timeout=%s maps=%v config=%s", *addrs, *device, *channel, debounce.String(), ratelimit.String(), timeout.String(), []string(mapNotes), *configPath)
    }

    // MIDI ドライバをオープン（ビルドタグ未指定の通常ビルドではエラーになるスタブ）
    drv, events, err := midi.OpenInput(*device)
    if err != nil {
        log.Printf("MIDI 入力のオープンに失敗: %v", err)
        log.Println("ネイティブMIDI機能はビルドタグ 'midi_native' が必要です。詳細は docs/MIDI_SCENE_SWITCH.md を参照してください。")
        os.Exit(1)
    }
    defer drv.Close()

    // マッピングの構築（NoteOn）: JSON→CLI の順にマージ（CLI優先）
    noteMap := map[string]string{}
    for k, v := range cfgNoteMap { noteMap[k] = v }
    for k, v := range parseNoteMaps(mapNotes) { noteMap[k] = v }
    if *debug && len(noteMap) > 0 {
        log.Printf("NoteMap: %d entries", len(noteMap))
    }
    if len(noteMap) == 0 {
        log.Println("警告: ノート→シーンのマッピングが指定されていません。-map-note \"1:36=Scene\" のように指定してください。")
    }

    log.Printf("MIDI 受信開始: device=%s", *device)
    lastAt := map[string]time.Time{}
    targets := strings.Split(*addrs, ",")
    for ev := range events {
        if *channel != "" && !containsChannel(parseChannels(*channel), int(ev.Channel)) {
            continue
        }
        if *debug {
            log.Printf("MIDI: type=%s ch=%d data1=%d data2=%d t=%s", ev.Type, ev.Channel, ev.Data1, ev.Data2, ev.Time.Format(time.RFC3339Nano))
        }
        if ev.Type == midi.NoteOn {
            key := fmt.Sprintf("%d:%d", ev.Channel, ev.Data1)
            if scene, ok := noteMap[key]; ok {
                if cool, ok2 := lastAt[key]; ok2 {
                    if since := time.Since(cool); since < *ratelimit {
                        if *debug {
                            log.Printf("skip by ratelimit %s for %s (remain %s)", ratelimit.String(), key, (*ratelimit - since).String())
                        }
                        continue
                    }
                }
                lastAt[key] = time.Now()

                opts := obsws.TriggerOptions{
                    Addrs:    targets,
                    Password: *password,
                    Scene:    scene,
                    Media:    "",
                    Action:   "none",
                    FireTime: time.Now(),
                    SpinWin:  0,
                    Timeout:  *timeout,
                    SkewLog:  false,
                }
                if err := obsws.Trigger(opts); err != nil {
                    log.Printf("シーン切替失敗: %v", err)
                } else {
                    log.Printf("シーン切替: %s (from CH%d Note%d)", scene, ev.Channel, ev.Data1)
                }
            }
        }
    }
}

func parseChannels(s string) []int {
    if strings.TrimSpace(s) == "" {
        return nil
    }
    var out []int
    for _, p := range strings.Split(s, ",") {
        p = strings.TrimSpace(p)
        if p == "" {
            continue
        }
        var v int
        _, err := fmt.Sscanf(p, "%d", &v)
        if err == nil && v >= 1 && v <= 16 {
            out = append(out, v)
        }
    }
    return out
}

func containsChannel(list []int, v int) bool {
    if len(list) == 0 {
        return true
    }
    for _, x := range list {
        if x == v {
            return true
        }
    }
    return false
}

// multiFlag は同名フラグの複数指定を受け取るためのヘルパ。
type multiFlag []string
func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }

// parseNoteMaps は "ch:note=Scene Name" 形式の配列を解析し、
// key "ch:note" → scene の map を返す。
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

// JSON設定の読み込みと反映。
// device, channel, debounce, ratelimit は未指定時のデフォルトとして上書きし、noteMap に mappings を追加する。
func loadJSONConfig(path string, device *string, channel *string, debounce *time.Duration, ratelimit *time.Duration, noteMap map[string]string) error {
    bt, err := os.ReadFile(path)
    if err != nil { return err }
    var cfg struct {
        Device    string `json:"device"`
        Channel   int    `json:"channel"`
        Debounce  string `json:"debounce"`
        RateLimit string `json:"rate_limit"`
        Mappings  []struct{
            Type       string `json:"type"`
            Channel    int    `json:"channel"`
            Note       int    `json:"note"`
            Scene      string `json:"scene"`
            Transition string `json:"transition"`
        } `json:"mappings"`
    }
    if err := json.Unmarshal(bt, &cfg); err != nil { return err }

    if *device == "" && strings.TrimSpace(cfg.Device) != "" { *device = cfg.Device }
    if strings.TrimSpace(*channel) == "" && cfg.Channel >= 1 && cfg.Channel <= 16 { *channel = fmt.Sprintf("%d", cfg.Channel) }
    if d := strings.TrimSpace(cfg.Debounce); d != "" && *debounce == 0 {
        if dv, err := time.ParseDuration(d); err == nil { *debounce = dv }
    }
    if r := strings.TrimSpace(cfg.RateLimit); r != "" && *ratelimit == 0 {
        if rv, err := time.ParseDuration(r); err == nil { *ratelimit = rv }
    }
    for _, m := range cfg.Mappings {
        if strings.ToLower(strings.TrimSpace(m.Type)) != "note_on" { continue }
        if m.Channel < 1 || m.Channel > 16 { continue }
        if m.Note < 0 || m.Note > 127 { continue }
        if strings.TrimSpace(m.Scene) == "" { continue }
        key := fmt.Sprintf("%d:%d", m.Channel, m.Note)
        noteMap[key] = m.Scene
    }
    return nil
}

// runMidiGenJSON は OBS からシーン一覧を取得し、NoteOnの連番マッピングをJSONで出力する。
// 例: obsctl midi gen-json -addr 127.0.0.1:4455 -password ****** -channel 1 -start-note 36 -device "IACドライバ バス1"
func runMidiGenJSON(args []string) {
    fs := flag.NewFlagSet("midi gen-json", flag.ExitOnError)
    addr := fs.String("addr", "127.0.0.1:4455", "OBS のアドレス (host:port)")
    password := fs.String("password", "", "OBS のパスワード")
    channel := fs.Int("channel", 1, "割り当てる MIDI チャネル (1-16)")
    start := fs.Int("start-note", 36, "割り当て開始ノート番号 (0-127、既定36:C1)")
    device := fs.String("device", "", "推奨デバイス名（出力JSONに記録するだけ）")
    transition := fs.String("transition", "fade", "トランジション: fade|cut（JSONに記録）")
    pretty := fs.Bool("pretty", true, "インデント付きで出力")
    _ = fs.Parse(args)

    if *channel < 1 || *channel > 16 {
        log.Fatalf("-channel は 1..16 を指定してください: %d", *channel)
    }
    if *start < 0 || *start > 127 {
        log.Fatalf("-start-note は 0..127 を指定してください: %d", *start)
    }

    // 接続
    cli, err := goobs.New(obsws.NormalizeObsAddr(*addr), goobs.WithPassword(*password))
    if err != nil {
        log.Fatalf("OBS 接続失敗: %v", err)
    }
    defer cli.Disconnect()

    lst, err := cli.Scenes.GetSceneList(nil)
    if err != nil {
        log.Fatalf("シーン一覧取得失敗: %v", err)
    }

    // マッピング生成
    type mapping struct {
        Type       string `json:"type"`
        Channel    int    `json:"channel"`
        Note       int    `json:"note"`
        Scene      string `json:"scene"`
        Transition string `json:"transition,omitempty"`
    }
    payload := struct {
        Device     string    `json:"device,omitempty"`
        Channel    int       `json:"channel"`
        Debounce   string    `json:"debounce,omitempty"`
        RateLimit  string    `json:"rate_limit,omitempty"`
        Mappings   []mapping `json:"mappings"`
    }{
        Device:    *device,
        Channel:   *channel,
        Debounce:  "30ms",
        RateLimit: "50ms",
    }

    n := *start
    for _, s := range lst.Scenes {
        if n > 127 {
            // これ以上割り当て不可。残りは無視（重複名もそのまま）。
            break
        }
        payload.Mappings = append(payload.Mappings, mapping{
            Type:       "note_on",
            Channel:    *channel,
            Note:       n,
            Scene:      s.SceneName,
            Transition: *transition,
        })
        n++
    }

    var out []byte
    if *pretty {
        out, err = json.MarshalIndent(payload, "", "  ")
    } else {
        out, err = json.Marshal(payload)
    }
    if err != nil {
        log.Fatalf("JSON 生成失敗: %v", err)
    }

    os.Stdout.Write(out)
    os.Stdout.WriteString("\n")
}
