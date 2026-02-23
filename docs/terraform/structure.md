# Terraform 構成

## 概要

Terraform を使って AWS インフラを Infrastructure as Code (IaC) で管理する。
モジュール化により再利用性を高め、環境ごとの差分は変数で制御する。

---

## モジュール構成

```
terraform/
├── modules/              # 再利用可能なモジュール
│   ├── networking/       # VPC, Subnet, NAT Gateway, Security Group
│   ├── eks/              # EKS Cluster, Node Group, IRSA
│   ├── database/         # RDS (PostgreSQL), DynamoDB Tables
│   ├── messaging/        # SQS Queues, SNS Topics, Subscriptions
│   ├── storage/          # S3 Buckets, ECR Repositories
│   ├── auth/             # Cognito User Pool, App Client
│   ├── cache/            # ElastiCache (Redis) Cluster
│   └── observability/    # CloudWatch Log Groups, X-Ray
├── environments/         # 環境別の設定
│   ├── dev/
│   ├── staging/
│   └── prod/
└── global/               # 環境横断リソース
    ├── s3-backend/       # Terraform State 用 S3 + DynamoDB
    └── ecr/              # ECR リポジトリ（全環境共通）
```

---

## モジュール詳細

### networking モジュール

```hcl
# modules/networking/main.tf

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = "${var.project}-${var.environment}-vpc"
  cidr = var.vpc_cidr

  azs             = var.availability_zones
  private_subnets = var.private_subnet_cidrs
  public_subnets  = var.public_subnet_cidrs

  enable_nat_gateway   = true
  single_nat_gateway   = var.environment == "dev" ? true : false
  enable_dns_hostnames = true
  enable_dns_support   = true

  # EKS 用のタグ
  public_subnet_tags = {
    "kubernetes.io/role/elb"                    = 1
    "kubernetes.io/cluster/${var.cluster_name}"  = "owned"
  }

  private_subnet_tags = {
    "kubernetes.io/role/internal-elb"           = 1
    "kubernetes.io/cluster/${var.cluster_name}"  = "owned"
  }

  tags = var.common_tags
}
```

**出力**: VPC ID, Subnet IDs, NAT Gateway IDs

### eks モジュール

```hcl
# modules/eks/main.tf

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = var.cluster_name
  cluster_version = "1.29"

  vpc_id     = var.vpc_id
  subnet_ids = var.private_subnet_ids

  cluster_endpoint_public_access = true

  eks_managed_node_groups = {
    default = {
      instance_types = var.node_instance_types
      min_size       = var.node_min_size
      max_size       = var.node_max_size
      desired_size   = var.node_desired_size

      labels = {
        Environment = var.environment
      }
    }
  }

  # IRSA (IAM Roles for Service Accounts)
  enable_irsa = true

  tags = var.common_tags
}

# サービスごとの IAM ロール（IRSA）
resource "aws_iam_role" "chat_service" {
  name = "${var.cluster_name}-chat-service-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = module.eks.oidc_provider_arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "${module.eks.oidc_provider}:sub" = "system:serviceaccount:chat-app:chat-service"
        }
      }
    }]
  })
}
```

### database モジュール

```hcl
# modules/database/main.tf

# --- RDS (PostgreSQL) ---
resource "aws_db_instance" "users" {
  identifier = "${var.project}-${var.environment}-users"
  engine     = "postgres"
  engine_version = "16.1"

  instance_class        = var.db_instance_class
  allocated_storage     = var.db_allocated_storage
  max_allocated_storage = var.db_max_allocated_storage

  db_name  = "users"
  username = var.db_username
  password = var.db_password

  multi_az               = var.environment == "prod" ? true : false
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  storage_encrypted = true
  skip_final_snapshot = var.environment != "prod"

  tags = var.common_tags
}

# --- DynamoDB ---
resource "aws_dynamodb_table" "chat_messages" {
  name         = "${var.project}-${var.environment}-chat-messages"
  billing_mode = var.environment == "prod" ? "PROVISIONED" : "PAY_PER_REQUEST"

  hash_key  = "pk"
  range_key = "sk"

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  attribute {
    name = "gsi1pk"
    type = "S"
  }

  attribute {
    name = "gsi1sk"
    type = "S"
  }

  global_secondary_index {
    name            = "sender-index"
    hash_key        = "gsi1pk"
    range_key       = "gsi1sk"
    projection_type = "ALL"
  }

  point_in_time_recovery {
    enabled = var.environment == "prod"
  }

  tags = var.common_tags
}
```

