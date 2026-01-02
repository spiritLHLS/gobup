# GoBup - B站录播自动上传工具

[![Build and Release](https://github.com/spiritlhls/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml)

一个用Go语言实现的B站录播自动上传工具，支持自动上传录播文件到B站，支持多账号管理、WxPusher消息推送等功能。

## 快速部署

1. **安装脚本部署**：使用环境变量 `GOBUP_USERNAME` 和 `GOBUP_PASSWORD`
2. **Docker 部署**：使用环境变量 `USERNAME` 和 `PASSWORD`
3. **手动部署**：使用命令行参数 `-username` 和 `-password`

### 方式一：一键安装脚本

使用一键安装脚本，自动下载并安装最新版本的服务器和Web文件：

**带认证部署：**

```bash
# 下载脚本
curl -fsSL https://cdn.spiritlhl.net/https://raw.githubusercontent.com/spiritlhls/gobup/main/install.sh -o install.sh && chmod +x install.sh

# 设置用户名密码安装
GOBUP_USERNAME=admin GOBUP_PASSWORD=your_secure_password bash install.sh
```

**支持的选项：**
- `install` - 完整安装（默认）
- `upgrade` - 升级到最新版本
- `help` - 显示帮助信息

**环境变量：**
- `INSTALL_VERSION=vYYYYMMDD-HHMMSS` - 指定安装版本
- `GOBUP_USERNAME=admin` - 管理员用户名（推荐设置）
- `GOBUP_PASSWORD=password` - 管理员密码（推荐设置）

**示例：**

```bash
# 完整安装并设置认证（推荐）
GOBUP_USERNAME=admin GOBUP_PASSWORD=123456 bash install.sh

# 升级到最新版本
bash install.sh upgrade
```

**安装后访问：**
- Web界面: http://localhost:12380
- 如果设置了认证，使用设置的用户名密码登录
- 如果未设置认证，首次访问会要求输入用户名密码
- 服务管理: `systemctl status gobup`

### 方式二：使用预构建 Docker 镜像

所有镜像均支持 `linux/amd64` 和 `linux/arm64` 架构。

**完整配置运行**

```bash
docker pull spiritlhl/gobup:latest

docker run -d \
  --name gobup \
  -p 22380:12380 \
  -v /root/bilirecord:/rec \
  -v /root/data:/app/data \
  -e USERNAME=admin \
  -e PASSWORD=your_secure_password \
  --restart unless-stopped \
  spiritlhl/gobup:latest
```

> 注意：USERNAME 和 PASSWORD 仅用于首次启动时创建管理员账户，后续修改环境变量不会更新已存在的账户

### 方式三：使用 Docker Compose

创建 `docker-compose.yml`：

```yaml
version: '3.8'

services:
  gobup:
    image: spiritlhl/gobup:latest
    container_name: gobup
    restart: unless-stopped
    ports:
      - "22380:12380"
    volumes:
      - ./bilirecord:/rec
      - ./data:/app/data
    environment:
      - TZ=Asia/Shanghai
      - USERNAME=admin  # 可选，仅首次启动时创建管理员账户
      - PASSWORD=your_password  # 可选，仅首次启动时创建管理员账户
```

运行：

```bash
docker-compose up -d
```

### 方式四：从源码构建 Docker 镜像

```bash
# 克隆仓库
git clone https://github.com/spiritlhls/gobup.git
cd gobup

# 构建镜像
docker build -t gobup .

# 运行容器
docker run -d \
  --name gobup \
  -p 22380:12380 \
  -v /path/to/bilirecord:/rec \
  -v /path/to/data:/app/data \
  -e USERNAME=admin \
  -e PASSWORD=your_password \
  --restart unless-stopped \
  gobup
```

### 方式五：下载发布版本手动部署

从 [Releases](https://github.com/spiritlhls/gobup/releases) 页面下载对应平台的二进制文件：

**单二进制部署版本（前端已嵌入）：**
- `gobup-server-linux-amd64.tar.gz` - Linux AMD64 版本
- `gobup-server-linux-arm64.tar.gz` - Linux ARM64 版本
- `gobup-server-darwin-amd64.tar.gz` - macOS Intel 版本
- `gobup-server-darwin-arm64.tar.gz` - macOS Apple Silicon 版本
- `gobup-server-windows-amd64.zip` - Windows AMD64 版本

> 所有二进制文件已包含嵌入的前端页面，直接运行即可，无需额外部署前端文件。

解压后运行：

```bash
# Linux/macOS（无认证）
tar -xzf gobup-server-linux-amd64.tar.gz
./gobup-server-linux-amd64 -port 12380 -work-path /path/to/bilirecord

# Linux/macOS（带认证，推荐）
./gobup-server-linux-amd64 -port 12380 -work-path /path/to/bilirecord \
  -username admin -password your_password
```

```powershell
# Windows（无认证）
# 解压 gobup-server-windows-amd64.zip
gobup-server-windows-amd64.exe -port 12380 -work-path C:\path\to\bilirecord

# Windows（带认证，推荐）
gobup-server-windows-amd64.exe -port 12380 -work-path C:\path\to\bilirecord ^
  -username admin -password your_password
```

**命令行参数说明：**
- `-port`: Web 服务端口（默认 12380）
- `-work-path`: 录播文件工作目录
- `-username`: 管理员用户名（可选，首次启动时创建）
- `-password`: 管理员密码（可选，首次启动时创建）
- `-data-path`: 数据目录（默认 ./data）

**访问 Web 界面：**
- 所有部署方式统一访问：`http://localhost:12380` 或 `http://localhost:22380`（Docker映射端口）
- 或使用服务器IP：`http://你的IP:12380` 或 `http://你的IP:22380`
- 如果设置了认证，使用设置的用户名密码登录
- 如果未设置认证，首次访问会要求输入用户名密码

## 配置说明

### 配置Docker网络（Docker部署）

为了让录播姬和本项目能够相互通信，需要配置同一网络：

```bash
# 创建网络
docker network create bili-net

# 连接容器到网络
docker network connect bili-net brec  # 或 blrec
docker network connect bili-net gobup
```

### 配置扫盘目录

GoBup会自动扫描录制文件并入库，无需配置Webhook：

1. 访问Web界面 -> 控制面板
2. 在"工作目录"中配置录播软件的录制目录（如 `/rec`）
3. （可选）在"自定义扫描目录"中添加额外的扫描路径，用逗号分隔
4. 系统会自动按设置的扫盘间隔扫描这些目录
5. 也可以手动点击"扫描录入"按钮立即扫描

> 提示：
> - Docker部署时，确保已将录播目录挂载到容器（如 `-v /path/to/recordings:/rec`）
> - 系统会优先扫描自定义目录，然后扫描工作目录
> - 默认会跳过12小时内修改的文件（防止扫描正在写入的文件）

### 添加B站账号

访问Web界面 -> 用户管理 -> 添加用户：

1. 点击"生成登录二维码"
2. 使用哔哩哔哩App扫码登录
3. 登录成功后，Cookie会自动保存

### 配置直播间

访问Web界面 -> 房间管理 -> 添加房间：

- **房间ID**: 直播间房间号
- **上传用户**: 选择已登录的B站账号
- **分区**: 视频投稿分区（如游戏、娱乐等）
- **标题模板**: 视频标题（支持变量）
- **简介模板**: 视频简介（支持变量）
- **标签**: 视频标签，用逗号分隔
- **上传线路**: upos/app，建议upos（支持多条UPOS线路选择）
- **合集ID**: 自动添加到指定合集（可选）
- **分P设置**: 
  - 是否分P
  - 单个视频最大大小
  - 分P标题模板

### 配置WxPusher消息推送（可选）

1. 注册WxPusher账号：https://wxpusher.zjiecode.com/
2. 创建应用获取AppToken
3. 在 **用户管理** 页面点击"配置推送"，填写WxPusher Token
4. 在房间配置中填写推送UID（微信UID），选择推送类型：
   - 开播提醒
   - 上传完成通知
   - 投稿成功通知

> 注意：每个B站用户可以配置自己的WxPusher Token，实现个性化推送

## 使用指南

### 工作原理

1. **录播软件录制** - 录播姬/blrec监控直播并录制视频文件到指定目录
2. **自动扫盘入库** - GoBup定时扫描录制目录，自动发现并入库新文件
3. **自动上传** - 根据房间配置，自动上传录制文件到B站
4. **自动投稿** - 根据房间的自动投稿设置，上传完成后自动提交投稿
5. **消息推送** - 完成后通过WxPusher推送通知（如已配置）

> 关键提示：录播姬和本项目必须能访问同一个文件路径（Docker部署需映射同一宿主机目录）

## 模板变量

### 标题/简介模板

在标题和简介模板中可以使用以下变量：

| 变量 | 说明 | 示例 |
|------|------|------|
| `${uname}` | 主播名称 | 某某主播 |
| `${title}` | 直播标题 | 今日直播 |
| `${roomId}` | 房间ID | 123456 |
| `${areaName}` | 分区名称 | 网络游戏 |
| `${yyyy年MM月dd日HH点mm分}` | 完整日期时间 | 2025年12月30日20点30分 |
| `${MM月dd日HH点mm分}` | 简短日期时间 | 12月30日20点30分 |
| `${@uid}` | @用户格式 | @uid:123456 |

#### 示例

```
标题模板: ${uname}的${areaName}直播录像 ${MM月dd日HH点mm分}
结果: 某某主播的网络游戏直播录像 12月30日20点30分

简介模板: 
主播：${uname}
直播间：https://live.bilibili.com/${roomId}
录制时间：${yyyy年MM月dd日HH点mm分}
```

### 分P标题模板

当启用分P上传时，每个分P的标题可以使用：

| 变量 | 说明 |
|------|------|
| `${index}` | 分P序号（从1开始） |
| `${MM月dd日HH点mm分}` | 日期时间 |
| `${areaName}` | 分区名称 |

#### 示例

```
分P标题模板: P${index} ${MM月dd日HH点mm分}
结果: P1 12月30日20点30分
```

## 项目结构

```
gobup/
├── server/                 # 后端服务
│   ├── main.go            # 程序入口
│   ├── go.mod             # Go依赖管理
│   ├── Dockerfile         # 后端Docker配置
│   └── internal/          # 内部包
│       ├── bili/          # B站API客户端
│       │   ├── auth.go    # 认证相关
│       │   ├── client.go  # HTTP客户端
│       │   ├── upload_app.go   # App上传
│       │   ├── upload_kodo.go  # Kodo上传
│       │   └── upload_upos.go  # Upos上传
│       ├── config/        # 配置管理
│       ├── controllers/   # HTTP控制器
│       ├── database/      # 数据库操作
│       ├── middleware/    # 中间件
│       ├── models/        # 数据模型
│       ├── routes/        # 路由配置
│       ├── scheduler/     # 定时任务
│       ├── services/      # 业务服务
│       ├── upload/        # 上传服务
│       └── webhook/       # Webhook处理
├── web/                   # 前端界面
│   ├── src/
│   │   ├── views/         # 页面组件
│   │   ├── api/           # API调用
│   │   ├── router/        # 路由配置
│   │   └── App.vue        # 根组件
│   ├── package.json       # npm依赖
│   ├── vite.config.js     # Vite配置
│   ├── Dockerfile         # 前端Docker配置
│   └── nginx.conf         # Nginx配置
└── README.md              # 本文档
```

## 致谢

- [FQrabbit/biliupforjava](https://github.com/FQrabbit/biliupforjava) - 功能参考
- [mwxmmy/biliupforjava](https://github.com/mwxmmy/biliupforjava) - 原始项目
- [spiritLHLS/LotteryAutoScript_Station](https://github.com/spiritLHLS/LotteryAutoScript_Station) - 相关项目
- [BililiveRecorder](https://rec.danmuji.org/) - 录播姬
- [blrec](https://github.com/acgnhiki/blrec) - 录播工具
