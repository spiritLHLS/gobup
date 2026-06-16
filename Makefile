.PHONY: help build build-embed build-cross dev-frontend dev-backend docker-build check-docker clean

PACKAGE_MANAGER ?= $(shell if [ -f web/pnpm-lock.yaml ] && command -v pnpm >/dev/null 2>&1; then echo pnpm; elif [ -f web/yarn.lock ] && command -v yarn >/dev/null 2>&1; then echo yarn; else echo npm; fi)

ifeq ($(PACKAGE_MANAGER),pnpm)
FRONTEND_INSTALL = pnpm install --frozen-lockfile
FRONTEND_RUN = pnpm
else ifeq ($(PACKAGE_MANAGER),yarn)
FRONTEND_INSTALL = yarn install --frozen-lockfile
FRONTEND_RUN = yarn
else
FRONTEND_INSTALL = npm ci
FRONTEND_RUN = npm run
endif

help:
	@echo "GoBup Makefile"
	@echo ""
	@echo "可用命令:"
	@echo "  make build          - 构建前端和后端（非嵌入模式，用于开发）"
	@echo "  make build-embed    - 构建带嵌入前端的二进制文件（生产环境）"
	@echo "  make build-cross    - 构建 5 个跨平台嵌入式二进制"
	@echo "  make dev-frontend   - 启动前端开发服务器"
	@echo "  make dev-backend    - 启动后端开发服务器"
	@echo "  make docker-build   - 构建 Docker 镜像"
	@echo "  make clean          - 清理构建文件"

# 构建前端
build-frontend:
	@echo "构建前端..."
	cd web && $(FRONTEND_INSTALL) && $(FRONTEND_RUN) build

# 构建后端（非嵌入模式）
build-backend:
	@echo "构建后端（非嵌入模式）..."
	@mkdir -p bin
	cd server && go build -o ../bin/gobup .

# 构建后端（嵌入模式）
build-backend-embed: build-frontend
	@echo "复制前端dist到routes目录..."
	@mkdir -p server/internal/routes/dist
	@cp -r web/dist/* server/internal/routes/dist/
	@echo "构建后端（嵌入模式）..."
	@mkdir -p bin
	cd server && go build -tags embed -o ../bin/gobup-embed .
	@echo "清理临时文件..."
	@rm -rf server/internal/routes/dist

# 完整构建（非嵌入）
build: build-frontend build-backend
	@echo "构建完成！"
	@echo "前端: web/dist/"
	@echo "后端: bin/gobup"

# 完整构建（嵌入模式，生产环境）
build-embed: build-backend-embed
	@echo "嵌入式构建完成！"
	@echo "二进制文件: bin/gobup-embed"

# 跨平台嵌入式构建
build-cross: build-frontend
	@echo "复制前端dist到routes目录..."
	@mkdir -p server/internal/routes/dist
	@cp -r web/dist/* server/internal/routes/dist/
	@echo "构建跨平台二进制..."
	@mkdir -p build
	cd server && \
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed -ldflags="-s -w" -o ../build/gobup-server-linux-amd64 . && \
		CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags embed -ldflags="-s -w" -o ../build/gobup-server-linux-arm64 . && \
		CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -tags embed -ldflags="-s -w" -o ../build/gobup-server-darwin-amd64 . && \
		CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags embed -ldflags="-s -w" -o ../build/gobup-server-darwin-arm64 . && \
		CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags embed -ldflags="-s -w" -o ../build/gobup-server-windows-amd64.exe .
	@rm -rf server/internal/routes/dist
	@echo "跨平台构建完成: build/"

# 前端开发服务器
dev-frontend:
	@echo "启动前端开发服务器..."
	cd web && $(FRONTEND_RUN) dev

# 后端开发服务器
dev-backend:
	@echo "启动后端开发服务器..."
	cd server && go run main.go -port 12380

# 构建 Docker 镜像
check-docker:
	@docker info >/dev/null 2>&1 || (echo "Docker daemon 不可用，请先启动 Docker 后重试。" && exit 1)

docker-build: check-docker
	@echo "构建 Docker 镜像..."
	docker build -t gobup:latest .

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf web/dist
	@rm -rf bin
	@rm -rf server/internal/routes/dist
	@echo "清理完成！"
