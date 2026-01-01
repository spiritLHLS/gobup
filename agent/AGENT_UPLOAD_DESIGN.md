# Agent分布式上传方案设计

## 1. 背景与问题

### 1.1 当前问题
- B站上传接口存在风控机制，可能触发406错误
- 单一服务器IP频繁上传容易被风控系统识别
- 上传大文件需要稳定的网络环境和充足的带宽

### 1.2 解决思路
通过部署多个Agent节点，将上传任务分发到不同的Agent执行，每个Agent使用独立的IP地址进行上传，降低单IP风控风险。

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────┐
│   Server    │ (主控节点)
│  (gobup)    │
└──────┬──────┘
       │
       │ HTTP API / gRPC
       │
       ├─────────────┬─────────────┬─────────────┐
       │             │             │             │
   ┌───▼───┐    ┌───▼───┐    ┌───▼───┐    ┌───▼───┐
   │Agent 1│    │Agent 2│    │Agent 3│    │Agent N│
   │IP: A  │    │IP: B  │    │IP: C  │    │IP: N  │
   └───────┘    └───────┘    └───────┘    └───────┘
```

### 2.2 组件职责

#### 2.2.1 Server (主控节点)
- 管理录播任务和元数据
- 管理Agent节点注册和健康检查
- 分发上传任务到Agent节点
- 聚合上传进度和结果
- 提供Web管理界面

#### 2.2.2 Agent (执行节点)
- 向Server注册并维持心跳
- 接收并执行上传任务
- 从Server下载或从本地读取待上传文件
- 执行B站上传流程
- 实时上报上传进度
- 处理上传失败和重试

## 3. 核心功能模块

### 3.1 Agent注册与管理

#### 3.1.1 Agent注册信息
```
- agent_id: 唯一标识
- agent_name: 名称
- ip_address: 公网IP地址
- status: 状态 (online/offline/busy)
- capabilities: 能力标签
  - max_concurrent_uploads: 最大并发上传数
  - bandwidth_mbps: 可用带宽
  - storage_type: 存储类型 (local/network/hybrid)
- last_heartbeat: 最后心跳时间
- created_at: 注册时间
- tags: 自定义标签 (地区、运营商等)
```

#### 3.1.2 心跳机制
- Agent每30秒向Server发送心跳
- 心跳内容包含：状态、当前任务数、可用资源
- Server超过90秒未收到心跳则标记为offline

### 3.2 任务分发策略

#### 3.2.1 负载均衡策略
1. 轮询 (Round Robin): 平均分配任务
2. 最少连接 (Least Connections): 优先分配给空闲Agent
3. 加权轮询 (Weighted Round Robin): 根据Agent带宽权重分配
4. IP分散 (IP Diversity): 优先选择近期未上传的IP
5. 地理位置优先: 根据文件所在位置就近选择Agent

#### 3.2.2 风控优化策略
- 同一IP的上传任务间隔至少5分钟
- 单个IP每小时上传次数限制
- 记录每个IP的风控触发历史
- 被风控的IP自动降低优先级或暂时禁用

### 3.3 文件传输方案

基于需求：**gRPC流式传输 + Agent零存储代理上传**

#### 3.3.1 方案架构

**核心思路**
```
Server通过gRPC流式传输文件数据给Agent
Agent接收数据流的同时直接转发上传到B站
Agent不在本地存储文件（零磁盘占用）
```

**数据流向**
```
┌────────┐  gRPC Stream   ┌───────┐  HTTPS Upload  ┌──────┐
│ Server │ ─────────────> │ Agent │ ─────────────> │ B站  │
│ (文件)  │  文件数据块     │(内存)  │   同步转发      │      │
└────────┘                └───────┘                └──────┘
         ├─────────────────────────────────────────┤
                   整个过程Agent不落盘
```

#### 3.3.2 gRPC接口设计

**protobuf定义**
```protobuf
syntax = "proto3";

package agent;

