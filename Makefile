.PHONY: help build build-embed dev-frontend dev-backend docker-build clean

help:
	@echo "GoBup Makefile"
	@echo ""
	@echo "可用命令:"
	@echo "  make build          - 构建前端和后端（非嵌入模式，用于开发）"
	@echo "  make build-embed    - 构建带嵌入前端的二进制文件（生产环境）"
	@echo "  make dev-frontend   - 启动前端开发服务器"
	@echo "  make dev-backend    - 启动后端开发服务器"
	@echo "  make docker-build   - 构建 Docker 镜像"
	@echo "  make clean          - 清理构建文件"

# 构建前端
build-frontend:
	@echo "构建前端..."
	cd web && npm install && npm run build

# 构建后端（非嵌入模式）
build-backend:
	@echo "构建后端（非嵌入模式）..."
	cd server && go build -o ../bin/gobup .

# 构建后端（嵌入模式）
build-backend-embed: build-frontend
	@echo "复制前端dist到routes目录..."
	@mkdir -p server/internal/routes/dist
	@cp -r web/dist/* server/internal/routes/dist/
	@echo "构建后端（嵌入模式）..."
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

# 前端开发服务器
dev-frontend:
	@echo "启动前端开发服务器..."
	cd web && npm run dev

# 后端开发服务器
dev-backend:
	@echo "启动后端开发服务器..."
	cd server && go run main.go -port 12380

# 构建 Docker 镜像
docker-build:
	@echo "构建 Docker 镜像..."
	docker build -t gobup:latest .

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf web/dist
	@rm -rf bin
	@rm -rf server/internal/routes/dist
	@echo "清理完成！"
