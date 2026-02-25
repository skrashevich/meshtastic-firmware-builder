# Multi-stage all-in-one Dockerfile for Meshtastic Firmware Builder
# Supports: linux/amd64, linux/arm64

# ============================================
# Stage 1: Backend Build (Go)
# ============================================
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend-builder

# Install build dependencies for cross-compilation
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Download dependencies first (layer caching)
COPY backend/go.mod ./
RUN go mod download && go mod tidy

# Copy backend source
COPY backend/ ./

# Build for target platform
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o /out/server ./cmd/server

# ============================================
# Stage 2: Frontend Build (Node + Vite)
# ============================================
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /app

# Install dependencies first (layer caching)
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci && \
    npm cache clean --force

# Copy frontend source
COPY frontend/ ./

# Build for production
ARG VITE_API_BASE_URL=http://localhost:8080
ARG VITE_API_BASE_URLS=
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
ENV VITE_API_BASE_URLS=$VITE_API_BASE_URLS

RUN npm run build

# ============================================
# Stage 3: Final All-in-One Image
# ============================================
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    nginx \
    supervisor \
    ca-certificates \
    git \
    docker-cli \
    curl

# Create non-privileged users
# Note: nginx user/group already created by apk add nginx
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Create directories
RUN mkdir -p \
    /app/build-workdir \
    /app/logs \
    /var/log/supervisor \
    /run/nginx \
    /var/lib/nginx/tmp && \
    chown -R appuser:appuser /app && \
    chown -R nginx:nginx /var/lib/nginx && \
    chown -R nginx:nginx /run/nginx

# Copy backend binary from backend-builder
COPY --from=backend-builder /out/server /usr/local/bin/server

# Copy frontend static files from frontend-builder
COPY --from=frontend-builder /app/dist /usr/share/nginx/html

# Copy nginx configuration (all-in-one specific: backend on localhost:8080)
COPY docker/all-in-one/nginx.conf /etc/nginx/nginx.conf

# Copy supervisord configuration
COPY docker/all-in-one/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Set permissions
RUN chmod +x /usr/local/bin/server && \
    chown -R appuser:appuser /usr/local/bin/server && \
    chown -R appuser:appuser /usr/share/nginx/html

# Expose ports
# 80: Nginx (frontend + API proxy)
# 8080: Backend (direct access, optional)
EXPOSE 80 8080

# Set environment variables with sensible defaults
ENV APP_PORT=8080 \
    APP_WORKDIR=/app/build-workdir \
    APP_CONCURRENT_BUILDS=1 \
    APP_RETENTION_HOURS=168 \
    APP_BUILD_TIMEOUT_MINUTES=90 \
    APP_BUILDER_IMAGE=meshtastic-pio-builder:latest \
    APP_PLATFORMIO_JOBS=1 \
    APP_ALLOWED_ORIGINS=http://localhost \
    APP_MAX_LOG_LINES=20000 \
    APP_BUILD_RATE_LIMIT_PER_MINUTE=10 \
    APP_REQUIRE_CAPTCHA=1 \
    DOCKER_HOST=unix:///var/run/docker.sock

# Use supervisord to run both nginx and backend
# supervisord runs as root to manage nginx, backend runs as appuser (see supervisord.conf)
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
