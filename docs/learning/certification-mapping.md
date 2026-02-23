# 認定試験マッピング

## 概要

本プロジェクトで学ぶ内容と、3 つの認定試験（AWS SAA-C03, CKA, CKAD）の試験範囲との対応をまとめる。

---

## AWS Solutions Architect Associate (SAA-C03)

### 試験概要

| 項目 | 内容 |
|------|------|
| 試験コード | SAA-C03 |
| 問題数 | 65 問（採点対象外 15 問含む） |
| 試験時間 | 130 分 |
| 合格ライン | 720/1000 |
| 受験料 | $150 USD |

### ドメイン 1: セキュアなアーキテクチャの設計 (30%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| IAM ポリシー・ロール設計 | IRSA（IAM Roles for Service Accounts）で Pod ごとに最小権限を付与 | 5, 6 |
| VPC 設計（SG, NACL, Subnet） | Public/Private Subnet 分離、SG で DB アクセス制限 | 6 |
| データ暗号化（at rest） | RDS 暗号化、S3 SSE、DynamoDB 暗号化 | 4, 6 |
| データ暗号化（in transit） | TLS/HTTPS、gRPC の TLS、ACM 証明書 | 4, 5 |
| Cognito による認証 | JWT ベースの API 認証、User Pool 設計 | 4 |
| Secrets Manager | DB パスワード、API キーの管理 | 5, 6 |
| S3 バケットポリシー | Presigned URL によるアクセス制御、パブリックアクセスブロック | 4 |

### ドメイン 2: レジリエントなアーキテクチャの設計 (26%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| マルチ AZ 配置 | RDS Multi-AZ (prod)、EKS ノードの AZ 分散 | 5, 6 |
| Auto Scaling | EKS Cluster Autoscaler、K8s HPA | 5 |
| 疎結合アーキテクチャ | SQS/SNS による非同期メッセージング | 4 |
| デッドレターキュー | SQS DLQ でメッセージ処理失敗をハンドリング | 4 |
| ヘルスチェック | ALB ヘルスチェック + K8s Liveness/Readiness Probe | 5 |
| バックアップ・リストア | RDS 自動バックアップ、DynamoDB PITR、S3 バージョニング | 6 |
| 障害分離 | マイクロサービスの独立性、サーキットブレーカー | 2, 4 |

### ドメイン 3: 高パフォーマンスなアーキテクチャの設計 (24%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| ElastiCache | Redis によるプレゼンスキャッシュ、Pub/Sub | 3 |
| DynamoDB スループット最適化 | パーティションキー設計、GSI 活用 | 4 |
| S3 パフォーマンス | プレフィックス設計、マルチパートアップロード | 4 |
| ELB の選択と設計 | ALB（L7）+ WebSocket サポート | 5 |
| データベース選択 | RDB vs NoSQL の使い分け（PostgreSQL vs DynamoDB） | 1, 4 |

### ドメイン 4: コスト最適化されたアーキテクチャの設計 (20%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| 適切なインスタンスサイズ | 環境ごとのライトサイジング（dev は最小構成） | 5, 6 |
| DynamoDB キャパシティモード | On-Demand（dev）vs Provisioned + Auto Scaling（prod） | 4, 6 |
| S3 ストレージクラス | ライフサイクルポリシーによる自動移行 | 4 |
| NAT Gateway 最適化 | dev は Single NAT、prod は AZ ごと | 6 |
| タグ戦略 | Terraform でリソースタグを統一管理 | 6 |

### SAA-C03 カバー率の目安

```
ドメイン 1 (30%): ███████████████████████░░░░░░░ ~75%
ドメイン 2 (26%): ████████████████████████░░░░░░ ~80%
ドメイン 3 (24%): ██████████████████░░░░░░░░░░░░ ~60%
ドメイン 4 (20%): ████████████████████░░░░░░░░░░ ~65%

全体カバー率:     ████████████████████░░░░░░░░░░ ~70%
```

> プロジェクトだけで合格水準に到達可能だが、CloudFront, Lambda, Step Functions, Organizations など未使用サービスは別途学習が必要。

---

## Certified Kubernetes Administrator (CKA)

### 試験概要

| 項目 | 内容 |
|------|------|
| 試験コード | CKA |
| 形式 | パフォーマンスベース（実技） |
| 試験時間 | 120 分 |
| 合格ライン | 66% |
| 受験料 | $395 USD |
| Kubernetes バージョン | 1.29+ |

