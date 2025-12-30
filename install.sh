#!/bin/bash
# GoBup 一键安装脚本
# from https://github.com/spiritlhls/gobup

VERSION="" 
REPO="spiritlhls/gobup"
BASE_URL=""
GOBUP_USERNAME="${GOBUP_USERNAME:-}"
GOBUP_PASSWORD="${GOBUP_PASSWORD:-}"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

reading() { 
    printf "\033[32m\033[01m%s\033[0m" "$1"
    read "$2"
}

get_latest_version() {
    if [ -n "$INSTALL_VERSION" ]; then
        log_info "使用指定版本: $INSTALL_VERSION"
        VERSION="$INSTALL_VERSION"
        BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
        return 0
    fi
    
    log_info "正在获取最新版本信息..."
    
    local response
    if response=$(curl -sL --connect-timeout 10 --max-time 30 "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null); then
        VERSION=$(echo "$response" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
        
        if [ -n "$VERSION" ] && [ "$VERSION" != "null" ]; then
            log_success "成功获取最新版本: $VERSION"
            BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
            return 0
        fi
    fi
    
    log_error "无法获取最新版本信息"
    return 1
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "此脚本需要以root身份运行"
        exit 1
    fi
}

detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            log_error "不支持的架构: $arch"
            exit 1
            ;;
    esac
}

detect_system() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        SYSTEM=$ID
    elif [ -f /etc/lsb-release ]; then
        . /etc/lsb-release
        SYSTEM=$DISTRIB_ID
    else
        SYSTEM=$(uname -s)
    fi
    
    log_success "检测到系统: $SYSTEM"
}

