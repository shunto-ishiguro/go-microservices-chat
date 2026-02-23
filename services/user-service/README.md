# User Service

ユーザーの CRUD 操作を提供する REST API サービス。

## 技術スタック

| 項目 | 技術 |
|------|------|
| 言語 | Go 1.22 |
| ルーター | Chi v5 |
| DB | PostgreSQL 15 |
| ドライバー | pgx v5 + pgxpool |
| ログ | log/slog (JSON) |
| マイグレーション | golang-migrate |

## エンドポイント

| メソッド | パス | 説明 |
|---------|------|------|
| POST | `/api/v1/users` | ユーザー作成 |
| GET | `/api/v1/users` | ユーザー一覧（ページネーション付き） |
| GET | `/api/v1/users/{id}` | ユーザー詳細 |
| PUT | `/api/v1/users/{id}` | ユーザー更新 |
| DELETE | `/api/v1/users/{id}` | ユーザー削除 |
| GET | `/api/v1/health` | ヘルスチェック |

## ディレクトリ構成

```
services/user-service/
│
├── cmd/server/
│   └── main.go                    # アプリのエントリポイント
│                                  #   - PostgreSQL に接続
│                                  #   - 各層（Repository → Service → Handler）を組み立てる
│                                  #   - HTTP サーバーを起動
│                                  #   - Ctrl+C で安全に停止（Graceful Shutdown）
│
├── internal/
│   ├── config/
│   │   └── config.go              # 環境変数から設定を読み込む
│   │                              #   - PORT, DATABASE_URL, LOG_LEVEL
│   │                              #   - 未設定ならデフォルト値を使う
│   │
│   ├── domain/
│   │   └── user.go                # User エンティティの定義
│   │                              #   - User 構造体（ID, email, username 等）
│   │                              #   - CreateUserInput（作成時の入力）
│   │                              #   - UpdateUserInput（更新時の入力）
│   │
│   ├── repository/
│   │   ├── repository.go          # UserRepository インターフェース（設計図）
│   │   │                          #   - Create, GetByID, List, Update, Delete 等を定義
│   │   │                          #   - 実装は持たない。「何ができるか」だけ決める
│   │   │                          #   - これにより本番（PostgreSQL）とテスト（メモリ）を差し替え可能
│   │   │
│   │   └── postgres.go            # PostgreSQL 用の実装
│   │                              #   - 上のインターフェースを実際に SQL で実装
│   │                              #   - pgxpool でコネクションプーリング
│   │                              #   - ユニーク制約違反（重複メール等）のエラーハンドリング
│   │
│   ├── service/
│   │   ├── user_service.go        # ビジネスロジック
│   │   │                          #   - バリデーション（メール形式、ユーザー名の長さ等）
│   │   │                          #   - メール・ユーザー名の重複チェック
│   │   │                          #   - UUID 生成、タイムスタンプ設定
│   │   │                          #   - Repository を呼び出してデータを操作
│   │   │
│   │   └── user_service_test.go   # Service 層のユニットテスト
│   │                              #   - DB 不要（メモリ上の偽リポジトリを使用）
│   │                              #   - バリデーション、重複チェック、CRUD の検証
│   │
│   └── handler/rest/
│       ├── handler.go             # ルーター設定
│       │                          #   - Chi Router でエンドポイントを登録
│       │                          #   - ミドルウェア（ログ、リクエストID、パニック復帰）を適用
│       │
│       ├── user_handler.go        # 各エンドポイントのハンドラー
│       │                          #   - HTTP リクエストの JSON を読み取る
│       │                          #   - Service 層を呼び出す
│       │                          #   - 結果を JSON レスポンスとして返す
│       │
│       ├── user_handler_test.go   # Handler 層のテスト
│       │                          #   - httptest で実際に HTTP リクエストを送って検証
│       │                          #   - ステータスコード、レスポンス形式を確認
│       │
│       └── response.go            # 共通レスポンスフォーマット
│                                  #   - 成功: { "data": ..., "meta": { "request_id": ... } }
│                                  #   - エラー: { "error": { "code": ..., "message": ... }, "meta": ... }
│                                  #   - エラー種別に応じた HTTP ステータスコードの振り分け
│
└── migrations/
    ├── 000001_create_users_table.up.sql    # テーブル作成 SQL（users テーブル + インデックス）
    └── 000001_create_users_table.down.sql  # ロールバック SQL（テーブル削除）
```

## 環境変数

| 変数名 | デフォルト値 | 説明 |
|--------|-------------|------|
| `PORT` | `8001` | HTTPサーバーのポート |
| `DATABASE_URL` | `postgres://chat:chat@localhost:5432/userdb?sslmode=disable` | PostgreSQL 接続文字列 |
| `LOG_LEVEL` | `info` | ログレベル（debug/info/warn/error） |

## 起動方法

### 初回セットアップ

```bash
# 1. PostgreSQL を起動（Docker）
docker compose up -d

# 2. マイグレーション実行（テーブル作成）
migrate -path migrations -database "postgres://chat:chat@localhost:5432/userdb?sslmode=disable" up

# 3. Go サーバーを起動（ローカル）
go run ./cmd/server
```

### 2回目以降（データはボリュームに残っているのでマイグレーション不要）

```bash
# 1. PostgreSQL を起動（Docker）
docker compose up -d

# 2. Go サーバーを起動（ローカル）
go run ./cmd/server
```

### 停止

```bash
# PostgreSQL を停止（データは保持される）
docker compose down

# PostgreSQL を停止してデータも全削除（次回マイグレーション再実行が必要）
docker compose down -v
```

## マイグレーション

```bash
# マイグレーション実行（テーブル作成）
migrate -path migrations -database "postgres://chat:chat@localhost:5432/userdb?sslmode=disable" up

# マイグレーションをロールバック（テーブル削除）
migrate -path migrations -database "postgres://chat:chat@localhost:5432/userdb?sslmode=disable" down

# 新しいマイグレーションファイルを作成
migrate create -ext sql -dir migrations -seq create_xxx
```

## テスト

```bash
go test ./...
```

## 使用例

```bash
# ユーザー作成
curl -X POST localhost:8001/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","username":"testuser","display_name":"Test User"}'

# ユーザー一覧
curl localhost:8001/api/v1/users?limit=10&offset=0

# ユーザー詳細
curl localhost:8001/api/v1/users/{id}

# ユーザー更新
curl -X PUT localhost:8001/api/v1/users/{id} \
  -H "Content-Type: application/json" \
  -d '{"display_name":"New Name"}'

# ユーザー削除
curl -X DELETE localhost:8001/api/v1/users/{id}

# ヘルスチェック
curl localhost:8001/api/v1/health
```
