# Phase 3: Dockerfile + イメージビルド

---

## ディレクトリ構成 (Phase 3 完了時)

```
go-microservices-chat/
├── services/
│   ├── user-service/
│   │   └── Dockerfile                  # ★ Phase 3 で追加 (multi-stage / distroless)
│   ├── chat-service/
│   │   └── Dockerfile                  # ★ 同上
│   └── realtime-service/
│       └── Dockerfile                  # ★ 同上
└── Makefile                            # image-build-all ターゲットを追加
```

Go / proto / docs の追加はなし。**3 サービス分の Dockerfile を書いて、ローカルでイメージが普通にビルドできるところまで** がこのフェーズのゴール。

---

## スコープ

3 サービス分の Docker イメージをビルドして、後続のオーケストレーション (本リポジトリ Phase 4 の compose + Envoy standalone / infra リポジトリ側の K8s) が引き取れる状態にする受け渡し点 (ハンドオフ) を作る。本リポジトリ側では:

- `docker build` が通る
- distroless 採用で最小イメージサイズ (15-20 MB 程度)
- 適切に `EXPOSE` 宣言 / `ENTRYPOINT` 設定 / 非 root 実行

**前提**: Phase 2 完了 (3 サービスすべて `go run` で起動できる)。

> 複数コンテナ連携 / DB / Redis 起動 / WebSocket E2E の動作確認は **Phase 4 (本リポジトリの compose + Envoy standalone)** で行う。**本番向け K8s での動作確認**は infra リポジトリ側の責務。

---

## ステップ構成

| ステップ | 内容 |
|---------|------|
| 1 | Dockerfile の構造を理解する (3 サービス共通) |
| 2 | 3 サービス分の Dockerfile を作成 |
| 3 | `Makefile` に `image-build-all` ターゲットを追加 |
| 4 | `make image-build-all` でビルド検証 |

---

## ステップ 1: Dockerfile の構造を理解する (3 サービス共通)

3 サービスの Dockerfile はほぼ同じテンプレートで、サービスごとに変わるのは「サービス名」と「公開ポート」だけ。テンプレートは **ビルドステージ** と **ランタイムステージ** の 2 段構成 (multi-stage) になっている。

### ビルドステージ — Go バイナリを生成する

- ベースイメージは `golang:1.22-alpine`。`go.work` が指している Go のバージョンに合わせる。
- 作業ディレクトリは `/src`。
- **`ENV GOWORK=off` を設定する**。リポジトリ直下の `go.work` は 3 サービスすべてを参照しているが、Dockerfile は 1 サービスのソースしか COPY しない。workspace モードのままだと `cannot load module ../chat-service` のように落ちるため、module モードに固定する。各サービスの `go.mod` には既に `gen/go` と `pkg` への `replace` ディレクティブが書かれているので、これだけで依存解決は問題なく回る。
- COPY するのは次の 3 つだけ:
  - `gen/go` (proto から生成された Go コード)
  - `pkg` (共通 util)
  - `services/<対象サービス>` (ビルド対象本体)

  他のサービスは持ち込まない。1 サービスのソースだけが変わったときに他サービスのレイヤキャッシュを壊さないという副次的な効果もある。
