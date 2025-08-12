# ビルドガイド（Windows / macOS / Linux）

obsctl の配布用バイナリを作るための手順です。Windows 向けビルド、Linux 向けビルド、そして macOS のユニバーサル（amd64+arm64）バイナリの作り方をまとめています。

クイックに全プラットフォームをビルドする場合は、リポジトリ同梱の以下を利用できます。

- `scripts/build_all.sh`: POSIXシェル用の一括ビルドスクリプト
- `Makefile`: `make build-all` で Windows/Linux/macOS をまとめて出力、`make macos-universal` でユニバーサル作成

## 前提

- Go 1.23 以上（`go.mod` は `toolchain go1.24.6`）
- OBS 29+ かつ obs-websocket v5 系での動作を想定
- 依存は純 Go（cgo 不要）。クロスコンパイルは基本 `CGO_ENABLED=0` でOK
- mac のユニバーサル化には macOS 上で `lipo` が必要（Xcode Command Line Tools）

## バージョン情報の埋め込み

バージョン・コミット・ビルド日時を `-ldflags` で埋め込み可能です。

POSIX シェル（macOS/Linux）例:

```sh
mkdir -p dist
export VERSION=1.0.0
export COMMIT=$(git rev-parse --short HEAD)
export DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
export LDFLAGS="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE"
```

PowerShell（Windows）例:

```powershell
New-Item -ItemType Directory -Force dist | Out-Null
$env:VERSION = "1.0.0"
$env:COMMIT  = (git rev-parse --short HEAD)
$env:DATE    = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$LDFLAGS = "-s -w -X main.version=$($env:VERSION) -X main.commit=$($env:COMMIT) -X main.date=$($env:DATE)"
```

## Windows 向けバイナリ

POSIX シェル:

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/obsctl_windows_amd64.exe ./cmd/obsctl
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o dist/obsctl_windows_arm64.exe ./cmd/obsctl
```

PowerShell:

```powershell
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -trimpath -ldflags $LDFLAGS -o dist/obsctl_windows_amd64.exe ./cmd/obsctl
$env:GOOS = "windows"; $env:GOARCH = "arm64"; go build -trimpath -ldflags $LDFLAGS -o dist/obsctl_windows_arm64.exe ./cmd/obsctl
```

## Linux 向けバイナリ

POSIX シェル:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/obsctl_linux_amd64 ./cmd/obsctl
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o dist/obsctl_linux_arm64 ./cmd/obsctl
```

## macOS ユニバーサル（amd64+arm64）

1) それぞれのアーキテクチャでビルド（macOS 以外でも可）

```sh
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o dist/obsctl_darwin_amd64 ./cmd/obsctl
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o dist/obsctl_darwin_arm64 ./cmd/obsctl
```

2) macOS 上で `lipo` を使ってユニバーサル化（この手順は mac 上で実行してください）

```sh
# Xcode Command Line Tools が未導入なら: xcode-select --install
lipo -create \
  -output dist/obsctl_darwin_universal \
  dist/obsctl_darwin_amd64 \
  dist/obsctl_darwin_arm64

# 確認
lipo -info dist/obsctl_darwin_universal
# => Architectures in the fat file: x86_64 arm64
```

3) 署名/公証（必要に応じて）

配布する場合は組織のポリシーに従って `codesign` や Apple 公証を実施してください。

## よくある注意

- アドレスは `host:port` 形式で指定（`ws://`/`wss://` は付けない。付けても自動で除去されます）。
- Windows パスにスペースがある場合は二重引用符で囲む（例: `-dir "C:\\My Videos"`）。
- `import` は再帰走査なし。対象拡張子は `.mp4 .mov .mkv .webm`。
- すべての OS で OBS のファイアウォール許可（デフォルト 4455）とパスワード設定を確認。