// 上传服务
service UploadService {
  // 流式上传文件
  rpc StreamUpload(stream UploadRequest) returns (UploadResponse);
  
  // 获取上传进度
  rpc GetProgress(ProgressRequest) returns (ProgressResponse);
}

// 上传请求（流式消息）
message UploadRequest {
  oneof data {
    // 第一条消息：元数据
    UploadMetadata metadata = 1;
    // 后续消息：数据块
    bytes chunk = 2;
  }
}

// 上传元数据
message UploadMetadata {
  string task_id = 1;
  int64 part_id = 2;
  string filename = 3;
  int64 file_size = 4;
  string file_md5 = 5;
  
  // B站上传凭证
  BiliCredentials bili_creds = 6;
}

message BiliCredentials {
  string access_key = 1;
  string cookies = 2;
  int64 mid = 3;
  string line = 4;  // 上传线路
}

// 上传响应
message UploadResponse {
  bool success = 1;
  string message = 2;
  int64 biz_id = 3;  // B站返回的视频ID
  string error = 4;
}

// 进度请求
message ProgressRequest {
  string task_id = 1;
}

// 进度响应
message ProgressResponse {
  int64 uploaded_bytes = 1;
  int64 total_bytes = 2;
  double upload_speed = 3;  // bytes/s
  string status = 4;
}
```

#### 3.3.3 技术实现细节

**1. Server端实现**
```go
// 伪代码示例
func (s *Server) StreamUploadToAgent(taskID string, partID int64, filePath string) error {
    // 1. 连接到Agent的gRPC服务
    conn, err := grpc.Dial(agentAddr, grpc.WithTransportCredentials(...))
    client := agent.NewUploadServiceClient(conn)
    
    // 2. 创建流
    stream, err := client.StreamUpload(ctx)
    
    // 3. 发送元数据（第一条消息）
    metadata := &agent.UploadMetadata{
        TaskId: taskID,
        PartId: partID,
        Filename: filepath.Base(filePath),
        FileSize: fileSize,
        FileMd5: fileMD5,
        BiliCreds: &agent.BiliCredentials{
            AccessKey: user.AccessKey,
            Cookies: user.Cookies,
            Mid: user.Mid,
            Line: "cs_txa",
        },
    }
    stream.Send(&agent.UploadRequest{
        Data: &agent.UploadRequest_Metadata{Metadata: metadata},
    })
    
    // 4. 打开本地文件
    file, err := os.Open(filePath)
    defer file.Close()
    
    // 5. 分块读取并发送（每块4MB）
    buffer := make([]byte, 4*1024*1024)
    for {
        n, err := file.Read(buffer)
        if n > 0 {
            stream.Send(&agent.UploadRequest{
                Data: &agent.UploadRequest_Chunk{Chunk: buffer[:n]},
            })
        }
        if err == io.EOF {
            break
        }
    }
    
    // 6. 关闭发送流并等待响应
    resp, err := stream.CloseAndRecv()
    
    return err
}
```

**2. Agent端实现**
```go
// 伪代码示例
func (a *Agent) StreamUpload(stream agent.UploadService_StreamUploadServer) error {
    // 1. 接收第一条消息（元数据）
    req, err := stream.Recv()
    metadata := req.GetMetadata()
    
    // 2. 初始化B站上传器
    biliClient := bili.NewBiliClient(
        metadata.BiliCreds.AccessKey,
        metadata.BiliCreds.Cookies,
        metadata.BiliCreds.Mid,
    )
    uploader := bili.NewUposUploader(biliClient)
    
    // 3. B站预上传
    preResp, err := uploader.PreUpload(metadata.Filename, metadata.FileSize)
    
    // 4. 创建内存管道（用于数据流转）
    pipeReader, pipeWriter := io.Pipe()
    
    // 5. 启动goroutine上传到B站
    uploadErr := make(chan error, 1)
    go func() {
        defer pipeReader.Close()
        // 从管道读取数据，直接上传到B站
        err := uploader.UploadStream(pipeReader, metadata.FileSize, preResp)
        uploadErr <- err
    }()
    
    // 6. 接收gRPC数据流并写入管道
    go func() {
        defer pipeWriter.Close()
        for {
            req, err := stream.Recv()
            if err == io.EOF {
                break
            }
            chunk := req.GetChunk()
            pipeWriter.Write(chunk)
        }
    }()
    
    // 7. 等待上传完成
    err = <-uploadErr
    
    // 8. 返回结果
    return stream.SendAndClose(&agent.UploadResponse{
        Success: err == nil,
        BizId: preResp.BizID,
        Error: err.Error(),
    })
}
```

**3. 内存缓冲区管理**
```go
// Agent端使用环形缓冲区优化
type RingBuffer struct {
    buffer []byte
    size   int
    read   int
    write  int
    mu     sync.Mutex
    cond   *sync.Cond
}

