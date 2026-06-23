# GoBup - B站录播自动上传工具

[English](README.en.md) | [架构文档](docs/architecture.md) | [Architecture](docs/architecture.en.md)

[![Build and Release](https://github.com/spiritlhl/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml)

GoBup 是一个 Go + Vue 实现的 B 站录播管理工具。它面向录播姬、blrec 等录制工具产生的本地视频文件，提供自动入库、上传、投稿、弹幕处理、文件清理、多账号管理和 Web 控制台。

## 功能概览

- 录播目录自动扫描，并支持目录事件监听触发即时扫盘。
- 多 B 站账号管理，账号列表显示 Cookie 有效期，并支持启用/禁用账号。
- 上传队列按账号隔离，支持固定账号、轮询、最短队列和每日剩余配额策略。
- 上传时间窗口、失败重试、速率限制退避和重启恢复。
- 可选上传前 FFmpeg 转码压缩，可设置 H.264/H.265、preset、CRF、宽度和音频码率，并在历史页展示转码进度。
- 可从视频第 N 秒自动截取封面。
- 自动投稿、历史筛选、CSV 导出和支持断线重连的 WebSocket 上传进度推送。
- Rust Agent 可独立安装为上传投稿端、录制文件检查端或两者兼用，并支持控制端/GitHub/CDN 下载来源。
- DanmakuFactory + FFmpeg 弹幕烧录，支持 default、compact、large 样式、字号/颜色/显示区域配置和阶段进度；烧录失败不会阻塞原始视频上传。
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
$EDITOR .env  # 设置 GOBUP_PASSWORD 为强密码
docker compose up -d
```

挂载约定：

- `/rec`：录播文件目录，必须和录播软件输出目录对应。
- `/app/data`：GoBup 数据目录，默认数据库文件为 `/app/data/gobup.db`。

## Rust Agent

控制面板的「投稿与 Agent」区域可以选择 Agent 用途：

- `upload`：作为远程投稿端，接收控制端 `/agent/v1/publish` 请求，并转发给 Agent 所在机器的本地 GoBup 服务执行投稿。
- `filescan`：作为录制文件检查端，扫描 Agent 所在机器的录制目录并返回文件数量、总大小和样本列表。
- `both`：同时启用上传投稿和录制文件检查能力。

保存 Agent Token、用途和来源后，在面板生成安装命令。控制端来源会下载当前 GoBup 服务托管的 `/agent/install-agent.sh` 和 `/agent/releases/gobup-agent-linux-*.tar.gz`；如果控制端未内嵌 release 包，会回退到 GitHub Releases。CDN 来源会优先按 `agentCdnBaseUrl` 或内置 CDN 镜像下载 GitHub release 资源。

## 源码运行

依赖：

- Go 1.25+
- Node.js 24+
- Rust stable（仅构建 Agent 时需要）
- FFmpeg 和 ffprobe
- 可选：DanmakuFactory

构建：

```bash
make build
```

构建 Rust Agent 分发包：

```bash
AGENT_TARGETS="x86_64-unknown-linux-musl aarch64-unknown-linux-musl" ./scripts/build_agent.sh
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
- 上传队列支持暂停、恢复、取消、重试和详情查看，暂停/取消状态会持久化，自动调度会跳过这些任务；上传失败会记录网络、限流、鉴权、文件、转码、窗口等错误分类。
- 上传请求超时可通过环境变量调整；B 站返回 429 时会优先按 `Retry-After` 提示退避。
- 负载均衡策略会在入队时选择账号，实际上传仍使用选定账号的独立 Cookie；每日配额策略会按账号当日已上传分P和当前队列长度估算剩余额度。
- 自动投稿仍建议为房间配置固定投稿账号，避免不同账号上传资源与投稿账号不一致。
- 弹幕烧录依赖 `DANMAKU_FACTORY_PATH`、`ffmpeg` 和字体目录。
- 配置导出默认会脱敏账号 Cookie、access key、refresh token 和 WxPusher token；只有显式选择包含账号密钥时才用于完整账号备份。

## 常用环境变量

```bash
USERNAME=admin
PASSWORD=your_secure_password
GOBUP_USERNAME=admin
GOBUP_PASSWORD=your_secure_password
TZ=Asia/Shanghai
GOBUP_UPLOAD_TIMEOUT_MINUTES=30
GOBUP_TLS_HANDSHAKE_TIMEOUT_SECONDS=30
GOBUP_ALLOWED_ORIGINS=https://panel.example.com
BILI_APP_KEY=replace_with_bilibili_app_key
BILI_APP_SECRET=replace_with_bilibili_app_secret
GOBUP_AGENT_TOKEN=replace_with_agent_token
GOBUP_AGENT_PURPOSE=both
GOBUP_AGENT_LISTEN=0.0.0.0:12381
GOBUP_AGENT_WORK_PATH=/rec
GOBUP_AGENT_UPSTREAM_BASE_URL=http://127.0.0.1:12380
DANMAKU_FACTORY_PATH=/usr/local/bin/danmakufactory/DanmakuFactory
DANMAKU_FONT_NAME="WenQuanYi Zen Hei"
DANMAKU_FONTS_DIR=/usr/share/fonts
```

更多示例见 [.env.example](.env.example)。

## 历史导入

从 BililiveRecorder 录制目录导入：

```bash
python3 import_brec_history_db.py --dir /path/to/bilirecord --db /app/data/gobup.db --container-prefix /rec
```

导入脚本会兼容当前 Go 服务的 `duration`、`c_id`、上传暂停/取消字段结构，并把宿主机录制路径映射为容器内路径。

## 文档

- [中文架构文档](docs/architecture.md)
- [English Architecture](docs/architecture.en.md)
- [English README](README.en.md)

## FAQ

**Cookie 怎么获取？**
优先在用户管理页使用扫码登录。手动 Cookie 登录仅用于已有 Cookie 的场景，提交后服务端会验证有效性。

**上传失败怎么办？**
先查看 Dashboard 上传队列详情里的最近错误。网络、429 限流和窗口外任务会自动退避或重新入队；Cookie 失效需要在用户管理页刷新或重新登录。

**为什么任务在上传窗口外没有立即上传？**
启用上传窗口后，窗口外提交的分P会留在队列体系内，并按下一个窗口开始时间延迟重试。

**弹幕烧录失败会影响原视频吗？**
不会。弹幕烧录失败只记录错误和进度状态，原始视频上传流程会继续。

**配置导出会包含账号密钥吗？**
默认不会。用户信息导出会脱敏 Cookie、access key、refresh token 和 WxPusher token；只有显式选择包含账号密钥时才导出完整账号备份。

## 致谢

- [FQrabbit/biliupforjava](https://github.com/FQrabbit/biliupforjava)
- [mwxmmy/biliupforjava](https://github.com/mwxmmy/biliupforjava)
- [BililiveRecorder](https://rec.danmuji.org/)
- [blrec](https://github.com/acgnhiki/blrec)
- [DanmakuFactory](https://github.com/hihkm/DanmakuFactory)
- [bilibili-API-collect](https://github.com/AkagiYui/bilibili-API-collect)
