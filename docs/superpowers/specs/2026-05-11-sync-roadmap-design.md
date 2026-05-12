# Every-Sync 同步功能路线设计

## 目标

功能上对标 OneDrive，作为平替。支持 WebDAV 并可扩展多网盘。

## 1. 数据模型

### SyncPair 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| Mode | string | `"normal"` 或 `"virtual"` |
| Direction | string | `"up"` / `"down"` / `"both"` |
| SelectedFolders | string | JSON 数组，相对路径，如 `["work","photos/2024"]` |
| IncludePatterns | string | 文件类型白名单，如 `*.pdf,*.docx` |
| ExcludePatterns | string | 文件类型黑名单，如 `*.tmp,*.log` |

### Mode 语义

- **normal**：合并原 mirror + selective。SelectedFolders 为空 = 全量同步；有值 = 只同步选中目录。
- **virtual**：远程文件仅建索引不下载，按需 materialize。固定方向 `both`。

### 过滤链

```
SelectedFolders（目录级过滤）
  ↓ 空 = 全量；有值 = 只同步选中目录
IncludePatterns（文件类型白名单）
  ↓ 有值 = 只同步匹配的文件类型
ExcludePatterns（文件类型黑名单）
  ↓ 排除匹配的文件类型
```

### SelectedFolders 规则

- 路径相对于 LocalPath/RemotePath，不带前导 `/`
- 自动父子合并：添加 `docs/work` 时，已有的 `docs/work/2024` 自动移除
- 反向同理：如果已选 `docs/work`，再添加 `docs/work/2024` 时忽略（已被父目录包含）

## 2. 文件夹选择与同步方向的交互

### 增删文件夹时的文件处理

| 操作 | 方向 up（本地→远程） | 方向 down（远程→本地） | 方向 both |
|------|---------------------|----------------------|-----------|
| 新增选中文件夹 A | 本地 A 的文件上传到远程 A | 远程 A 的文件下载到本地 A | 双向同步 A |
| 取消选中文件夹 A | 远程 A 中已同步的文件被删除，本地保留 | 本地 A 中已同步的文件被删除，远程保留 | 已同步文件双向清理 |

### both 模式取消文件夹

只处理 `sync_state == "synced"` 的文件。本地独有或远程独有的文件不动。

### 执行时机

修改 SelectedFolders 后立即生效。RefreshPairs() 检测到变更，生成对应任务加入队列。

## 3. 目录跟踪与删除清理

### 目录视为特殊文件

- file_entries 表新增 `is_dir` 布尔字段
- 同步时先处理文件，再处理目录
- 目录条目参与完整的同步生命周期

### 目录同步行为

| 场景 | 行为 |
|------|------|
| 远程有目录 A，本地没有 | 创建本地 A（down/both） |
| 本地有目录 A，远程没有 | 创建远程 A（up/both） |
| 远程删除了目录 A | 删除本地 A 及其下所有已同步文件（down/both） |
| 本地删除了目录 A | 删除远程 A 及其下所有已同步文件（up/both） |
| 目录 A 下文件被清空但目录还在 | 目录保留，不删除 |

### 清理原则

- 删除目录时，其下已同步的子文件也一起删除
- 空目录保留——只有来源方删除目录本身时，目标方才删除
- 不留残留：如果云端删了整个文件夹，本地对应文件夹及全部子文件清理干净

## 4. Virtual 模式

### 固定方向 both，不可更改

### 行为

| 事件 | 行为 |
|------|------|
| 远程新文件/目录 | 本地建虚拟条目（sync_state = virtual），只记录元数据 |
| 本地新文件/目录 | 自动上传到远程，sync_state = synced |
| 本地修改已同步文件 | 自动上传更新 |
| 远程修改已同步文件 | 重新虚拟化，更新元数据，标记 virtual，等待 materialize |
| 远程删除文件/目录 | 本地同步删除 |
| 本地删除文件/目录 | 远程同步删除 |

### Materialize

- 通过 Web UI / CLI / API 触发
- 下载实际内容后 sync_state 从 virtual 变为 synced
- 后续本地修改正常上传

### 过滤规则

SelectedFolders 和 IncludePatterns/ExcludePatterns 对 virtual 模式同样生效。不在选中范围内的远程文件不建虚拟条目。

## 5. 测试计划

### 第一层：Local ↔ Local 测试

用两个本地目录模拟同步双方，无外部依赖。