### ドメイン 1: クラスターアーキテクチャ、インストール、構成 (25%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| クラスターコンポーネント理解 | EKS のコントロールプレーン・ワーカーノード理解 | 5 |
| RBAC の設定 | ServiceAccount + IRSA で権限管理 | 5 |
| kubeconfig の管理 | EKS の kubeconfig 設定、コンテキスト切り替え | 5 |
| etcd のバックアップ | EKS マネージドのため直接操作しないが概念を理解 | 5 |
| クラスターのアップグレード | EKS のバージョンアップグレード手順 | 5 |

### ドメイン 2: ワークロードとスケジューリング (15%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| Deployment の管理 | 6 サービスの Deployment 作成・更新 | 5 |
| Rolling Update | Deployment の更新戦略設計 | 5 |
| ConfigMap と Secret | 環境変数・設定ファイルの管理 | 5 |
| リソース制限 | requests/limits の設定、LimitRange | 5 |
| スケジューリング | topologySpreadConstraints, nodeAffinity | 5 |
| Pod のスケーリング | HPA, VPA の理解と設定 | 5 |

### ドメイン 3: サービスとネットワーキング (20%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| Service の種類 | ClusterIP（全サービス）, Ingress（API Gateway） | 5 |
| Ingress | AWS ALB Ingress Controller | 5 |
| NetworkPolicy | default-deny + サービスごとの allowlist | 5 |
| DNS | Service 名による内部 DNS 解決 | 5 |
| CoreDNS | DNS トラブルシューティング | 5 |

### ドメイン 4: ストレージ (10%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| PV / PVC | Prometheus データ永続化 | 6 |
| StorageClass | EBS CSI Driver の設定 | 5, 6 |
| Volume の種類 | emptyDir, configMap, secret, persistentVolumeClaim | 5 |

### ドメイン 5: トラブルシューティング (30%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| Pod のデバッグ | kubectl logs, describe, exec, debug | 5 |
| クラスターイベント | kubectl get events | 5 |
| ノードのトラブルシュート | ノードの状態確認、リソース枯渇対応 | 5 |
| ネットワークの問題 | NetworkPolicy のデバッグ、DNS 解決確認 | 5 |
| アプリケーションの問題 | Probe 失敗時のデバッグ、OOMKilled 対応 | 5 |

### CKA カバー率の目安

```
ドメイン 1 (25%): ██████████████████░░░░░░░░░░░░ ~60%
ドメイン 2 (15%): ████████████████████████░░░░░░ ~80%
ドメイン 3 (20%): █████████████████████████████░ ~90%
ドメイン 4 (10%): ████████████░░░░░░░░░░░░░░░░░░ ~40%
ドメイン 5 (30%): ██████████████████████░░░░░░░░ ~70%

全体カバー率:     ██████████████████████░░░░░░░░ ~70%
```

> EKS はマネージド K8s のため、クラスター構築・etcd 管理の実践は限定的。CKA 合格にはこれらのトピックを追加で学習する必要がある（kubeadm, etcd バックアップ/リストアなど）。

---

## Certified Kubernetes Application Developer (CKAD)

### 試験概要

| 項目 | 内容 |
|------|------|
| 試験コード | CKAD |
| 形式 | パフォーマンスベース（実技） |
| 試験時間 | 120 分 |
| 合格ライン | 66% |
| 受験料 | $395 USD |
| Kubernetes バージョン | 1.29+ |

### ドメイン 1: アプリケーション設計とビルド (20%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| コンテナイメージの構築 | マルチステージビルド Dockerfile | 5 |
| Jobs と CronJobs | DB マイグレーション Job、クリーンアップ CronJob | 5 |
| マルチコンテナ Pod パターン | Sidecar（ログ収集）, Init Container（DB 待機） | 5 |
| リソース要件の定義 | requests/limits の適切な設定 | 5 |

### ドメイン 2: アプリケーションのデプロイメント (20%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| Deployment | 6 つのマイクロサービスの Deployment | 5 |
| Rolling Update | maxSurge, maxUnavailable の設定 | 5 |
| Blue-Green / Canary | Kustomize overlay での段階的デプロイ | 5 |
| Helm | Helm chart の理解（Prometheus/Grafana インストール） | 6 |

### ドメイン 3: アプリケーションの可観測性とメンテナンス (15%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| Liveness/Readiness/Startup Probe | gRPC ヘルスチェック Probe | 5 |
| ログの取得 | kubectl logs, 構造化ログ (JSON) | 5, 6 |
| モニタリング | Prometheus メトリクス、Grafana ダッシュボード | 6 |
| デバッグ | kubectl exec, port-forward, debug | 5 |

### ドメイン 4: アプリケーション環境、構成、セキュリティ (25%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| ConfigMap | アプリケーション設定の外部化 | 5 |
| Secret | DB パスワード、API キーの管理 | 5 |
| ServiceAccount | IRSA 連携の ServiceAccount | 5 |
| SecurityContext | runAsNonRoot, readOnlyRootFilesystem | 5 |
| ResourceQuota | Namespace レベルのリソース制限 | 5 |

