# Every-Sync 同步功能路线 + 冲突优化 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 重构同步引擎为 normal/virtual 双模式，实现文件夹选择、目录跟踪、内容哈希冲突检测，并用 React 重写 Web UI。

**Architecture:** 后端保持分层架构（store → engine → provider → server），核心变更是合并 mirror/selective 为 normal 模式，引入 SelectedFolders 机制，增加两阶段哈希检测，补全边界冲突场景。前端从单文件 vanilla JS 迁移到 React + Vite。

**Tech Stack:** Go 1.26, SQLite, xxhash/v2, React 18, Vite, WebSocket

---

## 文件结构

### 新建文件

| 文件 | 职责 |
|------|------|
| `internal/store/migrations/004_normal_mode.sql` | 新数据模型迁移：添加 selected_folders, scan_interval, local_hash, remote_hash, remote_etag, is_dir, conflict_type 等字段 |
| `internal/pkg/hash/hash.go` | xxhash 哈希计算：全量哈希、快速哈希（前1MB+后1MB+size） |
| `internal/pkg/hash/hash_test.go` | 哈希计算测试 |
| `web/package.json` | React 前端项目配置 |
| `web/vite.config.ts` | Vite 构建配置，输出到 dist/ 供 Go embed |
| `web/src/App.tsx` | React 应用入口，路由 |
| `web/src/theme.ts` | CSS 变量主题系统（亮/暗） |
| `web/src/hooks/useWebSocket.ts` | WebSocket 连接 hook |
| `web/src/api/client.ts` | REST API 客户端 |
| `web/src/pages/Dashboard.tsx` | Dashboard 页面 |
| `web/src/pages/FileBrowser.tsx` | 文件浏览器页面 |
| `web/src/pages/SyncPairs.tsx` | 同步对管理页面 |
| `web/src/pages/Conflicts.tsx` | 冲突解决页面 |
| `web/src/pages/Providers.tsx` | Provider 管理页面 |
| `web/src/pages/Logs.tsx` | 日志页面 |
| `web/src/pages/Versions.tsx` | 版本历史页面 |
| `web/src/components/Sidebar.tsx` | 侧边栏导航 |
| `web/src/components/FileList.tsx` | 文件列表组件 |
| `web/src/components/StatusIcon.tsx` | 同步状态图标组件 |
| `web/src/components/Breadcrumb.tsx` | 面包屑导航组件 |
| `web/src/components/FolderCheck.tsx` | 文件夹勾选组件 |
| `web/src/components/ContextMenu.tsx` | 右键菜单组件 |

### 修改文件

| 文件 | 修改范围 | 说明 |
|------|---------|------|
| `internal/store/store.go` | SyncPair CRUD、FileEntry 字段、新查询 | 适配新数据模型 |
| `internal/store/migrations/004_normal_mode.sql` | 新建 | 数据库迁移 |
| `internal/engine/engine.go` | 模式合并、任务生成、哈希检测、边界冲突 | 核心引擎改动 |
| `internal/engine/engine_test.go` | 大幅扩展 | 新场景测试 |
| `internal/provider/provider.go` | Provider 接口新增方法 | 增量扫描支持 |
| `internal/provider/local/local.go` | 适配接口变更 | 目录跟踪 |
| `internal/provider/webdav/webdav.go` | ETag 缓存、增量 PROPFIND | 远程检测优化 |
| `internal/server/handler/ws.go` | 新 API 端点 | 文件浏览、文件夹选择 |
| `internal/server/server.go` | 路由更新 | 新 API 路由 |
| `internal/server/static/index.html` | 删除 | 被 React 替换 |
| `cmd/every-sync/main.go` | 适配新 SyncPair 字段 | CLI 命令更新 |
| `go.mod` | 添加 xxhash 依赖 | 哈希库 |

---

## Phase 1: 数据模型与哈希基础

### Task 1: 数据库迁移

**Files:**
- Create: `internal/store/migrations/004_normal_mode.sql`
- Modify: `internal/store/store.go`

