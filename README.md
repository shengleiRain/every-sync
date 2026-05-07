# EverySync

多源云盘同步工具，支持本地与 WebDAV 之间的双向/单向文件同步，具备增量检测和插件化存储后端。

## 特性

- **多方向同步**：支持上传（up）、下载（down）、双向（both）
- **增量同步**：通过文件修改时间和大小对比，只同步变更文件
- **插件化存储**：统一 Provider 接口，V1 支持 Local + WebDAV，后续扩展 S3/OneDrive 等
- **Worker Pool**：多线程并发传输，可配置并发数
- **REST API**：`/api/v1/` 接口，支持前后端分离部署
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
- [x] 冲突检测（latest_wins 策略）
- [x] CLI 命令（serve / sync / pair / provider / status / version）
- [x] REST API（/api/v1/pairs CRUD + CORS）
- [x] 带宽控制配置
- [x] 单元测试（15 个测试 + race detection）

### Phase 2 - Web UI + 生产就绪（计划中）

- [ ] Web 管理界面（React + Ant Design）
- [ ] WebSocket 实时状态推送
- [ ] 分块传输（大文件分块并发上传）
- [ ] 断点续传
- [ ] inotify 实时监听（替代定时扫描）
- [ ] Docker + docker-compose 部署

### Phase 3 - 高级特性（计划中）

- [ ] virtual 模式（按需下载）
- [ ] selective 模式（过滤规则）
- [ ] Web UI 冲突处理界面
- [ ] 多冲突解决策略
- [ ] 文件版本历史

### Phase 4 - 生态扩展（计划中）

- [ ] S3 Provider（MinIO/OSS/COS）
- [ ] OneDrive Provider
- [ ] macOS / Windows 支持

## 快速开始

### 安装依赖

- Go 1.22+
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

log:
  level: "info"
  format: "console"            # console（人类可读）或 json

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
    mode: "mirror"
    enabled: true
```

```bash
# CLI 方式
every-sync pair add \
  --name "sync-photos" \
  --local /home/user/photos \
  --remote /photos \
  --provider my-webdav \               # provider 名称
  --direction both
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

```bash
# 添加同步对（默认禁用，--provider 填 provider 名称，非类型）
every-sync pair add \
  --name "我的照片" \
  --local /home/user/photos \
  --remote /photos \
  --provider alist \
  --direction both

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
#   --mode       同步模式（mirror）
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
#       Direction: both | Mode: mirror | Provider: alist
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
    "mode": "mirror",
    "direction": "up"
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
  -d '{"direction": "both", "enabled": true}'
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

| 方向 | 值 | 行为 | 场景 |
|------|----|------|------|
| 仅上传 | `up` | 本地 → 远程 | 本地文件备份到云盘 |
| 仅下载 | `down` | 远程 → 本地 | 从云盘拉取文件到本地 |
| 双向 | `both` | 双向同步 | 多设备文件同步 |

## 开发

```bash
# 安装依赖
make deps

# 编译
make build

# 运行测试
make test

# 格式化代码
make fmt

# 清理
make clean
```

## 项目结构

```
every-sync/
├── cmd/every-sync/              # CLI 入口
├── internal/
│   ├── config/                  # 配置管理
│   ├── engine/                  # 同步引擎（调度器 + Worker Pool）
│   ├── provider/                # 存储后端
│   │   ├── provider.go          # Provider 接口 + 注册表
│   │   ├── local/               # 本地文件系统
│   │   └── webdav/              # WebDAV 协议
│   ├── server/                  # HTTP API 服务
│   │   └── handler/             # API handlers
│   ├── store/                   # SQLite 持久层
│   │   └── migrations/          # 数据库迁移
│   └── pkg/                     # 内部工具库
├── docs/plans/                  # 设计文档
├── config.example.yaml          # 示例配置
└── Makefile                     # 构建脚本
```

## License

MIT