### messaging モジュール

```hcl
# modules/messaging/main.tf

# --- SNS Topics ---
resource "aws_sns_topic" "message_events" {
  name = "${var.project}-${var.environment}-message-events"
  tags = var.common_tags
}

resource "aws_sns_topic" "media_events" {
  name = "${var.project}-${var.environment}-media-events"
  tags = var.common_tags
}

# --- SQS Queues ---
resource "aws_sqs_queue" "notification_queue" {
  name = "${var.project}-${var.environment}-notification-queue"

  visibility_timeout_seconds = 60
  message_retention_seconds  = 86400  # 1日

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.notification_dlq.arn
    maxReceiveCount     = 3
  })

  tags = var.common_tags
}

resource "aws_sqs_queue" "notification_dlq" {
  name = "${var.project}-${var.environment}-notification-dlq"
  message_retention_seconds = 1209600  # 14日
  tags = var.common_tags
}

resource "aws_sqs_queue" "realtime_queue" {
  name = "${var.project}-${var.environment}-realtime-queue"

  visibility_timeout_seconds = 30
  message_retention_seconds  = 3600  # 1時間

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.realtime_dlq.arn
    maxReceiveCount     = 3
  })

  tags = var.common_tags
}

resource "aws_sqs_queue" "realtime_dlq" {
  name = "${var.project}-${var.environment}-realtime-dlq"
  message_retention_seconds = 1209600
  tags = var.common_tags
}

# --- SNS → SQS Subscriptions ---
resource "aws_sns_topic_subscription" "message_to_notification" {
  topic_arn = aws_sns_topic.message_events.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.notification_queue.arn
}

resource "aws_sns_topic_subscription" "message_to_realtime" {
  topic_arn = aws_sns_topic.message_events.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.realtime_queue.arn
}
```

### auth モジュール

```hcl
# modules/auth/main.tf

resource "aws_cognito_user_pool" "main" {
  name = "${var.project}-${var.environment}-users"

  username_attributes      = ["email"]
  auto_verified_attributes = ["email"]

  password_policy {
    minimum_length    = 8
    require_uppercase = true
    require_lowercase = true
    require_numbers   = true
    require_symbols   = false
  }

  schema {
    name                = "email"
    attribute_data_type = "String"
    required            = true
    mutable             = true
  }

  tags = var.common_tags
}

resource "aws_cognito_user_pool_client" "app" {
  name         = "${var.project}-${var.environment}-app"
  user_pool_id = aws_cognito_user_pool.main.id

  generate_secret = false

  explicit_auth_flows = [
    "ALLOW_USER_SRP_AUTH",
    "ALLOW_REFRESH_TOKEN_AUTH",
  ]

  access_token_validity  = 1   # 1時間
  id_token_validity      = 1
  refresh_token_validity = 30  # 30日
}
```

---

## 環境分離

### environments/dev/main.tf

