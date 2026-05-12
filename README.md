# EverySync

多源云盘同步工具，支持本地与 WebDAV 之间的双向/单向文件同步，具备增量检测和插件化存储后端。

## 特性

- **多方向同步**：支持上传（up）、下载（down）、双向（both）
- **增量同步**：基于 xxhash 内容哈希的双阶段变更检测，WebDAV 支持 ETag 缓存跳过未变目录
- **插件化存储**：统一 Provider 接口，V1 支持 Local + WebDAV，后续扩展 S3/OneDrive 等
- **多冲突策略**：latest_wins / local_wins / remote_wins / keep_both / rename / manual / skip
- **Worker Pool**：多线程并发传输，可配置并发数
- **REST API**：`/api/v1/` 接口，支持前后端分离部署
- **React Web UI**：现代化 SPA 管理界面（仪表盘、文件浏览、冲突处理、版本历史、实时日志）
- **CLI 管理**：完整的命令行管理工具
- **Linux 优先**：优先支持 Linux 平台

## 当前状态

### Phase 1 - MVP（已完成）

- [x] Provider 插件化接口 + 注册表
- [x] Local Provider（本地文件系统 CRUD + inotify 监听）
- [x] WebDAV Provider（WebDAV 协议 CRUD）
- [x] SQLite 存储层（自动迁移 + 文件索引）
- [x] 同步引擎（Worker Pool + 方向过滤）
- [x] 同步方向：up（仅上传）/ down（仅下载）/ both（双向）
- [x] 冲突检测（modify-modify / modify-delete / delete-modify 三种冲突类型）
- [x] CLI 命令（serve / sync / pair / provider / status / version）
- [x] REST API（/api/v1/pairs CRUD + CORS）
- [x] 带宽控制配置
- [x] 单元测试 + E2E 集成测试（Local-to-Local、WebDAV、HTTP API 全覆盖）

### Phase 2 - Web UI + 生产就绪（本地版已完成，Docker 跳过）

- [x] React SPA 管理界面（仪表盘、文件浏览、同步对管理、存储源管理、冲突处理、版本历史、实时日志）
- [x] WebSocket 实时状态推送 + 连接状态指示
- [x] 分块传输进度事件（大文件按 chunk_size 上报）
- [x] 断点续传能力检测（源端 Range 读取 + 目标端 offset 写入时启用；WebDAV 上传不伪装支持）
- [x] 传输限速与失败重试
- [x] inotify 实时监听（本地变更触发同步，保留定时扫描兜底）
- [x] systemd unit 文件
- [x] 日志轮转与审计日志
- [ ] Docker + docker-compose 部署

### Phase 3 - 高级特性（已完成）

- [x] normal 模式（统一 mirror + selective，支持 selected_folders 文件夹级选择）
- [x] virtual 模式（远端文件先索引为 virtual，通过 CLI/API/Web UI 按需下载）
- [x] Web UI 冲突处理界面
- [x] 多冲突解决策略（latest_wins / local_wins / remote_wins / keep_both / rename / manual / skip）
- [x] xxhash 双阶段变更检测（大文件 QuickHash 采样头尾，小文件全量哈希）
- [x] WebDAV 增量扫描（ETag 缓存 + 每对独立 scan_interval）
- [x] 目录级追踪（创建/删除目录任务，数据库记录 is_dir 标记）
- [x] 文件版本历史（覆盖/删除前记录版本元数据，支持搜索和恢复）
- [x] 同步流量/冲突/virtual 统计
- [x] 通知集成（Webhook / SMTP 邮件，默认关闭）

### Phase 4 - 生态扩展（计划中）

- [ ] S3 Provider（MinIO/OSS/COS）
- [ ] OneDrive Provider
- [ ] macOS / Windows 支持

## 快速开始

### 安装依赖

- Go 1.22+
- Node.js 18+（前端构建需要）
- GCC（SQLite CGO 编译需要）

### 编译

```bash
make build
# 二进制文件输出到 bin/every-sync
```

### 全局配置

首次运行会自动在 `~/.every-sync/` 创建数据目录和数据库。

如需自定义配置，创建 `~/.every-sync/config.yaml`：

