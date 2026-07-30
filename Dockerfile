FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w \
           -X github.com/baditaflorin/go-common/version.Tag=$(git describe --tags --always 2>/dev/null || echo dev) \
           -X github.com/baditaflorin/go-common/version.GitCommit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) \
           -X github.com/baditaflorin/go-common/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o /out/go_tos_finder .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget tini \
 && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=builder /out/go_tos_finder /app/go_tos_finder
COPY --from=builder /app/service.yaml /app/service.yaml

USER app

EXPOSE 8316

HEALTHCHECK --interval=60s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8316/health >/dev/null || exit 1

ENTRYPOINT ["/sbin/tini","--"]
CMD ["/app/go_tos_finder"]
