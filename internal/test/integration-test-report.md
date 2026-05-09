# 123pan WebDAV 集成测试报告

**测试日期**: 2026-05-09
**测试目标**: 123pan WebDAV 云存储
**测试端点**: `https://webdav.123pan.cn/webdav`
**测试目录**: `/test`

## 测试环境

- 同步引擎: every-sync engine (2 workers, queue=100, retry=2, retry_delay=2s)
- 本地存储: 临时目录 (t.TempDir)
- 远程存储: 123pan WebDAV `/test/<pair-name>`
- 超时设置: SyncPair 120s, Drain 60s

## 测试概要

| 分类 | 测试数量 | 通过 | 失败 |
|------|---------|------|------|
| 基础同步（双向/单向） | 6 | 6 | 0 |
| 同步模式（mirror/selective/virtual） | 4 | 4 | 0 |
| 冲突策略 | 4 | 4 | 0 |
| 包含/排除规则 | 2 | 2 | 0 |
| 增量同步 | 3 | 3 | 0 |
| 大文件/分块传输 | 3 | 3 | 0 |
| 大量小文件 | 3 | 3 | 0 |
| 目录/嵌套结构 | 3 | 3 | 0 |
| 删除传播 | 3 | 3 | 0 |
| **总计** | **31** | **31** | **0** |

**总耗时**: ~765 秒（约 12.75 分钟）

## 详细测试结果

### 1. 基础同步功能

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_UploadSync` | PASS | 本地→远程上传同步 |
| `TestIntegration_DownloadSync` | PASS | 远程→本地下载同步 |
| `TestIntegration_BidirectionalSync` | PASS | 双向同步：初始同步后双方一致 |
| `TestIntegration_BidirectionalSecondSync_NoDelete` | PASS | 二次同步不会删除已同步文件 |
| `TestIntegration_DryRun_NoWrite` | PASS | DryRun 模式不写入文件或索引 |
| `TestIntegration_IdempotentSync` | PASS | 多次同步不改变文件版本号 |

### 2. 同步模式

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_SelectiveMode_SkipsExcluded` | PASS | selective 模式跳过 *.tmp 文件 |
| `TestIntegration_SelectiveMode_OnlyIncludes` | PASS | 仅同步 include 指定的 .go/.txt 文件 |
| `TestIntegration_VirtualMode_IndexOnly` | PASS | virtual 模式只索引不下载文件 |
| `TestIntegration_VirtualMode_Materialize` | PASS | virtual 文件可按需物化下载 |

### 3. 冲突策略

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_Conflict_LocalWins` | PASS | 双方修改冲突时保留本地版本 |
| `TestIntegration_Conflict_RemoteWins` | PASS | 双方修改冲突时保留远程版本 |
| `TestIntegration_Conflict_LatestWins` | PASS | 双方修改冲突时保留较新版本 |
| `TestIntegration_Conflict_ManualResolution` | PASS | 冲突记录后可手动解决 |

### 4. 包含/排除规则

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_ExcludePatterns` | PASS | 排除 *.log 和 *.tmp 文件 |
| `TestIntegration_IncludePatterns` | PASS | 仅包含 *.txt 和 *.md 文件 |

### 5. 增量同步

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_IncrementalSync_OnlyChangedFiles` | PASS | 仅同步有变化的文件，未变化文件不重复传输 |
| `TestIntegration_ModifyAfterSync_Bidirectional` | PASS | 初始同步后修改一方，增量同步正确传播 |
| `TestIntegration_IncrementalSync_VersionUnchanged` | PASS | 无变化文件的版本号不变 |

### 6. 大文件/分块传输

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_LargeFile_Upload` | PASS | 上传 >8MB 文件（分块阈值以上） |
| `TestIntegration_LargeFile_Download` | PASS | 下载 >8MB 文件 |
| `TestIntegration_LargeFile_Bidirectional` | PASS | 双向同步大文件，内容一致 |

### 7. 大量小文件

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_ManySmallFiles_Upload_100` | PASS | 上传 100 个小文件，全部同步 |
| `TestIntegration_ManySmallFiles_Download_100` | PASS | 下载 100 个小文件，全部同步 |
| `TestIntegration_ManySmallFiles_Bidirectional_50` | PASS | 双向同步 50 个文件 |

### 8. 目录/嵌套结构

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_NestedDirectories` | PASS | 3 层嵌套目录结构正确同步 |
| `TestIntegration_DeeplyNestedFiles` | PASS | 5 层深度嵌套文件正确同步 |
| `TestIntegration_MixedFilesAndDirs` | PASS | 文件和目录混合同步 |

### 9. 删除传播

| 测试 | 结果 | 说明 |
|------|------|------|
| `TestIntegration_DeletePropagation_Up` | PASS | 本地删除→远程删除 |
| `TestIntegration_DeletePropagation_Down` | PASS | 远程删除→本地删除 |
| `TestIntegration_DeletePropagation_Bidirectional` | PASS | 双向删除传播 |

## 发现的 123pan WebDAV 特性问题

### 1. DELETE 操作最终一致性
- **现象**: `DELETE` 请求返回成功 (200/204)，但文件可能在数秒内仍然存在
- **影响**: 测试中删除文件后立即检查可能失败
- **解决方案**: 在测试中使用重试机制（最多 5 次，间隔 1 秒）确认删除完成

### 2. MTime 不稳定性
- **现象**: 同一文件的 `Stat` 和 `ListDir` 返回的修改时间可能不同
- **影响**: 基于时间戳的变更检测可能产生误判
- **解决方案**: 引擎使用 1 秒容差 (`timesClose`)，但极端情况下仍需注意

### 3. 无内容哈希验证
- **现象**: 123pan WebDAV 不返回 ETag 或内容哈希
- **影响**: 无法通过哈希验证文件完整性，完全依赖大小+时间戳
- **解决方案**: 同等内容但不同大小的文件可被检测；相同大小不同内容需依赖时间戳判断

## 测试文件

- 测试文件: `internal/test/integration/integration_123pan_test.go`
- 构建标签: `//go:build integration`
- 运行命令: `go test -tags=integration -v -timeout=1200s ./internal/test/integration/`
