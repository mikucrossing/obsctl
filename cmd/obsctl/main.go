package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "strings"
    "time"

    "awesomeProject/internal/obsws"
)

// これらは ldflags で上書き可能:
// go build -ldflags "-X main.version=1.2.3 -X main.commit=abcd123 -X main.date=2025-08-12T01:23:45Z"
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func main() {
    if len(os.Args) < 2 {
        usage()
        os.Exit(2)
    }

    switch os.Args[1] {
    case "trigger":
        runTrigger(os.Args[2:])
    case "import":
        runImport(os.Args[2:])
    case "midi":
        runMidi(os.Args[2:])
    case "version":
        printVersion()
    case "help", "-h", "--help":
        if len(os.Args) > 2 {
            switch os.Args[2] {
            case "trigger":
                triggerUsage()
            case "import":
                importUsage()
            case "midi":
                midiUsage()
            default:
                usage()
            }
        } else {
            usage()
        }
    case "-v", "--version":
        printVersion()
    default:
        log.Printf("不明なサブコマンド: %s", os.Args[1])
        usage()
        os.Exit(2)
    }
}

func usage() {
    fmt.Println("obsctl - OBS WebSocket 操作用CLI")
    fmt.Println("")
    fmt.Println("使用方法:")
    fmt.Println("  obsctl <command> [options]")
    fmt.Println("")
    fmt.Println("コマンド:")
    fmt.Println("  trigger   複数OBSへ同時発火（シーン切替/将来のメディア操作）")
    fmt.Println("  import    ディレクトリからシーン+Media Sourceを生成")
    fmt.Println("  midi      MIDI入力を待機してシーン切替（試験的）")
    fmt.Println("  version   バージョン情報を表示")
    fmt.Println("")
    fmt.Println("ヘルプ:")
    fmt.Println("  obsctl help trigger   トリガーの詳細ヘルプ")
    fmt.Println("  obsctl help import    インポートの詳細ヘルプ")
    fmt.Println("")
    fmt.Println("例:")
    fmt.Println("  obsctl trigger -addrs 127.0.0.1:4455,127.0.0.1:4456 -password ****** -scene SceneA -at 2025-08-12T01:30:00+09:00 -spinwin 2ms")
    fmt.Println("  obsctl trigger -addrs 10.0.0.21:4455,10.0.0.22:4455 -passwords passA,passB -scene SceneA  # 個別パスワードの例")
    fmt.Println("  obsctl import -addr 127.0.0.1:4455 -password ****** -dir ./videos -loop -activate")
    fmt.Println("  obsctl midi -addrs 127.0.0.1:4455 -password ****** -device 'IAC Driver Bus 1'")
}

func printVersion() {
    fmt.Printf("obsctl %s (commit %s, built %s)\n", version, commit, date)
}

func runTrigger(args []string) {
    fs := flag.NewFlagSet("trigger", flag.ExitOnError)

    addrs := fs.String("addrs", "127.0.0.1:4455,127.0.0.1:4456", "OBS WebSocket のアドレスをカンマ区切り（host:port）")
    password := fs.String("password", "", "OBS WebSocket のパスワード（共通）")
    passwords := fs.String("passwords", "", "複数接続の個別パスワード。-addrs と同じ順でカンマ区切り（数が合わない場合は無視）")
    scene := fs.String("scene", "", "切り替えるシーン名（省略可）")
    media := fs.String("media", "", "メディア入力名（省略可）")
    action := fs.String("action", "none", "メディア操作: none|play|pause|stop|restart|resume")
    at := fs.String("at", "", "発火時刻（RFC3339, 例: 2025-08-12T01:30:00+09:00）")
    delay := fs.Duration("delay", 0, "今からの遅延時間（例: 150ms, 2s）")
    timeout := fs.Duration("timeout", 5*time.Second, "各リクエストのタイムアウト")
    spinWin := fs.Duration("spinwin", 2*time.Millisecond, "精密発火のスピン待機時間")
    skewLog := fs.Bool("skewlog", true, "各インスタンスの実測ズレをログ出力する")

    fs.Usage = triggerUsage
    _ = fs.Parse(args)

    if *scene == "" && (*media == "" || *action == "none") {
        log.Fatal("実行内容がありません。-scene か、-media と -action のいずれかを指定してください。")
    }

    fireTime := time.Now()
    if *at != "" {
        t, err := time.Parse(time.RFC3339, *at)
        if err != nil {
            log.Fatalf("-at のパースに失敗しました: %v", err)
        }
        fireTime = t
    } else if *delay > 0 {
        fireTime = fireTime.Add(*delay)
    }

    targets := strings.Split(*addrs, ",")
    var pwlist []string
    if strings.TrimSpace(*passwords) != "" {
        pws := strings.Split(*passwords, ",")
        if len(pws) == len(targets) {
            for i := range pws { pws[i] = strings.TrimSpace(pws[i]) }
            pwlist = pws
        } else {
            log.Printf("警告: -passwords の数 (%d) が -addrs の数 (%d) と一致しません。-password（共通）を使用します。", len(pws), len(targets))
        }
    }
    opts := obsws.TriggerOptions{
        Addrs:     targets,
        Password:  *password,
        Passwords: pwlist,
        Scene:     *scene,
        Media:     *media,
        Action:    *action,
        FireTime:  fireTime,
        SpinWin:   *spinWin,
        Timeout:   *timeout,
        SkewLog:   *skewLog,
    }

    if err := obsws.Trigger(opts); err != nil {
        log.Fatal(err)
    }
}

