# Build stage
FROM golang:alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/keenetic_bot ./cmd/bot

# Runtime stage
FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/keenetic_bot /app/keenetic_bot

VOLUME ["/app/data"]

ENTRYPOINT ["/app/keenetic_bot", "-config", "/app/data/config.yaml"]
