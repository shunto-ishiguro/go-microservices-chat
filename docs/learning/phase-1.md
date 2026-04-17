# Phase 1: Go 基礎 - REST API (user-service + PostgreSQL)

---

## 学習目標

本フェーズでは、Go 言語の基本を習得し、実践的な REST API を構築する力を身につける。

| # | 目標 | 詳細 |
|---|------|------|
| 1 | Go の基本文法を理解する | 型システム、構造体、インターフェース、エラーハンドリング |
| 2 | HTTP サーバーを構築できる | net/http パッケージと Chi Router の活用 |
| 3 | REST API を設計・実装できる | RESTful な endpoint 設計と CRUD 操作 |
| 4 | PostgreSQL を操作できる | database/sql, pgx ドライバーを用いた DB 操作 |
| 5 | テストを書ける | unit test, table-driven tests, httptest パッケージ |

---

## 前提知識

- プログラミング基礎（変数、関数、条件分岐、ループ）
- ターミナル / コマンドラインの基本操作
- HTTP の基本概念（GET, POST, PUT, DELETE）
- SQL の基礎（SELECT, INSERT, UPDATE, DELETE）

---

## ステップ

### ステップ 1: Go 環境セットアップ

Go の開発環境を整え、最初のプログラムを動かす。

- [ ] Go のインストール（公式サイトから最新の stable 版）
- [ ] VS Code のインストールと Go 拡張機能の設定
- [ ] `GOPATH`, `GOROOT` の理解
- [ ] Go Modules (`go mod init`, `go mod tidy`) の基本
- [ ] `Hello, World!` プログラムの作成と実行

**確認ポイント**: `go version` でバージョンが表示され、`go run main.go` でプログラムが実行できること。

---

### ステップ 2: Go 基本文法

Go の型システムと主要な言語機能を学ぶ。

- [ ] 基本型（int, string, bool, float64）と変数宣言（`var`, `:=`）
- [ ] 配列、スライス、マップ
- [ ] 構造体（struct）の定義とメソッド
- [ ] インターフェース（interface）の定義と実装
- [ ] エラーハンドリング（`error` 型, `errors.New`, `fmt.Errorf`, `%w`）
- [ ] ポインタの基礎
- [ ] goroutine と channel の基礎（概念理解のみ、本格活用は Phase 4）

**確認ポイント**: 構造体にメソッドを定義し、インターフェースを満たす実装ができること。

---

### ステップ 3: HTTP サーバー構築

Go 標準ライブラリと Chi Router を使った HTTP サーバーの構築。

- [ ] `net/http` パッケージで基本的な HTTP サーバーを起動
- [ ] `http.HandlerFunc` と `http.Handler` インターフェースの理解
- [ ] Chi Router の導入（`go get github.com/go-chi/chi/v5`）
- [ ] ルーティング（URL パラメータ、クエリパラメータ）
- [ ] ミドルウェアの概念と実装（ログ、リクエスト ID、CORS）
- [ ] JSON のシリアライズ / デシリアライズ（`encoding/json`）
- [ ] リクエストバリデーション

**確認ポイント**: Chi Router を使って複数のエンドポイントを持つ HTTP サーバーが動作すること。

---

### ステップ 4: user-service REST API 実装

ユーザー管理の CRUD API を実装する。

- [ ] プロジェクト構造の設計（cmd/, internal/, pkg/）
- [ ] User モデルの定義
- [ ] REST API エンドポイントの実装:

| メソッド | パス | 説明 |
|----------|------|------|
| `POST` | `/api/v1/users` | ユーザー作成 |
| `GET` | `/api/v1/users` | ユーザー一覧取得 |
| `GET` | `/api/v1/users/{id}` | ユーザー詳細取得 |
| `PUT` | `/api/v1/users/{id}` | ユーザー更新 |
| `DELETE` | `/api/v1/users/{id}` | ユーザー削除 |

- [ ] DTO（Data Transfer Object）パターンの導入
- [ ] 適切な HTTP ステータスコードの返却
- [ ] エラーレスポンスの統一フォーマット設計

**確認ポイント**: curl や Postman で全 CRUD エンドポイントが期待通り動作すること。

---

### ステップ 5: PostgreSQL 接続

PostgreSQL データベースへの接続と操作。

- [ ] PostgreSQL のインストール / Docker での起動
- [ ] `database/sql` パッケージの基本（Open, Query, Exec, QueryRow）
- [ ] `pgx` ドライバーの導入と接続設定
- [ ] コネクションプール（`pgxpool`）の設定
- [ ] マイグレーションツールの導入（golang-migrate）
- [ ] マイグレーションファイルの作成（users テーブル）
- [ ] 環境変数による接続情報の管理

**確認ポイント**: Go アプリケーションから PostgreSQL に接続し、SQL が実行できること。

---

### ステップ 6: Repository パターン実装

データアクセス層を Repository パターンで整理する。

