# GoBup - B站录播自动上传工具

[![Build and Release](https://github.com/spiritlhls/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml)

一个用Go语言实现的B站录播自动上传工具，支持自动上传录播文件到B站，支持多账号管理、WxPusher消息推送等功能。

## 快速部署

### 方式一：使用预构建 Docker 镜像

使用已构建好的多架构镜像，会自动根据当前系统架构下载对应版本。

**镜像标签说明：**

| 镜像标签 | 说明 | 用途 |
|----------|------|------|
| `spiritlhls/gobup:latest` | 最新版本 | 快速部署 |
| `spiritlhl/gobup:YYYYMMDD` | 特定日期版本 | 需要固定版本 |

所有镜像均支持 `linux/amd64` 和 `linux/arm64` 架构。

#### 基础运行（无密码）

```bash
docker pull spiritlhl/gobup:latest

docker run -d \
  --name gobup \
  -p 80:80 \
  -p 12380:12380 \
  -v /path/to/recordings:/rec \
  -v /path/to/data:/app/data \
  --restart unless-stopped \
  spiritlhl/gobup:latest
```

或者使用 GitHub Container Registry：

```bash
docker pull ghcr.io/spiritlhl/gobup:latest

docker run -d \
  --name gobup \
  -p 80:80 \
  -p 12380:12380 \
  -v /path/to/recordings:/rec \
  -v /path/to/data:/app/data \
  --restart unless-stopped \
  ghcr.io/spiritlhl/gobup:latest
```

#### 完整配置运行（有密码）

```bash
docker run -d \
  --name gobup \
  -p 80:80 \
  -p 12380:12380 \
  -v /path/to/recordings:/rec \
  -v /path/to/data:/app/data \
  -e USERNAME=admin \
  -e PASSWORD=your_password \
  --restart unless-stopped \
  spiritlhl/gobup:latest
```

> 注意：USERNAME 和 PASSWORD 仅用于首次启动时创建管理员账户，后续修改环境变量不会更新已存在的账户

### 方式二：使用 Docker Compose

创建 `docker-compose.yml`：

```yaml
version: '3.8'

services:
  gobup:
    image: spiritlhl/gobup:latest
    container_name: gobup
    restart: unless-stopped
    ports:
      - "80:80"
      - "12380:12380"
    volumes:
      - ./recordings:/rec
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

### 方式三：自己编译打包

如果需要修改源码或自定义构建：

```bash
git clone https://github.com/spiritlhl/gobup.git
cd gobup
docker build -t gobup .
docker run -d \
  --name gobup \
  -p 80:80 \
  -p 12380:12380 \
  -v /path/to/recordings:/rec \
  -v /path/to/data:/app/data \
  --restart unless-stopped \
  gobup
```

### 方式四：下载发布版本手动部署

从 [Releases](https://github.com/spiritlhls/gobup/releases) 页面下载对应平台的二进制文件：

**独立部署版本（需要分别部署前后端）：**
- `gobup-server-linux-amd64.tar.gz` - 后端 Linux AMD64 版本
- `gobup-server-linux-arm64.tar.gz` - 后端 Linux ARM64 版本
- `gobup-server-darwin-amd64.tar.gz` - 后端 macOS Intel 版本
- `gobup-server-darwin-arm64.tar.gz` - 后端 macOS Apple Silicon 版本
- `gobup-server-windows-amd64.zip` - 后端 Windows AMD64 版本
- `web-dist.zip` - 前端静态文件

解压后运行：

```bash
# Linux/macOS
tar -xzf gobup-server-linux-amd64.tar.gz
./gobup-server-linux-amd64 -port 12380 -work-path /path/to/recordings

# Windows
# 解压 gobup-server-windows-amd64.zip
gobup-server-windows-amd64.exe -port 12380 -work-path C:\path\to\recordings
```

### 容器参数说明

| 类型 | 参数 | 说明 |
|------|------|------|
| 端口映射 | `-p 80:80` | 映射Web管理界面端口（Nginx） |
| 端口映射 | `-p 12380:12380` | 映射后端API端口 |
| 存储卷 | `-v /path/to/recordings:/rec` | 挂载录制文件目录（必须与录播姬一致） |
| 存储卷 | `-v /path/to/data:/app/data` | 挂载数据目录（数据库和配置文件） |
| 环境变量 | `-e USERNAME` | 初始管理员用户名（可选，仅首次启动时有效） |
| 环境变量 | `-e PASSWORD` | 初始管理员密码（可选，仅首次启动时有效） |
| 环境变量 | `-e TZ` | 时区设置，默认 Asia/Shanghai |
| 重启策略 | `--restart unless-stopped` | 容器异常退出时自动重启 |

> 重要提示：`/path/to/recordings` 必须和录播姬的录制目录保持一致

访问 Web 界面：
- 使用 Docker 镜像：`http://localhost` 或 `http://localhost:80`
- 使用二进制文件：`http://localhost:12380`

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

### 配置录播软件Webhook

在录播姬中配置Webhook地址：

| 录播软件 | Webhook地址 |
|---------|-------------|
| BililiveRecorder | `http://gobup/api/recordWebHook` 或 `http://192.168.x.x:12380/api/recordWebHook` |
| blrec | `http://gobup/api/recordWebHook` 或 `http://192.168.x.x:12380/api/recordWebHook` |

> 重要提示：
> - 使用容器名称 `gobup`（需配置Docker网络）
> - 或使用局域网IP：`http://192.168.x.x:12380/api/recordWebHook`
> - 不要使用 `localhost` 或 `127.0.0.1`
> - 不要使用容器内部IP

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
- **上传线路**: upos/kodo/app，建议upos
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

1. **录播软件录制** - 录播姬/blrec监控直播并录制视频文件
2. **Webhook通知** - 录制完成后发送Webhook到GoBup（携带文件路径）
3. **自动处理** - GoBup接收事件，读取房间配置
4. **上传投稿** - 根据配置自动上传视频并投稿到B站
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
- [BililiveRecorder](https://rec.danmuji.org/) - 录播姬
- [blrec](https://github.com/acgnhiki/blrec) - 录播工具