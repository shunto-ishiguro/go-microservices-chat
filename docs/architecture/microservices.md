# マイクロサービス詳細設計

## スコープ

本プロジェクトは学習目的のため、マイクロサービスの **本質的な構造を体験するのに必要な最小構成** に絞る。

| サービス | 実装 Phase | デプロイ Phase | 役割 |
|---------|-------|-------|------|
| user-service | 1 (Go) | 4 (K8s) | ユーザー管理・認証 |
| chat-service | 2 (Go) | 4 (K8s) | 公開ルーム・メンバーシップ・メッセージ永続化 |
| realtime-service | 3 (Go) | 4 (K8s) | WebSocket 接続・リアルタイム配信 |

**api-gateway は Go で実装しない**。代わりに **Envoy Gateway (Phase 4 で YAML 設定)** が以下を担当：
- REST↔gRPC 変換 (gRPC-JSON Transcoder)
- JWT 検証 (SecurityPolicy)
- レートリミット (BackendTrafficPolicy)
- CORS / ログ / ルーティング

notification-service / media-service 等は将来の発展課題とし、主要フローの学習には含めない。

---

## 実行環境の 2 段階

**Phase 1〜3 (ローカル Go 開発期間)**:
- `go run` でサービスを並行起動
- `docker run postgres/redis` でミドルウェア起動
- grpcurl で `x-user-id` を手動注入してテスト

**Phase 4 (K8s + Envoy 期間)**:
- kind クラスタ上で全サービスが Pod として動作
- Envoy Gateway が JWT 検証 / REST 変換 / ルーティングを担当
- **サービス側の Go コードは Phase 3 から一切変更なし**

---

## サービス一覧と責務

### 1. User Service

**責務**: ユーザーのライフサイクル管理と認証

| 項目 | 内容 |
|------|------|
| ポート | gRPC: 50051 |
| データストア | PostgreSQL (userdb) |
| プロトコル | gRPC (Phase 1 から)。REST は Phase 4 で Envoy Transcoder 経由で外部公開 |
| K8s リソース | Deployment / Service / Secret / NetworkPolicy / Migration Job |

**機能**:
- ユーザー登録・ログイン (bcrypt + 自前 JWT)
- リフレッシュトークン管理 (DB 保管、ローテーション)
- プロフィール管理（表示名、アバター、ステータス）
- JWKS 公開 (Envoy の SecurityPolicy が参照)

> friends / 1:1 DM / ユーザー単体検索は **スコープ外**。「好きな公開ルームに参加してそこで喋る」モデル。

---

### 2. Chat Service

**責務**: チャットルームとメッセージの永続化管理

| 項目 | 内容 |
|------|------|
| ポート | gRPC: 50052 |
| データストア | PostgreSQL (chatdb) |
| プロトコル | gRPC (Unary のみ) |
| K8s リソース | Deployment / Service / Migration Job |

**機能**:
- 公開ルームの作成・検索・詳細取得・一覧 (自分の参加 / 全公開)
- ルームへの参加 (`JoinRoom`) / 退出 (`LeaveRoom`) — 本人のみ
- メッセージの送信・保存・取得 (送信は realtime-service 経由)
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
| プロトコル | WebSocket (クライアント向け) + gRPC Unary (chat-service への保存依頼) |
| K8s リソース | Deployment (Phase 4 で **replicas: 2**) / Service |

**機能**:
- WebSocket 接続の確立・維持・切断管理 (Hub パターン)
- メッセージ受信 → chat-service へ保存 (gRPC Unary) + Redis Pub/Sub 経由で配信 **を並行実行**
- 起動時から `PSUBSCRIBE room:*` で全ルームのイベントを購読 → Hub → WebSocket に配信

> Phase 4 で **複数 Pod (replicas: 2)** に展開する前提の設計。Pod 間の配信は Redis Pub/Sub が自動で担うため Go コードは 1 Pod 想定のまま。

---

### 4. Envoy Gateway (Gateway API 実装)

**責務**: 外部リクエストの認証・ルーティング・REST↔gRPC 変換・レートリミット

**コード実装なし。すべて YAML で宣言。**

| 項目 | 内容 |
|------|------|
| ポート | 80 (REST / WebSocket) / 50051 (gRPC) |
| データストア | なし (Stateless)、レートリミット用 Redis と連携 |
| 設定方式 | Gateway API (Gateway / GRPCRoute / HTTPRoute) + Envoy Gateway 拡張 (SecurityPolicy / BackendTrafficPolicy / EnvoyPatchPolicy) |

**機能**:
- JWT トークン検証 (`SecurityPolicy` + JWKS)
- JWT claims (`sub` など) を内部 gRPC リクエストの metadata に注入 (`claimToHeaders`)
- REST→gRPC 変換 (`gRPC-JSON Transcoder` Envoy フィルター)
- WebSocket の透過的転送 (`HTTPRoute` + WebSocket upgrade)
- レート制限 (`BackendTrafficPolicy` + Redis カウンター)
- CORS 設定
- アクセスログ

