.PHONY: proto-gen proto-lint test build tidy

proto-gen:
	cd proto && buf generate

proto-lint:
	cd proto && buf lint

test:
	go test ./pkg/... ./gen/go/... ./services/user-service/... ./services/chat-service/...

build:
	go build ./pkg/... ./gen/go/... ./services/user-service/... ./services/chat-service/...

tidy:
	cd gen/go && go mod tidy
	cd pkg && go mod tidy
	cd services/user-service && go mod tidy
	cd services/chat-service && go mod tidy
