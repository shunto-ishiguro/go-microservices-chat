# マイクロサービス詳細設計

## スコープ

本プロジェクトは学習目的のため、マイクロサービスの **本質的な構造を体験するのに必要な最小構成** に絞る。また **本番向け K8s マニフェスト (Deployment / Gateway API / SecurityPolicy / NetworkPolicy / Helm) は別リポジトリ** に分離する前提。本リポジトリには Go + proto + Dockerfile + **dev/E2E 用の compose.yaml + envoy.yaml + scripts/e2e/** まで持つ (Phase 4)。

| サービス | 実装 Phase | 役割 |
|---------|-------|------|
| user-service | 1 | ユーザー管理・JWT 発行・JWKS 配信 |
| chat-service | 1 (Room) / 2 (Message) | 公開ルーム・メンバーシップ・メッセージ永続化 |
| realtime-service | 2 | WebSocket 接続・リアルタイム配信 (Redis Pub/Sub) |

**api-gateway 相当 (JWT 検証 / REST↔gRPC 変換 / ルーティング / Rate Limit) は Go では実装しない**。これは **Envoy** の責務とする (dev は本リポジトリの `envoy.yaml` で Envoy standalone を 1 コンテナ / 本番は infra リポジトリの K8s で Envoy Gateway + Gateway API、どちらも同じ Envoy コア)。

notification-service / media-service 等は将来の発展課題とし、主要フローの学習には含めない。

---

## 実行環境の扱い

**本リポジトリは Go バイナリ / Docker イメージ / dev E2E までを責務とする**。本番運用向けの K8s デプロイは infra リポジトリが担当。

| フェーズ | 本リポジトリでの検証 | infra リポジトリ側での検証 |
|---------|-------------------|--------------------------|
| Phase 1 | `go test ./...` (InMem Repository + bufconn) | — |
| Phase 2 | `go test ./...` (InMem pubsub + fake chatclient) | — |
| Phase 3 | `docker build` が 3 サービスとも通る | — |
| Phase 4 | `make e2e-all` (compose + Envoy standalone で JWT 検証経路 + Pub/Sub 複数インスタンス + 認証失敗ケース) | 本番 K8s クラスタでの production E2E / 負荷試験 / observability |

> `go test ./...` は **DB / Redis / ゲートウェイ無し** で PASS する設計 (InMem + fake を DI)。Phase 4 の E2E は `docker compose up` で実 PG / 実 Redis / 実 Envoy を立てて全経路を通す。

---

## サービス一覧と責務

### 1. User Service

**責務**: ユーザーのライフサイクル管理 + JWT 発行者としての責務 + JWKS 公開

| 項目 | 内容 |
|------|------|
| ポート | gRPC: 50051 / JWKS HTTP: 8082 |
| データストア | PostgreSQL (`userdb`) |
| プロトコル | gRPC Unary |
| 必須 env | `DATABASE_URL` / `JWT_PRIVATE_KEY` / `JWT_KEY_ID` |

**機能**:
- ユーザー登録・ログイン (bcrypt + RS256 JWT **発行**)
- リフレッシュトークン管理 (DB 保管、ローテーション)
- プロフィール管理 (表示名・アバター・ステータス)
- **JWKS 公開** (`/.well-known/jwks.json`) — infra 側 Envoyが取りに来る

> **JWT の "検証" は実装しない**。それは infra 側 Envoyの責務。user-service は発行者 + 公開鍵配布者に徹する。

> friends / 1:1 DM / ユーザー単体検索は **スコープ外**。「好きな公開ルームに参加してそこで喋る」モデル。

---

### 2. Chat Service

**責務**: チャットルームとメッセージの永続化管理

| 項目 | 内容 |
|------|------|
| ポート | gRPC: 50052 |
| データストア | PostgreSQL (`chatdb`) |
| プロトコル | gRPC Unary のみ |
| 必須 env | `DATABASE_URL` / `USER_SERVICE_ADDR` |

**機能**:
- 公開ルームの作成・検索・詳細取得・一覧 (自分の参加 / 全公開)
- ルームへの参加 (`JoinRoom`) / 退出 (`LeaveRoom`) — 本人のみ
- メッセージの送信・保存・取得 (送信は realtime-service から gRPC Unary で呼ばれる)
- チャット履歴のページネーション (cursor-based)

> chat-service は **永続化専任**。リアルタイム配信には一切関与しない (配信は realtime-service + Redis Pub/Sub に集約)。

> 全ルームは public。プライベート・招待制・1:1 DM は持たない。

---

### 3. Realtime Service

**責務**: WebSocket 接続管理とリアルタイムメッセージ配信

| 項目 | 内容 |
|------|------|
| ポート | WebSocket: 8081 |
| データストア | Redis (Pub/Sub のみ) |
| プロトコル | WebSocket (クライアント向け) + gRPC Unary client (chat-service への保存依頼) |
| 必須 env | `REDIS_ADDR` / `CHAT_SERVICE_ADDR` |

**機能**:
- WebSocket 接続の確立・維持・切断管理 (Hub パターン)
- メッセージ受信 → chat-service へ保存 (gRPC Unary) + Redis Pub/Sub 経由で配信 **を並行実行**
- 起動時から `PSUBSCRIBE room:*` で全ルームのイベントを購読 → Hub → WebSocket に配信

> **複数プロセス / 複数レプリカ前提の設計**。プロセス間の配信は Redis Pub/Sub が自動で担うため Go コードは単一プロセス想定のまま。infra 側で `replicas: 2+` にしても Go コード変更不要。

---

### 4. Envoy (infra リポジトリ側)

**責務**: 外部リクエストの認証・ルーティング・REST↔gRPC 変換・レート制限

**本リポジトリでは一切実装しない**。infra 側で **Envoy** を設定ファイルだけで立てる (compose では Envoy standalone を 1 コンテナ、K8s では Envoy Gateway で Gateway API + SecurityPolicy)。Go コードは書かない。

受け渡しの最低条件:
- Envoy が user-service の `/.well-known/jwks.json` を起動時に取得 (`remoteJWKS.uri`)
- JWT を検証して `claims.sub` を `x-user-id` metadata として下流に注入 (`claimToHeaders`)
- ルーティング先はサービス名 (`user-service`, `chat-service`, `realtime-service`) を env var 経由で解決

---

## サービス間通信

### gRPC Unary

```mermaid
graph LR
    GW[Envoy<br/>infra 側] -->|"gRPC (GetUser, x-user-id)"| US[user-service]
    GW -->|"gRPC (SendMessage は呼ばない)"| CS[chat-service]
    RS[realtime-service] -->|"gRPC (SendMessage)"| CS
    CS -->|"gRPC (GetUser)"| US
```

| 呼び出し元 | 呼び出し先 | RPC | 目的 |
|-----------|-----------|-----|------|
| Gateway | user-service | Register, Login, Refresh, GetUser, UpdateUser | 外部リクエスト転送 |
| Gateway | chat-service | CreateRoom, ListRooms, SearchRooms, GetRoom, JoinRoom, LeaveRoom, GetMessages | 外部リクエスト転送 |
| chat-service | user-service | GetUser | メンバー表示情報の取得 |
| realtime-service | chat-service | SendMessage | WebSocket 受信メッセージの永続化 |

> **ストリーミング RPC は使わない**。リアルタイム配信は Redis Pub/Sub に集約し、サービス間の同期通信は Unary のみに揃える。

### Pub/Sub (Redis) — リアルタイム配信の中核

realtime-service は **Phase 2 の実装時点から Redis Pub/Sub を最初から使う**。単一プロセスでも Pub/Sub を通す設計にしておけば、複数プロセス / 複数レプリカに増やした時に Go コードを 1 行も変えずに fan-out が成立する。

```mermaid
graph LR
    A[Alice ブラウザ] -->|WS| R1[realtime-svc プロセス A]
    R1 -->|PUBLISH room:general| Redis[("Redis")]
    R1 -->|gRPC SendMessage| CS[chat-service]
    CS --> PG[("PostgreSQL")]
    Redis -->|SUBSCRIBE push| R2[realtime-svc プロセス B]
    R2 -->|WS| B[Bob ブラウザ]
```

| 観点 | 内容 |
|------|------|
| 配信バス | Redis (`PSUBSCRIBE room:*` で全ルーム一括購読) |
| PUBLISH の起点 | WebSocket で受信した realtime-service インスタンス |
| SUBSCRIBE | 全 realtime-service インスタンスが起動時から張る |
| プロセス数を変える対応 | **Redis が N プロセスに fan-out する → Go コードを一切変えずに増やせる** |
| 責務の分離 | 永続化 (chat-service) と 配信 (Redis + realtime-service) が別経路 |

---

## 認証情報の伝搬

JWT 検証は **infra 側 Envoy** に集約し、内部サービスは gRPC メタデータで user_id を受け取る。

```mermaid
sequenceDiagram
    participant C as クライアント
    participant GW as Gateway (infra 側)
    participant Svc as user-svc / chat-svc / realtime-svc

    C->>GW: REST/gRPC (Bearer access_token)
    GW->>GW: JWT 検証 (JWKS で署名検証)
    GW->>GW: claims.sub → x-user-id ヘッダーに
    GW->>Svc: gRPC + metadata{x-user-id, x-username}
    Svc->>Svc: auth.RequesterID(ctx)<br/>呼び出し元 UserID を取得
    Svc->>GW: response
    GW->>C: REST/gRPC response
```

**信頼境界**: 外部呼び出しの JWT 検証は Envoy のみ。内部サービスは Envoy を信頼し、infra 側で外部からの直接アクセスを遮断 (NetworkPolicy / docker-compose の内部 network 等) する。

---

## Database-per-Service パターン

各サービスが独自のデータベースを所有する。PostgreSQL の **DB を分けて運用** する。

```mermaid
graph TD
    US[User Service] --> PG1[("userdb")]
    CS[Chat Service] --> PG2[("chatdb")]
    RS[Realtime Service] --> R[("Redis")]
```

**原則**:
1. 各サービスは自分のデータストアにのみ直接アクセス
2. 他サービスのデータが必要な場合は gRPC で問い合わせる
3. 外部キーをサービス境界を跨いで張らない

> 物理的に別インスタンスにするか同一インスタンスの別 DB にするかは **infra リポジトリ側の選択**。本リポジトリはどちらでも動くように `DATABASE_URL` 1 本で受ける。

---

## サービスディスカバリ

**接続先はすべて環境変数から読む** (12-factor)。デプロイターゲット (compose / K8s) ごとに値を差し替える。

| 環境変数 | 用途 | compose 例 | K8s 例 |
|---------|------|-----------|--------|
| `DATABASE_URL` | PostgreSQL | `postgres://chat:chat@postgres:5432/userdb` | `postgres://chat:chat@postgres.chat-app.svc:5432/userdb` |
| `REDIS_ADDR` | Redis | `redis:6379` | `redis.chat-app.svc:6379` |
| `USER_SERVICE_ADDR` | chat-svc → user-svc | `user-service:50051` | `user-service.chat-app.svc:50051` |
| `CHAT_SERVICE_ADDR` | realtime-svc → chat-svc | `chat-service:50052` | `chat-service.chat-app.svc:50052` |

> アプリコード側は DNS 名の形を意識しない。infra リポジトリが適切な値を env で注入する。

---

## 関連ドキュメント

- [データモデル設計](./data-model.md)
- [API 設計](./api-design.md)
- [ディレクトリ構成](./directory-structure.md)
