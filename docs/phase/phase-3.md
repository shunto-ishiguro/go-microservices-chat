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
- distroless 採用で最小イメージサイズ (数十 MB)
- 適切に `EXPOSE` 宣言 / `ENTRYPOINT` 設定

**前提**: Phase 2 完了 (3 サービスすべて `go run` で起動できる)。

> 複数コンテナ連携 / DB / Redis 起動 / WebSocket E2E の動作確認は **Phase 4 (本リポジトリの compose + Envoy standalone)** で行う。**本番向け K8s での動作確認**は infra リポジトリ側の責務。

---

## ステップ構成

| ステップ | 内容 |
|---------|------|
| 1 | user-service の Dockerfile |
| 2 | chat-service の Dockerfile |
| 3 | realtime-service の Dockerfile |
| 4 | `Makefile` に `image-build-all` ターゲット追加 |

---

## ステップ 1: user-service の Dockerfile

```dockerfile
# services/user-service/Dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src

# go.work + 依存 module をコピー (レイヤキャッシュ効かせる)
COPY go.work go.work.sum ./
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/user-service ./services/user-service

WORKDIR /src/services/user-service
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /app/user-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/user-service /app/user-service
EXPOSE 50051 8082
USER nonroot:nonroot
ENTRYPOINT ["/app/user-service"]
```

```bash
docker build -t user-service:0.1.0 -f services/user-service/Dockerfile .
```

**確認ポイント**:
- `docker image ls user-service` で ~30 MB 程度
- `docker run --rm user-service:0.1.0 --help` (もし `-help` 対応なら) か起動してみて必要な env var エラーで落ちる (= プロセスは起動できている)

---

## ステップ 2: chat-service の Dockerfile

ステップ 1 と同じ形で書く:

```dockerfile
# services/chat-service/Dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/chat-service ./services/chat-service
WORKDIR /src/services/chat-service
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /app/chat-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/chat-service /app/chat-service
EXPOSE 50052
USER nonroot:nonroot
ENTRYPOINT ["/app/chat-service"]
```

```bash
docker build -t chat-service:0.1.0 -f services/chat-service/Dockerfile .
```

---

## ステップ 3: realtime-service の Dockerfile

```dockerfile
# services/realtime-service/Dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/realtime-service ./services/realtime-service
WORKDIR /src/services/realtime-service
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /app/realtime-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/realtime-service /app/realtime-service
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/app/realtime-service"]
```

```bash
docker build -t realtime-service:0.1.0 -f services/realtime-service/Dockerfile .
```

---

## ステップ 4: `Makefile` に `image-build-all` ターゲット

```makefile
IMAGE_TAG ?= 0.1.0

image-build-all:
	docker build -t user-service:$(IMAGE_TAG)     -f services/user-service/Dockerfile .
	docker build -t chat-service:$(IMAGE_TAG)     -f services/chat-service/Dockerfile .
	docker build -t realtime-service:$(IMAGE_TAG) -f services/realtime-service/Dockerfile .

.PHONY: image-build-all
```

**確認ポイント**: `make image-build-all` で 3 イメージ全部がビルドできる。

---

## 成果物

- [ ] 3 つの Dockerfile (distroless + multi-stage)
- [ ] `make image-build-all` が PASS する
- [ ] 各イメージが 30 MB 前後で収まる
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
