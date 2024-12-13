FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g cmd/api/main.go

RUN CGO_ENABLED=0 GOOS=linux go build -o /docktrine-api ./cmd/api

FROM alpine:latest

WORKDIR /app

COPY --from=builder /docktrine-api /app/docktrine-api
COPY --from=builder /app/docs /app/docs
COPY config.json /app/config.json

EXPOSE 3000

VOLUME ["/var/run/docker.sock", "/app/config"]

ENV CONFIG_PATH=/app/config/config.json

CMD ["/app/docktrine-api"]