### ドメイン 5: サービスとネットワーキング (20%)

| 試験トピック | プロジェクトでの実践 | Phase |
|-------------|-------------------|-------|
| Service | ClusterIP による内部通信 | 5 |
| Ingress | ALB Ingress Controller、TLS 終端 | 5 |
| NetworkPolicy | マイクロサービス間のアクセス制御 | 5 |

### CKAD カバー率の目安

```
ドメイン 1 (20%): ████████████████████████░░░░░░ ~80%
ドメイン 2 (20%): ██████████████████████░░░░░░░░ ~75%
ドメイン 3 (15%): █████████████████████████████░ ~90%
ドメイン 4 (25%): █████████████████████████████░ ~85%
ドメイン 5 (20%): █████████████████████████████░ ~90%

全体カバー率:     ██████████████████████████░░░░ ~84%
```

> CKAD はアプリケーション開発者向けの試験であり、本プロジェクトの内容と高い一致度がある。

---

## Phase 別の試験範囲カバレッジ

| Phase | 主な内容 | SAA-C03 | CKA | CKAD |
|-------|---------|---------|-----|------|
| Phase 1 | Go 基礎 + REST | - | - | - |
| Phase 2 | gRPC + マルチサービス | - | - | - |
| Phase 3 | WebSocket + Redis | △ (ElastiCache) | - | - |
| Phase 4 | AWS 統合 | ★★★ | - | - |
| Phase 5 | Docker + Kubernetes | ★ (EKS) | ★★★ | ★★★ |
| Phase 6 | Terraform + 可観測性 + CI/CD | ★★ (IaC, Monitoring) | ★★ | ★★ |

凡例: ★★★ = 大きくカバー, ★★ = 中程度, ★ = 一部, △ = 関連あり, - = 対象外

---

## 推奨する追加学習

### AWS SAA-C03 の補完

プロジェクトで扱わないが試験に出るサービス:

| サービス | 概要 | 学習方法 |
|---------|------|---------|
| AWS Lambda | サーバーレスコンピューティング | ハンズオン別途実施 |
| Amazon CloudFront | CDN | S3 との組み合わせを学ぶ |
| AWS Step Functions | ワークフローオーケストレーション | ドキュメント学習 |
| Amazon Aurora | MySQL/PostgreSQL 互換 DB | RDS との違いを理解 |
| AWS Organizations | マルチアカウント管理 | 概念を理解 |
| Amazon EventBridge | イベントバス | SNS/SQS との使い分けを理解 |
| AWS WAF | Web Application Firewall | ALB との組み合わせ |

### CKA の補完

| トピック | 概要 | 学習方法 |
|---------|------|---------|
| kubeadm によるクラスター構築 | セルフマネージド K8s | VM 上で構築練習 |
| etcd バックアップ・リストア | 状態データの管理 | etcdctl コマンド練習 |
| クラスターアップグレード | コントロールプレーン・ノード更新 | kubeadm upgrade 手順 |
| 証明書管理 | TLS 証明書のローテーション | OpenSSL + K8s CSR |

### CKAD の補完

| トピック | 概要 | 学習方法 |
|---------|------|---------|
| Custom Resource Definition | K8s の拡張 | Operator パターンを学ぶ |
| Helm chart 作成 | パッケージ管理 | 自作 Chart を作成 |

---

## 学習ロードマップ

```
Month 1-2:   Phase 1 (Go 基礎)
Month 3-4:   Phase 2 (gRPC)
Month 5-6:   Phase 3 (リアルタイム)
Month 7-9:   Phase 4 (AWS)           → SAA-C03 受験準備開始
Month 10-12: Phase 5 (Kubernetes)     → CKA/CKAD 受験準備開始
Month 13-15: Phase 6 (Terraform+CI/CD)

Month 10:    SAA-C03 受験
Month 14:    CKAD 受験（CKA より先に受験推奨）
Month 16:    CKA 受験
```

> 期間は目安。個人のペースに合わせて調整すること。

## 関連ドキュメント

- [Phase 1: Go 基礎](./phase-1.md)
- [Phase 2: gRPC](./phase-2.md)
- [Phase 3: リアルタイム](./phase-3.md)
- [Phase 4: AWS 統合](./phase-4.md)
- [Phase 5: Kubernetes](./phase-5.md)
- [Phase 6: Terraform + CI/CD](./phase-6.md)
- [AWS サービス一覧](../aws/services.md)
- [Kubernetes アーキテクチャ](../kubernetes/architecture.md)
