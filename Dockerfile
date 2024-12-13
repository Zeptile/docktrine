FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g cmd/api/main.go

RUN CGO_ENABLED=0 GOOS=linux go build -o /docktrine-api ./cmd/api

FROM alpine:latest

COPY --from=builder /docktrine-api /docktrine-api
COPY --from=builder /app/docs /docs

EXPOSE 3000

VOLUME ["/var/run/docker.sock"]

CMD ["/docktrine-api"]
