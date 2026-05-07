# EverySync - 云盘同步工具设计文档

## 项目概述

一个类似 CloudDrive 的文件同步工具，支持本地与 WebDAV 之间的双向/单向同步，具备文件变更增量检测、智能冲突解决、按需访问等能力。采用插件化架构，预留后续多云盘接入扩展。

**核心特性**：
- 本地与 WebDAV 双向/单向同步
- 文件变更增量同步（inotify + ETag/CTag）
- 多种同步模式（mirror / selective / virtual）
- 插件化存储后端（V1: Local + WebDAV，后续扩展 S3/OneDrive 等）
- 多线程高性能（Worker Pool + 分块传输）
- CLI 后台服务 + Web 管理界面（前后端分离）
- 优先支持 Linux 平台

---

## 一、整体架构

```
┌─────────────────────────────────────────────────┐
│                   Web UI (React)                 │
│              配置管理 / 状态监控 / 日志             │
├─────────────────────────────────────────────────┤
│                  REST API (Hertz)                │
├─────────────────────────────────────────────────┤
│              Sync Engine (核心调度)                │
│  ┌──────────┐ ┌───────────┐ ┌────────────────┐  │
│  │ 事件监听  │ │ 冲突解决器 │ │  任务调度器     │  │
│  │ (inotify) │ │           │ │  (worker pool) │  │
│  └──────────┘ └───────────┘ └────────────────┘  │
├─────────────────────────────────────────────────┤
│          Storage Provider Interface              │
│  ┌─────────┐ ┌──────────┐ ┌───────┐ ┌───────┐  │
│  │ LocalFS │ │  WebDAV  │ │  S3   │ │ ...   │  │
│  └─────────┘ └──────────┘ └───────┘ └───────┘  │
├─────────────────────────────────────────────────┤
│              SQLite (元数据 & 索引)               │
└─────────────────────────────────────────────────┘
```

### 技术栈

| 层级 | 技术选型 | 理由 |
|------|---------|------|
| 语言 | Go 1.22+ | 原生并发、单二进制部署、系统级能力 |
| Web 框架 | Hertz (CloudWeGo) | 高性能 HTTP 框架 |
| 前端 | React 18 + Vite + Ant Design | 前后端分离，独立部署 |
| 元数据 | SQLite (go-sqlite3) | 嵌入式、零部署、事务支持 |
| 文件监听 | fsnotify (inotify) | Linux 原生文件变更事件 |
| WebDAV 客户端 | studio-b12/gowebdav | 成熟的 Go WebDAV 库 |
| 日志 | zerolog | 高性能结构化日志 |
| 配置 | Viper | 多格式配置支持 (YAML/TOML/JSON) |
| 进程管理 | systemd unit | Linux 标准服务管理 |

---

## 二、核心数据模型

### 文件索引表

```sql
CREATE TABLE file_entries (
    id          INTEGER PRIMARY KEY,
    path        TEXT NOT NULL,
    sync_pair_id INTEGER NOT NULL,
    local_hash  TEXT,
    remote_hash TEXT,
    local_mtime DATETIME,
    remote_mtime DATETIME,
    local_size  INTEGER,
    remote_size INTEGER,
    sync_state  TEXT DEFAULT 'synced',   -- synced/pending/conflict/deleted
    version     INTEGER DEFAULT 1,
    UNIQUE(path, sync_pair_id)
);
```

### 同步对配置

```sql
CREATE TABLE sync_pairs (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    local_path  TEXT NOT NULL,
    remote_path TEXT NOT NULL,
    provider    TEXT DEFAULT 'webdav',
    mode        TEXT DEFAULT 'mirror',   -- mirror/selective/virtual
    direction   TEXT DEFAULT 'both',     -- up/down/both
    enabled     BOOLEAN DEFAULT TRUE,
    schedule    TEXT                     -- cron 表达式，空则为实时
);
```

### 变更事件日志

```sql
CREATE TABLE change_events (
    id          INTEGER PRIMARY KEY,
    file_id     INTEGER REFERENCES file_entries(id),
    source      TEXT NOT NULL,           -- local/remote
    event_type  TEXT NOT NULL,           -- create/modify/delete/rename
    timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP,
    processed   BOOLEAN DEFAULT FALSE
);
```

---

## 三、同步引擎

### 同步方向

每个同步对独立配置同步方向：

| direction | 行为 | 适用场景 |
|-----------|------|---------|
| `up` | 仅本地 → 远程 | 备份本地文件到云盘 |
| `down` | 仅远程 → 本地 | 从云盘下载到本地 |
| `both` | 双向同步 | 多设备文件同步 |

### 同步模式