```yaml
server:
  host: "0.0.0.0"
  port: 10086

database:
  path: "~/.every-sync/every-sync.db"

sync:
  max_workers: 0              # 0 = 自动 (CPU * 2)
  upload_workers: 4
  download_workers: 8
  retry_max: 3
  retry_delay: 5s
  scan_interval: 5m
  upload_limit: "0"           # 0 = 不限速
  download_limit: "0"
  chunk_size: "8MB"
  chunk_threshold: "16MB"

log:
  level: "info"
  format: "console"            # console（人类可读）或 json

notification:
  webhook_url: ""              # 可选：重要事件以 JSON POST 到 Webhook
  email:
    smtp_addr: ""              # 例如 smtp.example.com:587
    username: ""
    password: ""
    from: ""
    to: []

# 配置 WebDAV 服务器连接
providers:
  - name: "my-webdav"
    type: "webdav"
    params:
      endpoint: "https://dav.example.com"   # WebDAV 服务地址
      username: "user"                       # 用户名
      password: "pass"                       # 密码
```

### 配置 WebDAV 服务器

支持三种方式配置 WebDAV 连接（优先级：CLI/API > 数据库 > 配置文件）：

#### 方式一：配置文件

在 `~/.every-sync/config.yaml` 中添加：

```yaml
providers:
  - name: "my-webdav"                        # 自定义名称
    type: "webdav"
    params:
      endpoint: "https://your-webdav-server.com/dav"  # WebDAV 服务地址
      username: "your-username"
      password: "your-password"
```

#### 方式二：命令行

```bash
# 添加 WebDAV 服务器
every-sync provider add \
  --name "my-webdav" \
  --type webdav \
  --endpoint "https://your-webdav-server.com/dav" \
  --username "your-username" \
  --password "your-password"
```

#### 方式三：REST API

```bash
curl -X POST http://localhost:10086/api/v1/providers \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "my-webdav",
    "type": "webdav",
    "params": {
      "endpoint": "https://your-webdav-server.com/dav",
      "username": "your-username",
      "password": "your-password"
    }
  }'
```

**支持的 WebDAV 服务**：

| 服务 | endpoint 示例 |
|------|--------------|
| AList | `http://alist:5244/dav` |
| NextCloud | `https://cloud.example.com/remote.php/dav/files/user` |
| Nginx WebDAV | `https://example.com/dav` |
| 坚果云 | `https://dav.jianguoyun.com/dav` |
| 群晖 WebDAV | `https://nas:5006` |

配置完成后，在同步对中通过 provider 名称引用（`--provider` 填的是 `every-sync provider list` 中的 Name，不是类型）：

```yaml
# config.yaml 方式
pairs:
  - name: "sync-photos"
    local_path: "/home/user/photos"
    remote_path: "/photos"
    provider: "my-webdav"              # 对应 providers 中的 name
    direction: "both"
    mode: "normal"                     # normal/virtual
    conflict_strategy: "latest_wins"   # latest_wins/local_wins/remote_wins/keep_both/rename/manual/skip
    exclude_patterns:
      - "*.tmp"
      - "cache/**"
    selected_folders: []               # 留空=同步全部文件夹，指定则只同步选中文件夹
    scan_interval: 300                 # 每对独立扫描间隔（秒），覆盖全局 scan_interval
    enabled: true
```

```bash
# CLI 方式
every-sync pair add \
  --name "sync-photos" \
  --local /home/user/photos \
  --remote /photos \
  --provider my-webdav \               # provider 名称
  --direction both \
  --mode normal \
  --exclude "*.tmp,cache/**"
```

## 使用说明

### 1. 启动后台服务

```bash
every-sync serve
# 默认监听 0.0.0.0:10086

# 指定端口
every-sync serve --port 9090

# 指定配置文件
every-sync serve --config /path/to/config.yaml

# 指定数据目录
every-sync serve --data-dir /data/every-sync
```

### 2. 管理存储后端

```bash
# 添加 WebDAV 服务器
every-sync provider add \
  --name "alist" \
  --type webdav \
  --endpoint "http://localhost:5244/dav" \
  --username "admin" \
  --password "123456"

# 查看已配置的存储后端
every-sync provider list
# 输出：
#   ID   Name                 Type       Endpoint
#   --   ----                 ----       --------
#   1    alist                webdav     http://localhost:5244/dav

# 更新配置（支持按名称或 ID）
every-sync provider update alist --password "new-pass"
every-sync provider update 1 --endpoint "http://new-host:5244/dav"

# 测试连接
every-sync provider test alist
# 输出：Connection successful!

# 删除（支持按名称或 ID）
every-sync provider remove alist
every-sync provider remove 1
```

### 3. 管理同步对

同步对定义了本地目录与远程目录的映射关系。新建的同步对默认为禁用状态。
已创建的同步对可以在 Web UI 的「同步对」页面点击「编辑」修改名称、本地路径、远程路径、存储源、方向、模式、冲突策略和 include/exclude 规则；如果同步对已启用，保存后后台会刷新运行时配置。