func runImport(args []string) {
    fs := flag.NewFlagSet("import", flag.ExitOnError)

    addr := fs.String("addr", "127.0.0.1:4455", "obs-websocket のアドレス (host:port。ws:// は不要)")
    password := fs.String("password", "", "obs-websocket のパスワード")
    dir := fs.String("dir", "", "動画ファイルを含むディレクトリ（再帰しない）")
    loop := fs.Bool("loop", false, "Media Source をループ再生にするか")
    activate := fs.Bool("activate", false, "最後に作成したシーンをプログラム終了時にアクティブ化するか")
    transition := fs.String("transition", "fade", "シーントランジション: fade|cut (デフォルト: fade)")
    monitoring := fs.String("monitoring", "off", "音声モニタリング: off|monitor-only|monitor-and-output (デフォルト: off)")
    debug := fs.Bool("debug", false, "デバッグログを有効化（詳細な失敗理由を表示）")

    fs.Usage = importUsage
    _ = fs.Parse(args)

    if *dir == "" {
        log.Fatal("-dir を指定してください")
    }

    // トランジションの正規化と検証
    tr := strings.ToLower(strings.TrimSpace(*transition))
    switch tr {
    case "", "fade":
        tr = "fade"
    case "cut":
        // ok
    default:
        log.Fatalf("-transition は fade か cut を指定してください（指定値: %s）", *transition)
    }

    // モニタリングの正規化と検証
    mon := strings.ToLower(strings.TrimSpace(*monitoring))
    switch mon {
    case "", "off", "none":
        mon = "off"
    case "monitor-only", "monitor_only":
        mon = "monitor-only"
    case "monitor-and-output", "monitor_and_output":
        mon = "monitor-and-output"
    default:
        log.Fatalf("-monitoring は off|monitor-only|monitor-and-output を指定してください（指定値: %s）", *monitoring)
    }

    opts := obsws.ImportOptions{
        Addr:     *addr,
        Password: *password,
        Dir:      *dir,
        Loop:     *loop,
        Activate: *activate,
        Transition: tr,
        Monitoring: mon,
        Debug:    *debug,
    }
    if err := obsws.ImportScenes(opts); err != nil {
        log.Fatal(err)
    }
}

func triggerUsage() {
    fmt.Fprintln(os.Stderr, "Usage: obsctl trigger [options]")
    fmt.Fprintln(os.Stderr, "\n説明: 複数の OBS WebSocket に対し、指定時刻に同時にシーン切替（または将来のメディア操作）を行います。")
    fmt.Fprintln(os.Stderr, "\n主なオプション:")
    // 手書きで主要なオプションを列挙
    fmt.Fprintln(os.Stderr, "  -addrs     OBSのアドレスをカンマ区切り (host:port)")
    fmt.Fprintln(os.Stderr, "  -password  パスワード（全接続共通）")
    fmt.Fprintln(os.Stderr, "  -passwords 個別パスワードをカンマ区切り（-addrs と同順・同数）。一致しない場合は無視して -password を使用")
    fmt.Fprintln(os.Stderr, "  -scene     切り替えるシーン名")
    fmt.Fprintln(os.Stderr, "  -media     メディア入力名（現状は未サポート動作）")
    fmt.Fprintln(os.Stderr, "  -action    none|play|pause|stop|restart|resume")
    fmt.Fprintln(os.Stderr, "  -at        RFC3339の発火時刻 (例: 2025-08-12T01:30:00+09:00)")
    fmt.Fprintln(os.Stderr, "  -delay     現在からの遅延時間 (例: 150ms, 2s)")
    fmt.Fprintln(os.Stderr, "  -timeout   各リクエストのタイムアウト")
    fmt.Fprintln(os.Stderr, "  -spinwin   発火前スピン時間 (精度/CPUバランス)")
    fmt.Fprintln(os.Stderr, "  -skewlog   実測ズレをログ出力 (true/false)")
}

