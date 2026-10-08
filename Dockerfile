# Stage 1: Build Go Backend
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o gostore ./cmd/server

# Stage 2: Final Minimal Runtime Image (~20MB)
FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-builder /app/gostore /app/gostore

# Default environment
ENV PORT=8080
ENV HOST=0.0.0.0
ENV STORAGE_PATH=/app/data/storage
ENV DATABASE_PATH=/app/data/gostore.db
ENV MASTER_API_KEY=gostore-master-secret-key

VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["/app/gostore"]
