module go-microservices-chat/services/realtime-service

go 1.22.2

require (
	github.com/coder/websocket v1.8.12
	github.com/redis/go-redis/v9 v9.7.0
	go-microservices-chat/gen/go v0.0.0
	go-microservices-chat/pkg v0.0.0
	google.golang.org/grpc v1.66.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240604185151-ef581f913117 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240604185151-ef581f913117 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

replace (
	go-microservices-chat/gen/go => ../../gen/go
	go-microservices-chat/pkg => ../../pkg
)
