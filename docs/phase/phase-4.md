# Phase 4: compose + Envoy standalone + E2E 検証

---

## ディレクトリ構成 (Phase 4 完了時)

```
go-microservices-chat/
├── compose.yaml                    # ★ dev / E2E 専用 (本番向けではない)
├── envoy.yaml                      # ★ Envoy standalone 設定 (JWT filter + routes)
├── scripts/
│   └── e2e/                        # ★ E2E シナリオ
│       ├── 00-up.sh                # compose up -d + migration 流し込み
│       ├── 01-register-login.sh    # 認証 golden path
│       ├── 02-chat.sh              # WebSocket + 2 インスタンス間 Pub/Sub
│       ├── 03-auth-failures.sh     # JWT 無し / 期限切れ / 改ざん / 他人更新
│       └── 99-down.sh              # compose down -v
└── Makefile                        # e2e-up / e2e-run / e2e-down ターゲットを追加
```

> 本番向けの K8s マニフェスト / Gateway API / SecurityPolicy / NetworkPolicy 等は **infra リポジトリ側の責務** であり、このリポジトリには追加しない。ここの `compose.yaml` / `envoy.yaml` はあくまで **dev / 動作検証専用** の軽量 stack。

---

## スコープ

Phase 3 でビルドした Docker イメージを **実際に起動し、JWT 検証経路を含む全フローが通ることを確認する**。`docker compose up --scale realtime-service=2` で Redis Pub/Sub によるプロセス間配信も同時に検証する。

**前提**: Phase 3 完了 (3 サービスのイメージが `docker image ls` にある)。

### なぜ app リポジトリで E2E までやるか

- infra リポジトリは K8s で組む想定 → iteration が遅い / 複雑 / 学習コスト高
- app リポジトリは compose で 5 秒で up/down できる dev 環境を持つ → コード変更の検証サイクルが速い
- 「ゲートウェイ経由の JWT 検証」という最重要経路を infra 到達前に確認できる
- infra 側の K8s 化は「compose で動くものを K8s manifest に写す」作業になって負荷が下がる

### app と infra の境界 (再整理)

| | app リポジトリ (本) | infra リポジトリ |
|---|---|---|
| Go サービス / proto / Dockerfile | ✅ | — |
| dev 用 `compose.yaml` + `envoy.yaml` | ✅ (本 Phase) | — |
| E2E シナリオスクリプト | ✅ (本 Phase) | — |
| K8s マニフェスト / Gateway API | — | ✅ |
| NetworkPolicy / Rate Limit / Observability | — | ✅ |
| 本番 Secret 管理 / CI/CD | — | ✅ |

---

## ステップ構成

| 部 | テーマ | ステップ |
|----|--------|----------|
| A | dev stack の宣言 | 1〜3 |
| B | E2E スクリプト | 4〜7 |
| C | 動作確認 + Makefile | 8 |

---

## A. dev stack の宣言

### ステップ 1: RSA 鍵ペア生成 (ローカル検証用)

```bash
mkdir -p .local-keys
openssl genrsa -out .local-keys/private.pem 2048
openssl rsa -in .local-keys/private.pem -pubout -out .local-keys/public.pem
echo ".local-keys/" >> .gitignore
```

**確認ポイント**: `private.pem` / `public.pem` が生成。**`.gitignore` に追加されているか要確認** (リポジトリに commit しない)。

---

### ステップ 2: `compose.yaml`

