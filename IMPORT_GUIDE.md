# BililiveRecorder 历史记录导入指南

## 概述

`import_brec_history_db.py` 是一个独立的导入工具，用于从 BililiveRecorder 录制文件夹批量导入历史记录到 gobup。

### 特性

- ✅ 批量扫描录制文件夹
- ✅ 自动从文件名提取元数据
- ✅ 自动去重（基于文件路径）
- ✅ 支持递归扫描子文件夹
- ✅ 详细的导入统计和错误报告
- ✅ 自动合并同一场直播的多个文件
- ⚡ 直接操作数据库，速度快、更可靠
- ❌ 无需认证，简单易用

## 前提条件

### 1. 确保 Python 3 已安装

```bash
# 检查 Python 版本
python3 --version

# 如果未安装，使用以下命令安装 (CentOS/RHEL)
sudo yum install python3

# Ubuntu/Debian
sudo apt-get install python3
```

### 2. 确保有数据库文件访问权限

```bash
# 检查数据库文件是否存在
ls -la /root/data/gobup.db
```

## 快速开始

### 方法一：数据库直接导入（推荐）

**适用场景**: 有数据库文件访问权限（本地或容器内）

```bash
# 下载脚本
```bash
# 下载脚本
cd /root
wget https://raw.githubusercontent.com/spiritlhls/gobup/main/import_brec_history_db.py

# 添加执行权限
chmod +x import_brec_history_db.py

# 执行导入
python3 import_brec_history_db.py \
  --dir /root/bilirecord \
  --db /root/data/gobup.db
```

**重要提示**: 
1. 导入前建议停止 gobup 服务或确保没有并发写入
2. 建议先备份数据库: `cp /root/data/gobup.db /root/data/gobup.db.backup`--dir` | `-d` | ✅ | - | BililiveRecorder 录制文件夹路径 |
| `--url` | `-u` | ❌ | `http://localhost:22380` | gobup API 地址 |
| `--user` | - | ❌ | `root` | gobup 用户名 |
| `--pass` | `-p` | ❌ | `spiritlhl` | gobup 密码 |

## 使用环境变量

为了避免在命令行中暴露密码，可以使用环境变量：

```bash
# 设置环境变量
export GOBUP_URL=http://localhost:22380
export GOBUP_USER=root
export GOBUP_PASS=spiritlhl

# 简化的命令
python3 import_brec_history.py --dir /root/bilirecord
```

## 路径映射说明

### Docker 容器路径映射

在你的配置中：
- **brec 容器**: `-v /root/bilirecord:/rec` （录制文件存储位置）
- **gobup 容器**: `-v /root/recordings:/rec` （gobup 访问录制文件）

### 重要提示

脚本会自动将宿主机路径转换为容器内路径：

- 宿主机路径: `/root/bilirecord/xxx.flv`
- 容器内路径: `/rec/xxx.flv`

**如果你的 gobup 容器挂载的是不同的目录**，需要调整：

```bash
# 如果 gobup 容器配置是：
# -v /root/bilirecord:/rec
# 那么直接使用脚本即可
### 数据库方式

```
1. 扫描录制文件夹
   └─> 查找 .flv, .mp4, .mkv 文件

2. 对于每个视频文件
   ├─> 从文件名提取元数据（房间号、标题、时间等）
   ├─db` | - | ❌ | `/root/data/gobup.db` | gobup 数据库文件路径 |├─> 从文件名提取元数据（房间号、标题、时间
## 工作流程

```
1. 扫描录制文件夹
   └─> 查找 .flv, .mp4, .mkv 文件

2. 对于每个视频文件
   ├─> 查找对应的 .xml 元数据文件
   ├─> 解析元数据（房间号、主播名、标题等）
   ├─> 检查是否已导入（去重）
   └─> 通过 webhook API 创建历史记录

3. 输出统计报告
   └─> 总数、成功、跳过、失败
