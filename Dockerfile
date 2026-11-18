# syntax=docker/dockerfile:1

# Stage 0: Build the Svelte frontend bundle (embedded via go:embed)
FROM node:20-alpine AS frontend

WORKDIR /web
COPY package.json package-lock.json ./
COPY web/frontend/package.json web/frontend/package-lock.json ./web/frontend/
RUN npm ci && npm --prefix web/frontend ci

COPY web/frontend/ web/frontend/
COPY scripts/ scripts/
COPY tailwind.config.* postcss.config.* ./ 2>/dev/null || true
COPY web/ web/
RUN npm run build:svelte

# Stage 1: Build the static pure-Go binary
FROM golang:alpine AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Overlay prebuilt frontend bundle (go:embed dist/*)
COPY --from=frontend /web/web/dist/ /src/web/dist/

# Build statically-linked single binary
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /bin/sqlfox \
    ./cmd/sqlfox

# Stage 2: Ultra-lightweight secure runtime
FROM alpine:3.20

# Install TLS root certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root system user and data volume mount point
RUN addgroup -S -g 10001 sqlfox && \
    adduser -S -u 10001 -G sqlfox -h /home/sqlfox -s /bin/sh sqlfox && \
    mkdir -p /data && \
    chown -R sqlfox:sqlfox /data /home/sqlfox

# Copy standalone binary
COPY --from=builder /bin/sqlfox /usr/local/bin/sqlfox

USER sqlfox
WORKDIR /home/sqlfox

# Persistent volume for vault config.db, master.key, and backups
VOLUME ["/data"]

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/sqlfox"]
CMD ["-host", "0.0.0.0", "-no-browser", "-db", "/data/config.db"]
