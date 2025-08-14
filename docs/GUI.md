# obsctl GUI（Wails）

本GUIは Wails v2 を用いたデスクトップアプリ（Windows/macOS）です。CLIのロジック（`internal/obsws`）を再利用し、
以下を提供します。

- 複数OBSの接続管理（共通パスワード）
- 共通シーン一覧の表示とワンクリック切替
- 動画/画像ディレクトリからの一括インポート（ループ/アクティブ化/トランジション/モニタリング）
- 実行ログ表示
 - MIDI（Note→シーン切替、デバイス選択、マッピング自動生成）

## ディレクトリ

- `cmd/obsctl-gui/` … Wailsアプリ（バックエンド + フロントエンド静的ファイル）
- `internal/gui/config/` … GUI設定の保存/読込（平文JSON）

## 必要要件（開発）

- Go 1.20+（本リポジトリは 1.24系を想定）
- Node.js（Wails CLIが内部で利用）
- Wails CLI の導入:
  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  # PATH に GOPATH/bin を追加（zshの例）
  echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
  ```
- macOS の場合は Xcode Command Line Tools が必要になることがあります:
  ```bash
  xcode-select --install
  ```

ネットワーク制限がある環境では、先に許可された環境で `go mod download` を実行してください。

## 実行（開発）

1) 依存の取得
```bash
go mod tidy
```

2) Wails で起動（推奨）
```bash
wails dev
```

3) 代替（Wails CLI 不使用で簡易起動）
```bash
go run -tags=dev .
```

フロントエンドは `cmd/obsctl-gui/frontend/` の静的ファイルを埋め込んで配信しています（テンプレート/バンドラ不要）。

## 配布（パッケージ）

Windows（.exe / NSISインストーラ）

```bash
wails build -platform windows/amd64 -nsis -o obsctl-gui.exe
```

macOS（.app / Universal）

```bash
wails build -platform darwin/universal -o obsctl-gui.app
```

Gatekeeper対策（署名/公証）は別途。

### MIDI 対応ビルド

ネイティブMIDI入力を有効にするにはビルドタグ `midi_native` が必要です。

- 開発起動:
  ```bash
  wails dev -tags midi_native
  # または
  go run -tags=dev,midi_native .
  ```
- 配布ビルド:
  ```bash
  wails build -tags midi_native -platform darwin/universal
  wails build -tags midi_native -platform windows/amd64 -nsis
  ```

ビルドタグを付けない場合はスタブ実装となり、デバイス一覧取得がエラーになります（UI上は「MIDI未対応ビルド」と表示）。

## CI（GitHub Actions）

タグ `v*` を push すると、GitHub Actions の `release` ワークフローが動き、以下をビルドしてリリースに添付します。

- CLI（midi_native）: Linux/macOS/Windows 向けバイナリ
- GUI（midi_native）: macOS（Universal `.zip` に `.app` 同梱）・Windows（`.exe` を `.zip`）

ワークフロー定義は `.github/workflows/release.yml` を参照してください。

## 使い方（MVP）

1. 左ペインの「接続」に OBS を追加（名前/`host:port`）。
2. 共通パスワードを入力し「設定を保存」。
3. 「接続テスト」で疎通確認。
4. 「共通シーンを読み込み」で右ペインに共通シーンが並ぶので、クリックで切替。
5. インポートは接続先/フォルダ/オプションを選び「インポート実行」。

### MIDI（任意）

1. 右ペイン「MIDI」でデバイスを選択（更新ボタンで再取得）
2. 既存のマッピングがあれば編集／なければ「自動生成」を利用可能
   - 接続先（1つ）・チャネル（例: 1）・開始ノート（例: 36）を選び「生成して置換」を押すと、
     選んだOBSのシーン一覧から `ch:note=Scene` の行が自動生成されます（CLIの `obsctl midi gen-json` 相当）。
3. 「MIDI設定を保存」→「開始」で受信を開始。Note Onで一致するシーンに切替されます。

注意:

- 現状、複数OBSのパスワードは共通前提（CLI `trigger` と同様）。
- 「共通シーン」は全接続に存在するシーン名の積集合のみを表示します。

## 補足（便利機能）

- QR/URL から追加: 接続画面の「QR/URLから追加」から、OBS が表示する接続用リンク/QRテキストを貼り付けると、自動で `host:port` とパスワードを取り込みます。
  - 例: `obsws://192.168.0.10:4455?password=xxxxx`
  - 例: `obsws://192.168.0.10:4455/xxxxx`（パス部分にパスワードが入っている形式）
  - 例: `ws://192.168.0.10:4455` / `wss://...`
  - フラグメント `#password=...` や `pass/pw/p/pwd/auth/token/passphrase` といった別名キーにも対応。
  - フリーテキストからも `host:port` とパスワードを簡易抽出します。
- カメラでQR読み取り: BarcodeDetector が使える環境ではネイティブAPIを、未対応環境では同梱の jsQR ライブラリを用いたオフライン解析で読み取ります。画像ファイルをドラッグ＆ドロップしての解析も可能です。
- 設定ファイル: `~/.config/obsctl-gui/config.json`（接続/共通パスワード/MIDI設定/Import既定値）。
- パスワード未設定のOBSにも接続自体は試行します（OBS側が必須なら認証エラーになります）。
