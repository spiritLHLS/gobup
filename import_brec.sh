#!/bin/bash
#
# BililiveRecorder 历史记录一键导入脚本
# 使用方法: bash import_brec.sh
#

set -e

# ============================================
# 配置区域 - 根据你的实际情况修改
# ============================================

# BililiveRecorder 录制文件夹（宿主机路径）
BREC_DIR="${BREC_DIR:-/root/bilirecord}"

# gobup API 地址
GOBUP_URL="${GOBUP_URL:-http://localhost:22380}"

# gobup 用户名（如果未设置环境变量，则提示输入）
if [ -z "$GOBUP_USER" ]; then
    read -p "请输入 gobup 用户名: " GOBUP_USER
fi

# gobup 密码（如果未设置环境变量，则提示输入）
if [ -z "$GOBUP_PASS" ]; then
    read -s -p "请输入 gobup 密码: " GOBUP_PASS
    echo ""
fi

# ============================================
# 脚本开始
# ============================================

echo "=========================================="
echo "  BililiveRecorder 历史记录导入工具"
echo "=========================================="
echo ""

# 检查录制文件夹是否存在
if [ ! -d "$BREC_DIR" ]; then
    echo "❌ 错误: 录制文件夹不存在: $BREC_DIR"
    echo "请检查路径或修改脚本中的 BREC_DIR 变量"
    exit 1
fi

# 检查 gobup 是否运行
echo "🔍 检查 gobup 容器状态..."
if ! docker ps | grep -q gobup; then
    echo "❌ 错误: gobup 容器未运行"
    echo "请先启动 gobup 容器"
    exit 1
fi
echo "✅ gobup 容器正在运行"
echo ""

# 检查 Python3
echo "🔍 检查 Python3..."
if ! command -v python3 &> /dev/null; then
    echo "⚠️  未安装 Python3，正在安装..."
    
    # 检测系统类型
    if [ -f /etc/redhat-release ]; then
        # CentOS/RHEL
        sudo yum install -y python3
    elif [ -f /etc/debian_version ]; then
        # Debian/Ubuntu
        sudo apt-get update
        sudo apt-get install -y python3
    else
        echo "❌ 无法自动安装 Python3，请手动安装"
        exit 1
    fi
fi
echo "✅ Python3 已安装: $(python3 --version)"
echo ""

# 检查 pip3
echo "🔍 检查 pip3..."
if ! command -v pip3 &> /dev/null; then
    echo "⚠️  未安装 pip3，正在安装..."
    
    if [ -f /etc/redhat-release ]; then
        sudo yum install -y python3-pip
    elif [ -f /etc/debian_version ]; then
        sudo apt-get install -y python3-pip
    fi
fi
echo "✅ pip3 已安装"
echo ""

# 安装 Python 依赖
echo "🔍 检查 Python 依赖..."
if ! pip3 show requests &> /dev/null; then
    echo "⚠️  正在安装 requests 库..."
    pip3 install requests
fi
echo "✅ Python 依赖已满足"
echo ""

# 下载导入脚本（如果不存在）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMPORT_SCRIPT="$SCRIPT_DIR/import_brec_history.py"

if [ ! -f "$IMPORT_SCRIPT" ]; then
    echo "⚠️  导入脚本不存在，尝试从当前目录查找..."
    
    # 尝试多个可能的位置
    POSSIBLE_LOCATIONS=(
        "./import_brec_history.py"
        "/root/import_brec_history.py"
        "/tmp/import_brec_history.py"
    )
    
    FOUND=false
    for loc in "${POSSIBLE_LOCATIONS[@]}"; do
        if [ -f "$loc" ]; then
            IMPORT_SCRIPT="$loc"
            FOUND=true
            echo "✅ 找到导入脚本: $IMPORT_SCRIPT"
            break
        fi
    done
    
    if [ "$FOUND" = false ]; then
        echo "❌ 错误: 找不到 import_brec_history.py"
        echo "请确保 import_brec_history.py 与本脚本在同一目录"
        exit 1
    fi
fi

# 确认信息
echo "=========================================="
echo "配置信息确认:"
echo "=========================================="
echo "录制文件夹: $BREC_DIR"
echo "gobup 地址: $GOBUP_URL"
echo "用户名: $GOBUP_USER"
echo "密码: ********"
echo ""

# 统计视频文件数量
VIDEO_COUNT=$(find "$BREC_DIR" -type f \( -name "*.flv" -o -name "*.mp4" -o -name "*.mkv" \) | wc -l)
echo "📹 找到 $VIDEO_COUNT 个视频文件"
echo ""

# 询问是否继续
read -p "是否继续导入? [Y/n] " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]] && [[ ! -z $REPLY ]]; then
    echo "❌ 已取消"
    exit 0
fi

echo ""
echo "=========================================="
echo "开始导入..."
echo "=========================================="
echo ""

# 执行导入
python3 "$IMPORT_SCRIPT" \
    --dir "$BREC_DIR" \
    --url "$GOBUP_URL" \
    --user "$GOBUP_USER" \
    --pass "$GOBUP_PASS"

EXIT_CODE=$?

echo ""
if [ $EXIT_CODE -eq 0 ]; then
    echo "=========================================="
    echo "✅ 导入完成！"
    echo "=========================================="
    echo ""
    echo "下一步："
    echo "1. 访问 gobup 管理界面: $GOBUP_URL"
    echo "2. 在「历史记录」页面查看导入的录制"
    echo "3. 选择需要上传的视频进行操作"
else
    echo "=========================================="
    echo "❌ 导入失败"
    echo "=========================================="
    echo ""
    echo "请检查上方的错误信息"
fi

exit $EXIT_CODE