- [ ] **Step 1: 编写迁移 SQL**

```sql
-- 004_normal_mode.sql
-- 重构同步模式：合并 mirror/selective → normal，新增字段

-- 1. sync_pairs 表变更
ALTER TABLE sync_pairs ADD COLUMN selected_folders TEXT DEFAULT '[]';
ALTER TABLE sync_pairs ADD COLUMN scan_interval INTEGER DEFAULT 300;

-- 2. file_entries 表变更
ALTER TABLE file_entries ADD COLUMN local_hash TEXT DEFAULT '';
ALTER TABLE file_entries ADD COLUMN remote_hash TEXT DEFAULT '';
ALTER TABLE file_entries ADD COLUMN remote_etag TEXT DEFAULT '';
ALTER TABLE file_entries ADD COLUMN is_dir INTEGER DEFAULT 0;

-- 3. conflicts 表变更
ALTER TABLE conflicts ADD COLUMN conflict_type TEXT DEFAULT 'modify_modify';
ALTER TABLE conflicts ADD COLUMN local_hash TEXT DEFAULT '';
ALTER TABLE conflicts ADD COLUMN remote_hash TEXT DEFAULT '';
```

- [ ] **Step 2: 更新 store.go 的 SyncPair 结构体**

在 `internal/store/store.go` 的 `SyncPair` struct 中添加新字段：

```go
SelectedFolders  string `json:"selected_folders"`
ScanInterval     int    `json:"scan_interval"`
```

更新所有涉及 SyncPair 的 CRUD 方法（`CreateSyncPair`, `UpdateSyncPair`, `ListSyncPairs`, `GetSyncPair`），加入新字段的读写。

- [ ] **Step 3: 更新 FileEntry 结构体**

添加 `LocalHash`, `RemoteHash`, `RemoteEtag`, `IsDir` 字段。更新 `UpsertFileEntry`, `GetFileEntry`, `ListFileEntries` 等方法。

- [ ] **Step 4: 更新 Conflict 结构体**

添加 `ConflictType`, `LocalHash`, `RemoteHash` 字段。更新冲突相关方法。

- [ ] **Step 5: 运行现有测试验证迁移**

Run: `go test ./internal/store/ -v`
Expected: PASS（现有测试应仍通过，新增字段有默认值）

- [ ] **Step 6: 提交**

```bash
git add internal/store/
git commit -m "feat: add new database fields for normal mode, hash, and conflict type"
```

### Task 2: 哈希计算包

**Files:**
- Create: `internal/pkg/hash/hash.go`
- Create: `internal/pkg/hash/hash_test.go`

- [ ] **Step 1: 编写哈希测试**

```go
// hash_test.go
package hash

import (
    "os"
    "path/filepath"
    "testing"
)

func TestFileHash(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "test.txt")
    os.WriteFile(p, []byte("hello world"), 0644)

    h, err := FileHash(p)
    if err != nil { t.Fatal(err) }
    if h == "" { t.Fatal("hash should not be empty") }

    // 相同文件应该得到相同哈希
    h2, err := FileHash(p)
    if err != nil { t.Fatal(err) }
    if h != h2 { t.Fatalf("same file different hash: %s vs %s", h, h2) }
}

func TestQuickHash(t *testing.T) {
    dir := t.TempDir()
    // 创建 > 2MB 文件以触发快速哈希
    p := filepath.Join(dir, "large.bin")
    data := make([]byte, 3*1024*1024)
    data[0] = 0xAA
    data[len(data)-1] = 0xBB
    os.WriteFile(p, data, 0644)

    h, err := QuickHash(p)
    if err != nil { t.Fatal(err) }
    if h == "" { t.Fatal("hash should not be empty") }

    // 修改中间内容，quick hash 不应该检测到
    data[1024*1024] = 0xCC
    os.WriteFile(p, data, 0644)
    h2, err := QuickHash(p)
    if err != nil { t.Fatal(err) }
    if h != h2 { t.Fatal("quick hash should not detect middle change") }

    // 修改头部，quick hash 应该检测到
    data[0] = 0xDD
    os.WriteFile(p, data, 0644)
    h3, err := QuickHash(p)
    if err != nil { t.Fatal(err) }
    if h == h3 { t.Fatal("quick hash should detect head change") }
}

func TestContentEqual(t *testing.T) {
    dir := t.TempDir()
    p1 := filepath.Join(dir, "a.txt")
    p2 := filepath.Join(dir, "b.txt")
    p3 := filepath.Join(dir, "c.txt")

    os.WriteFile(p1, []byte("same"), 0644)
    os.WriteFile(p2, []byte("same"), 0644)
    os.WriteFile(p3, []byte("different"), 0644)

    eq, err := ContentEqual(p1, p2)
    if err != nil { t.Fatal(err) }
    if !eq { t.Fatal("identical files should be equal") }

    eq, err = ContentEqual(p1, p3)
    if err != nil { t.Fatal(err) }
    if eq { t.Fatal("different files should not be equal") }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/pkg/hash/ -v`
