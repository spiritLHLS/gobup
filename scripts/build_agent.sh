#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="$ROOT_DIR/server/agent/Cargo.toml"
ASSET_DIR="$ROOT_DIR/server/assets/agent"
BUILD_DIR="$ROOT_DIR/build"
INSTALLER_SRC="$ROOT_DIR/scripts/install_agent.sh"

mkdir -p "$ASSET_DIR" "$BUILD_DIR"
cp "$INSTALLER_SRC" "$ASSET_DIR/install_agent.sh"

if ! command -v cargo >/dev/null 2>&1; then
  echo "cargo is required to build gobup-agent" >&2
  exit 1
fi

TARGETS="${AGENT_TARGETS:-}"
if [ -z "$TARGETS" ]; then
  TARGETS="$(rustc -vV | awk '/host:/ {print $2}')"
fi

for target in $TARGETS; do
  case "$target" in
    x86_64-unknown-linux-gnu|x86_64-unknown-linux-musl)
      suffix="linux-amd64"
      ;;
    aarch64-unknown-linux-gnu|aarch64-unknown-linux-musl)
      suffix="linux-arm64"
      ;;
    *)
      echo "skip unsupported agent package target: $target" >&2
      continue
      ;;
  esac

  echo "building gobup-agent for $target"
  cargo build --release --manifest-path "$MANIFEST" --target "$target"

  bin="$ROOT_DIR/server/agent/target/$target/release/gobup-agent"
  if [ ! -x "$bin" ]; then
    echo "built binary not found: $bin" >&2
    exit 1
  fi

  stage="$(mktemp -d)"
  cp "$bin" "$stage/gobup-agent"
  chmod +x "$stage/gobup-agent"
  tar -C "$stage" -czf "$BUILD_DIR/gobup-agent-$suffix.tar.gz" gobup-agent
  cp "$BUILD_DIR/gobup-agent-$suffix.tar.gz" "$ASSET_DIR/gobup-agent-$suffix.tar.gz"
  rm -rf "$stage"
done

echo "agent assets are ready in $ASSET_DIR"
