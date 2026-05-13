# Every-Sync 单文件同步进度设计

## 目标

在仪表盘、同步对列表、文件浏览页中展示单个文件的实时同步进度，让用户能知道当前正在传输哪个文件、传了多少、整体同步推进到哪里。

本设计只覆盖实时进度，不做长期传输历史、审计日志或数据库持久化任务流水。

## 已确认范围

- UI 方向：行内摘要 + 右侧详情。
- 展示内容：当前文件 + 最近完成/失败少量记录。
- 恢复策略：页面刷新或 WebSocket 重连后，只恢复后端仍在同步的当前状态。
- 最近完成/失败记录：前端内存短历史，刷新后清空。
- 技术方向：后端内存态进度快照 + WebSocket 增量事件 + 前端统一 progress store。

## UI 设计

### 仪表盘

仪表盘保留当前同步对表格。活跃同步对行内增加紧凑进度摘要：

- 当前文件路径，长路径从左侧省略，保留文件名和末尾目录。
- 文件级进度条和百分比。
- 字节进度，例如 `812 MB / 1.4 GB`。
- 同步对级文件计数，例如 `12 / 37`。

仪表盘不展示最近完成/失败记录，避免摘要页面变成传输日志。

### 同步对页面

同步对页面采用两层信息：

- 列表行内显示同步对级摘要：当前文件、文件级进度、字节进度、文件计数。
- 右侧详情面板显示所选同步对的完整实时上下文：
  - 当前文件。
  - 方向和任务类型。
  - 字节进度、百分比。
  - 已完成和剩余任务计数。
  - 最近完成/失败记录，默认每个同步对最多 5 条。

如果没有选中同步对，右侧面板默认显示第一个正在同步的同步对；没有同步活动时显示空状态。

### 文件浏览页面

文件浏览页只在当前正在同步的文件行显示文件级进度：

- 行背景使用同步中的强调色。
- 文件名下方显示进度条、方向、字节进度、百分比。
- 状态列显示 `Syncing`。

文件行匹配使用标准化后的完整路径精确匹配，不使用文件名匹配，避免同名文件误高亮。

## 数据结构

后端新增内存态 `ProgressTracker`，以 `pair_id` 为键保存当前同步快照。

```ts
type PairProgressSnapshot = {
  pair_id: string
  status: 'idle' | 'scanning' | 'syncing' | 'completed' | 'failed'
  direction: 'up' | 'down' | 'both'
  active_file?: {
    path: string
    task_type: 'upload' | 'download'
    bytes_transferred: number
    bytes_total: number
    percent: number
    started_at: string
    updated_at: string
  }
  files_synced: number
  files_total: number
  pending_tasks: number
  started_at?: string
  updated_at: string
  error?: string
}
```

前端 `useSyncProgress` 在后端快照基础上增加短历史：

```ts
type RecentProgressItem = {
  path: string
  task_type: 'upload' | 'download' | 'delete' | 'create_dir' | 'delete_dir' | 'virtual' | 'conflict'
  status: 'completed' | 'failed'
  bytes_total?: number
  error?: string
  finished_at: string
}

type PairProgressViewModel = PairProgressSnapshot & {
  recent_items: RecentProgressItem[]
}
```

`recent_items` 只存在于前端内存中，每个同步对最多保留 5 条，页面刷新后清空。

## 后端逻辑

### ProgressTracker 职责

`ProgressTracker` 是同步引擎内存态组件，负责：

- 在同步开始时创建或重置 pair 快照。
- 在文件传输开始时设置 `active_file`。
- 在 chunk 传输时更新 `bytes_transferred`、`bytes_total`、`percent`。
- 在任务完成或失败时更新同步对级计数。
- 在同步完成或失败后广播最终事件，并从可恢复快照中移除该 pair。

后端不持久化最近完成/失败记录。

### 事件

现有事件继续保留，并补充语义更明确的文件级事件：

| 事件 | 用途 |
|------|------|
| `sync_started` | 初始化同步对快照 |
| `task_queued` | 更新队列和总任务信息 |
| `task_started` | 设置当前 active file |
| `chunk_transferred` | 更新 active file 字节进度 |
| `task_completed` | 增加完成计数，清理或推进 active file |
| `task_failed` | 标记失败，清理 active file |
| `sync_completed` | 同步对完成，清空 active file |
| `sync_failed` | 同步对失败，记录 error |

小文件可能没有 chunk 事件。前端在收到 `task_started` 但尚无字节进度时显示处理中状态，收到 `task_completed` 后直接显示完成。

### 快照 API

新增进度快照接口：

- `GET /api/v1/progress`：返回所有仍处于 `scanning` 或 `syncing` 的同步快照。

前端启动和 WebSocket 重连后调用 `GET /api/v1/progress`，再继续使用 WebSocket 增量事件更新。
已完成或失败的同步不会通过该接口恢复；这些状态由同步对列表状态和当前页面内存中的 recent items 表达。

## 前端逻辑

扩展现有 `SyncProgressProvider`：

1. 初次挂载时请求 `/api/v1/progress`，填充 pair 快照。
2. WebSocket 事件到达后按 pair 增量更新。
3. `task_completed` 和 `task_failed` 写入 `recent_items`。
4. 每个 pair 的 `recent_items` 保留最近 5 条。
5. WebSocket 断开时保留最后一次快照；重连后重新拉快照对齐。

组件拆分：

| 组件 | 说明 |
|------|------|
| `ProgressBar` | 共享进度条 |
| `PairProgressInline` | 仪表盘、同步对列表复用的行内摘要 |
| `PairProgressDetail` | 同步对页右侧详情面板 |
| `FileProgressCell` | 文件浏览行内进度 |
| `SyncProgressProvider` | 快照初始化、WebSocket 增量、短历史维护 |

## 错误和边界

- WebSocket 断开：保留最后快照并显示连接状态，不主动清空进度。
- WebSocket 重连：重新拉 `/progress`，以后端当前状态为准。
- 同步失败：当前文件转入前端 recent failed，pair 状态为 failed。
- 小文件：无 chunk 时显示处理中，完成后直接 100%。
- 目录任务：参与队列计数，不展示字节进度。
- 多 worker 并发：本次 UI 只展示主 active file；不展示同一同步对的多个并发文件。
- 路径匹配：文件浏览页使用标准化完整路径精确匹配。

## 测试计划

### 后端

- `ProgressTracker` 单元测试覆盖状态转移：
  - `sync_started` 初始化。
  - `task_started` 设置 active file。
  - `chunk_transferred` 更新百分比。
  - `task_completed` 增加完成计数并清理 active file。
  - `task_failed` 标记失败。
  - `sync_completed` 清空 active file。
- API handler 测试覆盖 `/api/v1/progress` 返回当前快照。
- 同步引擎测试覆盖小文件无 chunk 事件时仍能完成状态更新。

### 前端

- `useSyncProgress` 测试覆盖：
  - 初始快照加载。
  - WebSocket 增量更新。
  - `recent_items` 限长。
  - 断线保留状态。
  - 重连后快照覆盖旧状态。
- 组件测试覆盖：
  - `PairProgressInline` 渲染当前文件和进度。
  - `PairProgressDetail` 渲染最近完成/失败。
  - `FileProgressCell` 只对完整路径匹配的文件显示进度。

## 非目标

- 不新增数据库迁移。
- 不实现长期传输历史。
- 不实现全局活动中心页面。
- 不展示同一同步对的多个并发 active files。
- 不改变同步任务调度策略。