Expected: FAIL（包不存在）

- [ ] **Step 3: 添加 xxhash 依赖**

Run: `go get github.com/cespare/xxhash/v2`

- [ ] **Step 4: 实现哈希包**

```go
// hash.go
package hash

import (
    "encoding/hex"
    "hash/fnv"
    "io"
    "os"

    "github.com/cespare/xxhash/v2"
)

const quickHashThreshold = 2 * 1024 * 1024 // 2MB

// FileHash calculates full xxhash of a file.
func FileHash(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil { return "", err }
    defer f.Close()

    h := xxhash.New()
    if _, err := io.Copy(h, f); err != nil { return "", err }
    return hex.EncodeToString(h.Sum(nil)), nil
}

// QuickHash reads head 1MB + tail 1MB + file size for fast comparison.
// Returns empty string for files smaller than threshold (use FileHash instead).
func QuickHash(path string) (string, error) {
    fi, err := os.Stat(path)
    if err != nil { return "", err }
    size := fi.Size()
    if size <= quickHashThreshold {
        return FileHash(path)
    }

    f, err := os.Open(path)
    if err != nil { return "", err }
    defer f.Close()

    h := fnv.New128a()
    // head 1MB
    head := make([]byte, 1024*1024)
    f.Read(head)
    h.Write(head)
    // size
    h.Write([]byte(fmt.Sprintf("%d", size)))
    // tail 1MB
    tail := make([]byte, 1024*1024)
    f.ReadAt(tail, size-int64(len(tail)))
    h.Write(tail)

    return hex.EncodeToString(h.Sum(nil)), nil
}

// ContentEqual checks if two files have identical content via hash.
func ContentEqual(p1, p2 string) (bool, error) {
    h1, err := FileHash(p1)
    if err != nil { return false, err }
    h2, err := FileHash(p2)
    if err != nil { return false, err }
    return h1 == h2, nil
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/pkg/hash/ -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/pkg/hash/ go.mod go.sum
git commit -m "feat: add xxhash-based file hashing with quick mode for large files"
```

---

## Phase 2: 同步引擎核心重构

### Task 3: Mode 合并 — Normal 模式引擎

**Files:**
- Modify: `internal/engine/engine.go` — `generateUpTasks`, `generateDownTasks`, `generateBothTasks` 及过滤逻辑

- [ ] **Step 1: 编写 Normal 模式测试 — SelectedFolders 过滤**

在 `engine_test.go` 中添加：

```go
func TestNormalMode_SelectedFolders(t *testing.T) {
    // 创建本地文件: work/a.txt, photos/b.txt, docs/c.txt
    // SyncPair: Mode="normal", SelectedFolders=`["work","docs"]`
    // 验证: 只有 work/ 和 docs/ 下的文件被同步
}

func TestNormalMode_SelectedFolders_ParentMerge(t *testing.T) {
    // 先添加 docs/work/2024
    // 再添加 docs/work
    // 验证: SelectedFolders 只包含 docs/work（子目录被合并）
}

func TestNormalMode_SelectedFolders_RemoveTrigger(t *testing.T) {
    // 方向 down, 已同步 work/a.txt 和 photos/b.txt
    // 取消选中 photos
    // 验证: 本地 photos/b.txt 被删除, work/a.txt 保留
}
```

