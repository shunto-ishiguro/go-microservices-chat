.PHONY: proto-gen proto-lint test build tidy run-realtime image-build-all

IMAGE_TAG ?= 0.1.0

proto-gen:
	cd proto && buf generate

proto-lint:
	cd proto && buf lint

test:
	go test ./pkg/... ./gen/go/... ./services/user-service/... ./services/chat-service/... ./services/realtime-service/...

build:
	go build ./pkg/... ./gen/go/... ./services/user-service/... ./services/chat-service/... ./services/realtime-service/...

tidy:
	cd gen/go && go mod tidy
	cd pkg && go mod tidy
	cd services/user-service && go mod tidy
	cd services/chat-service && go mod tidy
	cd services/realtime-service && go mod tidy

# realtime-service を手元で起動するためのショートカット (Phase 2 step 8 用)。
# Redis と chat-service が別途立ち上がっている前提。HTTP_ADDR を変えて 2 プロセス起動できる。
run-realtime:
	REDIS_ADDR=$${REDIS_ADDR:-localhost:6379} \
	CHAT_SERVICE_ADDR=$${CHAT_SERVICE_ADDR:-localhost:50052} \
	HTTP_ADDR=$${HTTP_ADDR:-:8081} \
	go run ./services/realtime-service/cmd/server

image-build-all:
	docker build -t user-service:$(IMAGE_TAG)     -f services/user-service/Dockerfile .
	docker build -t chat-service:$(IMAGE_TAG)     -f services/chat-service/Dockerfile .
	docker build -t realtime-service:$(IMAGE_TAG) -f services/realtime-service/Dockerfile .