- [ ] Repository インターフェースの定義
- [ ] PostgreSQL 用 Repository の実装
- [ ] インメモリ Repository の実装（テスト用）
- [ ] Service 層の導入（ビジネスロジックの分離）
- [ ] 依存性注入（DI）パターンの適用（コンストラクタインジェクション）

```go
// Repository インターフェースの例
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id string) (*User, error)
    List(ctx context.Context, limit, offset int) ([]*User, error)
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
}
```

**確認ポイント**: Repository インターフェースを通じて DB 操作が行え、実装を差し替えられること。

---

### ステップ 7: テスト

Go のテスト手法を学び、user-service にテストを追加する。

- [ ] `go test` の基本とテストファイルの命名規則（`_test.go`）
- [ ] `testing.T` とアサーション（`testify` の導入も可）
- [ ] Table-driven tests パターン
- [ ] `httptest` パッケージで HTTP ハンドラーのテスト
- [ ] モック Repository を使った Service 層のテスト
- [ ] テストカバレッジの確認（`go test -cover`）
- [ ] テストヘルパー関数の整理

```go
// Table-driven test の例
func TestCreateUser(t *testing.T) {
    tests := []struct {
        name    string
        input   CreateUserRequest
        wantErr bool
    }{
        {name: "正常系", input: CreateUserRequest{Name: "太郎"}, wantErr: false},
        {name: "名前が空", input: CreateUserRequest{Name: ""}, wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト実行
        })
    }
}
```

**確認ポイント**: `go test ./...` で全テストが PASS し、主要なコードパスがカバーされていること。

---

### ステップ 8: ログとエラーハンドリング

構造化ログとエラーハンドリングの統一。

- [ ] `log/slog` パッケージの基本（Go 1.21+）
- [ ] JSON 形式の構造化ログ出力
- [ ] ログレベル（Debug, Info, Warn, Error）の使い分け
- [ ] リクエスト ID をログに含める（ミドルウェア連携）
- [ ] カスタムエラー型の定義
- [ ] エラーのラッピングとアンラッピング（`errors.Is`, `errors.As`）
- [ ] Graceful Shutdown の実装

**確認ポイント**: 構造化ログが JSON で出力され、エラー時に適切なログとレスポンスが返ること。

---

## 成果物

Phase 1 完了時に以下が動作していること:

- [x] user-service REST API が起動し、ユーザーの CRUD 操作ができる
- [x] PostgreSQL にデータが永続化される
- [x] マイグレーションでスキーマ管理ができる
- [x] テストが整備され、`go test ./...` が PASS する
- [x] 構造化ログが出力される
- [x] Graceful Shutdown が動作する

### ディレクトリ構成イメージ

```
services/user-service/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handler/       # HTTP ハンドラー
│   ├── service/       # ビジネスロジック
│   ├── repository/    # データアクセス層
│   ├── model/         # ドメインモデル
│   └── config/        # 設定管理
├── migrations/        # DBマイグレーション
├── go.mod
└── go.sum
```

---

## 学べる技術

| カテゴリ | 技術 | 用途 |
|----------|------|------|
| 言語 | Go | メイン開発言語 |
| HTTP | net/http | 標準 HTTP サーバー |
| ルーティング | Chi Router | 軽量ルーター |
| データベース | PostgreSQL | リレーショナル DB |
| ドライバー | pgx | PostgreSQL Go ドライバー |
| DB 操作 | database/sql | 標準 DB インターフェース |
| テスト | testing, httptest | 標準テストパッケージ |
| ログ | log/slog | 構造化ロギング |
| マイグレーション | golang-migrate | スキーマ管理 |

---

## 参考リソース

### 公式ドキュメント

| リソース | URL | 説明 |
|----------|-----|------|
| A Tour of Go | https://go.dev/tour/ | Go 言語の対話型チュートリアル |
| Effective Go | https://go.dev/doc/effective_go | Go らしいコードの書き方 |
| Go by Example | https://gobyexample.com/ | 実例ベースの Go リファレンス |
| Go 標準ライブラリ | https://pkg.go.dev/std | 標準パッケージのドキュメント |

### 書籍・コース

| リソース | 著者 | 説明 |
|----------|------|------|
| Let's Go | Alex Edwards | Go で Web アプリケーションを構築する実践書 |
| Let's Go Further | Alex Edwards | REST API 構築に特化した続編 |
| Learning Go | Jon Bodner | Go の基礎から応用までカバー |

### ツール

| ツール | 用途 |
|--------|------|
| VS Code + Go 拡張 | 開発環境 |
| Postman / curl | API テスト |
| pgAdmin / DBeaver | DB 管理 |
| Docker | PostgreSQL のローカル実行 |

---

## 次のフェーズ

Phase 1 が完了したら [Phase 2: 認証・認可 (自前 JWT + bcrypt)](./phase-2.md) に進む。
