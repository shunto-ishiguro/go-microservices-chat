# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS builder
WORKDIR /src

# go.work を使うと他サービスの module も要求されるので、
# go.mod の replace ディレクティブだけでビルドする (GOWORK=off)。
ENV GOWORK=off

COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/user-service ./services/user-service

WORKDIR /src/services/user-service
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /app/user-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/user-service /app/user-service
EXPOSE 50051 8082
USER nonroot:nonroot
ENTRYPOINT ["/app/user-service"]
