# GoBup - Go语言B站录播上传工具

这是一个用Go语言实现的B站录播自动上传工具，功能完全参考 [biliupforjava](https://github.com/mwxmmy/biliupforjava) 项目。

## 主要功能

- 接收录播姬(BililiveRecorder)和blrec的Webhook事件
- 自动上传录制完成的视频到B站
- 支持分P上传和自动投稿
- 提供Web管理界面
- 支持多用户和多房间管理
- 支持三种上传线路(upos/kodo/app)
- **用户扫码登录** - TV端QR code登录方式
- **Cookie自动刷新** - 自动验证和刷新用户Cookie
- **模板系统** - 自定义标题/简介/标签模板，支持变量替换
- **WxPusher推送** - 开播/上传/投稿消息推送
- **配置导入导出** - 方便备份和迁移配置
- **合集支持** - 自动添加视频到合集

## 快速开始

### 编译运行

```bash
cd server
go build -o gobup main.go
./gobup -port 12380 -work-path /rec -username admin -password 123456 -wxpush-token YOUR_WXPUSHER_TOKEN
```

### 命令行参数

- `-port`: HTTP服务端口，默认12380
- `-work-path`: 录播文件工作目录
- `-username`: 登录用户名（可选）
- `-password`: 登录密码（可选）
- `-data-path`: 数据目录，默认./data
- `-wxpush-token`: WxPusher AppToken（可选，也可通过环境变量WXPUSH_TOKEN设置）

### Docker运行

```bash
docker build -t gobup:latest .
docker run -d \
  -p 12380:12380 \
  -v /path/to/recordings:/rec \
  --name gobup \
  gobup:latest
```

## 配置录播姬Webhook

在录播姬中配置Webhook地址：

```
http://your-server-ip:12380/api/recordWebHook
```

对于blrec也是相同的配置。

## 项目结构

```
server/
├── main.go                 # 主程序入口
├── internal/
│   ├── config/            # 配置管理
│   ├── database/          # 数据库操作
│   ├── models/            # 数据模型
│   ├── bili/              # B站API客户端
│   ├── webhook/           # Webhook事件处理
│   ├── upload/            # 上传服务
│   ├── controllers/       # HTTP控制器
│   ├── routes/            # 路由配置
│   ├── middleware/        # 中间件
│   └── scheduler/         # 定时任务
└── data/                  # 数据库文件目录
```

## 技术栈

- **Web框架**: Gin
- **数据库**: SQLite (GORM)
- **定时任务**: robfig/cron
- **HTTP客户端**: net/http

## API接口

### Webhook接收
- POST `/api/recordWebHook` - 接收录播事件

### 房间管理
- POST `/api/room` - 获取房间列表
- POST `/api/room/add` - 添加房间
- POST `/api/room/update` - 更新房间配置
- GET `/api/room/delete/:id` - 删除房间

### 用户管理
- GET `/api/biliUser/list` - 获取B站用户列表
- GET `/api/biliUser/login` - 生成登录二维码
- GET `/api/biliUser/loginReturn` - 轮询登录状态
- GET `/api/biliUser/refresh/:id` - 刷新用户Cookie
- POST `/api/biliUser/update` - 更新用户信息
- GET `/api/biliUser/delete/:id` - 删除用户

### 配置管理
- POST `/api/config/export` - 导出配置
- POST `/api/config/import` - 导入配置

## 模板变量

### 标题/简介模板支持的变量

- `${uname}` - 主播名称
- `${title}` - 直播标题
- `${roomId}` - 房间ID
- `${areaName}` - 分区名称
- `${yyyy年MM月dd日HH点mm分}` - 日期时间（中文格式）
- `${MM月dd日HH点mm分}` - 简短日期时间
- `${@uid}` - @用户（uid格式）

### 分P标题模板变量

- `${index}` - 分P序号
- `${MM月dd日HH点mm分}` - 日期时间
- `${areaName}` - 分区名称

## WxPusher配置

1. 注册WxPusher账号: https://wxpusher.zjiecode.com/
2. 创建应用获取AppToken
3. 启动程序时设置 `-wxpush-token` 参数或环境变量 `WXPUSH_TOKEN`
4. 在房间配置中填写UID，选择推送类型（开播/上传/投稿）

### 历史记录
- POST `/api/history/list` - 获取录制历史
- GET `/api/history/rePublish/:id` - 重新投稿

## 开发计划

- [x] 基础架构
- [x] Webhook事件处理
- [x] 视频上传功能
- [x] 自动投稿功能
- [ ] Web前端界面
- [ ] 二维码登录
- [ ] 高能片段剪辑
- [ ] 弹幕处理
- [ ] 封面上传

## License

Apache-2.0 License

## 致谢

本项目参考了 [mwxmmy/biliupforjava](https://github.com/mwxmmy/biliupforjava)
