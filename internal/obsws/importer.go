package obsws

import (
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"

    "github.com/andreykaipov/goobs"
    "github.com/andreykaipov/goobs/api/requests/inputs"
    "github.com/andreykaipov/goobs/api/requests/scenes"
    "github.com/andreykaipov/goobs/api/requests/transitions"
)

type ImportOptions struct {
    Addr     string
    Password string
    Dir      string
    Loop     bool
    Activate bool
    Transition string // "fade" or "cut"
    Monitoring string // "off" | "monitor-only" | "monitor-and-output"
    Debug      bool
}

// normalizeTransitionName は CLI から渡されるトランジション指定を
// OBS の既定トランジション名に正規化する。
// 現状は "fade"→"Fade"、"cut"→"Cut"。既定外はすべて "Fade"。
func normalizeTransitionName(opt string) string {
    s := strings.ToLower(strings.TrimSpace(opt))
    switch s {
    case "cut":
        return "Cut"
    default:
        return "Fade"
    }
}

// resolveTransitionName は OBS から取得したトランジション一覧を参照し、
// 希望する種類（fade/cut）に一致する実際の名称（ローカライズ含む）を選ぶ。
// 見つからない場合は既定の英語名にフォールバックする。
func resolveTransitionName(client *goobs.Client, want string) string {
    want = strings.ToLower(strings.TrimSpace(want))
    // 望む種類のキーワード
    key := "fade"
    if want == "cut" {
        key = "cut"
    }

    // 一覧取得（失敗したらフォールバック）
    lst, err := client.Transitions.GetSceneTransitionList(nil)
    if err == nil && lst != nil {
        for _, tr := range lst.Transitions {
            kind := strings.ToLower(tr.TransitionKind)
            if strings.Contains(kind, key) {
                if name := strings.TrimSpace(tr.TransitionName); name != "" {
                    return name
                }
            }
        }
    }
    // フォールバック（英語既定名）
    return normalizeTransitionName(want)
}

// normalizeMonitoringType は CLI の指定を obs-websocket のモニタリング種別定数へ変換する。
// 返り値は obs-websocket v5 の想定文字列:
//   - "OBS_MONITORING_TYPE_NONE"
//   - "OBS_MONITORING_TYPE_MONITOR_ONLY"
//   - "OBS_MONITORING_TYPE_MONITOR_AND_OUTPUT"
func normalizeMonitoringType(opt string) string {
    s := strings.ToLower(strings.TrimSpace(opt))
    switch s {
    case "monitor-only", "monitor_only":
        return "OBS_MONITORING_TYPE_MONITOR_ONLY"
    case "monitor-and-output", "monitor_and_output":
        return "OBS_MONITORING_TYPE_MONITOR_AND_OUTPUT"
    default: // "off", "none", 未指定 は NONE
        return "OBS_MONITORING_TYPE_NONE"
    }
}

