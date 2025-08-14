package main

import (
    "context"
    "embed"

    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"
    "github.com/wailsapp/wails/v2/pkg/options/mac"
    "github.com/wailsapp/wails/v2/pkg/options/windows"
)

// フロントエンド静的ファイル（cmd/obsctl-gui/frontend）をバンドル
//go:embed all:cmd/obsctl-gui/frontend
var assets embed.FS

func main() {
    app := NewApp()

    if err := wails.Run(&options.App{
        Title:  "obsctl GUI",
        Width:  1100,
        Height: 760,
        AssetServer: &assetserver.Options{ Assets: assets },
        OnStartup: func(ctx context.Context) { app.startup(ctx) },
        OnShutdown: func(ctx context.Context) { app.shutdown(ctx) },
        Bind: []any{app},
        // macOS のガラス(すりガラス)効果
        Mac: &mac.Options{
            TitleBar:             mac.TitleBarHiddenInset(),
            Appearance:           mac.NSAppearanceNameDarkAqua,
            WebviewIsTransparent: true,
            WindowIsTranslucent:  true,
        },
        // Windows のアクリル/ミカ効果（Win11 推奨）
        Windows: &windows.Options{
            WebviewIsTransparent: true,
            WindowIsTranslucent:  true,
            BackdropType:         windows.Acrylic, // 近い質感: Acrylic（柔らかめなら Mica）
        },
        // WebView 背景のアルファを 0 に（念のため）
        BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
    }); err != nil {
        panic(err)
    }
}