// 推荐缓冲区大小: 100MB-500MB
// - 太小: 网络波动时容易卡顿
// -太大: 占用内存过多
const BufferSize = 200 * 1024 * 1024  // 200MB
```

#### 3.3.4 容错与重试机制

**断点续传处理**
```
挑战: 流式传输无法简单断点续传
方案: 分段传输 + 状态记录

实现:
1. 大文件分段（如每段500MB）
2. Agent记录每个段的上传状态
3. 失败时只需重传失败的段

示例:
文件5GB，分为10个500MB的段
- 段1-5: 已完成
- 段6: 失败（从段6重新开始）
- 段7-10: 未开始
```

**网络中断处理**
```
场景1: Server到Agent网络中断
  - gRPC自动重连
  - 从中断的段重新传输
  
场景2: Agent到B站网络中断
  - Agent本地记录已上传偏移
  - 重新请求Server从偏移位置继续发送
  - B站上传API支持续传
  
场景3: Agent进程崩溃
  - 任务状态持久化到数据库
  - 重启后检查未完成任务
  - 重新分配给其他Agent或等待恢复
```

**流控与背压**
```
问题: Server发送速度 > Agent上传速度
解决: gRPC流控机制

gRPC自动处理:
- Agent处理慢时，自动暂停Server发送
- Agent缓冲区满时，阻塞Server的Send
- 防止内存溢出

额外优化:
- Server监听Agent的进度反馈
- 动态调整发送速率
- 匹配Agent上传到B站的速度
```

#### 3.3.5 性能优化

**1. 并发处理**
```
Agent可同时处理多个任务:
- 任务1: 接收数据 + 上传B站
- 任务2: 接收数据 + 上传B站
...

限制: 
- 根据内存限制并发数（每任务200MB缓冲）
- 根据带宽限制并发数（避免互相争抢）
- 建议: 2-3个并发任务
```

**2. 压缩传输（可选）**
```
gRPC支持内置压缩:
- 启用gzip压缩
- 适合文本/日志文件
- 视频文件已压缩，收益小

建议: 默认不启用，视频文件压缩浪费CPU
```

**3. 零拷贝优化**
```
减少内存拷贝:
- 使用io.Pipe避免中间缓冲
- 复用buffer（sync.Pool）
- 直接转发数据块
```

#### 3.3.6 时间与带宽估算

**传输时间分析**
```
文件大小: 5GB
Server到Agent带宽: 100Mbps (12.5MB/s)
Agent到B站带宽: 50Mbps (6.25MB/s)

瓶颈分析:
- Agent上传B站更慢（6.25MB/s < 12.5MB/s）
- 总时间取决于Agent到B站的速度
- 预计耗时: 5000MB / 6.25MB/s ≈ 800秒 ≈ 13分钟

Server到Agent的传输:
- 速度匹配Agent上传速度（6.25MB/s）
- gRPC流控自动调节
- 不会产生大量积压
```

**内存占用**
```
单任务内存占用:
- gRPC缓冲: ~20MB
- 应用缓冲区: 200MB
- 其他开销: ~30MB
- 总计: ~250MB

