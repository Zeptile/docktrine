FROM --platform=linux/amd64 node:22-alpine AS web-builder

WORKDIR /web

COPY web/package.json web/package-lock.json ./

RUN npm ci

COPY web/ ./

RUN npm run build

FROM --platform=linux/amd64 golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g cmd/api/main.go

RUN CGO_ENABLED=1 GOOS=linux go build -o /docktrine-api ./cmd/api

FROM --platform=linux/amd64 alpine:3.18

RUN apk add --no-cache sqlite-libs libc6-compat

WORKDIR /app

COPY --from=builder /docktrine-api /app/docktrine-api
COPY --from=builder /app/docs /app/docs
COPY --from=web-builder /web/dist /app/web/dist
RUN chmod +x /app/docktrine-api

RUN mkdir -p /app/data

EXPOSE 3000

VOLUME ["/var/run/docker.sock", "/app/data"]

ENV CONFIG_PATH=/app/data
ENV SQLITE_PATH=/app/data/docktrine.db

CMD ["/app/docktrine-api"]