| 场景 | 验证点 |
|------|--------|
| Normal 全量同步 | 空目录初始化，双方文件互相同步 |
| SelectedFolders 选中/取消 | 只同步选中目录；取消后按方向清理 |
| SelectedFolders 父子合并 | 添加父目录时自动移除已包含的子目录 |
| Include/Exclude 过滤 | 只同步匹配类型，排除指定类型 |
| 方向 up/down/both | 单向只影响目标方，双向双向传播 |
| 目录创建/删除 | 目录作为整体同步，删除时子文件一起删除 |
| 空目录保留 | 文件清空但目录存在时保留 |
| 冲突检测与解决 | 双方同时修改触发冲突，各策略正确处理 |
| Virtual 索引 | 远程文件不下载，只建虚拟条目 |
| Virtual materialize | 按需下载，状态正确转换 |
| Virtual 本地上传 | 本地新文件自动上传 |
| 大文件分块续传 | 中断后从断点继续 |
| 增删文件夹触发文件处理 | 配合方向正确下载/上传/删除 |

### 第二层：真实 WebDAV 测试

编译后连接真实 WebDAV 服务，用 `go test -tags=integration` 控制。

| 场景 | 验证点 |
|------|--------|
| WebDAV 连接与认证 | 正常连接、认证失败、权限不足 |
| 基本文件 CRUD | 上传、下载、删除、重命名 |
| 目录操作 | 创建、删除目录，含子文件的目录删除 |
| Normal 全量同步 | 本地 ↔ WebDAV 双向同步 |
| Virtual 模式 | WebDAV 文件虚拟索引 + 按需下载 |
| 大文件传输稳定性 | 100MB+ 文件完整传输 |
| 网络中断恢复 | 断网重连后续传 |
| 特殊字符文件名 | 中文、空格、特殊符号 |

## 6. Web UI 设计

### 技术方案

- **框架**：React 18 + Vite 构建
- **构建产物**：`dist/` 目录通过 Go embed 嵌入二进制
- **样式**：CSS Variables 主题系统，支持亮色/暗色模式（跟随系统偏好）
- **字体**：IBM Plex Sans（UI）+ JetBrains Mono（日志/代码）
- **通信**：WebSocket 实时事件 + REST API 操作

### 视觉规范

交互原型见 `docs/superpowers/specs/every-sync-ui-mockup.html`。

#### 色彩系统

| Token | Light | Dark | 用途 |
|-------|-------|------|------|
| bg-root | #F8F9FB | #0E1117 | 页面背景 |
| bg-surface | #FFFFFF | #161B25 | 卡片/面板背景 |
| text-primary | #1A1D26 | #E8ECF4 | 主文本 |
| text-secondary | #5C6170 | #8B93A7 | 辅助文本 |
| accent-blue | #3B6BF5 | #5B8AF5 | 主色调、按钮 |
| accent-green | #1FA463 | #2ECE80 | 成功/已同步 |
| accent-red | #E5484D | #F0666B | 错误/冲突 |
| accent-amber | #F5A623 | #F5B84A | 警告/文件夹图标 |
| accent-violet | #7C5CFC | #9B82FC | 虚拟文件 |

#### 圆角

- xs: 4px（小元素）
- sm: 6px（按钮、输入框）
- md: 8px（卡片、列表）
- lg: 12px（对话框）

#### 阴影

- sm: `0 1px 2px rgba(0,0,0,0.04)` — 微浮
- md: `0 4px 12px rgba(0,0,0,0.06)` — 卡片
- lg: `0 8px 32px rgba(0,0,0,0.08)` — 弹出层

### 页面结构

#### 布局

```
┌──────────┬──────────────────────────┐
│ Sidebar  │ Header (breadcrumb,      │
│ 240px    │ pair selector, actions)   │
│          ├──────────────────────────┤
│ - Logo   │                          │
│ - Nav    │     Content Area         │
│ - Theme  │                          │
│          │                          │
└──────────┴──────────────────────────┘
```

#### 侧边栏导航

| 导航项 | 图标 | 说明 |
|--------|------|------|
| Dashboard | 网格 | 引擎状态、指标概览 |
| File Browser | 文件夹 | 文件浏览器（核心页面） |
| Sync Pairs | 层叠 | 同步对管理 |
| Providers | 齿轮 | Provider 管理 |
| Conflicts | 警告 | 冲突列表（带计数徽章） |
| Versions | 时间 | 版本历史 |
| Logs | 文档 | 实时日志 |