| 模式 | 行为 | 适用场景 |
|------|------|---------|
| `mirror` | 双向全量镜像，本地与远程保持一致 | NAS 备份、多设备同步 |
| `selective` | 只同步指定规则的文件/目录 | 只同步文档、排除缓存目录 |
| `virtual` | 文件不实际下载，访问时按需拉取 | 云盘挂载、节省本地空间 |

### 同步引擎工作流

```
                        ┌─────────────┐
                        │  事件源接入   │
                        └──────┬──────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
        ┌──────────┐   ┌──────────┐     ┌──────────┐
        │ inotify   │   │ 轮询扫描  │     │ WebDAV   │
        │ 本地监听   │   │ 定时全量  │     │ CTag/ETag│
        └─────┬─────┘   └─────┬────┘     └─────┬────┘
              │               │                │
              └───────────────┼────────────────┘
                              ▼
                     ┌───────────────┐
                     │  事件归一化    │  ← 统一为 ChangeEvent
                     └───────┬───────┘
                             ▼
                     ┌───────────────┐
                     │  方向过滤      │  ← 根据 direction 过滤事件
                     └───────┬───────┘
                             ▼
                     ┌───────────────┐
                     │  变更比对器    │  ← 对比 local/remote 状态
                     └───────┬───────┘
                             ▼
                     ┌───────────────┐
                     │  冲突检测      │  ← 双方都改了同一文件？
                     └───────┬───────┘
                       ┌─────┴─────┐
                       ▼           ▼
                  无冲突         有冲突
                       │           │
                       ▼           ▼
               ┌────────────┐ ┌──────────┐
               │ Worker Pool │ │ 冲突策略  │
               │  执行同步   │ │ 自动解决  │
               └────────────┘ │ 或标记    │
                              └──────────┘
```

### 增量同步机制

- **本地**：`fsnotify` 实时监听 inotify 事件，只推送变更文件
- **远程**：WebDAV 通过 `CTag` 检测目录变化，`ETag` 检测文件变化，只拉取变更文件
- **hash 算法**：`xxhash`（比 MD5/SHA 快 5-10 倍），大文件分块计算

---

## 四、Storage Provider 插件化设计

### Provider 接口

```go
// Provider 是所有存储后端必须实现的接口
type Provider interface {
    // 生命周期
    Init(config ProviderConfig) error
    Close() error

    // 文件操作
    GetFile(ctx context.Context, remotePath string) (io.ReadCloser, *FileMeta, error)
    PutFile(ctx context.Context, remotePath string, reader io.Reader, meta *FileMeta) error
    DeleteFile(ctx context.Context, remotePath string) error
    MoveFile(ctx context.Context, src, dst string) error

    // 目录操作
    ListDir(ctx context.Context, remotePath string) ([]*FileMeta, error)
    CreateDir(ctx context.Context, remotePath string) error

    // 增量检测（可选，不支持则返回 ErrNotSupported）
    WatchChanges(ctx context.Context, remotePath string) (<-chan ChangeEvent, error)
    GetChangeToken(ctx context.Context, remotePath string) (string, error)
}

// FileMeta 统一的文件元信息
type FileMeta struct {
    Path     string
    Size     int64
    ModTime  time.Time
    ETag     string
    Hash     string
    IsDir    bool
}

// ProviderConfig 通用配置
type ProviderConfig struct {
    Type   string            `yaml:"type"`
    Params map[string]string `yaml:"params"`
}
```

### Provider 注册机制

```go
var registry = map[string]ProviderFactory{
    "local":   NewLocalProvider,
    "webdav":  NewWebDAVProvider,
    // 后续扩展：
    // "s3":           NewS3Provider,
    // "onedrive":     NewOneDriveProvider,
    // "aliyundrive":  NewAliyunDriveProvider,
}

func RegisterProvider(name string, factory ProviderFactory) {
    registry[name] = factory
}
```

### 扩展路径

```
V1: local + webdav           ← 最小可用
V2: + S3 (MinIO/OSS/COS)     ← 对象存储生态
V3: + OneDrive / Google Drive ← 主流商业云盘
V4: + 阿里云盘 / 百度网盘      ← 国内云盘
```

---

## 五、双向同步与冲突解决

### 冲突检测——三方时间戳 + hash 比较

```
         local_mtime    remote_mtime    last_sync_mtime
              │              │               │
              ▼              ▼               ▼
         ┌─────────────────────────────────────────┐
         │          三方时间戳 + hash 比对            │
         └─────────────────────┬───────────────────┘
                               │
          ┌────────────────────┼────────────────────┐
          ▼                    ▼                    ▼
     仅本地变更            双方都变更            仅远程变更
          │                    │                    │
          ▼                    ▼                    ▼
    推送到远程            进入冲突解决          拉取到本地
```

### 冲突解决策略（可配置）