```bash
# 添加同步对（默认禁用，--provider 填 provider 名称，非类型）
every-sync pair add \
  --name "我的照片" \
  --local /home/user/photos \
  --remote /photos \
  --provider alist \
  --direction both

# selective 模式（兼容旧写法，内部映射为 normal + include/exclude）
every-sync pair add \
  --name "文档" \
  --local /home/user/docs \
  --remote /docs \
  --provider alist \
  --mode selective \
  --include "*.md,*.pdf" \
  --exclude "drafts/**,*.tmp"

# virtual 模式：先索引远端文件，随后按需下载单个文件
every-sync pair add \
  --name "云端资料" \
  --local /home/user/cloud \
  --remote /cloud \
  --provider alist \
  --mode virtual \
  --direction down
every-sync sync --pair "云端资料"
every-sync sync --pair "云端资料" --materialize /manuals/readme.pdf

# 添加并立即启用同步（加 --enable 会在创建后自动执行一次同步）
every-sync pair add \
  --name "我的照片" \
  --local /home/user/photos \
  --remote /photos \
  --provider alist \
  --direction both \
  --enable

# 查看所有同步对
every-sync pair list
# 输出：
#   ID   Name                 Status     Dir    Provider   Local -> Remote
#   --   ----                 ------     ---    --------   ----- -> ------
#   1    我的照片             disabled   both   alist      /home/user/photos -> /photos

# 启用同步对（自动触发一次完整同步）
every-sync pair enable "我的照片"

# 禁用同步对（支持按名称或 ID）
every-sync pair disable "我的照片"
every-sync pair disable 1

# 删除同步对（支持按名称或 ID）
every-sync pair remove "我的照片"
every-sync pair remove 1

# 参数说明：
#   --name       同步对名称（唯一标识）
#   --local      本地目录路径
#   --remote     远程目录路径
#   --provider   存储后端名称（通过 'every-sync provider list' 查看）
#   --direction  同步方向（up / down / both）
#   --mode       同步模式（normal / virtual，旧写法 mirror/selective 兼容）
#   --include    include 规则，逗号或换行分隔
#   --exclude    exclude 规则，逗号或换行分隔
#   --conflict-strategy latest_wins / local_wins / remote_wins / keep_both / rename / manual / skip
```

### 4. 执行同步

```bash
# 同步指定同步对（使用配置的方向）
every-sync sync --pair "我的照片"

# 指定方向覆盖
every-sync sync --pair "我的照片" --direction up

# 同步所有同步对
every-sync sync

# 预览模式（不实际执行）
every-sync sync --pair "我的照片" --dry-run
```

### 5. 查看状态

```bash
every-sync status
# 输出：
# EverySync Status
# ================
# Sync pairs: 1
#
#   [1] 我的照片 (enabled)
#       Direction: both | Mode: normal | Provider: alist
#       Local: /home/user/photos -> Remote: /photos
#       Files: 128 indexed, 3 pending
```

### 6. 查看日志

日志默认输出到 stderr（人类可读格式，包含时间、tag、等级、事项和关键字段），同时可配置写入文件：

```bash
# 启动服务时查看日志
every-sync serve 2>sync.log

# 实时查看日志
every-sync serve 2>&1 | tail -f /dev/stdin

# 在配置文件中启用日志文件
# ~/.every-sync/config.yaml:
# log:
#   level: "info"              # debug/info/warn/error
#   format: "console"          # console（人类可读）或 json
#   path: "~/.every-sync/logs" # 日志文件目录（留空则不写文件）
```

日志文件位于 `~/.every-sync/logs/every-sync.log`（配置 path 后自动创建）。

## REST API

服务启动后，通过 HTTP 接口管理同步对。

### 基础信息

| 项 | 值 |
|---|---|
| 基路径 | `/api/v1/` |
| Content-Type | `application/json` |
| CORS | 开发环境允许所有来源 |

### 接口列表

#### 健康检查

```
GET /api/v1/health
```

```bash
curl http://localhost:10086/api/v1/health
# {"status":"ok"}
```

#### 运行状态

```
GET /api/v1/status
```

```bash
curl http://localhost:10086/api/v1/status
```

#### 触发同步

```
POST /api/v1/sync
```

```bash
# 同步全部
curl -X POST http://localhost:10086/api/v1/sync -d '{}'

# 同步指定 pair
curl -X POST http://localhost:10086/api/v1/sync \
  -H 'Content-Type: application/json' \
  -d '{"pair_id":1,"direction":"both"}'
```

#### 实时事件

```
GET /api/v1/events
```

这是 WebSocket 端点，推送引擎启动、文件变更、任务入队、任务完成、chunk 进度、冲突检测和同步结果事件。

