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
  TARGETS="x86_64-unknown-linux-musl aarch64-unknown-linux-musl"
fi

CARGO_BIN="${CARGO_BIN:-cargo}"

ensure_rust_target() {
  target="$1"
  if rustup target list --installed 2>/dev/null | grep -qx "$target"; then
    return 0
  fi
  if command -v rustup >/dev/null 2>&1; then
    rustup target add "$target"
  fi
}

check_linux_agent_binary() {
  bin="$1"
  suffix="$2"
  case "$suffix" in
    linux-*) ;;
    *) return 0 ;;
  esac

  if command -v readelf >/dev/null 2>&1; then
    if readelf -d "$bin" 2>/dev/null | grep -q 'Shared library: \[libc\.so\.6\]'; then
      if [ "${ALLOW_GLIBC_AGENT:-0}" != "1" ]; then
        echo "linux agent package links against glibc: $bin" >&2
        echo "build with a musl target or set ALLOW_GLIBC_AGENT=1 only for a deliberate compatibility exception" >&2
        exit 1
      fi
    fi
  fi

  if command -v strings >/dev/null 2>&1; then
    if strings "$bin" 2>/dev/null | grep -q 'GLIBC_[0-9]'; then
      if [ "${ALLOW_GLIBC_AGENT:-0}" != "1" ]; then
        echo "linux agent package contains GLIBC version requirements: $bin" >&2
        echo "release packages must be static/musl so old Linux hosts can run them" >&2
        exit 1
      fi
    fi
  fi
}

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
  ensure_rust_target "$target"
  case "$target" in
    x86_64-unknown-linux-musl)
      export CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER="${CARGO_TARGET_X86_64_UNKNOWN_LINUX_MUSL_LINKER:-rust-lld}"
      ;;
    aarch64-unknown-linux-musl)
      export CARGO_TARGET_AARCH64_UNKNOWN_LINUX_MUSL_LINKER="${CARGO_TARGET_AARCH64_UNKNOWN_LINUX_MUSL_LINKER:-rust-lld}"
      ;;
  esac
  "$CARGO_BIN" build --release --manifest-path "$MANIFEST" --target "$target"

  bin="$ROOT_DIR/server/agent/target/$target/release/gobup-agent"
  if [ ! -x "$bin" ]; then
    echo "built binary not found: $bin" >&2
    exit 1
  fi
  check_linux_agent_binary "$bin" "$suffix"

  stage="$(mktemp -d)"
  cp "$bin" "$stage/gobup-agent"
  chmod +x "$stage/gobup-agent"
  tar -C "$stage" -czf "$BUILD_DIR/gobup-agent-$suffix.tar.gz" gobup-agent
  cp "$BUILD_DIR/gobup-agent-$suffix.tar.gz" "$ASSET_DIR/gobup-agent-$suffix.tar.gz"
  rm -rf "$stage"
done

echo "agent assets are ready in $ASSET_DIR"
