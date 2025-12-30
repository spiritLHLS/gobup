# Gobup All-in-One Container

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

FROM golang:1.24-alpine AS backend-builder
ARG TARGETARCH
WORKDIR /app/server
RUN apk add --no-cache git ca-certificates build-base sqlite-dev
COPY server/ ./
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux GOARCH=${TARGETARCH} go build -a -installsuffix cgo -ldflags "-w -s" -o main .

FROM alpine:latest
ARG TARGETARCH

# Install required packages
RUN apk add --no-cache ca-certificates tzdata sqlite nginx supervisor

ENV TZ=Asia/Shanghai
WORKDIR /app

# Create necessary directories
RUN mkdir -p /var/log/nginx /var/log/supervisor \
    && mkdir -p /rec /app/data /var/run

# Copy backend binary
COPY --from=backend-builder /app/server/main ./gobup

# Copy frontend files
COPY --from=frontend-builder /app/web/dist /var/www/html

# Copy nginx configuration
COPY web/nginx.conf /etc/nginx/http.d/default.conf

# Set permissions
RUN chmod 755 /app/gobup && \
    chown -R nginx:nginx /var/www/html && \
    chmod -R 755 /var/www/html

# Create supervisor configuration
RUN echo '[supervisord]' > /etc/supervisord.conf && \
    echo 'nodaemon=true' >> /etc/supervisord.conf && \
    echo 'user=root' >> /etc/supervisord.conf && \
    echo 'logfile=/var/log/supervisor/supervisord.log' >> /etc/supervisord.conf && \
    echo 'pidfile=/var/run/supervisord.pid' >> /etc/supervisord.conf && \
    echo '' >> /etc/supervisord.conf && \
    echo '[program:gobup]' >> /etc/supervisord.conf && \
    echo 'command=/app/gobup -port 12380 -work-path /rec' >> /etc/supervisord.conf && \
    echo 'directory=/app' >> /etc/supervisord.conf && \
    echo 'autostart=true' >> /etc/supervisord.conf && \
    echo 'autorestart=true' >> /etc/supervisord.conf && \
    echo 'stdout_logfile=/var/log/supervisor/gobup.log' >> /etc/supervisord.conf && \
    echo 'stderr_logfile=/var/log/supervisor/gobup_error.log' >> /etc/supervisord.conf && \
    echo '' >> /etc/supervisord.conf && \
    echo '[program:nginx]' >> /etc/supervisord.conf && \
    echo 'command=/usr/sbin/nginx -g "daemon off;"' >> /etc/supervisord.conf && \
    echo 'autostart=true' >> /etc/supervisord.conf && \
    echo 'autorestart=true' >> /etc/supervisord.conf && \
    echo 'stdout_logfile=/var/log/supervisor/nginx.log' >> /etc/supervisord.conf && \
    echo 'stderr_logfile=/var/log/supervisor/nginx_error.log' >> /etc/supervisord.conf

EXPOSE 80 12380

VOLUME ["/rec", "/app/data"]

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
