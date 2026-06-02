# GoBup Architecture

[中文](architecture.md) | [README](../README.en.md)

## Components

GoBup has four main layers:

- Web frontend: `web/`, Vue 3, Vite, Element Plus.
- HTTP backend: `server/`, Gin routes, controllers, and services.
- Data layer: GORM + SQLite. The database defaults to `data/gobup.db` or `/app/data/gobup.db` in Docker.
- External tools: FFmpeg, ffprobe, DanmakuFactory, Bilibili HTTP APIs, and WxPusher.

Production builds embed `web/dist` into the Go binary. Development mode can run frontend and backend separately.

## Layout

```text
server/internal/controllers  HTTP controllers
server/internal/routes       API routes and embedded frontend assets
server/internal/models       GORM models
server/internal/services     scanning, file ops, danmaku, video processing, sync jobs
server/internal/upload       upload queues, progress, publishing flow
server/internal/bili         Bilibili clients, uploaders, rate limiters
web/src/views                pages
web/src/components           reusable components
```

## Data Model

Important tables:

- `record_rooms`: room settings, publish templates, upload strategy, file operation policy, danmaku burn settings.
- `record_histories`: one recording or livestream session.
- `record_history_parts`: parts, split files, danmaku-burned files, and upload state.
- `bili_bili_users`: admin account and Bilibili accounts, including Cookie, access key, and expiry time.
- `system_configs`: scan, file watcher, data repair, and proxy settings.

The project supports deleting the SQLite database and initializing a fresh schema. Current initialization uses GORM AutoMigrate plus only necessary compatibility adjustments.

## Scanning And Watching

`FileScanService` is the single import path. It:

- Traverses the work path and custom scan paths.
- Checks extension, file size, file age, duplicate records, and temporary file names.
- Creates histories and parts.

`FileWatcherService` uses fsnotify to watch the work path and custom directories. It does not write the database directly. It debounces video file events and triggers `ScanAndImport`. Scheduled scans remain as a fallback for Docker mounts, network storage, and missed OS events.

## Upload Pipeline

Manual uploads, auto uploads, queue status, and WebSocket progress use one shared `upload.Service` instance.

Flow:

1. A scheduled job or API calls `UploadPart`.
2. The room strategy selects an upload account: fixed, round-robin, or shortest queue.
3. Upload window and rate-limit cooldown are checked.
4. Large files are split when the part count would exceed the upload limit.
5. Optional FFmpeg pre-upload transcoding runs.
6. A Bilibili client is created for the selected account and uploads the file.
7. CID, upload line, retry counters, and error messages are saved.
8. Danmaku burn-in, file operations, and auto-publish may run.

Clients are not reused across accounts or tasks, which prevents Cookie, access token, or request-context leakage.

## Publish And Covers

Publishing lives in `server/internal/upload/publish.go`. Cover modes:

- Default cover.
- Live room cover.
- Video frame cover.
- Manual cover.

Frame extraction uses FFmpeg and writes a `.cover.jpg` next to the source video. Live cover can fall back to frame extraction when configured.

## Danmaku Processing

`DanmakuBurnService` wraps DanmakuFactory and FFmpeg:

- Locate the XML danmaku file.
- Convert XML to ASS with DanmakuFactory.
- Apply room style settings and video resolution.
- Burn ASS into MP4 with FFmpeg.
- Create a `danmaku_burn` temporary part and enqueue it for upload.

Burn failures are logged and the original upload continues. A review-backfill task can later detect missing burned parts and append them.

## File Operations

`FileMoverService` can delete, move, or copy files at these triggers:

- After part upload.
- Before publish when all parts are uploaded.
- After successful publish.
- After review approval.

Scope is a bitmask: video, danmaku, cover. Delayed operations write `scheduled_delete_at` and are consumed by scheduled jobs.

## WebSocket And Queue APIs

Endpoints:

- `/api/queue/upload/status`: upload queue summary, pending, running, and completed lists.
- `/api/ws/progress`: upload progress snapshots.
- `/api/history/export`: CSV export with current filters.

The Dashboard shows upload queue state. The History page uses WebSocket updates with polling fallback.

## Build And CI

- `make build`: frontend plus non-embedded backend.
- `make build-embed`: production binary with embedded frontend.
- Dockerfiles use Node.js 24 and Go 1.24.
- GitHub Actions opt into Node.js 24 with `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true`.

## Operations

- Back up `/app/data/gobup.db` regularly.
- Make sure the recorder and GoBup container see the same recording directory.
- Test move or copy policies before enabling deletion.
- Multi-account load balancing is useful for upload distribution. Auto-publish should still use a fixed publishing account.