---

## サービス間通信の詳細

### 同期通信 (gRPC Unary)

```mermaid
graph LR
    EG[Envoy Gateway] -->|"gRPC (GetUser, x-user-id)"| US[user-service]
    EG -->|"gRPC (SendMessage, x-user-id)"| CS[chat-service]
    RS[realtime-service] -->|"gRPC (SaveMessage)"| CS
    CS -->|"gRPC (GetUser)"| US
```

| 呼び出し元 | 呼び出し先 | RPC | 目的 |
|-----------|-----------|-----|------|
| Envoy Gateway | user-service | Login, Register, Refresh, Logout, GetUser, UpdateUser | 外部リクエスト転送 |
| Envoy Gateway | chat-service | CreateRoom, ListRooms, SearchRooms, GetRoom, JoinRoom, LeaveRoom, GetMessages | 外部リクエスト転送 |
| chat-service | user-service | GetUser | メンバー表示情報の取得 |
| realtime-service | chat-service | SendMessage | WebSocket 受信メッセージの永続化 |

> **ストリーミング RPC は使わない**。リアルタイム配信は Redis Pub/Sub に集約し、サービス間の同期通信は Unary のみに揃える。

### Pub/Sub (Redis Pub/Sub) — リアルタイム配信の中核

realtime-service は **Phase 3 から Redis Pub/Sub を最初から使う**。Phase 4 で Pod 数を増やしても Go コードを変更しないための設計。

```mermaid
graph LR
    A[Alice ブラウザ] -->|WS| R1[realtime-svc-A]
    R1 -->|PUBLISH room:general| Redis[("Redis")]
    R1 -->|gRPC SendMessage| CS[chat-service]
    CS --> PG[("PostgreSQL")]
    Redis -->|SUBSCRIBE push| R2[realtime-svc-B]
    R2 -->|WS| B[Bob ブラウザ]
```

| 観点 | 内容 |
|------|------|
| 配信バス | Redis (`PSUBSCRIBE room:*` で全ルーム一括購読) |
| PUBLISH の起点 | WebSocket で受信した realtime-service インスタンス |
| SUBSCRIBE | 全 realtime-service インスタンスが起動時から張る |
| Pod 数を変える対応 | **Redis は N 人にも fan-out する → Go コードを一切変えずに replicas を増やせる** |
| 責務の分離 | 永続化 (chat-service) と 配信 (Redis + realtime-service) が別経路 |

#### Redis の他の役割

| 用途 | キー形式 | 備考 |
|------|---------|------|
| レートリミット | `ratelimit:login:<ip>` | Envoy BackendTrafficPolicy から参照 |

---

## 認証情報の伝搬 (Phase 4 の Envoy 導入後)

JWT 検証は **Envoy Gateway に集約** し、内部サービスは gRPC メタデータで user_id を受け取る。

```mermaid
sequenceDiagram
    participant C as クライアント
    participant EG as Envoy Gateway
    participant Svc as user-svc / chat-svc

    C->>EG: REST/gRPC (Bearer access_token)
    EG->>EG: SecurityPolicy で JWT 検証<br/>(JWKS で署名検証)
    EG->>EG: claims.sub → x-user-id ヘッダーに
    EG->>Svc: gRPC + metadata{x-user-id, x-username}
    Svc->>Svc: TrustedUserID Interceptor<br/>Context に UserID
    Svc->>EG: response
    EG->>C: REST/gRPC response
```

**信頼境界**: 外部呼び出しの JWT 検証は Envoy Gateway のみ。内部サービスは Envoy を信頼し、K8s `NetworkPolicy` で外部からの直接アクセスを拒否する。

---

## Database-per-Service パターン

各サービスが独自のデータベースを所有する。本プロジェクトでは PostgreSQL の **DB を分けて運用** する。

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

> 物理的には単一の PostgreSQL StatefulSet 上に複数 DB を配置するが、**論理的には別物として扱う**。

---

## サービスディスカバリ

**Phase 1〜3 (ローカル)**:

```
localhost:50051    # user-service (go run)
localhost:50052    # chat-service (go run)
localhost:8081     # realtime-service (go run)
localhost:5432     # PostgreSQL (docker run)
localhost:6379     # Redis (docker run)
```

**Phase 4 (K8s)**: K8s の内部 DNS を使う：

```
user-service.chat-app.svc.cluster.local:50051
chat-service.chat-app.svc.cluster.local:50052
realtime-service.chat-app.svc.cluster.local:8081
postgres.chat-app.svc.cluster.local:5432
redis.chat-app.svc.cluster.local:6379
```

Namespace 内なら `user-service:50051` で解決可能。環境変数 `USER_SERVICE_ADDR` などを Phase 1〜3 と Phase 4 で切り替えることで、Go コードは同じものが動く。

---

## 関連ドキュメント

- [データモデル設計](./data-model.md)
- [API 設計](./api-design.md)
- [ディレクトリ構成](./directory-structure.md)
