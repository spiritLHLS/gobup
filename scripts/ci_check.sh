#!/usr/bin/env bash
set -uo pipefail

ERRORS=0

log_pass() { printf '\033[32m[PASS]\033[0m %s\n' "$*"; }
log_fail() { printf '\033[31m[FAIL]\033[0m %s\n' "$*"; ERRORS=$((ERRORS + 1)); }

check_workflows() {
  echo "=== GitHub Actions ==="
  local files=(.github/workflows/*.yml)
  for file in "${files[@]}"; do
    [[ -f "$file" ]] || continue
    grep -q 'FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true' "$file" \
      && log_pass "$file uses Node 24 actions runtime" \
      || log_fail "$file missing FORCE_JAVASCRIPT_ACTIONS_TO_NODE24"
    grep -q '^concurrency:' "$file" \
      && log_pass "$file has workflow concurrency" \
      || log_fail "$file missing workflow concurrency"
    grep -q 'timeout-minutes:' "$file" \
      && log_pass "$file has job timeouts" \
      || log_fail "$file missing timeout-minutes"
  done

  if grep -RE 'actions/(checkout|setup-go|setup-node|setup-python|upload-artifact)@v[1-5]\b|docker/(setup-buildx-action|login-action|build-push-action)@v[1-3]\b' .github/workflows/*.yml >/dev/null 2>&1; then
    log_fail "workflow contains an outdated core action version"
  else
    log_pass "core action versions are upgraded"
  fi

  grep -q "go-version: '1.25'" .github/workflows/main.yml \
    && log_pass "CI uses Go 1.25" \
    || log_fail "CI Go version is not aligned with go.mod"
}

check_security() {
  echo
  echo "=== Security Hygiene ==="
  grep -q '\*.env' .gitignore && grep -q '\*.db' .gitignore && grep -q 'screenshots/' .gitignore \
    && log_pass ".gitignore excludes env/db/screenshot artifacts" \
    || log_fail ".gitignore is missing env/db/screenshot exclusions"

  if grep -R '\${GOBUP_PASSWORD:-admin}\|\${PASSWORD:-admin}' docker-compose*.yml >/dev/null 2>&1; then
    log_fail "docker compose still has a default admin password"
  else
    log_pass "docker compose requires an explicit admin password"
  fi

  [[ ! -f s5proxy.sh ]] && log_pass "s5proxy.sh is absent" || log_fail "s5proxy.sh still exists"

  if grep -R '提取到的Cookie\|解析登录URL:' server/internal >/dev/null 2>&1; then
    log_fail "sensitive Cookie data may be logged"
  else
    log_pass "Cookie logging is redacted"
  fi

  if grep -R '"user":[[:space:]]*user\|"user": user' server/internal/controllers --include='*.go' >/dev/null 2>&1; then
    log_fail "Bilibili user API responses may expose raw credentials"
  else
    log_pass "Bilibili user API responses are sanitized"
  fi

  grep -q 'IncludeSecrets' server/internal/controllers/config.go && grep -q 'userListRedacted' server/internal/controllers/config.go \
    && log_pass "config export redacts account credentials by default" \
    || log_fail "config export credential redaction is missing"

  if grep -R '原始响应:' server/internal/bili/auth.go >/dev/null 2>&1; then
    log_fail "Bilibili auth raw API responses may be logged"
  else
    log_pass "Bilibili auth raw API responses are not logged"
  fi

  if grep -R 'AppSecret[[:space:]]*=' server/internal --include='*.go' >/dev/null 2>&1; then
    log_fail "Bilibili AppSecret is hardcoded"
  else
    log_pass "Bilibili AppSecret is loaded from environment"
  fi

  if grep -R 'CheckOrigin: func' server/internal/controllers/websocket.go >/dev/null 2>&1 && ! grep -R 'return true // 允许所有来源' server/internal/controllers/websocket.go >/dev/null 2>&1; then
    log_pass "WebSocket origin check is restricted"
  else
    log_fail "WebSocket origin check is not restricted"
  fi
}

check_compose() {
  echo
  echo "=== Docker Compose ==="
  grep -q './bilirecord:/rec' docker-compose.yml && grep -q './bilirecord:/rec' docker-compose.danmaku.yml \
    && log_pass "compose files mount /rec consistently" \
    || log_fail "compose /rec mounts are inconsistent"
  grep -q './data:/app/data' docker-compose.yml && grep -q './data:/app/data' docker-compose.danmaku.yml \
    && log_pass "compose files mount /app/data consistently" \
    || log_fail "compose /app/data mounts are inconsistent"
  grep -q '22380:12380' docker-compose.yml && grep -q '22380:12380' docker-compose.danmaku.yml \
    && log_pass "compose files expose 22380:12380 consistently" \
    || log_fail "compose ports are inconsistent"
  grep -q 'Dockerfile.danmaku' docker-compose.danmaku.yml \
    && log_pass "danmaku compose references Dockerfile.danmaku" \
    || log_fail "danmaku compose does not reference Dockerfile.danmaku"
  grep -q 'golang:1.25-alpine' Dockerfile && grep -q 'golang:1.25-alpine' Dockerfile.danmaku && grep -q 'golang:1.25-alpine' server/Dockerfile \
    && log_pass "Dockerfiles use Go 1.25 builders" \
    || log_fail "Dockerfile Go builders are not aligned with go.mod"
}

check_api_alignment() {
  echo
  echo "=== API Alignment ==="
  grep -q "target: 'http://127.0.0.1:12380'" web/vite.config.js \
    && log_pass "Vite dev proxy targets the Go backend port" \
    || log_fail "Vite dev proxy target is not aligned with the Go backend"
  if grep -q 'rewrite:.*api' web/vite.config.js; then
    log_fail "Vite dev proxy strips /api prefix"
  else
    log_pass "Vite dev proxy preserves /api prefix"
  fi
  grep -R 'api.GET("/health"' server/internal/routes >/dev/null 2>&1 \
    && log_pass "backend exposes /api/health for Docker healthcheck" \
    || log_fail "backend is missing /api/health"

  grep -R 'Retry-After' server/internal/bili >/dev/null 2>&1 \
    && log_pass "upload retry path handles Retry-After" \
    || log_fail "upload retry path does not handle Retry-After"

  grep -q 'UploadErrorType' server/internal/models/models.go && grep -q 'classifyUploadError' server/internal/upload/error_classification.go && grep -q 'uploadErrorType' web/src/components/dashboard/UploadQueueCard.vue \
    && log_pass "upload failures are classified for queue and history diagnostics" \
    || log_fail "upload failure classification is incomplete"

  grep -q 'GOBUP_UPLOAD_TIMEOUT_MINUTES' server/internal/bili/client.go && grep -q 'GOBUP_UPLOAD_TIMEOUT_MINUTES' .env.example \
    && log_pass "upload timeout is environment-configurable" \
    || log_fail "upload timeout environment configuration is missing"

  grep -q 'GOBUP_ALLOWED_ORIGINS' server/internal/controllers/websocket.go && grep -q 'GOBUP_ALLOWED_ORIGINS' .env.example \
    && log_pass "WebSocket allowed origins are environment-configurable" \
    || log_fail "WebSocket allowed origins configuration is missing"

  grep -q 'BILI_APP_KEY' server/internal/bili/auth.go && grep -q 'BILI_APP_SECRET' .env.example \
    && log_pass "Bilibili signing credentials are environment-configurable" \
    || log_fail "Bilibili signing credential configuration is missing"

  grep -q 'IssueWebSocketToken' server/internal/routes/routes.go && grep -q '/api/progress/ws-token' web/src/composables/useHistory.js \
    && log_pass "WebSocket progress uses authenticated short-lived tokens" \
    || log_fail "WebSocket progress token wiring is missing"

  grep -q 'StateTranscoding' server/internal/upload/progress.go && grep -q 'TRANSCODING' web/src/composables/useHistory.js \
    && log_pass "transcoding progress is exposed to the frontend" \
    || log_fail "transcoding progress frontend/backend alignment is missing"

  grep -q 'SetRetryCallback' server/internal/bili/upload_upos.go && grep -q 'MarkRetryWait' server/internal/upload/service.go && grep -q 'RETRY_WAIT' web/src/components/history/PartsDialog.vue \
    && log_pass "UPOS retry waits are visible in the upload progress stream" \
    || log_fail "UPOS retry-wait progress visibility is incomplete"

  grep -q 'TranscodeVideoCodec' server/internal/models/models.go && grep -q 'libx265' server/internal/services/video_processing.go && grep -q 'transcodeVideoCodec' web/src/components/rooms/tabs/VideoProcessingTab.vue && grep -q 'TestBuildTranscodeArgsSupportsH265MP4' server/internal/services/video_processing_test.go \
    && log_pass "pre-upload transcoding supports room-level H.264/H.265 configuration" \
    || log_fail "pre-upload transcoding codec configuration is incomplete"

  grep -q 'UploadWindowClosedError' server/internal/upload/helpers.go && grep -q 'requeueAfter' server/internal/upload/queue.go \
    && log_pass "upload window closures are delayed and re-queued" \
    || log_fail "upload window delayed requeue handling is missing"

  grep -q 'DailyUploadQuota' server/internal/models/models.go && grep -q 'daily_quota' server/internal/upload/helpers.go && grep -q 'daily_quota' web/src/components/rooms/tabs/BasicInfoTab.vue \
    && log_pass "daily quota upload account strategy is wired across model/backend/frontend" \
    || log_fail "daily quota upload account strategy wiring is incomplete"

  grep -q 'SetDanmakuTaskProgress' server/internal/progress/danmaku.go && grep -q 'setDanmakuBurnProgress' server/internal/services/danmaku_burn.go \
    && log_pass "danmaku burn progress is exposed through the progress API" \
    || log_fail "danmaku burn progress wiring is missing"

  grep -q 'DanmakuFontSize' server/internal/models/models.go && grep -q 'applyASSStyleOverrides' server/internal/services/danmaku_burn.go && grep -q 'danmakuFontSize' web/src/components/rooms/tabs/VideoProcessingTab.vue && grep -q 'TestApplyASSStyleOverrides' server/internal/services/danmaku_burn_test.go \
    && log_pass "danmaku burn style customization is wired across model/backend/frontend" \
    || log_fail "danmaku burn style customization is incomplete"

  grep -q 'isFileWatcherVideoChangeEvent' server/internal/services/file_watcher.go && grep -q 'TestIsFileWatcherVideoChangeEvent' server/internal/services/file_watcher_test.go \
    && log_pass "file watcher video event filtering is covered by tests" \
    || log_fail "file watcher event filtering tests are missing"
}

check_swagger() {
  echo
  echo "=== Swagger ==="

  grep -q 'github.com/gobup/server/docs' server/main.go \
    && log_pass "generated Swagger docs are imported by the server" \
    || log_fail "server does not import generated Swagger docs"

  grep -Fq 'auth.GET("/swagger/*any"' server/internal/routes/routes.go \
    && log_pass "Swagger UI is mounted under BasicAuth-protected API routes" \
    || log_fail "Swagger UI is not mounted under the authenticated API group"

  [[ -f server/docs/docs.go && -f server/docs/swagger.json && -f server/docs/swagger.yaml ]] \
    && log_pass "generated Swagger artifacts are present" \
    || log_fail "generated Swagger artifacts are missing"

  grep -q '"BasicAuth"' server/docs/swagger.json && grep -q '"/progress/ws-token"' server/docs/swagger.json \
    && log_pass "Swagger spec documents BasicAuth and progress token API" \
    || log_fail "Swagger spec is missing BasicAuth or progress token API"

  grep -q -- '--parseDependency --parseInternal' .github/workflows/main.yml \
    && log_pass "CI verifies Swagger generation with dependency/internal parsing" \
    || log_fail "CI does not verify Swagger generation with dependency/internal parsing"

  if node scripts/check_swagger_coverage.js >/tmp/gobup_swagger_coverage.log 2>&1; then
    log_pass "$(cat /tmp/gobup_swagger_coverage.log | head -n 1)"
  else
    cat /tmp/gobup_swagger_coverage.log
    log_fail "Swagger route coverage is below 90%"
  fi
}

check_code_structure() {
  echo
  echo "=== Code Structure ==="

  local file line_count
  for file in server/internal/services/filescan.go server/internal/upload/publish.go; do
    if [[ ! -f "$file" ]]; then
      log_fail "$file is missing"
      continue
    fi

    line_count=$(wc -l < "$file" | tr -d '[:space:]')
    if [[ "$line_count" -lt 1000 ]]; then
      log_pass "$file stays below 1000 lines ($line_count)"
    else
      log_fail "$file exceeds 1000 lines ($line_count)"
    fi
  done

  [[ -f server/internal/services/filescan_metadata.go && -f server/internal/upload/publish_burned_append.go ]] \
    && log_pass "large file responsibilities remain split into focused files" \
    || log_fail "large file split companion files are missing"
}

check_agent_distribution() {
  echo
  echo "=== Rust Agent Distribution ==="

  [[ -f server/agent/Cargo.toml && -f server/agent/src/main.rs ]] \
    && log_pass "Rust agent project is present" \
    || log_fail "Rust agent project is missing"

  [[ -x scripts/install_agent.sh && -x scripts/build_agent.sh ]] \
    && log_pass "agent install/build scripts are executable" \
    || log_fail "agent scripts are missing executable permissions"

  cmp -s scripts/install_agent.sh server/assets/agent/install_agent.sh \
    && log_pass "embedded agent installer matches scripts/install_agent.sh" \
    || log_fail "embedded agent installer is out of sync"

  grep -q 'DownloadAgentInstaller' server/internal/routes/routes.go && grep -q 'DownloadAgentRelease' server/internal/routes/routes.go \
    && log_pass "controller exposes public agent installer and release routes" \
    || log_fail "controller agent distribution routes are missing"

  grep -q 'AgentPurpose' server/internal/models/models.go && grep -q 'FileCheckMode' server/internal/models/models.go && grep -q 'agentPurpose' web/src/components/dashboard/PublishAgentConfig.vue \
    && log_pass "agent purpose and file-check mode are wired across backend/frontend" \
    || log_fail "agent purpose/file-check wiring is incomplete"

  grep -q 'gobup-agent-linux-amd64.tar.gz' .github/workflows/main.yml && grep -q 'AGENT_TARGETS=' .github/workflows/main.yml \
    && log_pass "release workflow builds and uploads Rust agent packages" \
    || log_fail "release workflow does not build Rust agent packages"

  grep -q 'x86_64-unknown-linux-musl' .github/workflows/main.yml && grep -q 'aarch64-unknown-linux-musl' .github/workflows/main.yml && grep -Fq 'libc\.so\.6' scripts/build_agent.sh \
    && log_pass "agent release packages are guarded against new glibc runtime requirements" \
    || log_fail "agent release packages are not guarded against glibc runtime drift"
}

check_docs() {
  echo
  echo "=== Documentation ==="
  local zh_headers en_headers
  zh_headers=$(grep -c '^## ' README.md 2>/dev/null || echo 0)
  en_headers=$(grep -c '^## ' README.en.md 2>/dev/null || echo 0)
  [[ "$zh_headers" -eq "$en_headers" ]] \
    && log_pass "README heading counts match ($zh_headers)" \
    || log_fail "README heading counts differ: zh=$zh_headers en=$en_headers"

  grep -q -- '--container-prefix' README.md && grep -q -- '--container-prefix' README.en.md \
    && log_pass "README documents import path mapping" \
    || log_fail "README import command is missing --container-prefix"

  grep -q 'upload_cancelled' import_brec_history_db.py && grep -q 'container-prefix' import_brec_history_db.py \
    && log_pass "Brec importer covers queue fields and path mapping" \
    || log_fail "Brec importer compatibility checks are incomplete"

  grep -q 'rateLimitCooldownAt' server/internal/controllers/queue.go && grep -q '任务详情' web/src/components/dashboard/UploadQueueCard.vue \
    && log_pass "upload queue exposes and renders task details" \
    || log_fail "upload queue task details are incomplete"

  grep -q '/api/progress/ws-token' docs/architecture.md && grep -q '/ws/progress' docs/architecture.md && grep -q '/api/progress/ws-token' docs/architecture.en.md && grep -q '/ws/progress' docs/architecture.en.md \
    && log_pass "architecture docs use the actual WebSocket route" \
    || log_fail "architecture docs contain a stale WebSocket route"

  grep -q '```mermaid' docs/architecture.md && grep -q '```mermaid' docs/architecture.en.md \
    && log_pass "architecture docs include Mermaid diagrams" \
    || log_fail "architecture docs are missing Mermaid diagrams"

  grep -q '^## FAQ' README.md && grep -q '^## FAQ' README.en.md && grep -q 'Cookie' README.md && grep -q 'upload fails' README.en.md \
    && log_pass "README FAQ sections are present in both languages" \
    || log_fail "README FAQ sections are incomplete"
}

check_workflows
check_security
check_compose
check_api_alignment
check_swagger
check_code_structure
check_agent_distribution
check_docs

echo
if [[ "$ERRORS" -eq 0 ]]; then
  log_pass "All local checks passed"
else
  log_fail "$ERRORS check(s) failed"
  exit 1
fi
