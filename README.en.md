# GoBup - Bilibili Recording Upload Manager

[中文](README.md) | [Architecture](docs/architecture.en.md) | [中文架构](docs/architecture.md)

[![Build and Release](https://github.com/spiritlhl/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhl/gobup/actions/workflows/build_docker.yml)

GoBup is a Go + Vue web application for managing local Bilibili livestream recordings. It imports files produced by recorders such as BililiveRecorder or blrec, uploads them to Bilibili, publishes videos, processes danmaku, and manages local files after upload.

## Features

- Automatic recording directory scan with fsnotify event triggers and scheduled fallback scans.
- Multiple Bilibili accounts with Cookie expiry display and enable/disable controls.
- Per-account upload queues with fixed account, round-robin, shortest-queue, and daily remaining-quota strategies.
- Upload time windows, retry, rate-limit backoff, and restart recovery.
- Optional FFmpeg pre-upload transcoding with H.264/H.265, preset, CRF, max width, audio bitrate settings, and History-page progress.
- Cover extraction from the Nth second of a video.
- Auto-publish, history filters, CSV export, and reconnecting WebSocket upload progress.
- A Rust Agent can be installed as an upload publisher, recording-file checker, or both, with controller, GitHub, and CDN download sources.
- DanmakuFactory + FFmpeg danmaku burn-in with default, compact, and large styles, font size/color/display-area settings, and stage progress. Burn failures do not block the original upload.
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
$EDITOR .env  # set GOBUP_PASSWORD to a strong password
docker compose up -d
```

Mounts:

- `/rec`: recording directory shared with your recorder.
- `/app/data`: GoBup data directory. The default SQLite database is `/app/data/gobup.db`.

## Rust Agent

Use the left-side "Agent Management" page to add a remote Agent. In the common case you only fill in the remote host IP/domain, then the panel generates the install command. All Agents use the same controller panel token by default.

- `upload`: receive controller `/agent/v1/publish` requests and forward them to the local GoBup service on the Agent machine.
- `filescan`: scan the Agent machine's recording directory and return file counts, total size, and sample files.
- `both`: enable both upload publishing and recording-file checking.

After saving the Agent, generate the install command on the "Agent Management" page. The controller source downloads `/agent/install-agent.sh` and `/agent/releases/gobup-agent-linux-*.tar.gz` from the current GoBup server; if release archives are not embedded, the controller redirects to GitHub Releases. The CDN source prefers `agentCdnBaseUrl` or built-in CDN mirrors for GitHub release assets.

## Build From Source

Requirements:

- Go 1.25+
- Node.js 24+
- Rust stable when building the Agent
- FFmpeg and ffprobe
- Optional: DanmakuFactory

Build frontend and backend:

```bash
make build
```

Build Rust Agent packages:

```bash
AGENT_TARGETS="x86_64-unknown-linux-musl aarch64-unknown-linux-musl" ./scripts/build_agent.sh
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
- Upload queue tasks can be paused, resumed, cancelled, retried, and inspected from the details dialog. Paused or cancelled states are persisted and skipped by automatic scheduling; failures are classified as network, rate-limit, auth, file, transcode, window, user, or unknown errors.
- Upload request timeouts are configurable through environment variables. When Bilibili returns 429, retries prefer the server's `Retry-After` hint.
- Load-balancing chooses the upload account when a task is queued. The upload itself always uses the chosen account's own Cookie; the daily quota strategy estimates remaining quota from today's uploaded parts and current queue length.
- For auto-publish, configure a fixed publishing account for the room to avoid account ownership mismatches.
- Danmaku burn-in requires `DANMAKU_FACTORY_PATH`, `ffmpeg`, and a usable font directory.
- Config export redacts account Cookies, access keys, refresh tokens, and WxPusher tokens by default; include account secrets only when creating a full account backup.

## Environment

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
GOBUP_AGENT_UPSTREAM_BASE_URL=https://panel.example.com
DANMAKU_FACTORY_PATH=/usr/local/bin/danmakufactory/DanmakuFactory
DANMAKU_FONT_NAME="WenQuanYi Zen Hei"
DANMAKU_FONTS_DIR=/usr/share/fonts
```

See [.env.example](.env.example) for a fuller template.

## Import Existing History

```bash
python3 import_brec_history_db.py --dir /path/to/bilirecord --db /app/data/gobup.db --container-prefix /rec
```

The script is aligned with the current Go service schema, including `duration`, `c_id`, upload pause/cancel fields, and host-to-container path mapping.

## More Docs

- [Architecture](docs/architecture.en.md)
- [中文架构文档](docs/architecture.md)
- [中文 README](README.md)

## FAQ

**How do I get Cookies?**
Prefer QR login on the Users page. Manual Cookie login is for existing Cookies; the server validates them before saving.

**What should I do when upload fails?**
Check the latest error in the Dashboard upload queue details. Network errors, 429 rate limits, and upload-window closures are retried or delayed automatically; expired Cookies require refresh or re-login on the Users page.

**Why does a queued task not upload outside the upload window?**
When upload windows are enabled, tasks submitted outside the window stay in the queue flow and are retried when the next window starts.

**Does danmaku burn failure block the original video?**
No. Burn failures only record error and progress state; the original video upload continues.

**Does config export include account secrets?**
Not by default. User exports redact Cookies, access keys, refresh tokens, and WxPusher tokens unless account secrets are explicitly included for a full account backup.

## Credits

- [FQrabbit/biliupforjava](https://github.com/FQrabbit/biliupforjava)
- [mwxmmy/biliupforjava](https://github.com/mwxmmy/biliupforjava)
- [BililiveRecorder](https://rec.danmuji.org/)
- [blrec](https://github.com/acgnhiki/blrec)
- [DanmakuFactory](https://github.com/hihkm/DanmakuFactory)
- [bilibili-API-collect](https://github.com/AkagiYui/bilibili-API-collect)
