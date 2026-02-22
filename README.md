# GoBup - B站录播自动上传工具

[![Build and Release](https://github.com/spiritlhls/gobup/actions/workflows/main.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/main.yml)
[![Build and Push Docker Images](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml/badge.svg)](https://github.com/spiritlhls/gobup/actions/workflows/build_docker.yml)

一个用Go语言实现的B站录播自动上传工具，支持自动上传录播文件到B站，支持多账号管理、WxPusher消息推送等功能。

## 快速部署

- **Docker 部署**：使用环境变量 `USERNAME` 和 `PASSWORD`

### 方式一：使用预构建 Docker 镜像

所有镜像均支持 `linux/amd64` 和 `linux/arm64` 架构，**已内置 DanmakuFactory 专业弹幕转换工具**。

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

> 注意：
> - USERNAME 和 PASSWORD 仅用于首次启动时创建管理员账户，后续修改环境变量不会更新已存在的账户
> - 镜像已内置 DanmakuFactory，启用弹幕烧录功能可获得专业弹幕效果

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

### 方式三：从源码构建 Docker 镜像

```bash
# 克隆仓库
git clone https://github.com/spiritlhls/gobup.git
cd gobup

# 构建镜像（默认使用 Dockerfile.danmaku，包含 DanmakuFactory）
docker build -f Dockerfile.danmaku -t gobup .

# 或构建标准版（仅内置弹幕转换）
# docker build -f Dockerfile -t gobup .

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

## 致谢

- [FQrabbit/biliupforjava](https://github.com/FQrabbit/biliupforjava) - 功能参考
- [mwxmmy/biliupforjava](https://github.com/mwxmmy/biliupforjava) - 原始项目
- [spiritLHLS/LotteryAutoScript_Station](https://github.com/spiritLHLS/LotteryAutoScript_Station) - 相关项目
- [BililiveRecorder](https://rec.danmuji.org/) - 录播姬
- [blrec](https://github.com/acgnhiki/blrec) - 录播工具
- [DanmakuFactory](https://github.com/hihkm/DanmakuFactory) - 专业弹幕转换工具
- [bilibili-API-collect](https://github.com/AkagiYui/bilibili-API-collect) - API合集