```yaml
# compose.yaml (dev / E2E 専用)
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: chat
      POSTGRES_PASSWORD: chat
      POSTGRES_DB: userdb
    ports: ["5432:5432"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U chat"]
      interval: 2s

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

  user-service:
    image: user-service:0.1.0
    depends_on:
      postgres: {condition: service_healthy}
    environment:
      DATABASE_URL: postgres://chat:chat@postgres:5432/userdb
      JWT_PRIVATE_KEY_FILE: /keys/private.pem
      JWT_KEY_ID: k1
    volumes:
      - ./.local-keys:/keys:ro

  chat-service:
    image: chat-service:0.1.0
    depends_on:
      postgres: {condition: service_healthy}
      user-service: {condition: service_started}
    environment:
      DATABASE_URL: postgres://chat:chat@postgres:5432/chatdb
      USER_SERVICE_ADDR: user-service:50051

  realtime-service:
    image: realtime-service:0.1.0
    depends_on:
      redis: {condition: service_started}
      chat-service: {condition: service_started}
    environment:
      REDIS_ADDR: redis:6379
      CHAT_SERVICE_ADDR: chat-service:50052

  envoy:
    image: envoyproxy/envoy:v1.30-latest
    depends_on:
      user-service: {condition: service_started}
    volumes:
      - ./envoy.yaml:/etc/envoy/envoy.yaml:ro
    ports:
      - "8080:8080"      # REST / WebSocket
      - "50051:50051"    # gRPC
      - "9901:9901"      # Envoy admin
```

> `realtime-service` は `docker compose up --scale realtime-service=2` で 2 プロセスに増やす。Envoy が round-robin で振り分けるので、Pub/Sub 跨ぎ配信の検証がそのままできる。

**確認ポイント**: `docker compose config` でパース成功。

---

### ステップ 3: `envoy.yaml` (Envoy standalone)

```yaml
# envoy.yaml - dev / E2E 専用の最小設定
admin:
  address:
    socket_address: {address: 0.0.0.0, port_value: 9901}

static_resources:
  listeners:
    - name: grpc_listener
      address: {socket_address: {address: 0.0.0.0, port_value: 50051}}
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: AUTO
                stat_prefix: grpc
                route_config:
                  name: grpc_routes
                  virtual_hosts:
                    - name: grpc
                      domains: ["*"]
                      routes:
                        - match: {prefix: "/user.v1.UserService/Register"}
                          route: {cluster: user_service}
                        - match: {prefix: "/user.v1.UserService/Login"}
                          route: {cluster: user_service}
                        - match: {prefix: "/user.v1.UserService/Refresh"}
                          route: {cluster: user_service}
                        - match: {prefix: "/user.v1.UserService/"}
                          route: {cluster: user_service}
                        - match: {prefix: "/chat.v1.ChatService/"}
                          route: {cluster: chat_service}
                http_filters:
                  - name: envoy.filters.http.jwt_authn
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication
                      providers:
                        chat-app:
                          issuer: chat-app
                          remote_jwks:
                            http_uri:
                              uri: http://user-service:8082/.well-known/jwks.json
                              cluster: user_service_jwks
                              timeout: 3s
                            cache_duration: {seconds: 300}
                          claim_to_headers:
                            - {claim_name: sub, header_name: x-user-id}
                            - {claim_name: preferred_username, header_name: x-username}
                      rules:
                        - match: {prefix: "/user.v1.UserService/Register"}
                          requires: {}
                        - match: {prefix: "/user.v1.UserService/Login"}
                          requires: {}
                        - match: {prefix: "/user.v1.UserService/Refresh"}
                          requires: {}
                        - match: {prefix: "/"}
                          requires: {provider_name: chat-app}
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

    - name: http_listener
      address: {socket_address: {address: 0.0.0.0, port_value: 8080}}
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: http
                upgrade_configs: [{upgrade_type: websocket}]
                route_config:
                  virtual_hosts:
                    - name: http
                      domains: ["*"]
                      routes:
                        - match: {prefix: "/ws"}
                          route: {cluster: realtime_service, upgrade_configs: [{upgrade_type: websocket}]}
                http_filters:
                  - name: envoy.filters.http.jwt_authn
                    # (grpc_listener と同じ JwtAuthentication 設定を展開 or 参照)
                  - name: envoy.filters.http.router

  clusters:
    - name: user_service
      type: STRICT_DNS
      http2_protocol_options: {}
      load_assignment:
        cluster_name: user_service
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: user-service, port_value: 50051}}}

    - name: user_service_jwks
      type: STRICT_DNS
      load_assignment:
        cluster_name: user_service_jwks
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: user-service, port_value: 8082}}}

    - name: chat_service
      type: STRICT_DNS
      http2_protocol_options: {}
      load_assignment:
        cluster_name: chat_service
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: chat-service, port_value: 50052}}}

    - name: realtime_service
      type: STRICT_DNS
      load_assignment:
        cluster_name: realtime_service
        endpoints:
          - lb_endpoints:
              - endpoint: {address: {socket_address: {address: realtime-service, port_value: 8081}}}
```

