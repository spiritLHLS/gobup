# GoBup - B站录播自动上传工具

[![Build and Release](https://github.com/spiritlhls/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml)

一个用Go语言实现的B站录播自动上传工具，支持自动上传录播文件到B站，支持多账号管理、WxPusher消息推送等功能。

## 快速部署

**GoBup 使用 HTTP Basic Auth 进行身份认证**

**首次使用必须设置用户名和密码：**

1. **安装脚本部署**：使用环境变量 `GOBUP_USERNAME` 和 `GOBUP_PASSWORD`
2. **Docker 部署**：使用环境变量 `USERNAME` 和 `PASSWORD`
3. **手动部署**：使用命令行参数 `-username` 和 `-password`

**如果不设置认证信息：**
- 首次访问会进入登录页面，要求输入用户名和密码
- 输入的凭证将保存在浏览器本地，用于后续请求认证
- 建议：在部署时就设置好用户名密码，避免浏览器弹窗

**认证信息仅在首次启动时创建管理员账户使用，后续修改需要删除数据库重新初始化。**

### 方式一：一键安装脚本（推荐）

使用一键安装脚本，自动下载并安装最新版本的服务器和Web文件：

**无认证部署（首次访问需登录）：**

```bash
curl -fsSL https://raw.githubusercontent.com/spiritlhls/gobup/main/install.sh -o install.sh && chmod +x install.sh && bash install.sh
```

**带认证部署（推荐）：**

```bash
# 下载脚本
curl -fsSL https://raw.githubusercontent.com/spiritlhls/gobup/main/install.sh -o install.sh && chmod +x install.sh

# 设置用户名密码安装
GOBUP_USERNAME=admin GOBUP_PASSWORD=your_secure_password bash install.sh
```

或使用 wget：

```bash
wget -O install.sh https://raw.githubusercontent.com/spiritlhls/gobup/main/install.sh && chmod +x install.sh

# 设置用户名密码安装
GOBUP_USERNAME=root GOBUP_PASSWORD=123456 bash install.sh
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
# 完整安装（无认证，首次访问需登录）
bash install.sh

# 完整安装并设置认证（推荐）
GOBUP_USERNAME=admin GOBUP_PASSWORD=123456 bash install.sh

# 安装指定版本并设置认证
INSTALL_VERSION=v20250101-120000 GOBUP_USERNAME=root GOBUP_PASSWORD=secret bash install.sh

# 升级到最新版本
bash install.sh upgrade
```

**安装后访问：**
- Web界面: http://localhost:12380
- 如果设置了认证，使用设置的用户名密码登录
- 如果未设置认证，首次访问会要求输入用户名密码
- 服务管理: `systemctl status gobup`

### 方式二：使用预构建 Docker 镜像

使用已构建好的多架构镜像，会自动根据当前系统架构下载对应版本。

**镜像标签说明：**

| 镜像标签 | 说明 | 用途 |
|----------|------|------|
| `spiritlhls/gobup:latest` | 最新版本 | 快速部署 |
| `spiritlhl/gobup:20251230-062908` | 特定日期版本 | 需要固定版本 |

所有镜像均支持 `linux/amd64` 和 `linux/arm64` 架构。

**基础运行（无认证，首次访问需登录）：**

```bash
docker pull spiritlhl/gobup:latest

docker run -d \
  --name gobup \
  -p 22380:12380 \
  -v /path/to/recordings:/rec \
  -v /path/to/data:/app/data \
  --restart unless-stopped \
  spiritlhl/gobup:latest
```

**完整配置运行（带认证，推荐）：**

```bash
docker pull spiritlhl/gobup:latest

docker run -d \
  --name gobup \
  -p 22380:12380 \
  -v /root/recordings:/rec \
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
  -v /path/to/recordings:/rec \
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
./gobup-server-linux-amd64 -port 12380 -work-path /path/to/recordings

# Linux/macOS（带认证，推荐）
./gobup-server-linux-amd64 -port 12380 -work-path /path/to/recordings \
  -username admin -password your_password
```

```powershell
# Windows（无认证）
# 解压 gobup-server-windows-amd64.zip
gobup-server-windows-amd64.exe -port 12380 -work-path C:\path\to\recordings

# Windows（带认证，推荐）
gobup-server-windows-amd64.exe -port 12380 -work-path C:\path\to\recordings ^
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

## 认证配置

### 认证方式说明

GoBup 使用 HTTP Basic Auth 进行身份认证，保护您的管理界面和数据安全。

**首次部署时强烈建议设置用户名和密码！**

### 如何设置认证

#### 方法一：安装脚本部署

```bash
# 使用环境变量
GOBUP_USERNAME=admin GOBUP_PASSWORD=your_password bash install.sh
```

#### 方法二：Docker 部署

```bash
# Docker run
docker run -d \
  -e USERNAME=admin \
  -e PASSWORD=your_password \
  ...其他参数

# Docker Compose
environment:
  - USERNAME=admin
  - PASSWORD=your_password
```

#### 方法三：手动运行

```bash
./gobup-server -username admin -password your_password -port 12380 -work-path /path/to/recordings
```

### 常见问题

**Q: 为什么一直提示要认证？**

A: 可能的原因：
1. **未设置用户名密码** - 首次部署时没有设置 USERNAME 和 PASSWORD 环境变量
2. **浏览器未保存凭证** - 清除了浏览器缓存或使用了隐私模式
3. **凭证错误** - 输入的用户名密码与启动时设置的不一致

**Q: 如何首次登录？**

A: 
- 如果部署时设置了 USERNAME 和 PASSWORD，使用这些凭证登录
- 如果未设置，访问时会自动跳转到登录页面，输入任意用户名密码即可（建议使用强密码）

**Q: 忘记密码怎么办？**

A: 
1. 停止服务：`systemctl stop gobup` 或 `docker stop gobup`
2. 删除数据库：`rm /app/data/gobup.db` 或 `rm ./data/gobup.db`
3. 重新启动并设置新密码：
   ```bash
   # systemd
   sudo systemctl edit gobup
   # 添加：
   [Service]
   Environment="USERNAME=newadmin"
   Environment="PASSWORD=newpassword"
   sudo systemctl restart gobup
   
   # Docker
   docker rm gobup
   docker run -d -e USERNAME=newadmin -e PASSWORD=newpassword ...
   ```

**Q: 可以修改密码吗？**

A: 当前版本暂不支持在线修改密码，需要删除数据库重新初始化（会丢失所有数据）。建议部署时就设置好强密码并妥善保管。

**Q: 为什么浏览器一直弹出认证窗口？**

A: 
- 浏览器的 HTTP Basic Auth 弹窗是原生行为
- 使用新的登录页面（已在本次更新中添加）可以避免浏览器弹窗
- 确保前端代码已更新到最新版本

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
