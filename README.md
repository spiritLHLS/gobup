# GoBup - B站录播自动上传工具

一个用Go语言实现的B站录播自动上传工具

## 快速开始

### Docker运行

这是最简单的运行方式：

#### 基础运行（无密码）

```bash
docker pull your-registry/gobup:latest

docker run -d \
  --name gobup \
  -p 12380:12380 \
  -v /path/to/recordings:/rec \
  -v /path/to/data:/app/data \
  --restart unless-stopped \
  your-registry/gobup:latest
```

#### 完整配置运行

```bash
docker run -d \
  --name gobup \
  -p 12380:12380 \
  -v /path/to/recordings:/rec \
  -v /path/to/data:/app/data \
  -e WXPUSH_TOKEN=your_wxpusher_token \
  -e USERNAME=admin \
  -e PASSWORD=your_password \
  --restart unless-stopped \
  your-registry/gobup:latest
```

### Docker Compose

创建 `docker-compose.yml`：

```yaml
version: '3'
services:
  gobup:
    image: your-registry/gobup:latest
    container_name: gobup
    ports:
      - "12380:12380"
    volumes:
      - /path/to/recordings:/rec
      - ./data:/app/data
    environment:
      - WXPUSH_TOKEN=your_wxpusher_token
      - USERNAME=admin
      - PASSWORD=your_password
    restart: unless-stopped
```

运行：

```bash
docker-compose up -d
```

### 容器参数说明

| 类型 | 参数 | 说明 |
|------|------|------|
| 端口映射 | `-p 12380:12380` | 映射Web管理界面端口 |
| 存储卷 | `-v /path/to/recordings:/rec` | 挂载录制文件目录（必须与录播姬一致） |
| 存储卷 | `-v /path/to/data:/app/data` | 挂载数据目录（数据库和配置文件） |
| 环境变量 | `-e WXPUSH_TOKEN` | WxPusher AppToken（可选） |
| 环境变量 | `-e USERNAME` | Web管理界面登录用户名（可选） |
| 环境变量 | `-e PASSWORD` | Web管理界面登录密码（可选） |
| 重启策略 | `--restart unless-stopped` | 容器异常退出时自动重启 |

> 重要提示：`/path/to/recordings` 必须和录播姬的录制目录保持一致

访问 Web 界面：`http://localhost:12380`

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
3. 启动程序时设置 `-wxpush-token` 参数或环境变量
4. 在房间配置中填写推送UID，选择推送类型：
   - 开播提醒
   - 上传完成通知
   - 投稿成功通知

## 使用指南

### 工作原理

1. **录播软件录制** - 录播姬/blrec监控直播并录制视频文件
2. **Webhook通知** - 录制完成后发送Webhook到GoBup（携带文件路径）
3. **自动处理** - GoBup接收事件，读取房间配置
4. **上传投稿** - 根据配置自动上传视频并投稿到B站
5. **消息推送** - 完成后通过WxPusher推送通知（如已配置）

> 关键提示：录播姬和本项目必须能访问同一个文件路径（Docker部署需映射同一宿主机目录）

### 工作流程

1. **录播软件录制** - 录播姬/blrec录制直播并保存视频文件
2. **Webhook通知** - 录制完成后发送Webhook到GoBup
3. **自动处理** - GoBup接收事件，读取房间配置
4. **上传投稿** - 根据配置自动上传视频并投稿到B站
5. **消息推送** - 完成后通过WxPusher推送通知（如已配置）

### 用户管理

#### 添加用户
1. Web界面 -> 用户管理 -> 添加用户
2. 点击"生成登录二维码"
3. 使用B站App扫码登录

#### 刷新Cookie
- Cookie过期时会自动刷新
- 也可手动点击"刷新Cookie"按钮

#### 查看用户信息
- 显示用户名、UID
- Cookie状态和过期时间
- 上次刷新时间

### 房间管理

#### 添加房间
配置要上传的直播间信息和上传参数。

#### 编辑房间
修改房间配置，立即生效。

#### 删除房间
删除房间配置，不影响已上传的视频。

### 历史记录

查看所有上传历史，包括：
- 录制时间
- 视频标题
- 上传状态
- 投稿链接
- 重新投稿功能

### 配置导入导出

#### 导出配置
1. Web界面 -> 配置管理 -> 导出配置
2. 下载JSON文件，包含所有房间和用户配置

#### 导入配置
1. Web界面 -> 配置管理 -> 导入配置
2. 选择之前导出的JSON文件
3. 确认导入，会覆盖现有配置

## API接口

### Webhook接收

```http
POST /api/recordWebHook
Content-Type: application/json

{
  "EventType": "SessionEnded",
  "EventData": {
    "RoomId": 123456,
    "Name": "主播名称",
    "Title": "直播标题",
    "RelativePath": "录播文件路径",
    "FileSize": 123456789
  }
}
```

### 房间管理

```http
# 获取房间列表
POST /api/room

# 添加房间
POST /api/room/add
Content-Type: application/json
{
  "room_id": 123456,
  "bili_user_id": 1,
  "tid": 171,
  "title": "${uname}的直播间录像",
  ...
}

# 更新房间
POST /api/room/update

# 删除房间
GET /api/room/delete/:id
```

### 用户管理

```http
# 获取用户列表
GET /api/biliUser/list

# 生成登录二维码
GET /api/biliUser/login

# 轮询登录状态
GET /api/biliUser/loginReturn?key=xxx

# 刷新Cookie
GET /api/biliUser/refresh/:id

# 删除用户
GET /api/biliUser/delete/:id
```

### 历史记录

```http
# 获取历史记录
POST /api/history/list

# 重新投稿
GET /api/history/rePublish/:id
```

### 配置管理

```http
# 导出配置
POST /api/config/export

# 导入配置
POST /api/config/import
```

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