Agent同时处理3个任务:
- 总内存: 750MB
- 建议Agent机器至少2GB内存
```

#### 3.3.7 安全考虑

**1. 传输加密**
```
gRPC + TLS:
- Server和Agent之间TLS加密
- 使用证书认证
- 防止中间人攻击
```

**2. 认证授权**
```
- Agent注册时获取Token
- gRPC metadata携带Token
- Server验证Token合法性
```

**3. 敏感信息保护**
```
- BiliCredentials在传输中加密
- Agent接收后仅保存在内存
- 任务完成后立即清理
```

### 3.4 上传流程

#### 3.4.1 完整流程
```
1. Server接收上传请求
2. Server选择合适的Agent
3. Server创建上传任务并分发给Agent
4. Agent确认任务并准备文件
   4.1 如果需要，从Server/对象存储下载文件
   4.2 验证文件完整性 (MD5/SHA256)
5. Agent执行B站上传流程
   5.1 预上传 (获取上传凭证)
   5.2 分片上传
   5.3 完成上传
6. Agent实时上报进度给Server
7. Agent上报最终结果
8. Server更新任务状态
9. 如果失败，根据策略重试或分配给其他Agent
```

#### 3.4.2 进度同步
- Agent每上传10%或每10秒上报一次进度
- 进度信息包含：
  - uploaded_bytes: 已上传字节数
  - total_bytes: 总字节数
  - upload_speed: 当前速度
  - eta: 预计剩余时间

### 3.5 失败处理与重试

#### 3.5.1 失败类型
- 网络错误: 超时、连接断开
- 风控错误: 406错误
- 文件错误: 文件损坏、MD5不匹配
- Agent错误: Agent崩溃、失联

#### 3.5.2 重试策略
- 普通网络错误: 当前Agent重试3次
- 风控错误: 立即切换到其他IP的Agent
- Agent失联: 重新分配任务给其他Agent
- 重试间隔: 指数退避 (1s, 2s, 4s, 8s...)

## 4. 数据模型

### 4.1 数据库表设计

#### 4.1.1 agents表
```sql
CREATE TABLE agents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT UNIQUE NOT NULL,
    agent_name TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    status TEXT NOT NULL,  -- online/offline/busy
    max_concurrent_uploads INTEGER DEFAULT 2,
    bandwidth_mbps INTEGER DEFAULT 100,
    storage_type TEXT DEFAULT 'network',
    last_heartbeat TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tags TEXT,  -- JSON格式的标签
    metadata TEXT  -- JSON格式的扩展信息
);
```

#### 4.1.2 agent_tasks表
```sql
CREATE TABLE agent_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT UNIQUE NOT NULL,
    agent_id TEXT NOT NULL,
    part_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_md5 TEXT,
    status TEXT NOT NULL,  -- pending/downloading/uploading/completed/failed
    progress REAL DEFAULT 0,
    upload_speed INTEGER DEFAULT 0,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id),
    FOREIGN KEY (part_id) REFERENCES record_history_parts(id)
);
```

#### 4.1.3 agent_upload_logs表
```sql
CREATE TABLE agent_upload_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- start/progress/success/error/retry
    message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (task_id) REFERENCES agent_tasks(task_id)
);
```

## 5. API接口设计

### 5.1 gRPC接口（主要通信）

基于3.3.2节的protobuf定义，gRPC接口包括：

1. **StreamUpload** - 流式上传文件到Agent
2. **GetProgress** - 查询上传进度
3. **CancelTask** - 取消任务
4. **GetAgentStatus** - 获取Agent状态

gRPC端口：12382（建议）

### 5.2 Agent管理API（HTTP RESTful）

#### 5.2.1 注册Agent
```
POST /api/agent/register
Request:
{
  "agent_id": "agent-001",
  "agent_name": "Tokyo-Agent-1",
  "ip_address": "1.2.3.4",
  "max_concurrent_uploads": 2,
  "bandwidth_mbps": 500,
  "tags": {
    "region": "tokyo",
    "isp": "aws"
  },
  "grpc_address": "agent-001.example.com:12382"
}