- [ ] **Step 2: 实现过滤函数**

在 `engine.go` 中替换原 `filterPairFiles`：

```go
// filterBySelectedFolders returns true if the file path should be synced
// based on SelectedFolders. Empty SelectedFolders = all pass.
func filterBySelectedFolders(pair *store.SyncPair, relativePath string) bool {
    if pair.SelectedFolders == "" || pair.SelectedFolders == "[]" {
        return true
    }
    var folders []string
    json.Unmarshal([]byte(pair.SelectedFolders), &folders)
    for _, f := range folders {
        if strings.HasPrefix(relativePath, f+"/") || relativePath == f {
            return true
        }
    }
    return false
}

// normalizeSelectedFolders merges child paths into parent.
// e.g. ["docs/work/2024", "docs/work"] → ["docs/work"]
func normalizeSelectedFolders(folders []string) []string {
    sort.Strings(folders)
    var result []string
    for _, f := range folders {
        contained := false
        for _, existing := range result {
            if strings.HasPrefix(f, existing+"/") {
                contained = true
                break
            }
        }
        if !contained {
            result = append(result, f)
        }
    }
    return result
}
```

- [ ] **Step 3: 更新 generateUpTasks/generateDownTasks/generateBothTasks**

将所有 Mode 判断从 `"mirror"` / `"selective"` 改为统一用 `"normal"`。在 `generateTasks` 入口处统一调用 `filterBySelectedFolders` + `pathAllowed`（include/exclude）。

- [ ] **Step 4: 实现 SelectedFolders 变更时的文件处理**

在 `RefreshPairs` 检测到 `SelectedFolders` 变更时，对比新旧值：
- 新增的文件夹 → 根据方向生成下载/上传任务
- 移除的文件夹 → 根据方向删除已同步的文件

- [ ] **Step 5: 运行测试**

Run: `go test ./internal/engine/ -run TestNormalMode -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add internal/engine/
git commit -m "feat: merge mirror+selective into normal mode with SelectedFolders"
```

### Task 4: 目录跟踪与删除清理

**Files:**
- Modify: `internal/engine/engine.go` — 扫描和任务生成逻辑
- Modify: `internal/provider/local/local.go` — 目录感知
- Modify: `internal/provider/webdav/webdav.go` — 目录感知

- [ ] **Step 1: 编写目录同步测试**

```go
func TestDirectorySync_CreateRemote(t *testing.T) {
    // 远程有 docs/work/ 目录, 本地没有
    // 方向 down → 验证本地创建 docs/work/
}

func TestDirectorySync_DeleteWithContents(t *testing.T) {
    // 远程删除 docs/work/ 目录（含子文件）
    // 方向 down → 验证本地 docs/work/ 及子文件全部删除
}

func TestDirectorySync_EmptyDirPreserved(t *testing.T) {
    // docs/work/ 下所有文件被删除, 但目录还在
    // 验证: 目录本身不被删除
}
```

- [ ] **Step 2: 更新 scanRecursive 识别目录**

在扫描结果中为目录条目设置 `IsDir = true`，写入 `file_entries` 时标记 `is_dir`。

- [ ] **Step 3: 更新任务生成逻辑**

在 `generateUpTasks`/`generateDownTasks`/`generateBothTasks` 中：
- 先处理文件任务
- 再处理目录任务（确保目录内文件都处理完）
- 目录删除时，同时生成其下所有已同步文件的删除任务

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/engine/ -run TestDirectorySync -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/engine/ internal/provider/
git commit -m "feat: track directories as sync entities with recursive delete"
```

### Task 5: Virtual 模式固定方向 + 行为

**Files:**
- Modify: `internal/engine/engine.go`

- [ ] **Step 1: 编写 Virtual 模式测试**

```go
func TestVirtualMode_LocalUpload(t *testing.T) {
    // Virtual 模式, 本地新建文件 → 验证自动上传
}

