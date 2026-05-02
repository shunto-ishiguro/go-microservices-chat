# syntax=docker/dockerfile:1.7
FROM golang:1.22-alpine AS builder
WORKDIR /src

ENV GOWORK=off

COPY gen/go ./gen/go
COPY pkg ./pkg
COPY services/realtime-service ./services/realtime-service

WORKDIR /src/services/realtime-service
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /app/realtime-service ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/realtime-service /app/realtime-service
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/app/realtime-service"]
