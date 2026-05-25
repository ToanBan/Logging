FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY service/fiber-service/go.mod service/fiber-service/go.sum ./
COPY shared/logger/go /shared/logger/go

RUN go mod download

COPY service/fiber-service/main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o fiber-service .

FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/fiber-service .

EXPOSE 3004

CMD ["./fiber-service"]