# Phase 0: プロジェクト骨組み

---

## 学習目標

モノレポの **骨格だけ** を整える。Go Workspace、Protocol Buffers、Buf CLI、ディレクトリ規約、Makefile スケルトン。**K8s や Envoy には触れない**。

Phase 0 の成果物は「`buf generate` が通って、ディレクトリが規約通りに切られている」状態。アプリケーションの実装は Phase 1 から、インフラ (K8s / Envoy) は Phase 4 でまとめて。

| # | 目標 | 詳細 |
|---|------|------|
| 1 | Go Workspace でモノレポを扱える | `go.work` に複数モジュールを登録 |
| 2 | Protocol Buffers の基本を理解する | `message`, `service`, `rpc`, フィールド番号 |
| 3 | Buf CLI で lint と Go コード生成ができる | `buf.yaml`, `buf.gen.yaml` |
| 4 | ディレクトリ規約を決めて記録する | 垂直分割 + `deploy/` はまだ空 |
| 5 | Makefile の骨格を作れる | Phase 1 以降で肉付けする土台 |

---

## 前提知識

- プログラミング基礎
- ターミナル / コマンドラインの基本操作
- git の基本

> Go 言語の文法は Phase 1 で扱うので、ここでは `go version` が通ることだけ確認できれば OK。

---

## ステップ

### ステップ 1: ツールのインストール

Phase 0 で使う最低限のツールだけ入れる。K8s 系ツールは Phase 4 で。

- [ ] Go 1.22+ (`go version` で確認)
- [ ] [Buf CLI](https://buf.build/docs/installation) (`buf --version`)
- [ ] [grpcurl](https://github.com/fullstorydev/grpcurl) (Phase 1 以降の動作確認で使う)
- [ ] Docker (Phase 1 で PostgreSQL を `docker run` するため)

---

### ステップ 2: Go Workspace の設定

- [ ] モノレポのルートに `go.work` を作成
- [ ] 空の `use` セクションで最小構成から始め、Phase 1 以降で追加していく

```go
go 1.22.2

use (
    ./gen/go
    ./pkg
    // Phase 1 で ./services/user-service を追加
    // Phase 2 で ./services/chat-service を追加
    // Phase 3 で ./services/realtime-service を追加
)
```

- [ ] `pkg` モジュールの下地:

```bash
mkdir -p pkg && cd pkg && go mod init go-microservices-chat/pkg
```

**確認ポイント**: `go work sync` がエラーなく通る。

---

### ステップ 3: Buf CLI + proto の確認

`proto/user/v1/user.proto` は先行して用意されている前提。Buf の設定だけ確認する。

- [ ] `proto/buf.yaml` / `proto/buf.gen.yaml` を確認 (存在しなければ作成)

```yaml
# proto/buf.yaml
version: v1
lint:
  use:
    - DEFAULT
  except:
    - PACKAGE_VERSION_SUFFIX
```

```yaml
# proto/buf.gen.yaml
version: v1
plugins:
  - plugin: go
    out: ../gen/go
    opt:
      - paths=source_relative
  - plugin: go-grpc
    out: ../gen/go
    opt:
      - paths=source_relative
```

- [ ] `gen` モジュールの下地:

```bash
mkdir -p gen/go && cd gen/go && go mod init go-microservices-chat/gen/go
```

- [ ] `buf lint` と `buf generate` を実行

```bash
cd proto && buf lint && buf generate
```

**確認ポイント**: `gen/go/user/v1/user.pb.go` と `user_grpc.pb.go` が生成される。`buf lint` が STANDARD ルールで pass。

---

### ステップ 4: ディレクトリ規約の整備

最終形 (Phase 4 完了後) のディレクトリ構造を想定し、空ディレクトリは `.gitkeep` で予約しておく。

```
go-microservices-chat/
├── docs/                            # 既存
├── proto/                           # 既存、Phase 2 で chat/v1、Phase 3 で realtime/v1 を追加
├── gen/go/                          # Buf 生成コード (ステップ 3 で作った)
├── pkg/                             # 共有パッケージ (Phase 1 で auth/ interceptor/ を追加)
│   └── .gitkeep
├── services/                        # マイクロサービス (Phase 1-3 で user/chat/realtime を追加)
│   └── .gitkeep
├── deploy/                          # K8s マニフェスト (Phase 4 でまとめて使う)
│   └── .gitkeep
├── Makefile                         # ステップ 5 で作る
├── go.work                          # ステップ 2 で作った
└── README.md                        # 既存
```

> **`services/api-gateway/` は作らない** (Phase 4 で Envoy Gateway が YAML で担当)。
> **`docker-compose.yml` は使わない**。Phase 1-3 の開発では `docker run` (単発) で PostgreSQL / Redis を使う。

**確認ポイント**: `git status` で新規ディレクトリが認識され、リポジトリのトップビューが規約通り。

---

### ステップ 5: Makefile の骨格

Phase 0 で使うのは `proto-gen` くらい。Phase 1 以降で肉付けする前提。

```makefile
# Makefile (Phase 0 版)
.PHONY: proto-gen proto-lint test

proto-gen:
	cd proto && buf generate

proto-lint:
	cd proto && buf lint

test:
	go test ./...
```

**確認ポイント**: `make proto-gen` が通り、`make proto-lint` で警告なし。

---

## 成果物

Phase 0 完了時に以下が整っていること:

- [ ] `go version` / `buf --version` / `docker --version` / `grpcurl --version` が通る
- [ ] `go.work` でモノレポが構成されている (`gen/go`, `pkg`)
- [ ] `buf generate` で `gen/go/user/v1/` に Go コードが生成される
- [ ] ディレクトリ規約が整っている (`services/`, `deploy/` は空、`.gitkeep` 済み)
- [ ] Makefile に `proto-gen` / `proto-lint` / `test` がある

---

## 学べる技術

| カテゴリ | 技術 | 用途 |
|----------|------|------|
| Go プロジェクト管理 | Go Workspace (`go.work`) | モノレポ構成 |
| スキーマ定義 | Protocol Buffers | サービス間通信の型定義 |
| コード生成 | Buf CLI | proto から Go コード生成 |
| プロジェクト規約 | 垂直分割 + deploy 分離 | Phase 1 以降を迷いなく進める土台 |

> Go の文法、gRPC サーバー実装、K8s、Envoy はそれぞれの Phase で学ぶ。Phase 0 は「準備だけ」。

---

## 参考リソース

| リソース | URL |
|----------|-----|
| Go Workspaces | https://go.dev/ref/mod#workspaces |
| Buf Docs | https://buf.build/docs/introduction |
| Protocol Buffers | https://protobuf.dev/ |

---

## 次のフェーズ

Phase 0 が完了したら [Phase 1: user-service (Go で完結)](./phase-1.md) に進む。**K8s や Envoy には触れず**、Go コードと `go run` + `docker run postgres` だけでローカル完結に user-service を作る。
