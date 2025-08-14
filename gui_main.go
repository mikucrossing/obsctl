package main

import (
    "context"
    "embed"

    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"
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
    }); err != nil {
        panic(err)
    }
}