```

## 输出示例

```
🔍 开数据库方式常见问题

#### 1. 数据库文件不存在

```
❌ 错误: 数据库文件不存在: /root/data/gobup.db
```

**解决方案**:
- 确认数据库文件路径: `ls -la /root/data/gobup.db`
- 检查 gobup 容器挂载配置: `docker inspect gobup | grep data`

#### 2. 数据库锁定

# 如果 gobup 容器配置是：
# -v /root/recordings:/rec
# 你需始扫描目录: /root/bilirecord
💾 数据库路径: /root/data/gobup.db
------------------------------------------------------------
📹 找到 15 个视频文件

📄 处理: 录制-123456-20231230-103000-001-标题.flv
   ✅ 导入成功
📄 处理: 录制-123456-20231230-150000-001-标题.flv
   ⏭️  已存在，跳过
📄 处理: 录制-789012-20231230-200000-001-标题.flv
   ✅ 导入成功

============================================================
📊 导入统计
============================================================
总文件数: 15
✅ 成功通用问题

#### 1. 找不到文件

```
❌ 目录不存在: /root/bilirecord
```

**解决方案**:
- 确认录制文件夹路径: `ls -la /root/bilirecord`
- 检查权限: `ls -ld /root/bilirecord`

#### 2. 导入后在界面看不到

**解决方案**:
- 刷新浏览器页面
- 检查是否真的导入成功（查看统计报告）
- 使用调试模式查看详情: `DEBUG=1 python3 import_brec_history_db.py ...`

## 定期导入（可选）

如果需要定期自动导入新录制的文件，可以使用 cron：

## 工作流程
 高级用法

### 仅导入特定房间的录制

```bash
# 如果录制文件按房间号分文件夹存储
python3 import_brec_history
# 添加定时任务（每小时执行一次）
# 注意：需要先停止 gobup 再导入，导入完成后启动
0 * * * * docker stop gobup && python3 /root/import_brec_history_db.py --dir /root/bilirecord --db /root/data/gobup.db >> /var/log/gobup_import.log 2>&1 && docker start gobup
```

### API 方式

#### 2. 认证失败

```
⚠️  导入失败 (HTTP 401): Unauthorized
```

**解决方案**:
- 检查用户名和密码是否正确
- 确认 gobup 容器的环境变量: `docker inspect gobup | grep -E 'USERNAME|PASSWORD'`

### 通用问题_db.py --dir /root/bilirecord/123456 --db /root/data/gobup.db
```

### 导入前备份数据库（强烈推荐）

```bash
# 方法一：直接复制数据库文件
cp /root/data/gobup.db /root/data/gobup.db.backup

# 方法二：通过容器备份（如果数据库在容器内）
docker exec gobup cp /app/data/gobup.db /app/data/gobup.db.backup

# 执行导入
python3 import_brec_history_db.py --dir /root/bilirecord --db /root/data/gobup.db

# 如需恢复
cp /root/data/gobup.db.backup /root/data/gobup.db
# 或
docker exec gobup cp /app/data/gobup.db.backup /app/data/gobup.db
docker restart gobup
```

### 调试模式

```bash
# 启用详细日志输出
DEBUG=1 python3 import_brec_history_db.py --dir /root/bilirecord --db /root/data/gobup.db
```

### 批量导入多个目录

```bash
#!/bin/bash
# import_all.sh

DB="/root/data/gobup.db"

# 备份数据库
cp $DB ${DB}.backup

# 停止 gobup 服务
docker stop gobup

# 导入多个目录
for dir in /root/bilirecord/*; do
  if [ -d "$dir" ]; then 导入前备份数据库

```bash
# 备份 gobup 数据库
docker exec gobup cp /app/data/gobup.db /app/data/gobup.db.backup

# 执行导入
python3 import_brec_history.py --dir /root/bilirecord

# 如需恢复
docker exec gobup cp /app/data/gobup.db.backup /app/data/gobup.db
docker restart gobup
```