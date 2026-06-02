# Gobup All-in-One Container - Frontend Embedded Build

# Stage 1: Build Frontend
FROM node:24-slim AS frontend-builder
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
RUN apk add --no-cache git ca-certificates

# Copy backend source
COPY server/ ./server/

# Copy frontend dist to embed location
COPY --from=frontend-builder /app/web/dist ./server/internal/routes/dist

# Build backend with embed tag
WORKDIR /app/server
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -tags embed -ldflags "-w -s" -o gobup .

# Stage 3: Final Runtime Image
FROM alpine:latest
ARG TARGETARCH

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata sqlite ffmpeg wget unzip fontconfig font-wqy-zenhei

# Install DanmakuFactory (如果可用)
# 注意: DanmakuFactory 需要 .NET runtime，如果需要使用请取消注释以下行
# RUN apk add --no-cache dotnet8-runtime
# WORKDIR /tmp
# RUN ARCH=$([ "$TARGETARCH" = "amd64" ] && echo "x64" || echo "$TARGETARCH") && \
#     wget -q https://github.com/hihkm/DanmakuFactory/releases/latest/download/DanmakuFactory-linux-${ARCH}.zip || true && \
#     if [ -f DanmakuFactory-linux-${ARCH}.zip ]; then \
#         unzip -q DanmakuFactory-linux-${ARCH}.zip -d /usr/local/bin/danmakufactory && \
#         chmod +x /usr/local/bin/danmakufactory/DanmakuFactory && \
#         rm DanmakuFactory-linux-${ARCH}.zip; \
#     fi

ENV TZ=Asia/Shanghai
ENV DANMAKU_FACTORY_PATH=/usr/local/bin/danmakufactory/DanmakuFactory
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
CMD ["/bin/sh", "-c", "/app/gobup -port 12380 -work-path /rec -data-path /app/data -username \"${USERNAME:-}\" -password \"${PASSWORD:-}\""]