func TestVirtualMode_RemoteReVirtualize(t *testing.T) {
    // 已 synced 文件, 远程修改 → 验证重新虚拟化 (sync_state → virtual)
}

func TestVirtualMode_FilterApplied(t *testing.T) {
    // Virtual 模式 + SelectedFolders=["docs"]
    // 验证: 只有 docs/ 下的远程文件建虚拟条目
}
```

- [ ] **Step 2: 实现 Virtual 模式固定 both**

在 `SyncPair` 入口处，如果 `Mode == "virtual"` 则强制 `Direction = "both"`。

- [ ] **Step 3: 实现远程修改重新虚拟化**

在 `generateBothTasks` 中，检测到远程已同步文件被修改时，如果是 virtual 模式，更新元数据但不下载，将 `sync_state` 改回 `virtual`。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/engine/ -run TestVirtualMode -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/engine/
git commit -m "feat: virtual mode fixed both direction with re-virtualization"
```

---

## Phase 3: 冲突检测优化

### Task 6: 两阶段哈希检测

**Files:**
- Modify: `internal/engine/engine.go` — `metaMatchesEntry` 和任务生成逻辑

- [ ] **Step 1: 编写哈希检测测试**

```go
func TestHashDetection_TouchNoChange(t *testing.T) {
    // 文件被 touch (ModTime 变, Size 不变, 内容不变)
    // 验证: 哈希相同, 不生成同步任务, 只更新 DB 中的 ModTime
}

func TestHashDetection_RealChange(t *testing.T) {
    // 文件内容真的变了
    // 验证: 哈希不同, 生成同步任务
}

func TestHashDetection_CachedReuse(t *testing.T) {
    // Size 和 ModTime 都没变
    // 验证: 不重新计算哈希, 复用缓存值
}
```

- [ ] **Step 2: 修改 metaMatchesEntry 为两阶段**

```go
func (e *Engine) detectChange(meta *provider.FileMeta, entry *store.FileEntry, side string) (bool, error) {
    // 第一阶段：元数据比较
    if meta.Size == entry.Size && timesClose(meta.ModTime, entry.ModTime) {
        return false, nil // 元数据没变
    }
    // 第二阶段：内容哈希确认
    var cachedHash string
    if side == "local" {
        cachedHash = entry.LocalHash
    } else {
        cachedHash = entry.RemoteHash
    }
    currentHash, err := e.computeHash(meta, side)
    if err != nil { return true, err } // 无法计算哈希时保守处理
    if currentHash == cachedHash {
        return false, nil // 误触发
    }
    return true, nil
}
```

- [ ] **Step 3: 更新 generateBothTasks 使用 detectChange**

替换原来的 `localChanged` / `remoteChanged` 计算逻辑。

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/engine/ -run TestHashDetection -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/engine/
git commit -m "feat: two-stage change detection with content hash verification"
```

### Task 7: 边界冲突场景

**Files:**
- Modify: `internal/engine/engine.go`

- [ ] **Step 1: 编写边界冲突测试**

```go
func TestConflict_ModifyVsDelete(t *testing.T) {
    // 本地修改 + 远程删除 + 数据库有 synced 记录
    // 验证: conflict_type = "modify_delete"
}

func TestConflict_DeleteVsModify(t *testing.T) {
    // 本地删除 + 远程修改 + 数据库有 synced 记录
    // 验证: conflict_type = "delete_modify"
}

func TestConflict_BothDelete(t *testing.T) {
    // 双方都删除 → 清理数据库记录, 不产生冲突
}

func TestConflict_FirstSyncSameContent(t *testing.T) {
    // 首次同步, 双方都有同名文件, 内容相同
    // 验证: 标记 synced, 不冲突
}