func ImportScenes(opts ImportOptions) error {
    debugf := func(format string, args ...any) {
        if opts.Debug {
            log.Printf("[DEBUG] "+format, args...)
        }
    }
    // 接続
    addrVal := NormalizeObsAddr(opts.Addr)
    client, err := goobs.New(addrVal, goobs.WithPassword(opts.Password))
    if err != nil {
        return fmt.Errorf("OBS への接続に失敗しました: %w", err)
    }
    defer client.Disconnect()

    // 既存シーン名を取得して重複回避
    sceneList, err := client.Scenes.GetSceneList(nil)
    if err != nil {
        return fmt.Errorf("シーン一覧の取得に失敗しました: %w", err)
    }
    existing := map[string]struct{}{}
    for _, s := range sceneList.Scenes {
        existing[s.SceneName] = struct{}{}
    }

    // ディレクトリ走査
    entries, err := os.ReadDir(opts.Dir)
    if err != nil {
        return fmt.Errorf("ディレクトリの読み取りに失敗しました: %w", err)
    }

    var lastScene string
    count := 0
    for _, e := range entries {
        if e.IsDir() {
            debugf("サブディレクトリをスキップ: %s", e.Name())
            continue
        }
        ext := strings.ToLower(filepath.Ext(e.Name()))
        if !isVideoExt(ext) {
            debugf("動画拡張子ではないためスキップ: %s", e.Name())
            continue
        }
        full := filepath.Join(opts.Dir, e.Name())
        sceneName := sanitizeName(strings.TrimSuffix(e.Name(), ext))

        // 既存ならスキップ
        if _, ok := existing[sceneName]; ok {
            log.Printf("既存シーンのためスキップ: %s", sceneName)
            continue
        }

        // シーン作成
        if _, err = client.Scenes.CreateScene(&scenes.CreateSceneParams{
            SceneName: &sceneName,
        }); err != nil {
            log.Printf("シーン作成失敗 (%s): %v", sceneName, err)
            debugf("CreateScene params: sceneName=%s", sceneName)
            continue
        }
        existing[sceneName] = struct{}{}

        // Media Source 追加（ffmpeg_source）
        inputSettings := map[string]any{
            "local_file":          full,    // 再生するローカル動画
            "is_local_file":       true,    // 念のため明示
            "looping":             opts.Loop,
            "restart_on_activate": true,    // シーン切替時に頭出し
            "close_when_inactive": false,   // 非アクティブ時に閉じない
            "hw_decode":           true,    // 環境により有効/無効
        }

        inputName := sceneName + " Media"
        inputKind := "ffmpeg_source"
        _, err = client.Inputs.CreateInput(&inputs.CreateInputParams{
            SceneName:     &sceneName,
            InputName:     &inputName,
            InputKind:     &inputKind,
            InputSettings: inputSettings,
        })
        if err != nil {
            log.Printf("Media Source 追加失敗 (%s): %v", sceneName, err)
            debugf("CreateInput params: scene=%s, name=%s, kind=%s, file=%s, loop=%v", sceneName, inputName, inputKind, full, opts.Loop)
            continue
        }

        // 音声モニタリング設定（既定: off）。失敗しても致命ではない。
        monType := normalizeMonitoringType(opts.Monitoring)
        if _, err := client.Inputs.SetInputAudioMonitorType(
            inputs.NewSetInputAudioMonitorTypeParams().
                WithInputName(inputName).
                WithMonitorType(monType),
        ); err != nil {
            log.Printf("音声モニタリング設定に失敗しました (%s -> %s): %v", inputName, monType, err)
        } else {
            debugf("音声モニタリング設定: input=%q type=%q", inputName, monType)
        }

        log.Printf("作成完了: シーン「%s」 <- %s", sceneName, full)
        lastScene = sceneName
        count++
    }

    // 最後に作成したシーンをアクティブに
    if opts.Activate && lastScene != "" {
        // トランジション設定（デフォルト: Fade）。ローカライズ環境でも動くよう
        // 現在のトランジション一覧から目的の種類に該当する名称を探す。
        trName := resolveTransitionName(client, opts.Transition)
        debugf("トランジション解決: want=%q resolved=%q", opts.Transition, trName)

        // 可能ならトランジションを設定（失敗しても致命ではない）
        if _, err := client.Transitions.SetCurrentSceneTransition(
            (&transitions.SetCurrentSceneTransitionParams{}).WithTransitionName(trName),
        ); err != nil {
            log.Printf("トランジション設定に失敗しました (%s): %v", trName, err)
            if lst, e := client.Transitions.GetSceneTransitionList(nil); e == nil {
                for _, tr := range lst.Transitions {
                    debugf("候補: name=%q kind=%q fixed=%v configurable=%v", tr.TransitionName, tr.TransitionKind, tr.TransitionFixed, tr.TransitionConfigurable)
                }
            } else {
                debugf("トランジション一覧取得失敗: %v", e)
            }
        }
        _, err = client.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{
            SceneName: &lastScene,
        })
        if err != nil {
            log.Printf("シーン切替に失敗しました: %v", err)
            debugf("SetCurrentProgramScene params: sceneName=%s", lastScene)
        } else {
            log.Printf("アクティブ化: %s", lastScene)
        }
    }

    if count == 0 {
        log.Println("作成されたシーンはありません。拡張子やディレクトリ指定を確認してください。")
    } else {
        log.Printf("合計 %d 件のシーンを作成しました。", count)
    }
    return nil
}

func isVideoExt(ext string) bool {
    switch ext {
    case ".mp4", ".mov", ".mkv", ".webm":
        return true
    default:
        return false
    }
}

func sanitizeName(s string) string {
    s = strings.TrimSpace(s)
    s = strings.ReplaceAll(s, "/", "_")
    s = strings.ReplaceAll(s, "\\", "_")
    if s == "" {
        return "untitled"
    }
    const limit = 120
    if len([]rune(s)) > limit {
        return string([]rune(s)[:limit])
    }
    return s
}