func importUsage() {
    fmt.Fprintln(os.Stderr, "Usage: obsctl import [options]")
    fmt.Fprintln(os.Stderr, "\n説明: ディレクトリ内の動画ファイルから、シーンを作成し Media Source を追加します。")
    fmt.Fprintln(os.Stderr, "\n主なオプション:")
    fmt.Fprintln(os.Stderr, "  -addr      OBSのアドレス (host:port)")
    fmt.Fprintln(os.Stderr, "  -password  パスワード")
    fmt.Fprintln(os.Stderr, "  -dir       動画を含むディレクトリ")
    fmt.Fprintln(os.Stderr, "  -loop      Media Sourceをループ再生にする")
    fmt.Fprintln(os.Stderr, "  -activate  最後に作成したシーンをアクティブにする")
    fmt.Fprintln(os.Stderr, "  -transition フェードかカットの選択: fade|cut (default: fade)")
    fmt.Fprintln(os.Stderr, "  -monitoring 音声モニタリング: off|monitor-only|monitor-and-output (default: off)")
    fmt.Fprintln(os.Stderr, "  -debug     デバッグログ（詳細なエラー/検出情報）を出力する")
}

func midiUsage() {
    fmt.Fprintln(os.Stderr, "Usage: obsctl midi [options] | ls-devices | gen-json [options]")
    fmt.Fprintln(os.Stderr, "\n説明: MIDI 入力を監視し、イベントに応じて OBS のシーンを切り替えます（試験的）。")
    fmt.Fprintln(os.Stderr, "\n主なコマンド:")
    fmt.Fprintln(os.Stderr, "  ls-devices     利用可能な MIDI 入力デバイス一覧を表示")
    fmt.Fprintln(os.Stderr, "  gen-json       OBSのシーン一覧から NoteOn マッピングJSONを生成し標準出力へ")
    fmt.Fprintln(os.Stderr, "\n主なオプション:")
    fmt.Fprintln(os.Stderr, "  -addrs         OBS のアドレスをカンマ区切り (host:port)")
    fmt.Fprintln(os.Stderr, "  -password      パスワード（全接続共通）")
    fmt.Fprintln(os.Stderr, "  -passwords     個別パスワードをカンマ区切り（-addrs と同順・同数）。一致しない場合は無視して -password を使用")
    fmt.Fprintln(os.Stderr, "  -device        監視する MIDI 入力デバイス名")
    fmt.Fprintln(os.Stderr, "  -channel       受け付ける MIDI チャネル (1-16、カンマ区切り)")
    fmt.Fprintln(os.Stderr, "  -debounce      デバウンス間隔 (例: 30ms)")
    fmt.Fprintln(os.Stderr, "  -ratelimit     レート制限の最短間隔 (例: 50ms)")
    fmt.Fprintln(os.Stderr, "  -timeout       OBS リクエストのタイムアウト (例: 5s)")
    fmt.Fprintln(os.Stderr, "  -map-note      ノート→シーンの対応（複数可）。例: 1:36=028_エンドロール（ch:note=scene）")
    fmt.Fprintln(os.Stderr, "  -config        JSON設定ファイルパス（device/channel/debounce/rate_limit/mappings）")
    fmt.Fprintln(os.Stderr, "  -debug         デバッグログを有効化")
    fmt.Fprintln(os.Stderr, "\n注: ネイティブMIDI入出力はビルドタグ 'midi_native' が必要です。詳細は docs/MIDI_SCENE_SWITCH.md を参照。")
}