- ビルドコマンドは `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /app/<service> ./cmd/server`。それぞれの意味は:
  - `CGO_ENABLED=0` — C ライブラリへのリンクを切り、完全な静的バイナリにする。これが後段で distroless/**static** を使える前提。
  - `-trimpath` — ビルドホストのファイルパスをバイナリに埋め込まない (再現性確保 / 情報漏れ防止)。
  - `-ldflags="-s -w"` — シンボルテーブルと DWARF デバッグ情報を削ってバイナリを小さくする。
- 出力先は `/app/<service>`。次のステージから `--from=builder` で取り出す目印。

### ランタイムステージ — distroless にバイナリだけ載せる

- ベースは `gcr.io/distroless/static-debian12`。シェルもパッケージマネージャも入っていない最小ランタイム。Go の静的バイナリを動かすには十分で、攻撃面が極端に小さい。
- `COPY --from=builder /app/<service> /app/<service>` でビルドステージからバイナリ 1 つだけを引き継ぐ。
- `EXPOSE` でサービスが listen するポートを宣言する。実際にポートを開く効果はないが、後段の compose / K8s が読みやすくなる目印。
- `USER nonroot:nonroot` で非 root 実行に固定する。distroless にあらかじめ用意されている `nonroot` ユーザを指定するだけ。
- `ENTRYPOINT ["/app/<service>"]` でバイナリ直叩き。`CMD` は使わない。

---

## ステップ 2: 3 サービス分の Dockerfile を作成

ステップ 1 のテンプレートを 3 サービス分作る。差分は次の表だけ。

| サービス | 配置 | EXPOSE するポート | 出力バイナリ名 |
|---|---|---|---|
| user-service | `services/user-service/Dockerfile` | `50051 8082` (gRPC + JWKS HTTP) | `user-service` |
| chat-service | `services/chat-service/Dockerfile` | `50052` (gRPC) | `chat-service` |
| realtime-service | `services/realtime-service/Dockerfile` | `8081` (WebSocket HTTP) | `realtime-service` |

### ビルドする際の注意点

- ビルドコンテキストは **必ずリポジトリのルート** にする。各サービスディレクトリで `docker build` してしまうと `gen/go` や `pkg` を COPY できない。
- 単発で 1 サービスだけビルドする場合の例:

  `docker build -t user-service:0.1.0 -f services/user-service/Dockerfile .`

### 確認ポイント

- `docker image ls <service>` で **15-20 MB 程度** に収まっている (distroless + 静的バイナリ + `-s -w` の効果)。
- `docker run --rm <service>:0.1.0` を素で実行すると、必須 env var (`DATABASE_URL` / `REDIS_ADDR` 等) が無いというエラー (slog 1 行) で落ちる。これは「プロセス自体は起動できている = ENTRYPOINT も非 root 切り替えも問題ない」状態の確認になる。

---

## ステップ 3: `Makefile` に `image-build-all` ターゲットを追加

3 サービス分の `docker build` を 1 コマンドで回せるようにする。タグを切り替えたいときに困らないよう、`IMAGE_TAG` は変数化して上書きできるようにしておく (デフォルト `0.1.0`)。

ターゲットの中身は、ステップ 2 の `docker build` コマンドを 3 サービス並べるだけ。例:

```makefile
IMAGE_TAG ?= 0.1.0

image-build-all:
	docker build -t user-service:$(IMAGE_TAG)     -f services/user-service/Dockerfile .
	docker build -t chat-service:$(IMAGE_TAG)     -f services/chat-service/Dockerfile .
	docker build -t realtime-service:$(IMAGE_TAG) -f services/realtime-service/Dockerfile .
```

`.PHONY` に `image-build-all` を追加するのも忘れない。タグを変えたいときは `make image-build-all IMAGE_TAG=0.2.0` のように呼ぶ。

---

## ステップ 4: `make image-build-all` でビルド検証

- `make image-build-all` を実行して 3 サービスすべてのビルドが PASS する。
- `docker image ls | grep -E 'user-service|chat-service|realtime-service'` で 3 つのイメージが `0.1.0` タグで並ぶ。
- 各イメージのサイズが 15-20 MB 程度。
- 念のため `docker run --rm <service>:0.1.0` を 1 つずつ実行し、必須 env var エラーで落ちることを確認する (= プロセスは起動できている)。

---

## 成果物

- [ ] 3 つの Dockerfile (multi-stage + distroless + `GOWORK=off`)
- [ ] `make image-build-all` が PASS する
- [ ] 各イメージが 15-20 MB 程度で収まる
- [ ] `docker image ls` で 3 サービスが `user-service:0.1.0` / `chat-service:0.1.0` / `realtime-service:0.1.0` として存在する

---

## 後続フェーズ / infra リポジトリへの受け渡し

このフェーズで完成した Docker イメージを以下 2 つの場所が引き取って実行する:

- **本リポジトリ Phase 4** (`compose.yaml` + `envoy.yaml`): dev / 動作検証用。Envoy standalone で JWT 検証経路を含む E2E まで通す
- **infra リポジトリ** (K8s + Gateway API + SecurityPolicy + Helm): 本番運用用

両者に渡すべき共通情報:

| 項目 | 内容 |
|------|------|
| イメージタグ | `user-service:0.1.0` / `chat-service:0.1.0` / `realtime-service:0.1.0` |
| 公開ポート | user: 50051 (gRPC) + 8082 (JWKS HTTP) / chat: 50052 (gRPC) / realtime: 8081 (WebSocket) |
| 必須 env var | **user**: `DATABASE_URL` / `JWT_PRIVATE_KEY` / `JWT_KEY_ID` / **chat**: `DATABASE_URL` / `USER_SERVICE_ADDR` / **realtime**: `REDIS_ADDR` / `CHAT_SERVICE_ADDR` |
| 前提: `x-user-id` の注入 | Envoy が JWT 検証して gRPC metadata / HTTP header の `x-user-id` に注入する (Phase 4 では Envoy standalone、本番では Envoy Gateway) |
| 前提: JWKS 公開 | user-service の `/.well-known/jwks.json` を Envoy が起動時に fetch (`remote_jwks.http_uri` 等) |

Phase 4 (本リポジトリ) が compose で動作確認済みの構成を、infra リポジトリは K8s 化する (compose service → Deployment、`--scale` → `replicas`、Envoy standalone → Envoy Gateway 等)。

---

## 前のフェーズ / 次のフェーズ

- 前: [Phase 2: chat (Message) + realtime-service](./phase-2.md)
- 次: [Phase 4: compose + Envoy standalone + E2E 検証](./phase-4.md)
