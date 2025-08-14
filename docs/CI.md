# GitHub Actions（CI/Release）

本リポジトリのCI/リリース構成の概要と、公開バイナリの一覧を記します。

- 設定ファイル: `.github/workflows/ci.yml`, `.github/workflows/release.yml`
- 方針: ユーザーがそのまま使える実行ファイルをリリースで配布（すべて cgo 有効、`-tags midi_native` を含む）

## トリガー
- CI: ブランチへの `push`、および `pull_request`
- Release: タグ `v*` の `push`、または手動実行（`workflow_dispatch`）

## CI（ci.yml）
- テスト: `ubuntu-latest`, `macos-latest`, `windows-latest` で `go test ./...` を実行
- サニティビルド: 主要OSでコンパイル確認（`CGO_ENABLED=0`）。成果物は公開しません。

## リリース（release.yml）
- 事前処理: `go test ./...` を実行し、バージョン情報（`version/commit/date`）を `-ldflags` で埋め込み
- 依存の導入:
  - Linux: `build-essential`, `pkg-config`, `libasound2-dev`
  - Windows: LLVM/clang（Chocolatey `llvm`）
- ビルド方針: すべて cgo 有効＋ビルドタグ `midi_native`
  - macOS: `arm64`/`amd64` をビルド後、`lipo` でユニバーサル化
  - Windows: `amd64` をビルド、`arm64` は可能な範囲で試行（失敗してもリリースは継続）
  - Linux: `amd64`
- 公開: 生成物をアーティファクトとして集約後、GitHub Release に添付

## リリースで配布されるバイナリ（アーカイブ）
すべて「cgo有効 + `-tags midi_native`」を含み、展開後のファイル名は `obsctl`（Windowsは`obsctl.exe`）です。アーカイブにより実行権限が保持されるため、`chmod +x` は不要です。

- macOS（ユニバーサル）: `obsctl_darwin_universal_midi_native.zip`
  - 対応: x86_64, arm64（単一バイナリ）
- Windows（x86_64）: `obsctl_windows_amd64_midi_native.zip`
- Windows（arm64, ベストエフォート）: `obsctl_windows_arm64_midi_native.zip`
  - 備考: Windows にはユニバーサル形式がないため、アーキ毎に別ファイルを配布します。
- Linux（x86_64）: `obsctl_linux_amd64_midi_native.tar.gz`

展開例:
- macOS: Finderで解凍、または `unzip obsctl_darwin_universal_midi_native.zip`
- Linux: `tar -xzf obsctl_linux_amd64_midi_native.tar.gz`
- Windows: エクスプローラでzip展開

## バージョン情報の埋め込み
- `main.version`: タグ（先頭の `v` を除去した値）
- `main.commit`: 対象コミットの短縮SHA
- `main.date`: UTCのビルド時刻（`YYYY-MM-DDTHH:MM:SSZ`）
- いずれも `-s -w` によりデバッグ情報を削減

## ランタイム要件（利用者向け）
- macOS: 標準の CoreMIDI を使用（追加インストール不要）
- Linux: `libasound2` が必要（例: `sudo apt-get install -y libasound2`）
- Windows: 追加ランタイムは不要です（WinMM を利用）
