# obsctl ドキュメント

obsctl は OBS WebSocket を操作するためのシンプルなCLIツールです。複数OBSへの同時シーン切替や、動画ディレクトリからのシーン+Media Source 一括生成を提供します。

## インストール/ビルド

```
go build -o obsctl ./cmd/obsctl
```

より詳しい配布用ビルド（Windows/Linux、macOSユニバーサル含む）の手順は `docs/BUILD.md` を参照してください。
テストの実行方法は `docs/TESTING.md` を参照してください。

バージョン情報を埋め込む場合:

```
go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o obsctl ./cmd/obsctl
```

## 使い方の概要

```
obsctl <command> [options]
```

- `trigger`: 複数 OBS へ同時発火（シーン切替/将来のメディア操作）
- `import`: ディレクトリからシーン+Media Source を生成
- `version`: バージョン情報を表示

+### MIDI 連携

`obsctl midi` サブコマンドで、MIDI 入力イベントに応じてシーン切替を行う機能を提供しています。

- 使い方の設計/仕様は `docs/MIDI_SCENE_SWITCH.md` を参照
- シーン→NoteのJSON雛形は `obsctl midi gen-json` で生成可能（例は `docs/midi.example.json`）。
- デフォルトビルドではネイティブMIDIは無効（スタブ）。ネイティブ入力を使うにはビルドタグ `midi_native` を有効にしてビルドしてください。
  - 例: `GOCACHE=$(pwd)/.gocache GOMODCACHE=$(pwd)/.gomodcache go build -tags midi_native -o obsctl ./cmd/obsctl`

## trigger コマンド

指定した時刻（または遅延）に、複数の OBS へ同時にシーン切り替えを行います。

例:

```
obsctl trigger \
  -addrs 127.0.0.1:4455,127.0.0.1:4456 \
  -password ****** \
  -scene SceneA \
  -at 2025-08-12T01:30:00+09:00 \
  -spinwin 2ms
```

主なオプション:

- `-addrs`: OBS のアドレスをカンマ区切りで指定（`host:port`）。
- `-password`: すべての接続で使用するパスワード。
- `-scene`: 切り替えるシーン名。
- `-media`, `-action`: 将来のメディア操作用（現状の依存では未サポート、ログのみ）。
- `-at`: RFC3339 の発火時刻（例: `2025-08-12T01:30:00+09:00`）。
- `-delay`: 現在からの遅延時間（例: `150ms`, `2s`）。
- `-timeout`: 各リクエストのタイムアウト。
- `-spinwin`: 発火前のスピン待機時間（精度/CPU負荷のトレードオフ）。
- `-skewlog`: 実測ズレをログ出力（true/false）。

## import コマンド

ディレクトリ内の動画ファイルから、シーンを作成し Media Source（`ffmpeg_source`）を追加します。必要に応じて最後に作成したシーンをアクティブにします。

例:

```
obsctl import \
  -addr 127.0.0.1:4455 \
  -password ****** \
  -dir ./videos \
  -loop \
  -activate \
  -transition fade \
  -monitoring monitor-and-output
```

主なオプション:

- `-addr`: OBS のアドレス（`host:port`）。
- `-password`: パスワード。
- `-dir`: 動画ファイルを含むディレクトリ（再帰しない）。
- `-loop`: Media Source をループ再生にする。
- `-activate`: 最後に作成したシーンをプログラム終了時にアクティブ化する。
- `-transition`: シーントランジションを選択（`fade` | `cut`、デフォルト `fade`）。`-activate` 指定時に、切替直前に OBS の現在トランジションを設定します。ローカライズ環境でも種類（fade/cut）で自動検出して適切な名称を選びます。
- `-monitoring`: 生成する各 Media Source の音声モニタリングを設定（`off` | `monitor-only` | `monitor-and-output`、デフォルト `off`）。
- `-debug`: デバッグログを有効化。トランジション検出結果や失敗時の候補一覧、CreateScene/CreateInput/Scene切替のパラメータなど詳細情報を出力します。

使用例:

```
# フェードで切替（デフォルト）
obsctl import -addr 127.0.0.1:4455 -dir ./videos -activate

# カットで切替
obsctl import -addr 127.0.0.1:4455 -dir ./videos -activate -transition cut

# 音声モニタリングをモニター+出力に設定
obsctl import -addr 127.0.0.1:4455 -dir ./videos -monitoring monitor-and-output

# 詳細なデバッグログを出す
obsctl import -addr 127.0.0.1:4455 -dir ./videos -activate -transition cut -debug
```

## 注意事項

- メディア操作（play/pause/stop等）は、現在利用中の `goobs` バージョンではAPIの直接サポートが不足しているため、ログ通知のみです。
- 高精度発火のため `-spinwin` は環境に応じて調整してください。小さすぎるとズレが増え、大きすぎるとCPU負荷が上がります。
