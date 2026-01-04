#!/bin/bash
set -e

### ===== 可修改参数 =====
PORT=20338
USER_NAME=""
USER_PASS=""
VERSION="0.9.5"
### ======================

CONF_DIR="/etc/3proxy/conf"
PASSWD_FILE="${CONF_DIR}/passwd"
CFG_FILE="${CONF_DIR}/3proxy.cfg"
LOG_FILE="/var/log/3proxy.log"

ARCH="$(uname -m)"

if [[ "$ARCH" == "x86_64" ]]; then
  DEB="3proxy-${VERSION}.x86_64.deb"
elif [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  DEB="3proxy-${VERSION}.arm64.deb"
else
  echo "❌ 不支持的架构: $ARCH"
  exit 1
fi

echo "▶ 安装依赖"
apt update -y
apt install -y wget curl openssl

echo "▶ 检查并安装 3proxy"
if ! command -v 3proxy >/dev/null 2>&1; then
  cd /tmp
  rm -f ${DEB}
  wget -q https://github.com/3proxy/3proxy/releases/download/${VERSION}/${DEB}
  dpkg -i ${DEB} || apt --fix-broken install -y
else
  echo "✓ 3proxy 已安装，跳过安装"
fi

echo "▶ 写入认证用户（覆盖）"
mkdir -p "${CONF_DIR}"
PASS_HASH=$(openssl passwd -1 "${USER_PASS}")
echo "${USER_NAME}:${PASS_HASH}" > "${PASSWD_FILE}"
chmod 600 "${PASSWD_FILE}"

echo "▶ 写入配置文件（覆盖）"
cat > "${CFG_FILE}" <<EOF
daemon
pidfile /var/run/3proxy.pid

maxconn 1024
nscache 65536

users \$/etc/3proxy/conf/passwd
auth strong
allow ${USER_NAME}

socks -p${PORT} -a

log ${LOG_FILE} D
rotate 7
EOF

echo "▶ 初始化日志"
touch "${LOG_FILE}"
chmod 644 "${LOG_FILE}"

echo "▶ 重启并设置开机启动"
systemctl daemon-reexec
systemctl enable 3proxy >/dev/null 2>&1 || true
systemctl restart 3proxy

### 获取公网 IPv4
SERVER_IP=$(curl -4 -s --max-time 5 https://ipv4.icanhazip.com || curl -4 -s --max-time 5 https://ifconfig.me || echo "YOUR_SERVER_IP")

echo
echo "========================================"
echo "🎉 3proxy SOCKS5 已部署完成（可重复执行）"
echo "----------------------------------------"
echo "【标准代理格式（直接可用）】"
echo
echo "socks5://${USER_NAME}:${USER_PASS}@${SERVER_IP}:${PORT}"
echo
echo "----------------------------------------"
echo "管理命令："
echo "systemctl status 3proxy"
echo "journalctl -u 3proxy -f"
echo "========================================"