Response:
{
  "code": 0,
  "message": "success",
  "data": {
    "agent_id": "agent-001",
    "token": "eyJhbGc..."  // 用于gRPC认证
  }
}
```

#### 5.2.2 心跳
```
POST /api/agent/heartbeat
Headers:
  Authorization: Bearer {token}

Request:
{
  "agent_id": "agent-001",
  "status": "online",
  "current_tasks": 1,
  "available_slots": 1,
  "memory_usage_mb": 650,
  "tasks_completed": 156
}

Response:
{
  "code": 0,
  "message": "success"
}
```

#### 5.2.3 任务状态上报
```
POST /api/agent/task-status
Request:
{
  "task_id": "task-123",
  "status": "uploading",
  "uploaded_bytes": 104857600,
  "total_bytes": 524288000,
  "upload_speed": 6291456,
  "error_message": null
}

Response:
{
  "code": 0,
  "message": "success"
}
```

### 5.3 管理端API

#### 5.3.1 Agent列表
```
GET /api/manage/agents

Response:
{
  "code": 0,
  "data": {
    "agents": [
      {
        "agent_id": "agent-001",
        "agent_name": "Tokyo-Agent-1",
        "ip_address": "1.2.3.4",
        "grpc_address": "agent-001.example.com:12382",
        "status": "online",
        "current_tasks": 1,
        "total_tasks": 156,
        "success_rate": 98.5,
        "memory_usage_mb": 650,
        "last_heartbeat": "2025-12-31T10:00:00Z"
      }
    ]
  }
}
```

#### 5.3.2 手动分配任务
```
POST /api/manage/assign-task
Request:
{
  "part_id": 456,
  "agent_id": "agent-001"
}

Response:
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": "task-123"
  }
}
```

#### 5.3.3 取消任务
```
POST /api/manage/cancel-task
Request:
{
  "task_id": "task-123"
}

Response:
{
  "code": 0,
  "message": "Task cancelled"
}
```

## 6. 安全设计

### 6.1 认证授权
- Agent注册需要预共享密钥 (PSK)
- 注册后颁发JWT Token
- 所有API请求需要Token认证
- Token定期刷新，过期后需要重新认证

### 6.2 传输安全
- Agent与Server之间使用HTTPS/TLS加密
- 文件传输支持加密传输
- 敏感信息 (access_key, cookies) 加密存储

### 6.3 访问控制
- Agent只能访问分配给自己的任务
- Agent不能查看其他Agent的信息
- 管理API需要额外的管理员权限

## 7. 监控与运维

### 7.1 监控指标
- Agent状态: 在线/离线/繁忙
- 任务统计: 成功率、失败率、平均耗时
- IP风控统计: 每个IP的风控触发次数
- 上传速度: 实时速度、平均速度
- 带宽使用: 每个Agent的带宽占用

### 7.2 告警规则
- Agent离线超过5分钟
- 任务失败率超过10%
- IP触发风控
- 上传速度异常低

### 7.3 日志记录
- Agent操作日志
- 任务执行日志
- 错误日志
- 审计日志

## 8. 实施步骤

### 8.1 第一阶段：基础框架
1. 设计并实现数据模型
2. 实现Agent注册和心跳机制
3. 实现基础的任务分发逻辑
4. 开发Agent端基础程序框架

### 8.2 第二阶段：文件传输
1. 实现HTTP文件传输
2. 支持断点续传
3. 文件完整性校验

### 8.3 第三阶段：上传功能
1. Agent端集成B站上传逻辑
2. 实现进度上报
3. 实现失败重试

### 8.4 第四阶段：优化与增强
1. 实现智能调度算法
2. 添加风控策略
3. 完善监控告警
4. 开发管理界面

### 8.5 第五阶段：测试与部署
1. 单元测试
2. 集成测试
3. 压力测试
4. 生产环境部署

## 9. 部署方案

### 9.1 Server部署
```
现有gobup Server，增加Agent管理模块
- 端口: 12380 (现有) + 12381 (Agent通信)
- 数据库: SQLite (现有)
- 依赖: 无额外依赖
```

### 9.2 Agent部署

#### 9.2.1 Docker部署（推荐）
```bash
docker run -d \
  --name gobup-agent \
  -e SERVER_URL=server.example.com:12382 \
  -e AGENT_ID=agent-001 \
  -e AGENT_NAME="Tokyo-Agent-1" \
  -e AGENT_TOKEN=your-pre-shared-key \
  -e MAX_CONCURRENT=2 \
  -e MEMORY_LIMIT=2G \
  -p 12382:12382 \
  gobup/agent:latest
