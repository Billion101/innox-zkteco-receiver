# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/server ./cmd/server

# Final Lightweight Runtime Image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Vientiane

WORKDIR /app
COPY --from=builder /app/server /app/server

EXPOSE 8087

HEALTHCHECK --interval=15s --timeout=5s --retries=3 CMD wget -q -O- http://localhost:8087/healthz || exit 1

ENTRYPOINT ["/app/server"]
