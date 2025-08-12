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
)

type ImportOptions struct {
    Addr     string
    Password string
    Dir      string
    Loop     bool
    Activate bool
}

func ImportScenes(opts ImportOptions) error {
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
            continue
        }
        ext := strings.ToLower(filepath.Ext(e.Name()))
        if !isVideoExt(ext) {
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
            continue
        }

        log.Printf("作成完了: シーン「%s」 <- %s", sceneName, full)
        lastScene = sceneName
        count++
    }

    // 最後に作成したシーンをアクティブに
    if opts.Activate && lastScene != "" {
        _, err = client.Scenes.SetCurrentProgramScene(&scenes.SetCurrentProgramSceneParams{
            SceneName: &lastScene,
        })
        if err != nil {
            log.Printf("シーン切替に失敗しました: %v", err)
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

