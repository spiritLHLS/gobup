# BililiveRecorder 历史记录导入指南

## 概述

`import_brec_history.py` 是一个独立的导入工具，用于从 BililiveRecorder 录制文件夹批量导入历史记录到 gobup。

### 特性

- ✅ 批量扫描录制文件夹
- ✅ 自动读取 `.xml` 元数据文件
- ✅ 自动去重（基于文件路径）
- ✅ 支持递归扫描子文件夹
- ✅ 不修改项目代码，通过 API 导入
- ✅ 详细的导入统计和错误报告

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

### 2. 安装依赖

```bash
pip3 install requests
```

### 3. 确保 Docker 容器正在运行

```bash
# 检查容器状态
docker ps | grep -E 'gobup|brec'
```

## 快速开始

### 基础用法

```bash
# 下载脚本
cd /root
wget https://raw.githubusercontent.com/yourusername/gobup/main/import_brec_history.py
# 或者从项目目录复制
# cp /path/to/gobup/import_brec_history.py /root/

# 添加执行权限
chmod +x import_brec_history.py

# 执行导入（使用默认配置）
python3 import_brec_history.py --dir /root/bilirecord
```

### 完整示例

```bash
python3 import_brec_history.py \
  --dir /root/bilirecord \
  --url http://localhost:22380 \
  --user root \
  --pass spiritlhl
```

## 参数说明

| 参数 | 简写 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| `--dir` | `-d` | ✅ | - | BililiveRecorder 录制文件夹路径 |
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

# 如果 gobup 容器配置是：
# -v /root/recordings:/rec
# 你需要先复制或移动文件到 recordings 目录
# 或者修改 gobup 容器的挂载配置
```

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
🔍 开始扫描目录: /root/bilirecord
📡 gobup 地址: http://localhost:22380
------------------------------------------------------------
📹 找到 15 个视频文件

📄 处理: 123456-20231230-103000.flv
   ✅ 导入成功
📄 处理: 123456-20231230-150000.flv
   ⏭️  已存在，跳过
📄 处理: 789012-20231230-200000.flv
   ✅ 导入成功

============================================================
📊 导入统计
============================================================
总文件数: 15
✅ 成功: 10
⏭️  跳过: 3
❌ 失败: 2

错误详情:
  - video1.flv: 解析 XML 失败
  - video2.flv: 导入失败
```

## 故障排查

### 1. 连接失败

```
❌ 导入出错: HTTPConnectionPool(...): Max retries exceeded
```

**解决方案**:
- 检查 gobup 容器是否运行: `docker ps | grep gobup`
- 检查端口映射是否正确: `-p 22380:12380`
- 测试连接: `curl http://localhost:22380/api/recordWebHook`

### 2. 认证失败

```
⚠️  导入失败 (HTTP 401): Unauthorized
```

**解决方案**:
- 检查用户名和密码是否正确
- 确认 gobup 容器的环境变量: `docker inspect gobup | grep -E 'USERNAME|PASSWORD'`

### 3. 找不到文件

```
❌ 目录不存在: /root/bilirecord
```

**解决方案**:
- 确认录制文件夹路径: `ls -la /root/bilirecord`
- 检查权限: `ls -ld /root/bilirecord`

### 4. 无法读取 XML

```
⚠️  解析 XML 失败
```

**解决方案**:
- 检查 XML 文件是否损坏
- 脚本会为没有 XML 的文件创建默认元数据，仍可导入

## 定期导入（可选）

如果需要定期自动导入新录制的文件，可以使用 cron：

```bash
# 编辑 crontab
crontab -e

# 添加定时任务（每小时执行一次）
0 * * * * cd /root && python3 import_brec_history.py --dir /root/bilirecord >> /var/log/gobup_import.log 2>&1
```

## 高级用法

### 仅导入特定房间的录制

```bash
# 如果录制文件按房间号分文件夹存储
python3 import_brec_history.py --dir /root/bilirecord/123456
```

### 导入前备份数据库

```bash
# 备份 gobup 数据库
docker exec gobup cp /app/data/gobup.db /app/data/gobup.db.backup

# 执行导入
python3 import_brec_history.py --dir /root/bilirecord

# 如需恢复
docker exec gobup cp /app/data/gobup.db.backup /app/data/gobup.db
docker restart gobup
```