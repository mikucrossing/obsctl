package obsws

import (
    "errors"
    "fmt"
    "log"
    "strings"
    "sync"
    "time"

    "github.com/andreykaipov/goobs"
    "github.com/andreykaipov/goobs/api/requests/scenes"
)

type TriggerOptions struct {
    Addrs     []string
    Password  string   // common password (fallback)
    Passwords []string // optional: aligned with Addrs for per-connection passwords
    Scene     string
    Media     string
    Action    string
    FireTime  time.Time
    SpinWin   time.Duration
    Timeout   time.Duration
    SkewLog   bool
}

func Trigger(opts TriggerOptions) error {
    if opts.Scene == "" && (opts.Media == "" || strings.ToLower(opts.Action) == "none") {
        return errors.New("実行内容がありません。-scene か、-media と -action のいずれかを指定してください。")
    }

    // 事前接続
    type clientWrap struct {
        addr string
        c    *goobs.Client
    }
    var clients []clientWrap
    var failed []string
    for i, raw := range opts.Addrs {
        a := strings.TrimSpace(raw)
        a = NormalizeObsAddr(a)
        if a == "" {
            continue
        }
        // choose per-addr password if provided; otherwise use common password; empty means no auth
        var pw string
        if len(opts.Passwords) == len(opts.Addrs) {
            pw = strings.TrimSpace(opts.Passwords[i])
        } else {
            pw = strings.TrimSpace(opts.Password)
        }
        var c *goobs.Client
        var err error
        if pw == "" {
            c, err = goobs.New(a)
        } else {
            c, err = goobs.New(a, goobs.WithPassword(pw))
        }
        if err != nil {
            log.Printf("接続失敗[%d]: ws://%s: %v", i, a, err)
            failed = append(failed, a)
            continue
        }
        clients = append(clients, clientWrap{addr: a, c: c})
        log.Printf("接続完了[%d]: ws://%s", i, a)
    }
    if len(clients) == 0 {
        if len(failed) > 0 {
            return fmt.Errorf("全ての接続に失敗しました。対象: %s", strings.Join(failed, ", "))
        }
        return errors.New("有効な接続先がありません。-addrs を確認してください。")
    }
    if len(failed) > 0 {
        log.Printf("一部接続に失敗しました（スキップされます）: %s", strings.Join(failed, ", "))
    }
    defer func() {
        for _, cw := range clients {
            _ = cw.c.Disconnect()
        }
    }()

    // 予定情報
    now := time.Now()
    if opts.FireTime.After(now) {
        wait := time.Until(opts.FireTime)
        log.Printf("発火予定: %s（残り: %s）", opts.FireTime.Format(time.RFC3339Nano), wait)
    } else {
        log.Printf("即時発火（指定時刻は過去）: %s", opts.FireTime.Format(time.RFC3339Nano))
    }

    // 同時発火
    var wg sync.WaitGroup
    errCh := make(chan error, len(clients))

    for _, cw := range clients {
        wg.Add(1)
        go func(cw clientWrap) {
            defer wg.Done()
            WaitUntil(opts.FireTime, opts.SpinWin)
            firedAt := time.Now()
            if opts.SkewLog {
                delta := firedAt.Sub(opts.FireTime)
                log.Printf("[%s] 発火タイムスタンプ: %s (ズレ: %v)", cw.addr, firedAt.Format(time.RFC3339Nano), delta)
            }

            // シーン切替
            if opts.Scene != "" {
                // goobs のリクエストにタイムアウトが無いので goroutine でラップ
                call := func() error {
                    _, err := cw.c.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{
                        SceneName: &opts.Scene,
                    })
                    return err
                }
                if err := withTimeout(call, opts.Timeout); err != nil {
                    errCh <- fmt.Errorf("[%s] SetCurrentProgramScene 失敗: %w", cw.addr, err)
                    return
                }
                log.Printf("[%s] シーン切替完了: %s", cw.addr, opts.Scene)
            }

            // メディア操作（依存が未サポートのためスキップ）
            if opts.Media != "" && strings.ToLower(opts.Action) != "none" {
                log.Printf("[%s] 注意: メディア操作は現在の依存関係では未サポートのためスキップされました: input=%s action=%s", cw.addr, opts.Media, opts.Action)
            }
        }(cw)
    }

    wg.Wait()
    close(errCh)

    var hadErr bool
    for e := range errCh {
        hadErr = true
        log.Println("ERROR:", e)
    }
    if hadErr {
        return errors.New("一部インスタンスで失敗しました。ログをご確認ください。")
    }
    if len(failed) > 0 {
        log.Println("接続できなかったインスタンスがありました。接続済みインスタンスのみで完了しました。")
        return nil
    }
    log.Println("全インスタンスで完了しました。")
    return nil
}

func withTimeout(fn func() error, d time.Duration) error {
    if d <= 0 {
        return fn()
    }
    ch := make(chan error, 1)
    go func() { ch <- fn() }()
    select {
    case err := <-ch:
        return err
    case <-time.After(d):
        return fmt.Errorf("timeout after %s", d)
    }
}

func toMediaActionConst(a string) (string, bool) {
    switch strings.ToLower(a) {
    case "play":
        return "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_PLAY", true
    case "pause":
        return "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_PAUSE", true
    case "stop":
        return "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_STOP", true
    case "restart":
        return "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART", true
    case "resume":
        return "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESUME", true
    case "none":
        return "", true
    default:
        return "", false
    }
}

