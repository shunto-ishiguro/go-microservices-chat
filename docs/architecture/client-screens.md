# クライアント画面設計

**API 設計の妥当性を検証するための画面リスト**。UI デザインやコンポーネント設計には踏み込まない。「どの画面がどの API を叩くか」だけを明確にする。

このプロジェクトはバックエンド学習が主眼のため、フロント実装は必須ではないが、この画面セットを前提に API を設計している。

---

## サービスコンセプト

**「好きな公開ルーム (コミュニティ) を探して参加し、そこでチャットする」**

- friends 機能なし (1:1 DM なし)
- 全ルームは public (誰でも検索・参加可能)
- メンバーシップ = 「参加ボタンを押した」という自発的アクション

---

## 画面一覧

| # | 画面 | 役割 |
|---|------|------|
| 1 | ログイン | 既存ユーザーの認証 |
| 2 | 新規登録 | アカウント作成 |
| 3 | 公開ルームを探す | ルーム検索・参加 |
| 4 | ルーム作成 | 新しい公開ルームを作る |
| 5 | 自分の参加ルーム一覧 | ホーム画面 (ログイン後の起点) |
| 6 | チャット | メッセージ送受信 |
| 7 | ユーザー編集 | 自分のプロフィール更新 |

---

## 画面 → API マッピング

| # | 画面 | マウント時に叩く | 操作による API 呼び出し |
|---|------|-----------------|------------------------|
| 1 | ログイン | — | REST `POST /api/v1/auth/login` |
| 2 | 新規登録 | — | REST `POST /api/v1/auth/register` |
| 3 | 公開ルームを探す | REST `GET /api/v1/rooms/search?q=` | **参加ボタン**: REST `POST /api/v1/rooms/:id/join` |
| 4 | ルーム作成 | — | **作成ボタン**: REST `POST /api/v1/rooms` → 作成されたルームを返すので自動で #6 チャットへ遷移 |
| 5 | 自分の参加ルーム一覧 | REST `GET /api/v1/rooms` (自分のメンバーシップのみ) | — (ルームクリックで #6 へ) |
| 6 | チャット | REST `GET /api/v1/rooms/:id` + REST `GET /api/v1/rooms/:id/messages` + **WebSocket 接続** + WS `subscribe` | **送信**: WS `send_message` / **退出ボタン**: REST `DELETE /api/v1/rooms/:id/members/me` |
| 7 | ユーザー編集 | REST `GET /api/v1/users/me` | **保存**: REST `PUT /api/v1/users/me` |

### 共通ヘッダー操作

- **ログアウト**: REST `POST /api/v1/auth/logout` → #1 へ

### WebSocket で push されるもの (チャット画面 #6 に表示)

- `new_message` (新着メッセージ)

---

## 画面遷移図

```
                     ┌─ [1] ログイン ──────────┐
                     │                          ↓
[2] 新規登録 ────→ [1] ──────→ [5] 参加ルーム一覧 ←────┐
                                    │                      │
                     ┌──────────────┼──────────────┐       │
                     ↓              ↓              ↓       │
              [3] ルーム探す  [4] ルーム作成  [6] チャット ─退出┘
                     │              │              ↑
                     └──参加────────┴──作成完了────┘

どの画面からも → [7] ユーザー編集 / ログアウト
```

---

## この画面セットから導かれる API (最終形)

### user-service

| 種別 | エンドポイント/RPC |
|------|-------------------|
| 認証 | `Register` / `Login` / `Refresh` / `Logout` |
| プロフィール | `GetUser` / `UpdateUser` |

**消える**:
- `SearchUsers` (ユーザー単体検索の画面なし)
- `ListFriends` / `SendFriendRequest` / `AcceptFriendRequest` (friends 機能なし)

### chat-service

| 種別 | RPC |
|------|-----|
| ルーム管理 | `CreateRoom` / `GetRoom` / `ListRooms` / `SearchRooms` |
| 参加管理 | `JoinRoom` / `LeaveRoom` |
| メッセージ履歴 | `GetMessages` |
| 内部 (WS 経由) | `SendMessage` |

**消える**:
- `AddMember` / `RemoveMember` (招待・追放なし、自己管理のみ)

### realtime-service

gRPC サーバーを公開しない。WebSocket のみ:

- クライアント → サーバー: `subscribe` / `unsubscribe` / `send_message` / `ping`
- サーバー → クライアント: `new_message` / `error` / `pong`

---

## 認可ルール (画面操作の権限)

| 操作 | 認可 |
|------|------|
| ルーム検索・閲覧 | 認証ユーザーなら誰でも可 |
| ルーム作成・参加・退出 | 認証ユーザーなら誰でも可 |
| メッセージ履歴取得 | **そのルームの `room_members` に居ること** |
| メッセージ送信 | **そのルームの `room_members` に居ること** |
| プロフィール更新 | 自分自身のみ |

---

## 関連ドキュメント

- [API 設計](./api-design.md)
- [データモデル](./data-model.md)
- [マイクロサービス詳細](./microservices.md)
- [リアルタイムメッセージ配信フロー](../flow/realtime-message-flow.md)
