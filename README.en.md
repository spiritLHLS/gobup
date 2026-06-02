# GoBup - Bilibili Recording Upload Manager

[中文](README.md) | [Architecture](docs/architecture.en.md) | [中文架构](docs/architecture.md)

[![Build and Release](https://github.com/spiritlhl/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml)

GoBup is a Go + Vue web application for managing local Bilibili livestream recordings. It imports files produced by recorders such as BililiveRecorder or blrec, uploads them to Bilibili, publishes videos, processes danmaku, and manages local files after upload.

## Features

- Automatic recording directory scan with fsnotify event triggers and scheduled fallback scans.
- Multiple Bilibili accounts with Cookie expiry display.
- Per-account upload queues with fixed account, round-robin, and shortest-queue strategies.
- Upload time windows, retry, rate-limit backoff, and restart recovery.
- Optional FFmpeg pre-upload transcoding with preset, CRF, max width, and audio bitrate settings.
- Cover extraction from the Nth second of a video.
- Auto-publish, history filters, CSV export, and WebSocket upload progress.
- DanmakuFactory + FFmpeg danmaku burn-in with default, compact, and large styles. Burn failures do not block the original upload.
- File operation policies after part upload, before publish, after publish, or after review: delete, move, or copy videos, danmaku files, and covers.
- Docker deployment for amd64 and arm64 with embedded frontend assets.

## Quick Start

The DanmakuFactory image is recommended:

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

Open `http://SERVER_IP:22380`. `USERNAME` and `PASSWORD` only create the initial admin account when the database is first initialized.

Docker Compose:

```bash
cp .env.example .env
docker compose up -d
```

Mounts:

- `/rec`: recording directory shared with your recorder.
- `/app/data`: GoBup data directory. The default SQLite database is `/app/data/gobup.db`.

## Build From Source

Requirements:

- Go 1.24+
- Node.js 24+
- FFmpeg and ffprobe
- Optional: DanmakuFactory

Build frontend and backend:

```bash
make build
```

Build a production binary with embedded frontend:

```bash
make build-embed
./bin/gobup-embed -port 12380 -work-path /path/to/recordings -data-path ./data -username admin -password your_password
```

Development:

```bash
make dev-backend
make dev-frontend
```

## How It Works

1. A recorder writes video, XML danmaku, and cover files into the recording directory.
2. GoBup scans directories through fsnotify events and scheduled jobs. File size, age, and duplicate checks are still enforced.
3. Auto-upload adds eligible parts to account queues according to room settings.
4. Each upload task creates an isolated Bilibili client for the selected account, preventing Cookie or Token context leakage.
5. Optional transcoding, cover extraction, danmaku burn-in, and large-file splitting happen inside the upload pipeline.
6. When all parts are uploaded, the room can auto-publish the video.
7. File operation policies run after upload, publish, or review.

## Configuration Notes

- The work path is configured in the dashboard. Docker defaults to `/rec`.
- Custom scan paths accept comma-separated paths.
- File watcher events trigger a scan but do not bypass minimum file age checks.
- Upload windows support overnight ranges, for example `23:00` to `06:00`.
- Load-balancing chooses the upload account when a task is queued. The upload itself always uses the chosen account's own Cookie.
- For auto-publish, configure a fixed publishing account for the room to avoid account ownership mismatches.
- Danmaku burn-in requires `DANMAKU_FACTORY_PATH`, `ffmpeg`, and a usable font directory.

## Environment

```bash
USERNAME=admin
PASSWORD=your_secure_password
TZ=Asia/Shanghai
DANMAKU_FACTORY_PATH=/usr/local/bin/danmakufactory/DanmakuFactory
DANMAKU_FONT_NAME="WenQuanYi Zen Hei"
DANMAKU_FONTS_DIR=/usr/share/fonts
```

See [.env.example](.env.example) for a fuller template.

## Import Existing History

```bash
python3 import_brec_history_db.py /path/to/brec.db /app/data/gobup.db
```

The script is aligned with the current Go service schema and uses the `c_id` column.

## More Docs

- [Architecture](docs/architecture.en.md)
- [中文架构文档](docs/architecture.md)
- [中文 README](README.md)

## Credits

- [FQrabbit/biliupforjava](https://github.com/FQrabbit/biliupforjava)
- [mwxmmy/biliupforjava](https://github.com/mwxmmy/biliupforjava)
- [BililiveRecorder](https://rec.danmuji.org/)
- [blrec](https://github.com/acgnhiki/blrec)
- [DanmakuFactory](https://github.com/hihkm/DanmakuFactory)
- [bilibili-API-collect](https://github.com/AkagiYui/bilibili-API-collect)