func TestConflict_FirstSyncDifferentContent(t *testing.T) {
    // 首次同步, 双方都有同名文件, 内容不同
    // 验证: 按策略处理冲突
}
```

- [ ] **Step 2: 更新 generateBothTasks 补全场景**

在 `hasLocal && !hasRemote` 和 `!hasLocal && hasRemote` 分支中增加冲突检测逻辑：

```go
// 场景 1: 本地修改 + 远程删除
if hasLocal && !hasRemote && entry != nil && isSynced(entry) {
    localChanged := entry.LocalHash != "" || !metaMatchesEntry(localMeta, entry.LocalMTime, entry.LocalSize)
    if localChanged {
        return conflictWithTasks(pair, key, "modify_delete", localMeta, nil, entry)
    }
    // 本地没变, 远程删了 → 正常删除本地
    return []SyncTask{newDeleteTask(pair.ID, key, "local")}
}

// 场景 2: 本地删除 + 远程修改
if !hasLocal && hasRemote && entry != nil && isSynced(entry) {
    remoteChanged := !metaMatchesEntry(remoteMeta, entry.RemoteMTime, entry.RemoteSize)
    if remoteChanged {
        return conflictWithTasks(pair, key, "delete_modify", nil, remoteMeta, entry)
    }
    // 远程没变, 本地删了 → 正常删除远程
    return []SyncTask{newDeleteTask(pair.ID, key, "remote")}
}
```

- [ ] **Step 3: 实现首次同步哈希比较**

```go
if hasLocal && hasRemote && entry == nil {
    // 首次遇到, 比较内容
    equal, err := e.compareContent(pair, key, localMeta, remoteMeta)
    if err == nil && equal {
        return nil // 内容相同, 后续 indexCleanFiles 会标记 synced
    }
    return conflictStrategyTasks(pair, key, localMeta, remoteMeta)
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/engine/ -run TestConflict -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/engine/
git commit -m "feat: handle modify-delete, delete-modify, and first-sync conflict scenarios"
```

### Task 8: 冲突策略精细化 — keep_both 和 rename

**Files:**
- Modify: `internal/engine/engine.go` — `conflictStrategyTasks` 和执行逻辑

- [ ] **Step 1: 编写新策略测试**

```go
func TestConflictStrategy_KeepBoth(t *testing.T) {
    // ConflictStrategy = "keep_both"
    // 验证: 两个版本都保存到 file_versions, 较新版本为当前文件
}

func TestConflictStrategy_Rename(t *testing.T) {
    // ConflictStrategy = "rename"
    // 验证: 被覆盖方重命名为 "file (冲突副本 2026-05-12).txt"
}
```

- [ ] **Step 2: 实现 keep_both 策略**

在 `doConflict` 中增加 `"keep_both"` case：

```go
case "keep_both":
    // 保存两个版本到 file_versions
    e.recordProviderVersion(ctx, pair, local, conflict.Path, "local")
    e.recordProviderVersion(ctx, pair, local, conflict.Path, "remote")
    // 取较新版本覆盖
    if localMeta.ModTime.After(remoteMeta.ModTime) {
        e.doUpload(ctx, pair, local, remote, conflict.Path)
    } else {
        e.doDownload(ctx, pair, local, remote, conflict.Path)
    }
```

- [ ] **Step 3: 实现 rename 策略**

```go
case "rename":
    base := strings.TrimSuffix(conflict.Path, filepath.Ext(conflict.Path))
    ext := filepath.Ext(conflict.Path)
    date := time.Now().Format("2006-01-02")
    newName := fmt.Sprintf("%s (冲突副本 %s)%s", base, date, ext)
    // 被覆盖方重命名
    if localMeta.ModTime.After(remoteMeta.ModTime) {
        remote.MoveFile(ctx, conflict.Path, newName)
        e.doUpload(ctx, pair, local, remote, conflict.Path)
    } else {
        local.MoveFile(ctx, conflict.Path, newName)
        e.doDownload(ctx, pair, local, remote, conflict.Path)
    }
```

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/engine/ -run TestConflictStrategy -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/engine/
git commit -m "feat: add keep_both and rename conflict strategies"
```

---

## Phase 4: 远程检测加速

### Task 9: WebDAV ETag 缓存与增量扫描

**Files:**
- Modify: `internal/provider/webdav/webdav.go` — ETag 支持
- Modify: `internal/provider/provider.go` — 新增接口方法

- [ ] **Step 1: 在 Provider 接口新增 IncrementalList 方法**

```go
// IncrementalList lists directory contents with ETag info for change detection.
// Returns unchanged=true if the directory ETag matches cachedTag.
type IncrementalLister interface {
    IncrementalList(ctx context.Context, path string, cachedTag string) ([]*FileMeta, bool, error)
}
```

- [ ] **Step 2: WebDAV 实现 IncrementalList**

```go
func (w *WebDAVProvider) IncrementalList(ctx context.Context, path string, cachedTag string) ([]*FileMeta, bool, error) {
    // PROPFIND depth=1 获取目录及其一级子项
    // 如果目录 ETag == cachedTag → unchanged=true, 不递归
    // 如果目录 ETag != cachedTag → 返回子项列表，带各自 ETag
}
```

- [ ] **Step 3: 引擎使用增量扫描**

在 `scanRemote` 中：对每个子目录检查 ETag，未变化的跳过。

- [ ] **Step 4: 可配置 scan_interval**

在 SyncPair 的 `ScanInterval` 字段基础上，更新 `periodicScan` 使用每个 pair 自己的间隔。

- [ ] **Step 5: 编写测试并提交**

```bash
go test ./internal/provider/webdav/ -v
git add internal/provider/ internal/engine/
git commit -m "feat: WebDAV incremental scan with ETag cache and per-pair scan interval"
```

---

## Phase 5: API 端点扩展

### Task 10: 文件浏览与文件夹选择 API

**Files:**
- Modify: `internal/server/handler/ws.go` — 新增 handler
- Modify: `internal/server/server.go` — 新增路由

- [ ] **Step 1: 实现文件列表 API**

```
GET /api/v1/pairs/{id}/files?path=xxx&side=local|remote
→ 返回指定路径下的文件和目录列表（一层），含 sync_state
```

- [ ] **Step 2: 实现文件夹选择 API**

```
POST /api/v1/pairs/{id}/folders/select
Body: {"selected_folders": ["work", "docs"]}
→ 更新 SelectedFolders，触发 RefreshPairs
```

- [ ] **Step 3: 实现 materialize API**

```
POST /api/v1/files/materialize
Body: {"pair_id": 1, "path": "docs/report.pdf"}
→ 触发虚拟文件实体化
```

- [ ] **Step 4: 编写 API 测试并提交**

```bash
go test ./internal/server/ -v
git add internal/server/
git commit -m "feat: add file browser, folder selection, and materialize APIs"
```

---

## Phase 6: React 前端

### Task 11: React 项目搭建 + 设计系统

**Files:**
- Create: `web/` 目录下所有文件

- [ ] **Step 1: 初始化 React 项目**

```bash
cd /home/rain/project/every-sync
npm create vite@latest web -- --template react-ts
cd web && npm install
```

- [ ] **Step 2: 配置 Vite 输出供 Go embed**

```ts
// vite.config.ts
export default defineConfig({
  build: { outDir: '../internal/server/static_dist' }
})
```

更新 `server.go` 中的 embed 路径指向 `static_dist`。

- [ ] **Step 3: 实现主题系统**

创建 `theme.ts`，包含亮/暗模式的 CSS 变量（参考 mockup 中的色值）。

- [ ] **Step 4: 实现 WebSocket hook 和 API 客户端**

`useWebSocket.ts`：自动重连、事件分发。
`client.ts`：REST API 封装。

- [ ] **Step 5: 实现共享组件**

Sidebar, StatusIcon, Breadcrumb, FolderCheck, ContextMenu, FileList

- [ ] **Step 6: 提交**

```bash
git add web/ internal/server/
git commit -m "feat: scaffold React frontend with theme system and shared components"
```

### Task 12: Dashboard 和 File Browser 页面

**Files:**
- Create: `web/src/pages/Dashboard.tsx`
- Create: `web/src/pages/FileBrowser.tsx`

- [ ] **Step 1: 实现 Dashboard**

指标卡片、流量统计、活跃同步对列表。参考 mockup 中的 Dashboard 页面。

- [ ] **Step 2: 实现 File Browser**

文件管理器风格（一层一层浏览），面包屑导航，文件夹勾选框，同步状态图标，Local/Remote 切换。参考 mockup 中的 File Browser 页面。

- [ ] **Step 3: 提交**

```bash
git add web/src/pages/
git commit -m "feat: implement Dashboard and File Browser pages"
```

### Task 13: Sync Pairs、Conflicts、其余页面

**Files:**
- Create: `web/src/pages/SyncPairs.tsx`
- Create: `web/src/pages/Conflicts.tsx`
- Create: `web/src/pages/Providers.tsx`
- Create: `web/src/pages/Versions.tsx`
- Create: `web/src/pages/Logs.tsx`

- [ ] **Step 1: 实现 SyncPairs 页面**

同步对列表（含 Mode/Direction/ScanInterval/SelectedFolders 展示），创建/编辑表单，SelectedFolders 可视化选择器。

- [ ] **Step 2: 实现 Conflicts 页面**

冲突类型分标签（All / Modify↔Modify / Modify↔Delete / Delete↔Modify），按类型显示不同的解决按钮。

- [ ] **Step 3: 实现 Providers、Versions、Logs 页面**

- [ ] **Step 4: 提交**

```bash
git add web/src/pages/
git commit -m "feat: implement Sync Pairs, Conflicts, Providers, Versions, and Logs pages"
```

---

## Phase 7: 集成测试

### Task 14: Local ↔ Local 集成测试

**Files:**
- Create: `internal/test/integration/integration_local_test.go`

- [ ] **Step 1: 编写本地集成测试**

覆盖设计文档中列出的全部 13 个本地测试场景。用 `go test -tags=integration` 控制。

- [ ] **Step 2: 运行并验证通过**

Run: `go test -tags=integration ./internal/test/integration/ -run TestLocal -v`
Expected: ALL PASS

- [ ] **Step 3: 提交**

```bash
git add internal/test/integration/
git commit -m "test: add comprehensive local sync integration tests"
```

### Task 15: WebDAV 真实环境测试

**Files:**
- Create: `internal/test/integration/integration_webdav_test.go`

- [ ] **Step 1: 编写 WebDAV 集成测试**

覆盖设计文档中列出的 8 个 WebDAV 测试场景。使用环境变量 `WEBDAV_URL`, `WEBDAV_USER`, `WEBDAV_PASS` 提供连接信息。

- [ ] **Step 2: 提交**

```bash
git add internal/test/integration/
git commit -m "test: add WebDAV integration tests for real environment validation"
```

---

## 依赖关系

```
Task 1 (迁移) → Task 2 (哈希包)
    ↓                ↓
Task 3 (Normal模式) → Task 4 (目录跟踪) → Task 5 (Virtual模式)
    ↓
Task 6 (哈希检测) → Task 7 (边界冲突) → Task 8 (新策略)
    ↓
Task 9 (远程检测) → Task 10 (API端点)
                         ↓
              Task 11 (React搭建) → Task 12 (Dashboard+FileBrowser) → Task 13 (其余页面)
              ↓
Task 14 (本地测试) → Task 15 (WebDAV测试)
```

**可并行的任务：**
- Task 2 (哈希) 和 Task 3 (Normal模式) 可以并行
- Phase 6 (React) 的 Task 11-13 和 Phase 3-4 (冲突/检测) 可以并行
- Task 14 和 Task 15 测试在对应功能完成后编写
