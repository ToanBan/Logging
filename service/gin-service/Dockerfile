FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o gin-service .

FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/gin-service .

EXPOSE 3003

CMD ["./gin-service"]