```yaml
conflict:
  strategy: "latest_wins"    # 默认策略

  # 可选策略：
  # latest_wins   - 修改时间新的覆盖旧的
  # local_wins    - 冲突时保留本地版本
  # remote_wins   - 冲突时保留远程版本
  # keep_both     - 保留两份，冲突文件重命名
  # skip          - 跳过冲突文件，等待手动处理
```

### 冲突处理流程

```
冲突发生
    │
    ▼
自动策略判断
    │
    ├─ latest_wins → 比较 mtime，新覆盖旧
    ├─ local_wins  → 保留本地，推送覆盖远程
    ├─ remote_wins → 保留远程，拉取覆盖本地
    │
    └─ keep_both / skip → 无法自动解决
         │
         ▼
    ┌──────────────────────────┐
    │  冲突记录写入 change_events│
    │  Web UI 显示冲突列表      │
    │  用户手动选择保留哪个版本  │
    └──────────────────────────┘
```

### rename/move 处理

通过 `file_entries` 表中 `path + hash` 组合追踪文件移动——检测到旧路径删除 + 新路径出现相同 hash，识别为 rename 操作，避免重新传输。

---

## 六、多线程高性能设计

### Worker Pool 任务调度

```go
const (
    PriorityCritical = 0  // 删除操作
    PriorityHigh     = 1  // 小文件 (< 1MB)
    PriorityNormal   = 2  // 普通文件
    PriorityLow      = 3  // 大文件、hash 计算
)

type SchedulerConfig struct {
    MaxWorkers       int           // 默认 CPU * 2
    UploadWorkers    int           // 上传并发 (默认 4)
    DownloadWorkers  int           // 下载并发 (默认 8)
    QueueSize        int           // 队列容量 (默认 10000)
    RetryMax         int           // 最大重试 (默认 3)
    RetryDelay       time.Duration // 重试间隔 (默认 5s)
}
```

### 分层 Worker Pool

```
┌──────────────────────────────────────────────────┐
│                  Task Dispatcher                  │
│              (优先级队列 + 去重)                    │
└──────────┬───────────┬───────────┬───────────────┘
           │           │           │
     ┌─────▼─────┐┌────▼────┐┌────▼─────┐
     │ Upload    ││ Download ││ Local    │
     │ Workers   ││ Workers  ││ Workers  │
     │ (x4)      ││ (x8)     ││ (x2)     │
     └───────────┘└─────────┘└──────────┘
```

### 大文件分块传输

```go
type ChunkConfig struct {
    ChunkSize     int64  // 默认 8MB
    MaxConcurrent int    // 分块并发 (默认 4)
    Threshold     int64  // 启用分块阈值 (默认 16MB)
}
```

### 性能优化

| 优化项 | 方案 | 效果 |
|--------|------|------|
| 传输去重 | 相同路径 pending 任务自动合并 | 避免重复上传 |
| 断点续传 | 记录已传输 chunk offset | 网络中断不重头开始 |
| 增量 hash | 大文件只计算变更部分 hash | 减少全文件扫描 |
| 目录并发 | ListDir 结果分批分发 Worker | 全量扫描充分利用带宽 |
| 带宽控制 | 令牌桶限速，可配置上下行 | 避免占满网络 |
| 缓存策略 | LRU 缓存最近访问文件元数据 | 减少远程 API 调用 |

### 带宽控制配置

```yaml
performance:
  upload_limit: "50MB/s"
  download_limit: "100MB/s"
  scan_interval: "5m"
  max_workers: 0              # 0 = 自动 (CPU*2)
```

---

## 七、CLI 与 Web UI

### CLI 命令结构

```
every-sync
├── serve              # 启动后台服务 (daemon 模式)
│   ├── --config       # 配置文件路径
│   ├── --port         # API 端口 (默认 8080)
│   └── --data-dir     # 数据目录 (默认 ~/.every-sync)
│
├── sync               # 手动触发同步
│   ├── --pair <name>  # 指定同步对
│   ├── --direction    # up / down / both
│   └── --dry-run      # 预览模式
│
├── pair               # 同步对管理
│   ├── add
│   ├── list
│   ├── enable <name>
│   ├── disable <name>
│   └── remove <name>
│
├── provider           # 存储后端管理
│   ├── add            # 交互式添加
│   ├── list
│   ├── test <name>    # 测试连接
│   └── remove <name>
│
├── status             # 同步状态概览
├── conflicts          # 冲突列表
├── resolve <id>       # 手动解决冲突
│   ├── --keep-local
│   ├── --keep-remote
│   └── --keep-both
│
└── version
```

### Web UI 页面

| 路由 | 功能 |
|------|------|
| `/dashboard` | 仪表盘：状态概览、实时活动 |
| `/pairs` | 同步对管理：CRUD、启停、模式切换 |
| `/tasks` | 任务监控：进行中/队列/历史 |
| `/conflicts` | 冲突处理：列表 + diff 预览 + 手动解决 |
| `/logs` | 日志查看：结构化、可筛选、实时推送 |
| `/settings` | 全局设置：Provider 配置、带宽、策略 |

