#!/bin/sh
# GoBup Agent Installer
# Usage:
#   curl -fsSL http://controller:12380/agent/install-agent.sh | sh -s -- \
#     --purpose upload --token <TOKEN> --source controller --controller-base-url http://controller:12380

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { printf "%b %s\n" "${BLUE}[INFO]${NC}" "$1"; }
log_success() { printf "%b %s\n" "${GREEN}[SUCCESS]${NC}" "$1"; }
log_warning() { printf "%b %s\n" "${YELLOW}[WARNING]${NC}" "$1"; }
log_error() { printf "%b %s\n" "${RED}[ERROR]${NC}" "$1" >&2; }

quote_env_value() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

PURPOSE="both"
TOKEN=""
LISTEN="0.0.0.0:12381"
WORK_PATH="/rec"
SOURCE="github"
CONTROLLER_BASE_URL=""
CDN_BASE_URL=""
UPSTREAM_BASE_URL="http://127.0.0.1:12380"
UPSTREAM_TOKEN=""
REPO="spiritlhls/gobup"
INSTALL_DIR="/opt/gobup/agent"
SERVICE_NAME="gobup-agent"
BIN_PATH="${INSTALL_DIR}/gobup-agent"
ENV_FILE="${INSTALL_DIR}/env"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
CDN_URLS="https://cdn0.spiritlhl.top https://cdn3.spiritlhl.net https://cdn1.spiritlhl.net https://cdn2.spiritlhl.net https://cdn.spiritlhl.net"
GITHUB_API_URLS="https://api.github.com https://githubapi.spiritlhl.workers.dev https://githubapi.spiritlhl.top"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --purpose) PURPOSE="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --listen) LISTEN="$2"; shift 2 ;;
    --work-path) WORK_PATH="$2"; shift 2 ;;
    --source|--agent-source) SOURCE="$2"; shift 2 ;;
    --controller-base-url) CONTROLLER_BASE_URL="$2"; shift 2 ;;
    --cdn-base-url) CDN_BASE_URL="$2"; shift 2 ;;
    --upstream-base-url) UPSTREAM_BASE_URL="$2"; shift 2 ;;
    --upstream-token) UPSTREAM_TOKEN="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    *) log_error "Unknown argument: $1"; exit 1 ;;
  esac
done

case "$PURPOSE" in
  upload|filescan|both) ;;
  *) log_error "Unsupported purpose: $PURPOSE. Use upload, filescan, or both."; exit 1 ;;
esac

case "$SOURCE" in
  controller|github|cdn) ;;
  *) log_error "Unsupported source: $SOURCE. Use controller, github, or cdn."; exit 1 ;;
esac

if [ -z "$TOKEN" ]; then
  log_error "--token is required"
  exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
  log_error "This script must be run as root."
  exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) SUFFIX="linux-amd64" ;;
  aarch64|arm64) SUFFIX="linux-arm64" ;;
  *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
esac

ARCHIVE_NAME="gobup-agent-${SUFFIX}.tar.gz"
TMP_ARCHIVE="/tmp/${ARCHIVE_NAME}"
TMP_EXTRACT="/tmp/gobup-agent-extract"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

install_deps() {
  missing=""
  for cmd in curl tar; do
    if ! need_cmd "$cmd"; then
      missing="$missing $cmd"
    fi
  done
  [ -z "$missing" ] && return 0

  log_warning "Missing required tools:$missing"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq
    apt-get install -y $missing
  elif command -v yum >/dev/null 2>&1; then
    yum install -y $missing
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y $missing
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache $missing
  else
    log_error "Cannot install missing tools automatically:$missing"
    exit 1
  fi
}

download_one() {
  url="$1"
  out="$2"
  log_info "Downloading: $url"
  if curl -fsSL --connect-timeout 20 --max-time 300 -o "$out" "$url" 2>/dev/null; then
    [ -s "$out" ] && return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    if wget -T 20 -t 3 -q -O "$out" "$url" 2>/dev/null; then
      [ -s "$out" ] && return 0
    fi
  fi
  rm -f "$out"
  return 1
}

latest_version() {
  if [ -n "${INSTALL_VERSION:-}" ]; then
    printf '%s' "$INSTALL_VERSION"
    return 0
  fi
  for api in $GITHUB_API_URLS; do
    response="$(curl -sL --connect-timeout 10 --max-time 30 "${api}/repos/${REPO}/releases/latest" 2>/dev/null || true)"
    version="$(printf '%s' "$response" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' | head -1)"
    if [ -n "$version" ] && [ "$version" != "null" ]; then
      printf '%s' "$version"
      return 0
    fi
  done
  return 1
}

download_archive() {
  version="$(latest_version || true)"
  if [ "$SOURCE" = "controller" ]; then
    if [ -z "$CONTROLLER_BASE_URL" ]; then
      log_error "--controller-base-url is required when source=controller"
      exit 1
    fi
    download_one "${CONTROLLER_BASE_URL%/}/agent/releases/${ARCHIVE_NAME}" "$TMP_ARCHIVE" && return 0
    log_warning "Controller source failed, falling back to GitHub release."
  fi

  if [ -z "$version" ]; then
    log_error "Cannot resolve latest release version"
    exit 1
  fi

  if [ "$SOURCE" = "cdn" ]; then
    if [ -n "$CDN_BASE_URL" ]; then
      download_one "${CDN_BASE_URL%/}/https://github.com/${REPO}/releases/download/${version}/${ARCHIVE_NAME}" "$TMP_ARCHIVE" && return 0
    fi
    for cdn in $CDN_URLS; do
      download_one "${cdn%/}/https://github.com/${REPO}/releases/download/${version}/${ARCHIVE_NAME}" "$TMP_ARCHIVE" && return 0
    done
    log_warning "CDN source failed, falling back to GitHub release."
  fi

  download_one "https://github.com/${REPO}/releases/download/${version}/${ARCHIVE_NAME}" "$TMP_ARCHIVE"
}