check_dependencies() {
    local deps=("curl" "tar")
    local missing=()
    
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            missing+=("$dep")
        fi
    done
    
    if [ ${#missing[@]} -ne 0 ]; then
        log_warning "缺少必要工具: ${missing[*]}"
        log_info "正在安装缺少的工具..."
        
        if command -v apt-get >/dev/null 2>&1; then
            apt-get update -qq
            apt-get install -y "${missing[@]}"
        elif command -v yum >/dev/null 2>&1; then
            yum install -y "${missing[@]}"
        elif command -v dnf >/dev/null 2>&1; then
            dnf install -y "${missing[@]}"
        else
            log_error "无法自动安装依赖，请手动安装: ${missing[*]}"
            exit 1
        fi
    fi
}

download_file() {
    local url="$1"
    local output="$2"
    local max_retries=3
    local retry_count=0
    
    while [ $retry_count -lt $max_retries ]; do
        if curl -L --connect-timeout 10 --max-time 60 -o "$output" "$url" 2>/dev/null; then
            return 0
        elif wget -T 10 -t 3 -O "$output" "$url" 2>/dev/null; then
            return 0
        fi
        
        retry_count=$((retry_count + 1))
        log_warning "下载失败，重试 (${retry_count}/${max_retries}): $url"
        sleep 2
    done
    
    log_error "下载失败: $url"
    return 1
}

create_directories() {
    local dirs=("/opt/gobup" "/opt/gobup/server")
    
    for dir in "${dirs[@]}"; do
        if [ ! -d "$dir" ]; then
            mkdir -p "$dir"
            log_info "创建目录: $dir"
        fi
    done
}

install_server() {
    local arch=$(detect_arch)
    local filename="gobup-server-linux-${arch}.tar.gz"
    local download_url="${BASE_URL}/${filename}"
    local temp_file="/tmp/${filename}"
    
    log_info "下载服务器二进制文件 (${arch})..."
    log_info "下载链接: $download_url"
    
    if download_file "$download_url" "$temp_file"; then
        log_success "下载完成: $filename"
    else
        log_error "下载失败: $download_url"
        exit 1
    fi
    
    log_info "解压服务器二进制文件..."
    if tar -xzf "$temp_file" -C /opt/gobup/server/; then
        # 重命名二进制文件
        local binary_name="gobup-server-linux-${arch}"
        if [ -f "/opt/gobup/server/$binary_name" ]; then
            mv "/opt/gobup/server/$binary_name" /opt/gobup/server/gobup-server
        fi
        
        chmod +x /opt/gobup/server/gobup-server
        rm -f "$temp_file"
        log_success "服务器二进制文件安装完成"
    else
        log_error "解压失败"
        exit 1
    fi
}

# Web 文件已嵌入到二进制文件中，无需单独安装

create_systemd_service() {
    local service_file="/etc/systemd/system/gobup.service"
    
    log_info "创建systemd服务文件..."
    
    # 构建环境变量配置
    local env_vars=""
    if [ -n "$GOBUP_USERNAME" ]; then
        env_vars="${env_vars}Environment=\"USERNAME=${GOBUP_USERNAME}\"\n"
    fi
    if [ -n "$GOBUP_PASSWORD" ]; then
        env_vars="${env_vars}Environment=\"PASSWORD=${GOBUP_PASSWORD}\"\n"
    fi
    
    cat > "$service_file" << EOF
[Unit]
Description=GoBup Server - B站录播自动上传工具
Documentation=https://github.com/spiritlhls/gobup
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/gobup/server
$(echo -e "$env_vars")ExecStart=/opt/gobup/server/gobup-server
Restart=always
RestartSec=5
StartLimitInterval=60
StartLimitBurst=3
StandardOutput=journal
StandardError=journal
SyslogIdentifier=gobup

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable gobup
    log_success "systemd服务文件创建完成"
}

create_symlink() {
    if [ ! -L "/usr/local/bin/gobup" ]; then
        ln -sf /opt/gobup/server/gobup-server /usr/local/bin/gobup
        log_success "创建命令行链接: /usr/local/bin/gobup"
    else
        log_info "命令行链接已存在"
    fi
}

create_readme() {
    local readme_file="/opt/gobup/README.md"
    
    log_info "创建使用说明文件..."
    
    cat > "$readme_file" << EOF
# GoBup 使用方法

## 版本信息
版本: $VERSION
系统: $SYSTEM
架构: $(detect_arch)

## 目录结构
- 安装目录: /opt/gobup
- 服务器文件: /opt/gobup/server/ (前端已嵌入)
- 数据目录: /app/data（Docker部署）

## 服务管理命令
- 启动服务: systemctl start gobup
- 停止服务: systemctl stop gobup
- 重启服务: systemctl restart gobup
- 开机自启: systemctl enable gobup
- 禁用自启: systemctl disable gobup
- 查看状态: systemctl status gobup
- 查看日志: journalctl -u gobup -f

## 访问地址
- Web界面: http://localhost:12380

## 升级方法
bash install.sh upgrade

## 卸载方法
- 停止服务: systemctl stop gobup
- 禁用服务: systemctl disable gobup
- 删除文件: rm -rf /opt/gobup /usr/local/bin/gobup
- 删除服务: rm /etc/systemd/system/gobup.service
- 重载systemd: systemctl daemon-reload
EOF

    log_success "使用说明文件创建完成"
}

upgrade_server() {
    if [ ! -f "/opt/gobup/server/gobup-server" ]; then
        log_error "未检测到已安装的版本，请使用 install 选项进行全新安装"
        exit 1
    fi
    
    log_info "开始升级到版本: $VERSION"
    
    local service_was_running=false
    if systemctl is-active --quiet gobup 2>/dev/null; then
        log_info "停止 gobup 服务..."
        systemctl stop gobup
        service_was_running=true
    fi
    
    log_info "升级服务器二进制文件（包含嵌入的前端）..."
    install_server
    
    if [ "$service_was_running" = true ]; then
        log_info "重新启动 gobup 服务..."
        systemctl start gobup
        sleep 2
        if systemctl is-active --quiet gobup; then
            log_success "服务已成功重启"
        else
            log_error "服务启动失败，请检查日志: journalctl -u gobup -n 50"
        fi
    fi
    
    log_success "升级完成!"
    log_info "版本: $VERSION"
    if [ "$service_was_running" = false ]; then
        log_warning "服务未自动启动，请手动启动: systemctl start gobup"
    fi
}

show_info() {
    log_success "GoBup 安装完成!"
    echo ""
    log_info "安装信息:"
    log_info "  版本: $VERSION"
    log_info "  系统: $SYSTEM"
    log_info "  架构: $(detect_arch)"
    log_info "  安装路径: /opt/gobup"
    log_info "  部署模式: 单二进制文件（前端已嵌入）"
    echo ""
    if [ -n "$GOBUP_USERNAME" ] && [ -n "$GOBUP_PASSWORD" ]; then
        log_info "认证信息:"
        log_info "  用户名: $GOBUP_USERNAME"
        log_info "  密码: ********"
        log_warning "请妥善保管用户名和密码！"
        echo ""
    else
        log_warning "未设置认证信息，首次访问需要在登录页面输入用户名密码"
        log_info "设置方法: GOBUP_USERNAME=admin GOBUP_PASSWORD=123456 bash install.sh"
        echo ""
    fi
    log_info "使用方法:"
    log_info "  启动服务: systemctl start gobup"
    log_info "  查看状态: systemctl status gobup"
    log_info "  访问地址: http://localhost:12380"
    log_info "  详细说明: /opt/gobup/README.md"
    echo ""
    log_warning "首次使用请配置录播软件的Webhook地址并添加B站账号"
}

show_help() {
    cat <<"EOF"
GoBup 一键安装脚本

用法: bash install.sh [选项]

选项:
  install              完整安装 (默认)
  upgrade              升级已安装的版本
  help                 显示此帮助信息
  
环境变量:
  INSTALL_VERSION=v1.0.0          指定要安装的版本 (默认: 自动获取最新版本)
  GOBUP_USERNAME=admin            管理员用户名 (默认: 无，需要首次登录时输入)
  GOBUP_PASSWORD=your_password    管理员密码 (默认: 无，需要首次登录时输入)

示例:
  # 完整安装最新版本（无密码）
  bash install.sh
  
  # 完整安装并设置认证
  GOBUP_USERNAME=admin GOBUP_PASSWORD=123456 bash install.sh
  
  # 升级到最新版本
  bash install.sh upgrade
  
  # 安装指定版本
  INSTALL_VERSION=v20250101-120000 bash install.sh
  
  # 安装指定版本并设置认证
  INSTALL_VERSION=v20250101-120000 GOBUP_USERNAME=root GOBUP_PASSWORD=secret bash install.sh

注意:
  - 认证信息仅在首次启动时创建管理员账户使用
  - 如果不设置 USERNAME 和 PASSWORD，访问时会提示登录
  - 后续修改认证需要删除数据库重新初始化
EOF
}

env_check() {
    log_info "开始环境检查..."
    
    if ! get_latest_version; then
        log_error "无法获取最新版本，安装终止"
        exit 1
    fi
    
    detect_system
    check_dependencies
    log_success "环境检查完成"
}

main() {
    case "${1:-install}" in
        "install")
            check_root
            env_check
            create_directories
            install_server
            create_readme
            create_systemd_service
            create_symlink
            show_info
            ;;
        "upgrade")
            check_root
            env_check
            upgrade_server
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            log_error "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
