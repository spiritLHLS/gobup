# GoBup - B站录播自动上传工具

[English](README.en.md) | [架构文档](docs/architecture.md) | [Architecture](docs/architecture.en.md)

[![Build and Release](https://github.com/spiritlhl/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml)

GoBup 是一个 Go + Vue 实现的 B 站录播管理工具。它面向录播姬、blrec 等录制工具产生的本地视频文件，提供自动入库、上传、投稿、弹幕处理、文件清理、多账号管理和 Web 控制台。

## 功能概览

- 录播目录自动扫描，并支持目录事件监听触发即时扫盘。
- 多 B 站账号管理，账号列表显示 Cookie 有效期。
- 上传队列按账号隔离，支持固定账号、轮询、最短队列策略。
- 上传时间窗口、失败重试、速率限制退避和重启恢复。
- 可选上传前 FFmpeg 转码压缩，可设置 preset、CRF、宽度和音频码率。
- 可从视频第 N 秒自动截取封面。
- 自动投稿、历史筛选、CSV 导出和 WebSocket 上传进度推送。
- DanmakuFactory + FFmpeg 弹幕烧录，支持 default、compact、large 样式；烧录失败不会阻塞原始视频上传。
- 文件操作策略：按分P上传后、投稿前、投稿后、审核后删除、移动或复制视频、弹幕、封面文件。
- Docker amd64/arm64 部署，内置前端静态资源。

## 快速部署

推荐使用 DanmakuFactory 版本镜像：

```bash
docker run -d \
  --name gobup \
  -p 22380:12380 \
  -v /root/bilirecord:/rec \
  -v /root/gobup-data:/app/data \
  -e USERNAME=admin \
  -e PASSWORD=your_secure_password \
  --restart unless-stopped \
  spiritlhl/gobup:latest
```

访问 `http://服务器IP:22380`。`USERNAME` 和 `PASSWORD` 只在首次初始化数据库时创建管理员账号，后续修改环境变量不会覆盖已有账号。

Docker Compose：

```bash
cp .env.example .env
docker compose up -d
```

挂载约定：

- `/rec`：录播文件目录，必须和录播软件输出目录对应。
- `/app/data`：GoBup 数据目录，默认数据库文件为 `/app/data/gobup.db`。

## 源码运行

依赖：

- Go 1.24+
- Node.js 24+
- FFmpeg 和 ffprobe
- 可选：DanmakuFactory

构建：

```bash
make build
```

生产单二进制构建：

```bash
make build-embed
./bin/gobup-embed -port 12380 -work-path /path/to/recordings -data-path ./data -username admin -password your_password
```

开发：

```bash
make dev-backend
make dev-frontend
```

## 基本流程

1. 录播工具把视频、XML 弹幕和封面写入录制目录。
2. GoBup 通过 fsnotify 事件和定时任务扫描录制目录，满足大小、年龄、重复检测后写入 SQLite。
3. 自动上传任务按房间配置把待上传分P加入账号队列。
4. 上传服务为每个任务创建独立 B 站客户端，避免账号 Cookie/Token 串扰。
5. 可选转码、封面截帧、弹幕烧录和文件切分在上传流程中执行。
6. 分P全部上传后按房间配置自动投稿。
7. 投稿或审核完成后按文件操作策略清理、移动或复制本地文件。

## 配置提示

- 工作目录在控制面板中配置，Docker 默认是 `/rec`。
- 自定义扫描目录可配置多个路径，用逗号分隔。
- 文件最小年龄用于避免导入正在写入的文件；目录事件监听只触发扫盘，不绕过安全检查。
- 上传时间窗口支持跨天，例如 `23:00` 到 `06:00`。
- 负载均衡策略会在入队时选择账号，实际上传仍使用选定账号的独立 Cookie。
- 自动投稿仍建议为房间配置固定投稿账号，避免不同账号上传资源与投稿账号不一致。
- 弹幕烧录依赖 `DANMAKU_FACTORY_PATH`、`ffmpeg` 和字体目录。

## 常用环境变量

```bash
USERNAME=admin
PASSWORD=your_secure_password
TZ=Asia/Shanghai
DANMAKU_FACTORY_PATH=/usr/local/bin/danmakufactory/DanmakuFactory
DANMAKU_FONT_NAME="WenQuanYi Zen Hei"
DANMAKU_FONTS_DIR=/usr/share/fonts
```

更多示例见 [.env.example](.env.example)。

## 历史导入

从 brec 历史数据库导入：

```bash
python3 import_brec_history_db.py /path/to/brec.db /app/data/gobup.db
```

导入脚本会兼容当前 Go 服务的 `c_id` 字段结构。

## 文档

- [中文架构文档](docs/architecture.md)
- [English Architecture](docs/architecture.en.md)
- [English README](README.en.md)

## 致谢

- [FQrabbit/biliupforjava](https://github.com/FQrabbit/biliupforjava)
- [mwxmmy/biliupforjava](https://github.com/mwxmmy/biliupforjava)
- [BililiveRecorder](https://rec.danmuji.org/)
- [blrec](https://github.com/acgnhiki/blrec)
- [DanmakuFactory](https://github.com/hihkm/DanmakuFactory)
- [bilibili-API-collect](https://github.com/AkagiYui/bilibili-API-collect)
