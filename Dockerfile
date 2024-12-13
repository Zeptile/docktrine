FROM --platform=linux/amd64 golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g cmd/api/main.go

RUN CGO_ENABLED=1 GOOS=linux go build -o /docktrine-api ./cmd/api

FROM alpine:latest

RUN apk add --no-cache sqlite-libs

WORKDIR /app

COPY --from=builder /docktrine-api /app/docktrine-api
COPY --from=builder /app/docs /app/docs

RUN mkdir -p /app/data

EXPOSE 3000

VOLUME ["/var/run/docker.sock", "/app/data"]

ENV CONFIG_PATH=/app/data
ENV SQLITE_PATH=/app/data/docktrine.db

CMD ["/app/docktrine-api"]