```hcl
terraform {
  required_version = ">= 1.7.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

locals {
  project     = "chat-app"
  environment = "dev"
  common_tags = {
    Project     = local.project
    Environment = local.environment
    ManagedBy   = "terraform"
  }
}

module "networking" {
  source = "../../modules/networking"

  project             = local.project
  environment         = local.environment
  vpc_cidr            = "10.0.0.0/16"
  availability_zones  = ["ap-northeast-1a", "ap-northeast-1c"]
  private_subnet_cidrs = ["10.0.1.0/24", "10.0.2.0/24"]
  public_subnet_cidrs  = ["10.0.101.0/24", "10.0.102.0/24"]
  cluster_name        = "${local.project}-${local.environment}"
  common_tags         = local.common_tags
}

module "eks" {
  source = "../../modules/eks"

  cluster_name        = "${local.project}-${local.environment}"
  environment         = local.environment
  vpc_id              = module.networking.vpc_id
  private_subnet_ids  = module.networking.private_subnet_ids
  node_instance_types = ["t3.medium"]
  node_min_size       = 1
  node_max_size       = 3
  node_desired_size   = 2
  common_tags         = local.common_tags
}

module "database" {
  source = "../../modules/database"

  project              = local.project
  environment          = local.environment
  db_instance_class    = "db.t3.micro"
  db_allocated_storage = 20
  db_max_allocated_storage = 50
  db_username          = "chatapp"
  db_password          = var.db_password  # tfvars or Secrets Manager
  subnet_ids           = module.networking.private_subnet_ids
  vpc_id               = module.networking.vpc_id
  common_tags          = local.common_tags
}

module "messaging" {
  source = "../../modules/messaging"

  project     = local.project
  environment = local.environment
  common_tags = local.common_tags
}

module "storage" {
  source = "../../modules/storage"

  project     = local.project
  environment = local.environment
  common_tags = local.common_tags
}

module "auth" {
  source = "../../modules/auth"

  project     = local.project
  environment = local.environment
  common_tags = local.common_tags
}

module "cache" {
  source = "../../modules/cache"

  project          = local.project
  environment      = local.environment
  node_type        = "cache.t3.micro"
  num_cache_nodes  = 1
  subnet_ids       = module.networking.private_subnet_ids
  vpc_id           = module.networking.vpc_id
  common_tags      = local.common_tags
}
```

### 環境別パラメータ比較

| パラメータ | dev | staging | prod |
|-----------|-----|---------|------|
| EKS ノードタイプ | t3.medium | t3.large | t3.xlarge |
| EKS ノード数 | 1-3 | 2-5 | 3-10 |
| RDS インスタンス | db.t3.micro | db.t3.small | db.r6g.large |
| RDS Multi-AZ | false | false | true |
| Redis ノードタイプ | cache.t3.micro | cache.t3.small | cache.r6g.large |
| Redis ノード数 | 1 | 1 | 3 (クラスター) |
| NAT Gateway | 1 (single) | 1 | 2 (AZ ごと) |
| DynamoDB | On-Demand | On-Demand | Provisioned + Auto Scaling |
| バックアップ | なし | 日次 | 日次 + PITR |

---

## S3 バックエンド + DynamoDB ロック

### バックエンド初期セットアップ

```hcl
# global/s3-backend/main.tf

resource "aws_s3_bucket" "terraform_state" {
  bucket = "chat-app-terraform-state"

  lifecycle {
    prevent_destroy = true
  }

  tags = {
    Project   = "chat-app"
    ManagedBy = "terraform"
  }
}

resource "aws_s3_bucket_versioning" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "aws:kms"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_dynamodb_table" "terraform_locks" {
  name         = "chat-app-terraform-locks"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  tags = {
    Project   = "chat-app"
    ManagedBy = "terraform"
  }
}
```

### バックエンド設定（各環境）

```hcl
# environments/dev/backend.tf

terraform {
  backend "s3" {
    bucket         = "chat-app-terraform-state"
    key            = "dev/terraform.tfstate"
    region         = "ap-northeast-1"
    dynamodb_table = "chat-app-terraform-locks"
    encrypt        = true
  }
}
```

---

## Terraform ワークフロー

```
1. terraform init        # プロバイダー・モジュールの初期化
2. terraform plan        # 変更内容のプレビュー
3. terraform apply       # インフラの作成・更新
4. terraform destroy     # インフラの削除（dev のみ）
```

### CI/CD での実行

```yaml
# .github/workflows/terraform.yml のイメージ
# PR 作成時: terraform plan を実行しコメントに投稿
# main マージ時: terraform apply を自動実行
```

## 関連ドキュメント

- [AWS サービス一覧](../aws/services.md)
- [Kubernetes アーキテクチャ](../kubernetes/architecture.md)
- [ディレクトリ構成](../architecture/directory-structure.md)