### 断点续传能力边界

EverySync 会按 Provider 能力启用严格断点续传：源端需要支持 Range 读取，目标端需要支持按 offset 写入。当前 `local -> local` 上传/下载可严格续传，`webdav -> local` 下载可基于 HTTP Range 续传；`local -> webdav` 上传仍使用流式上传、限速、分块进度和失败重试，因为通用 WebDAV PUT 不提供可靠的远端 append/compose 语义。`GET /api/v1/status` 会在每个同步对里返回 `resumable_upload` / `resumable_download`，Web UI 也会显示当前能力。

#### 同步对列表

```
GET /api/v1/pairs
```

```bash
curl http://localhost:10086/api/v1/pairs
```

#### 创建同步对

```
POST /api/v1/pairs
```

```bash
curl -X POST http://localhost:10086/api/v1/pairs \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "docs",
    "local_path": "/home/user/documents",
    "remote_path": "/backup/docs",
    "provider": "alist",
    "mode": "normal",
    "direction": "up",
    "exclude_patterns": "*.tmp,cache/**",
    "conflict_strategy": "keep_both"
  }'
```

#### 查看同步对详情

```
GET /api/v1/pairs/{id}
```

```bash
curl http://localhost:10086/api/v1/pairs/1
```

#### 更新同步对

```
PUT /api/v1/pairs/{id}
```

```bash
curl -X PUT http://localhost:10086/api/v1/pairs/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "docs",
    "local_path": "/home/user/documents",
    "remote_path": "/backup/docs",
    "provider": "alist",
    "mode": "normal",
    "direction": "both",
    "enabled": true,
    "exclude_patterns": "*.tmp,cache/**",
    "conflict_strategy": "latest_wins",
    "selected_folders": ["/photos", "/docs"],
    "scan_interval": 600
  }'
```

#### 文件列表与文件夹选择

```
GET /api/v1/pairs/{id}/files?path=/&side=local
```

```bash
# 浏览同步对的本地文件
curl 'http://localhost:10086/api/v1/pairs/1/files?path=/&side=local'

# 浏览远程文件
curl 'http://localhost:10086/api/v1/pairs/1/files?path=/docs&side=remote'
```

```
POST /api/v1/pairs/{id}/folders/select
```

```bash
# 设置 normal 模式下只同步指定文件夹
curl -X POST http://localhost:10086/api/v1/pairs/1/folders/select \
  -H 'Content-Type: application/json' \
  -d '{"folders":["/photos","/docs"]}'
```

#### 高级接口

```bash
# 按需下载 virtual 文件
curl -X POST http://localhost:10086/api/v1/pairs/1/materialize \
  -H 'Content-Type: application/json' \
  -d '{"path":"/manuals/readme.pdf"}'

# 查询/解决冲突
curl 'http://localhost:10086/api/v1/conflicts?pair_id=1&status=open'
curl -X POST http://localhost:10086/api/v1/conflicts/1/resolve \
  -H 'Content-Type: application/json' \
  -d '{"strategy":"remote_wins"}'

# 查询文件版本历史（支持搜索和恢复）
curl 'http://localhost:10086/api/v1/versions?pair_id=1&path=/docs/a.md'
```

#### 删除同步对

```
DELETE /api/v1/pairs/{id}
```

```bash
curl -X DELETE http://localhost:10086/api/v1/pairs/1
```

### 存储后端管理

#### 存储后端列表

```
GET /api/v1/providers
```

```bash
curl http://localhost:10086/api/v1/providers
```

#### 创建存储后端

```
POST /api/v1/providers
```

```bash
curl -X POST http://localhost:10086/api/v1/providers \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "alist",
    "type": "webdav",
    "params": {
      "endpoint": "http://localhost:5244/dav",
      "username": "admin",
      "password": "123456"
    }
  }'
```

#### 查看存储后端详情

```
GET /api/v1/providers/{id}
```

```bash
curl http://localhost:10086/api/v1/providers/1
```

#### 更新存储后端

```
PUT /api/v1/providers/{id}
```

也可以在 Web UI 的「存储源」页面点击「编辑」修改名称、类型和参数 JSON。保存后后台会刷新同步对运行时配置；如果改了存储源名称，引用它的同步对也需要改成新的名称。

```bash
curl -X PUT http://localhost:10086/api/v1/providers/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "params": {
      "endpoint": "http://new-host:5244/dav",
      "username": "admin",
      "password": "new-password"
    }
  }'
```

#### 删除存储后端

```
DELETE /api/v1/providers/{id}
```

```bash
curl -X DELETE http://localhost:10086/api/v1/providers/1
```

