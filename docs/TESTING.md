# テストガイド

obsctl のユニットテストの実行方法とカバレッジについてまとめます。

## 前提
- Go 1.23 以上（`go.mod` は `toolchain go1.24.6`）
- ネットワークは基本不要ですが、初回のみ Go モジュール取得で通信が発生します（CIや制限環境ではモジュールキャッシュを使うか、事前に依存を取得してください）。

## 実行方法

最短（推奨）:
- POSIX（macOS/Linux）: `make test`
- Windows PowerShell: `go test ./...`

直接 `go test` を使う場合（POSIX）:
```sh
# 通常
go test ./...

# サンドボックス等でホーム配下にキャッシュを書けない場合
mkdir -p .gocache .gomodcache
GOCACHE=$(pwd)/.gocache GOMODCACHE=$(pwd)/.gomodcache go test ./...
```

Windows PowerShell でキャッシュディレクトリを指定する例:
```powershell
New-Item -ItemType Directory -Force .gocache,.gomodcache | Out-Null
$env:GOCACHE = "$PWD/.gocache"
$env:GOMODCACHE = "$PWD/.gomodcache"
go test ./...
```

Makefile ターゲット:
```sh
make test    # go test ./... を実行
```

## 何をテストしているか
- `NormalizeObsAddr`: `ws://`/`wss://` の除去や空白の正規化
- `WaitUntil`: 未来/過去時刻での待機挙動（過度なフレーク回避の閾値あり）
- `isVideoExt` / `sanitizeName`: 拡張子の判定とシーン名整形
- `toMediaActionConst` / `withTimeout`: メディア操作の定数化とタイムアウトラッパ

いずれもネットワーク依存なしで実行できます。

## 何をテストしていないか（現状）
- OBS 実機との E2E（接続・シーン作成・切替）
  - 理由: OBS の起動や環境差分に依存するため CI で安定しにくい
  - 将来案: `goobs` を薄いインターフェースでラップしてモック注入 → `import`/`trigger` の結合テストを追加

## ヒント
- モジュールの取得が必要な初回だけネットワークが発生します。制限環境ではローカルキャッシュ（`GOMODCACHE`）をリポジトリ配下に向けると便利です。
- テストは短時間で終わるよう設計しています。もしフレークが発生する場合は `WaitUntil` の閾値（テスト内の許容上限）を環境に合わせて調整してください。

