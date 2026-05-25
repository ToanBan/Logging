FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY service/gin-service/go.mod service/gin-service/go.sum ./
COPY shared/logger/go /shared/logger/go

COPY shared/logger/go/go.mod /shared/logger/go/go.mod
COPY shared/logger/go/go.sum /shared/logger/go/go.sum
COPY shared/logger/go/logger.go /shared/logger/go/logger.go

RUN go mod download

COPY service/gin-service/main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o gin-service .

FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/gin-service .

EXPOSE 3003

CMD ["./gin-service"]