## 同步方向说明

同步方向和同步模式是两个不同维度：

- **同步方向**决定“变更从哪边流向哪边”。
- **同步模式**决定“哪些文件参与同步，以及远端文件是否立即下载到本地”。

| 方向 | 值 | 行为 | 场景 |
|------|----|------|------|
| 仅上传 | `up` | 本地 → 远程 | 本地文件备份到云盘 |
| 仅下载 | `down` | 远程 → 本地 | 从云盘拉取文件到本地 |
| 双向 | `both` | 双向同步 | 多设备文件同步 |

## 同步模式说明

| 模式 | 值 | 行为 | 适合场景 |
|------|----|------|----------|
| 普通同步 | `normal` | 同步所有未被规则排除的文件。支持 `exclude_patterns` 排除、`selected_folders` 指定只同步哪些文件夹。方向为 `both` 时两端尽量保持一致；方向为 `up`/`down` 时只按单方向复制和删除。 | 常规备份、多设备同步 |
| 虚拟文件 | `virtual` | 远端文件先写入索引，不自动下载内容到本地；需要文件时通过 Web UI/API/CLI materialize 单个文件。本地已有或本地新增文件仍可按方向上传/处理。强制 `direction: both`。 | 云端资料很多、本地磁盘有限、按需取用 |

旧写法 `mirror` 和 `selective` 仍被接受，内部统一映射为 `normal`。`mirror` → `normal`（无过滤规则）；`selective` → `normal`（保留 include/exclude 规则）。

几个容易混淆的点：

- `mode` 不是方向。`normal + up` 表示”把本地同步上传到远端”；`normal + down` 表示”把远端同步下载到本地”；`normal + both` 才是双向同步。
- `virtual` 的核心是不自动下载远端内容。配 `both` 时，本地新增文件仍会上传到远端，但远端新增文件仍先保持 virtual；远端变更时会重新 virtualize。
- 删除传播也受方向影响：`up` 会把本地删除同步到远端，`down` 会把远端删除同步到本地，`both` 会根据双方状态和历史索引判断。

## 冲突策略说明

| 策略 | 值 | 行为 |
|------|----|------|
| 最新优先 | `latest_wins` | 比较 ModTime，保留较新版本同步到两端（默认策略） |
| 本地优先 | `local_wins` | 冲突时始终保留本地版本 |
| 远程优先 | `remote_wins` | 冲突时始终保留远程版本 |
| 保留双方 | `keep_both` | 保留两个版本，将较新版本同步到两端 |
| 重命名 | `rename` | 将较旧版本重命名为”冲突副本 YYYY-MM-DD”，较新版本同步到两端 |
| 手动处理 | `manual` | 记录冲突但不自动解决，等待用户在 Web UI/API 手动处理 |
| 跳过 | `skip` | 跳过冲突文件，不做任何操作 |

冲突检测类型：
- **modify-modify**：两端都修改了同一文件（经典冲突）
- **modify-delete**：本地修改了文件但远端已删除
- **delete-modify**：远端修改了文件但本地已删除

## 开发

```bash
# 安装 Go 依赖
go mod tidy

# 安装前端依赖
make web-deps

# 编译（含前端构建）
make build

# 仅构建前端
make web-build

# 运行测试
make test

# 清理
make clean
```

## 项目结构

```
every-sync/
├── cmd/every-sync/              # CLI 入口
├── internal/
│   ├── config/                  # 配置管理
│   ├── engine/                  # 同步引擎（调度器 + Worker Pool + 冲突检测）
│   ├── pkg/
│   │   └── hash/                # xxhash 文件哈希（全量 + QuickHash 采样）
│   ├── provider/                # 存储后端
│   │   ├── provider.go          # Provider 接口 + 注册表 + 能力检测
│   │   ├── local/               # 本地文件系统
│   │   └── webdav/              # WebDAV 协议（ETag 缓存 + 增量扫描）
│   ├── server/                  # HTTP API 服务
│   │   └── handler/             # API handlers
│   └── store/                   # SQLite 持久层
│       └── migrations/          # 数据库迁移
├── web/                         # React 前端（Vite + TypeScript）
│   └── src/
│       ├── pages/               # Dashboard / FileBrowser / SyncPairs / Providers / Conflicts / Versions / Logs
│       ├── components/          # Sidebar / Modal / Toast / Icons / Breadcrumb / StatusIcon
│       ├── hooks/               # useWebSocket
│       └── api/                 # API client
├── docs/plans/                  # 设计文档
├── config.example.yaml          # 示例配置
└── Makefile                     # 构建脚本
```

## License

MIT
