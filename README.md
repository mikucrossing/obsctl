# obsctl

OBS WebSocket を操作するCLIツールです。複数OBSへの同時シーン切替と、動画ディレクトリからのシーン+Media Source 一括作成を提供します。

## クイックスタート

```
go build -o obsctl ./cmd/obsctl
```

例:

```
# 同時シーン切替
./obsctl trigger -addrs 127.0.0.1:4455,127.0.0.1:4456 -password ****** -scene SceneA

# ディレクトリからシーン作成
./obsctl import -addr 127.0.0.1:4455 -password ****** -dir ./videos -loop -activate -transition fade -debug
```

## コマンド概要

- `trigger`: 複数OBSに対し、指定時刻/遅延で同時にシーン切替を実行
- `import`: ディレクトリ内の動画からシーンと Media Source を一括作成
- `version`: バージョン情報を表示

詳細は `docs/README.md` を参照してください。

## 利用イメージ
```mermaid
flowchart LR
subgraph LAN[ローカルネットワーク（LAN）]
A[OBS PC A<br/>IP: 192.168.10.21<br/><code>obs-websocket : ws://192.168.10.21:4455</code>]
B[OBS PC B<br/>IP: 192.168.10.22<br/><code>obs-websocket : ws://192.168.10.22:4455</code>]
C["メイン PC（コントローラ）<br/>IP: 192.168.10.10<br/>obsctl trigger -addrs 192.168.10.21,192.168.10.22 -password ****** -scene &quot;028_エンドロール&quot;"]
end

C -- WebSocket: ws://…:4455 --> A
C -- WebSocket: ws://…:4455 --> B

A ===|同期してシーン切替 / 再生| B
```