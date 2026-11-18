# Build for the gateway with:
#   docker build --platform linux/amd64 -t docktrine .
#
# The UI is platform independent, so it builds natively; the API uses cgo
# (SQLite) and builds for the target platform.

FROM --platform=$BUILDPLATFORM node:22-alpine AS web-builder

WORKDIR /web

COPY web/package.json web/package-lock.json ./

RUN npm ci

COPY web/ ./

RUN npm run build

FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

# Same swag version as go.mod, so generated docs match the runtime
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4

COPY . .

RUN swag init -g cmd/api/main.go

RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /docktrine-api ./cmd/api

# Same Alpine release as the builder, so the cgo binary's musl matches
FROM alpine:3.22

# docker CLI + compose plugin: compose deploys shell out to `docker compose`
# openssh-client: ssh:// nodes, for both the Docker API and compose
# tini: the API would otherwise be PID 1 and never reap the ssh and docker
#   processes it starts once they exit
RUN apk add --no-cache \
    sqlite-libs \
    ca-certificates \
    docker-cli \
    docker-cli-compose \
    openssh-client \
    tini

WORKDIR /app

COPY --from=builder /docktrine-api /app/docktrine-api
COPY --from=web-builder /web/dist /app/web/dist
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh && mkdir -p /app/data

EXPOSE 3000

# Holds the database, the secrets master key (secret.key, unless
# DOCKTRINE_SECRET_KEY is set) and the first API key. Back it up.
VOLUME ["/app/data"]

ENV CONFIG_PATH=/app/data

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -q -O /dev/null http://127.0.0.1:${PORT:-3000}/healthz || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["/app/docktrine-api"]