> Envoy Gateway (K8s) と同じ JWT 検証ロジックだが、Gateway API CRD の抽象を剥がした **生 Envoy 設定**。infra 側で Envoy Gateway に移植する時は `SecurityPolicy` / `GRPCRoute` / `HTTPRoute` YAML に書き直すだけ。

**確認ポイント**: `docker run --rm -v $PWD/envoy.yaml:/etc/envoy/envoy.yaml envoyproxy/envoy:v1.30-latest --mode validate` でパースエラー無し。

---

## B. E2E スクリプト

### ステップ 4: `scripts/e2e/00-up.sh` (起動 + migration)

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1) compose を立ち上げ
docker compose up -d --scale realtime-service=2

# 2) postgres が healthy になるまで待つ (compose の healthcheck で自動待機済みだが念のため)
until docker compose exec -T postgres pg_isready -U chat; do sleep 1; done

# 3) chatdb を作成
docker compose exec -T postgres psql -U chat -c "CREATE DATABASE chatdb" || true

# 4) migration を流し込む
for sql in services/user-service/migrations/*.up.sql; do
  docker compose exec -T postgres psql -U chat -d userdb < "$sql"
done
for sql in services/chat-service/migrations/*.up.sql; do
  docker compose exec -T postgres psql -U chat -d chatdb < "$sql"
done

echo "✅ stack up + migrations applied"
```

**確認ポイント**: `docker compose ps` で全サービス + realtime-service ×2 が `Up`。Envoy admin (`curl localhost:9901/ready`) が `LIVE`。

---

### ステップ 5: `scripts/e2e/01-register-login.sh` (golden path)

```bash
#!/usr/bin/env bash
set -euo pipefail

GW=localhost:50051

# Register (公開 RPC、JWT 不要)
grpcurl -plaintext -d '{"email":"alice@example.com","password":"pw12345","username":"alice","display_name":"Alice"}' \
  $GW user.v1.UserService/Register

# Login (公開 RPC) → access_token 取得
ACCESS=$(grpcurl -plaintext -d '{"email":"alice@example.com","password":"pw12345"}' \
  $GW user.v1.UserService/Login | jq -r .accessToken)

# 保護 RPC (JWT 検証が Envoy で効く)
grpcurl -plaintext -H "authorization: Bearer $ACCESS" \
  -d '{"userId":"'$(echo $ACCESS | cut -d. -f2 | base64 -d 2>/dev/null | jq -r .sub)'"}' \
  $GW user.v1.UserService/GetUser

echo "✅ Register → Login → GetUser (JWT 検証経由) 成功"
```

**確認ポイント**: `GetUser` が **Envoy の JWT 検証を通過して** `User` を返す。JWT が Envoy で `x-user-id` に変換されて app に届いていることの証明。

---

### ステップ 6: `scripts/e2e/02-chat.sh` (WebSocket + 2 プロセス Pub/Sub)

```bash
#!/usr/bin/env bash
set -euo pipefail

# alice と bob を用意 (省略、01 と同じ要領)
ALICE=$(./get_token.sh alice)
BOB=$(./get_token.sh bob)

# Room 作成 + bob 参加
ROOM=$(grpcurl -plaintext -H "authorization: Bearer $ALICE" \
  -d '{"name":"general"}' localhost:50051 chat.v1.ChatService/CreateRoom | jq -r .room.id)

grpcurl -plaintext -H "authorization: Bearer $BOB" \
  -d '{"roomId":"'$ROOM'"}' localhost:50051 chat.v1.ChatService/JoinRoom

# bob が WebSocket で受信待機 (別プロセス、realtime-service の 2 インスタンスのどちらかに振り分けられる)
wscat -c "ws://localhost:8080/ws" -H "authorization: Bearer $BOB" > /tmp/bob.log &
BOB_WS_PID=$!
sleep 1

# alice が送信 (こちらも別インスタンスに振り分けられる可能性大)
echo '{"type":"message","room_id":"'$ROOM'","content":"hello bob"}' | \
  wscat -c "ws://localhost:8080/ws" -H "authorization: Bearer $ALICE" --wait 2

# bob のログに "hello bob" が入ったかを確認
sleep 1
grep "hello bob" /tmp/bob.log && echo "✅ Pub/Sub 経由で別プロセスに配信成功" || exit 1
kill $BOB_WS_PID
```

**確認ポイント**:
- alice と bob が **別の realtime-service プロセス** に接続される (Envoy の round-robin)
- alice の送信が Redis PUBLISH を経由して bob のプロセスに届く
- これが **Redis Pub/Sub による水平スケールの証明** — まさに Phase 2 で実装した設計の最終検証

---

### ステップ 7: `scripts/e2e/03-auth-failures.sh` (失敗ケース)

| ケース | 期待結果 | テスト方法 |
|---|---|---|
| JWT 無しで保護 RPC | `401 Unauthorized` (Envoy が弾く) | `grpcurl` に Authorization ヘッダ無し |
| 期限切れ JWT | `401 Unauthorized` | JWT_KEY_ID を別値で発行した JWT (署名は合うが iss / exp で落ちる用の別スクリプト) |
| 署名改ざん JWT | `401 Unauthorized` | JWT の一部を書き換える |
| alice の JWT で bob の `UpdateUser` | `PermissionDenied` | app 側の所有者認可 |
| 非メンバーの `SendMessage` | `PermissionDenied` | app 側の `EnsureMember` |

```bash
# 例: JWT 無し → Envoy が 401
grpcurl -plaintext -d '{"userId":"xxx"}' localhost:50051 user.v1.UserService/GetUser || \
  echo "✅ JWT 無しで弾かれた"

# 例: 他人更新 → app 側 PermissionDenied
grpcurl -plaintext -H "authorization: Bearer $ALICE" \
  -d '{"userId":"bob-uuid","displayName":"hacked"}' \
  localhost:50051 user.v1.UserService/UpdateUser || \
  echo "✅ 他人更新で PermissionDenied"
```

**確認ポイント**: Envoy 層での `401` と app 層での `PermissionDenied` が **責務通りに** 分担されている。

---

### `scripts/e2e/99-down.sh`

```bash
#!/usr/bin/env bash
docker compose down -v    # ボリューム (PG データ) も削除
```

---

## C. 動作確認 + Makefile

### ステップ 8: Makefile ターゲット

```makefile
e2e-up:
	./scripts/e2e/00-up.sh

e2e-run:
	./scripts/e2e/01-register-login.sh
	./scripts/e2e/02-chat.sh
	./scripts/e2e/03-auth-failures.sh

e2e-down:
	./scripts/e2e/99-down.sh

e2e-all: e2e-up e2e-run e2e-down

.PHONY: e2e-up e2e-run e2e-down e2e-all
```

`make e2e-all` で「立ち上げ → 全シナリオ実行 → 片付け」が 1 コマンド。

---

## 成果物

- [ ] `compose.yaml` + `envoy.yaml` で全サービスが Envoy 経由で繋がる
- [ ] `make e2e-up` で 5 秒程度で dev stack が立ち上がる (realtime ×2 含む)
- [ ] `scripts/e2e/01-register-login.sh` で **Envoy の JWT 検証経路を含む golden path** が通る
- [ ] `scripts/e2e/02-chat.sh` で **別プロセス間の Pub/Sub fan-out** が動く (Phase 2 設計の最終検証)
- [ ] `scripts/e2e/03-auth-failures.sh` で認証失敗ケースが Envoy / app の正しい層で弾かれる
- [ ] `make e2e-down` で環境が完全に片付く (`-v` で PG のデータも削除)

### E2E の全体像 (Phase 4 完了時)

```mermaid
flowchart TB
    Tester["開発者 / CI"]
    Tester -->|"make e2e-up"| Up
    Up["docker compose up<br/>--scale realtime-service=2"] --> Stack

    subgraph Stack["dev stack (compose.yaml 内)"]
      direction TB
      Envoy["envoy<br/>:8080 REST/WS<br/>:50051 gRPC<br/>+ JWT 検証 filter"]
      US["user-service<br/>JWT 発行 + JWKS 配信"]
      CS["chat-service<br/>Room + Message"]
      RS1["realtime-svc #1<br/>WebSocket + Hub"]
      RS2["realtime-svc #2<br/>WebSocket + Hub"]
      PG[("PostgreSQL<br/>userdb + chatdb")]
      R[("Redis")]

      Envoy -.->|起動時 JWKS fetch| US
      Envoy -->|gRPC + x-user-id| US
      Envoy -->|gRPC + x-user-id| CS
      Envoy -->|WebSocket| RS1
      Envoy -->|WebSocket| RS2
      US --> PG
      CS --> PG
      CS -->|gRPC GetUser| US
      RS1 -->|PUBLISH / PSUBSCRIBE| R
      RS2 -->|PUBLISH / PSUBSCRIBE| R
      RS1 -->|gRPC SendMessage| CS
      RS2 -->|gRPC SendMessage| CS
    end

    Stack --> Run["make e2e-run"]
    Run --> V1["01: Register → Login → GetUser<br/>(JWT 検証経路)"]
    Run --> V2["02: alice/bob を別プロセスに接続<br/>→ Pub/Sub fan-out 確認"]
    Run --> V3["03: 失敗ケース (401 / PermissionDenied)"]

    V1 --> Down["make e2e-down"]
    V2 --> Down
    V3 --> Down

    classDef user fill:#e8f5e9,stroke:#388e3c
    classDef infra fill:#fff3e0,stroke:#f57c00
    classDef app fill:#e3f2fd,stroke:#1976d2
    classDef store fill:#f3e5f5,stroke:#7b1fa2

    class Tester,Up,Run,V1,V2,V3,Down user
    class Envoy infra
    class US,CS,RS1,RS2 app
    class PG,R store
```

### infra リポジトリへの継承

この Phase 4 で「動くことが確認済み」の構成を、infra リポジトリが K8s に写す:

| 本リポジトリ (compose) | infra リポジトリ (K8s) |
|---|---|
| `compose.yaml` の service 定義 | `Deployment` + `Service` + `StatefulSet` (PG) |
| `envoy.yaml` の JWT filter | `SecurityPolicy` (Envoy Gateway 拡張) |
| `envoy.yaml` の routes | `GRPCRoute` / `HTTPRoute` (Gateway API) |
| `--scale realtime-service=2` | `Deployment.spec.replicas: 2` |
| docker network の分離 | `NetworkPolicy` (infra 側の追加責務) |
| migration をスクリプトで流し込み | `Job` (migration runner) |

compose での動作が担保されているので、infra 側は「本番化」に集中できる (networking / 永続ボリューム / HA / observability)。

---

## 前のフェーズ

[Phase 3: Dockerfile + イメージビルド](./phase-3.md)
