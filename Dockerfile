# Gobup All-in-One Container - Frontend Embedded Build

# Stage 1: Build Frontend
FROM node:20-slim AS frontend-builder
ARG TARGETARCH
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci --include=optional
RUN if [ "$TARGETARCH" = "amd64" ]; then \
        npm install --no-save @rollup/rollup-linux-x64-gnu; \
    elif [ "$TARGETARCH" = "arm64" ]; then \
        npm install --no-save @rollup/rollup-linux-arm64-gnu; \
    fi
COPY web/ ./
RUN npm run build

# Stage 2: Build Backend with Embedded Frontend
FROM golang:1.24-alpine AS backend-builder
ARG TARGETARCH
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates build-base sqlite-dev

# Copy backend source
COPY server/ ./server/

# Copy frontend dist to embed location
COPY --from=frontend-builder /app/web/dist ./server/internal/routes/dist

# Build backend with embed tag
WORKDIR /app/server
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux GOARCH=${TARGETARCH} go build -tags embed -a -installsuffix cgo -ldflags "-w -s" -o gobup .

# Stage 3: Final Runtime Image
FROM alpine:latest
ARG TARGETARCH

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata sqlite ffmpeg

ENV TZ=Asia/Shanghai
WORKDIR /app

# Create necessary directories
RUN mkdir -p /rec /app/data

# Copy binary with embedded frontend
COPY --from=backend-builder /app/server/gobup ./gobup

# Set permissions
RUN chmod 755 /app/gobup

EXPOSE 12380

VOLUME ["/rec", "/app/data"]

# USERNAME 和 PASSWORD 环境变量将在运行时传递给程序
CMD ["/bin/sh", "-c", "/app/gobup -port 12380 -work-path /rec -username \"${USERNAME:-}\" -password \"${PASSWORD:-}\""]
