# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS builder
WORKDIR /src

ENV GOWORK=off

COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/chat-service ./services/chat-service

WORKDIR /src/services/chat-service
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /app/chat-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/chat-service /app/chat-service
EXPOSE 50052
USER nonroot:nonroot
ENTRYPOINT ["/app/chat-service"]