install_binary() {
  mkdir -p "$INSTALL_DIR"
  rm -rf "$TMP_EXTRACT"
  mkdir -p "$TMP_EXTRACT"
  tar -xzf "$TMP_ARCHIVE" -C "$TMP_EXTRACT"
  found="$(find "$TMP_EXTRACT" -type f -name 'gobup-agent*' | head -1)"
  if [ -z "$found" ] || [ ! -f "$found" ]; then
    log_error "Agent binary was not found in $ARCHIVE_NAME"
    exit 1
  fi
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
    systemctl stop "$SERVICE_NAME" || true
  fi
  mv -f "$found" "$BIN_PATH"
  chmod +x "$BIN_PATH"
  ln -sf "$BIN_PATH" /usr/local/bin/gobup-agent
  rm -rf "$TMP_EXTRACT" "$TMP_ARCHIVE"
  log_success "Installed gobup-agent to $BIN_PATH"
}

check_binary_compatibility() {
  [ "${ALLOW_GLIBC_AGENT:-0}" = "1" ] && return 0

  if command -v strings >/dev/null 2>&1; then
    if strings "$BIN_PATH" 2>/dev/null | grep -q 'GLIBC_[0-9]'; then
      log_error "Downloaded gobup-agent requires glibc. This package may fail on older Linux hosts."
      log_error "Use a newer musl/static gobup-agent release, or set ALLOW_GLIBC_AGENT=1 only if this host is known compatible."
      exit 1
    fi
  fi

  if command -v ldd >/dev/null 2>&1; then
    ldd_output="$(ldd "$BIN_PATH" 2>&1 || true)"
    if printf '%s' "$ldd_output" | grep -Eq 'GLIBC_[0-9.]+.*not found|version .* not found|required by'; then
      log_error "gobup-agent is not compatible with this host libc:"
      printf '%s\n' "$ldd_output" >&2
      log_error "Install a musl/static gobup-agent package and rerun this command."
      exit 1
    fi
  fi
}

write_env() {
  mkdir -p "$INSTALL_DIR"
  [ -z "$UPSTREAM_TOKEN" ] && UPSTREAM_TOKEN="$TOKEN"
  cat > "$ENV_FILE" <<EOF
GOBUP_AGENT_TOKEN=$(quote_env_value "$TOKEN")
GOBUP_AGENT_PURPOSE=$(quote_env_value "$PURPOSE")
GOBUP_AGENT_LISTEN=$(quote_env_value "$LISTEN")
GOBUP_AGENT_WORK_PATH=$(quote_env_value "$WORK_PATH")
GOBUP_AGENT_UPSTREAM_BASE_URL=$(quote_env_value "$UPSTREAM_BASE_URL")
GOBUP_AGENT_UPSTREAM_TOKEN=$(quote_env_value "$UPSTREAM_TOKEN")
GOBUP_AGENT_SOURCE=$(quote_env_value "$SOURCE")
GOBUP_AGENT_CONTROLLER_BASE_URL=$(quote_env_value "$CONTROLLER_BASE_URL")
GOBUP_AGENT_CDN_BASE_URL=$(quote_env_value "$CDN_BASE_URL")
EOF
  chmod 600 "$ENV_FILE"
  chown root:root "$ENV_FILE" 2>/dev/null || true
}

install_helper() {
  cat > /usr/local/bin/gobup-agentctl <<'EOF'
#!/bin/sh
SVC="gobup-agent"
case "${1:-status}" in
  status) systemctl status "$SVC" --no-pager 2>/dev/null || pgrep -a gobup-agent ;;
  start) systemctl start "$SVC" ;;
  stop) systemctl stop "$SVC" ;;
  restart) systemctl restart "$SVC" ;;
  log) journalctl -u "$SVC" -n 80 --no-pager ;;
  *) echo "Usage: gobup-agentctl {status|start|stop|restart|log}" ;;
esac
EOF
  chmod +x /usr/local/bin/gobup-agentctl
}

install_service() {
  write_env
  install_helper
  if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=GoBup Agent
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=-${ENV_FILE}
ExecStart=${BIN_PATH}
Restart=always
RestartSec=10
StartLimitInterval=120
StartLimitBurst=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    systemctl restart "$SERVICE_NAME"
    sleep 2
    if systemctl is-active --quiet "$SERVICE_NAME"; then
      log_success "GoBup Agent started."
      log_info "Manage with: gobup-agentctl {status|start|stop|restart|log}"
      return 0
    fi
    log_error "GoBup Agent failed to start. Check: journalctl -u ${SERVICE_NAME} -xe"
    journalctl -u "$SERVICE_NAME" -n 30 --no-pager 2>/dev/null || true
    exit 1
  fi

  log_warning "No supported service manager found, starting with nohup."
  nohup sh -c ". ${ENV_FILE} && exec ${BIN_PATH}" >/var/log/gobup-agent.log 2>&1 &
  log_success "GoBup Agent started with PID $!. Log: /var/log/gobup-agent.log"
}

install_deps
if ! download_archive; then
  log_error "Failed to download ${ARCHIVE_NAME}"
  exit 1
fi
install_binary
check_binary_compatibility
install_service
