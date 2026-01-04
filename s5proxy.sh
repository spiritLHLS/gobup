#!/bin/bash
set -e

### ===== 可修改参数 =====
PORT=31338
VERSION="0.9.5"
### ======================

CONF_DIR="/etc/3proxy/conf"
CFG_FILE="${CONF_DIR}/3proxy.cfg"

ARCH="$(uname -m)"

if [[ "$ARCH" == "x86_64" ]]; then
  DEB="3proxy-${VERSION}.x86_64.deb"
elif [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  DEB="3proxy-${VERSION}.arm64.deb"
else
  echo "不支持的架构: $ARCH"
  exit 1
fi

echo "[1/6] 安装依赖"
apt update -y
apt install -y wget curl openssl

echo "[2/6] 检查并安装 3proxy"
if ! command -v 3proxy >/dev/null 2>&1; then
  cd /tmp
  rm -f ${DEB}
  wget -q https://github.com/3proxy/3proxy/releases/download/${VERSION}/${DEB}
  dpkg -i ${DEB} || apt --fix-broken install -y
else
  echo "3proxy 已安装，跳过安装"
fi

echo "[3/6] 创建配置文件目录"
mkdir -p "${CONF_DIR}"

echo "[4/6] 创建 pid 文件目录"
mkdir -p /run/3proxy
chown root:root /run/3proxy
chmod 755 /run/3proxy

echo "[5/6] 写入配置文件（覆盖）"
cat > "${CFG_FILE}" <<EOF
maxconn 1024
nscache 65536

auth none
allow *

socks -p${PORT}
EOF

echo "[6/6] 重启并设置开机启动"
systemctl daemon-reexec
systemctl enable 3proxy >/dev/null 2>&1 || true
systemctl restart 3proxy

### 获取公网 IPv4
SERVER_IP=$(curl -4 -s --max-time 5 https://ipv4.icanhazip.com || curl -4 -s --max-time 5 https://ifconfig.me || echo "YOUR_SERVER_IP")

echo
echo "========================================"
echo "3proxy SOCKS5 已部署完成（可重复执行）"
echo "----------------------------------------"
echo
echo "socks5://${SERVER_IP}:${PORT}"
echo
echo "----------------------------------------"
echo "管理命令："
echo "  systemctl status 3proxy"
echo "  journalctl -u 3proxy -f"
echo "========================================"