### 前后端交互

| 功能 | 方式 | 说明 |
|------|------|------|
| 配置管理 | REST API | 标准 CRUD，前缀 `/api/v1/` |
| 同步状态 | WebSocket | 实时推送任务进度 |
| 文件浏览 | REST API | 同步对内文件列表 |
| 日志流 | WebSocket / SSE | 实时日志输出 |

### 前端技术栈

```
React 18 + TypeScript
├── Vite          构建工具
├── Ant Design    UI 组件库
├── Zustand       轻量状态管理
├── React Query   数据请求缓存
└── WebSocket     实时状态推送
```

### 前后端分离部署

- 前后端独立部署，通过 REST API + WebSocket 通信
- Go 后端需配置 CORS（开发环境 `*`，生产环境限制具体域名）
- API 使用 `/api/v1/` 版本前缀
- Docker Compose 编排两个容器

---

## 八、项目目录结构

```
every-sync/
├── cmd/
│   └── every-sync/           # CLI 入口
│       └── main.go
│
├── internal/
│   ├── config/               # 配置加载与校验
│   ├── server/               # HTTP API 服务
│   │   ├── router.go
│   │   ├── middleware.go      # CORS、鉴权、日志
│   │   └── handler/          # 各 API handler
│   ├── engine/               # 同步引擎核心
│   │   ├── engine.go
│   │   ├── scheduler.go
│   │   ├── worker.go
│   │   ├── conflict.go
│   │   ├── watcher.go
│   │   └── sync.go
│   ├── provider/             # 存储后端
│   │   ├── provider.go       # 接口 + 注册表
│   │   ├── local/
│   │   └── webdav/
│   ├── store/                # 数据持久层
│   │   ├── sqlite.go
│   │   ├── file_entry.go
│   │   ├── sync_pair.go
│   │   ├── change_event.go
│   │   └── migrations/
│   └── pkg/                  # 内部工具
│       ├── hash/
│       ├── chunk/
│       ├── ratelimit/
│       └── retry/
│
├── web/                      # 前端项目（独立部署）
│   ├── src/
│   ├── package.json
│   ├── vite.config.ts
│   └── Dockerfile
│
├── config.example.yaml
├── Dockerfile
├── docker-compose.yaml
├── Makefile
├── go.mod
└── go.sum
```

---

## 九、产品技术路线图

### Phase 1 — MVP (4-6 周)

目标：本地与 WebDAV 之间双向/单向同步跑通

- [ ] 项目骨架搭建（目录、配置、CLI 框架）
- [ ] Provider 接口定义 + Local Provider
- [ ] WebDAV Provider（基础 CRUD）
- [ ] SQLite 存储层 + 文件索引
- [ ] 同步引擎 v1（全量扫描 + 增量同步）
- [ ] 同步方向支持：up（仅上传）/ down（仅下载）/ both（双向）
- [ ] 冲突检测（latest_wins 策略）
- [ ] CLI 核心命令（serve / sync / pair / status）
- [ ] 基础 REST API（/api/v1/）
- [ ] 单元测试 + 集成测试框架

### Phase 2 — Web UI + 生产就绪 (3-4 周)

目标：可视化管理和稳定运行

- [ ] Web UI 全部页面
- [ ] WebSocket 实时状态推送
- [ ] Worker Pool + 分块传输
- [ ] 断点续传
- [ ] 带宽控制
- [ ] inotify 实时监听（替代定时扫描）
- [ ] systemd unit 文件
- [ ] Docker + docker-compose 部署
- [ ] 日志轮转与审计

### Phase 3 — 高级特性 (3-4 周)

目标：virtual 模式 + 冲突管理

- [x] virtual 模式（按需下载）
- [x] selective 模式（过滤规则）
- [x] Web UI 冲突处理界面
- [x] 多冲突解决策略
- [x] 文件版本历史
- [x] 同步速度/流量统计
- [x] 通知集成（Webhook / 邮件）

### Phase 4 — 生态扩展 (持续)

目标：更多云盘 + 平台支持

- [ ] S3 Provider（MinIO/OSS/COS）
- [ ] OneDrive Provider
- [ ] macOS / Windows 支持
- [ ] 插件系统（第三方 Provider 热加载）
- [ ] 性能基准测试与调优

### 关键里程碑

| 节点 | 交付物 | 可用程度 |
|------|--------|---------|
| Phase 1 结束 | CLI 可用，双向/单向同步完整 | 自用测试 |
| Phase 2 结束 | Web 管理界面，Docker 部署 | 可推荐给朋友 |
| Phase 3 结束 | virtual 模式，冲突管理完善 | 可小范围发布 |
| Phase 4 结束 | 多云盘支持，跨平台 | 开源社区发布 |