```

#### 9.2.2 二进制部署
```bash
./gobup-agent \
  --server-url server.example.com:12382 \
  --agent-id agent-001 \
  --agent-name "Tokyo-Agent-1" \
  --token your-pre-shared-key \
  --max-concurrent 2 \
  --grpc-port 12382
```

#### 9.2.3 配置说明
```yaml
# agent.yml
server:
  url: server.example.com:12382
  tls: true
  cert_file: /path/to/cert.pem

agent:
  id: agent-001
  name: Tokyo-Agent-1
  token: your-pre-shared-key
  max_concurrent_uploads: 2
  buffer_size_mb: 200
  heartbeat_interval: 30s
  
resources:
  max_memory_mb: 2048
  
tags:
  region: tokyo
  isp: aws
```

### 9.3 多Agent配置示例
```yaml
# 多个Agent配置示例
agents:
  - id: agent-tokyo-1
    name: "Tokyo Agent 1"
    server_url: server.example.com:12382
    max_concurrent: 2
    buffer_size_mb: 200
    tags:
      region: tokyo
      isp: aws
      priority: high
      
  - id: agent-osaka-1
    name: "Osaka Agent 1"
    server_url: server.example.com:12382
    max_concurrent: 1
    buffer_size_mb: 150
    tags:
      region: osaka
      isp: vultr
      priority: medium

  - id: agent-backup-1
    name: "Backup Agent 1"
    server_url: server.example.com:12382
    max_concurrent: 1
    buffer_size_mb: 100
    tags:
      region: hongkong
      isp: digitalocean
      priority: low
```

### 9.4 网络配置建议

#### 9.4.1 端口规划
```
Server:
  - 12380: Web UI / HTTP API（现有）
  - 12382: gRPC Agent通信（新增）

Agent:
  - 无需开放端口（主动连接Server）
  - 或开放12382用于Server主动推送（可选）
```

#### 9.4.2 防火墙配置
```bash
# Server端
ufw allow 12380/tcp  # Web UI
ufw allow 12382/tcp  # gRPC

# Agent端（如果Server需要主动连接）
ufw allow from {SERVER_IP} to any port 12382
```

## 10. 成本与收益分析

### 10.1 开发成本
- Server端改造: 3-5人天
- Agent端开发: 5-7人天
- 测试与调试: 3-5人天
- 总计: 11-17人天

### 10.2 部署成本
- Agent服务器: 根据需要，建议3-5台
- 带宽成本: 根据上传量计算
- 对象存储 (可选): 根据使用量计算

### 10.3 收益
- 显著降低风控触发概率
- 提高上传成功率
- 支持更大规模的上传任务
- 提升系统可靠性和容错能力

## 11. 风险与挑战

### 11.1 技术风险
- Agent与Server的网络连接稳定性
- 大文件传输的带宽消耗
- 并发控制的复杂度

### 11.2 业务风险
- B站可能加强风控策略
- 需要维护多个Agent节点
- IP资源的获取和管理

### 11.3 缓解措施
- 实现健壮的错误处理和重试机制
- 提供详细的监控和日志
- 支持灵活的配置和调整
- 保持与现有上传方式的兼容