### 核心页面设计

#### 1. Dashboard

- **指标卡片**（4列）：Engine Status / Sync Pairs / Pending Tasks / Workers
- **流量统计**（3列）：上传量 / 下载量 / 冲突数+虚拟文件数
- **活跃同步对列表**：每行显示名称、模式、方向、状态徽章、同步按钮

#### 2. File Browser（核心新功能）

**头部区域**：
- 左侧：页面标题 + 同步对下拉选择器（显示名称+模式+方向）
- 右侧：Local/Remote 切换标签页

**面包屑导航**：
- 显示当前路径层级，点击可跳转到任意父级
- 最后一项为当前目录，不可点击

**文件列表**：
- 文件管理器风格：一次显示一层，点击文件夹进入下一层
- 列结构：勾选框 | 图标 | 名称 | 大小 | 修改时间 | 同步状态 | 操作按钮

**同步状态图标**（参考 OneDrive）：

| 状态 | 图标 | 颜色 |
|------|------|------|
| 已同步 | 绿色勾 | accent-green |
| 同步中 | 蓝色旋转箭头 | accent-blue |
| 虚拟 | 紫色云朵 | accent-violet |
| 冲突 | 红色叉号 | accent-red |
| 已排除 | 灰色横线 | text-tertiary |

**文件夹选中标记**：
- 每行前面的勾选框，表示该文件夹是否在 SelectedFolders 中
- 三态：选中（蓝色实心）、部分选中（蓝色横线）、未选中（空框）
- 点击可切换选中/取消，触发同步处理

**右键菜单**：
- Download / Materialize（虚拟文件时显示）
- View Versions
- Resolve Conflict（冲突文件时显示）
- Exclude from Sync

#### 3. Sync Pairs

- 列表视图：名称、Provider、模式、方向、状态、操作
- 创建/编辑表单：标准表单，含 SelectedFolders 可视化选择器（文件夹树勾选）
- 快速操作：同步、启用/禁用、编辑、删除

#### 4. Conflicts

- 列表视图：文件路径、同步对、本地修改时间/大小 vs 远程修改时间/大小
- 一键解决：Local Wins / Remote Wins / Latest Wins / Skip

#### 5. Versions

- 按文件路径和同步对过滤
- 版本列表：来源、大小、时间戳
- Restore 操作

#### 6. Logs

- 实时日志流，按级别颜色标注
- 过滤：All / Events / Info / Warn / Error
- 搜索框
- 暂停/恢复/清空控制

#### 7. Providers

- 列表 + CRUD 表单
- Provider 类型选择（Local / WebDAV）
- WebDAV：URL、用户名、密码配置 + 连接测试按钮

### 组件规范

#### 按钮

| 变体 | 样式 | 用途 |
|------|------|------|
| Primary | 蓝色背景+白字 | 主操作（Sync All、Save） |
| Secondary | 灰色背景+深色边框 | 次要操作（Cancel） |
| Ghost | 透明背景 | 表格行内操作 |
| Icon | 32x32 圆角方块 | 图标按钮 |

#### 状态徽章

| 类型 | 背景色 | 文字色 |
|------|--------|--------|
| Success | green-subtle | green |
| Warning | amber-subtle | amber |
| Error | red-subtle | red |
| Info | blue-subtle | blue |
| Neutral | surface-hover | secondary |

#### 动画

- 页面切换：淡入 + 向上移动 8px，200ms
- 列表项加载：交错动画，每项延迟 20ms
- 同步中图标：旋转动画 1.5s 循环
- 按钮悬停：120ms 过渡
- 右键菜单：出现时 scale 0.95 → 1，150ms

### 响应式断点

| 断点 | 行为 |
|------|------|
| ≥1024px | 完整侧边栏 + 内容区 |
| 768-1023px | 侧边栏收起为图标模式（48px） |
| <768px | 侧边栏隐藏，汉堡菜单触发 |

### 所需新增 API

| 端点 | 方法 | 说明 |
|------|------|------|
| /api/v1/pairs/{id}/files | GET | 列出同步对下指定路径的文件和目录 |
| /api/v1/pairs/{id}/files?path=xxx | GET | 列出指定子目录内容 |
| /api/v1/pairs/{id}/folders/select | POST | 更新 SelectedFolders |
| /api/v1/files/{id}/materialize | POST | 实体化虚拟文件 |
| /api/v1/files/{id}/versions | GET | 获取文件版本列表 |
