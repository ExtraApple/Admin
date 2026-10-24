## Why

A2 真实 MySQL/MinIO 审计发现，消息图片对象删除失败时，Runner 返回稳定存储失败分类，但清理 Job 仍可能保持 `pending` 且 `last_error_code` 为空。根因是 MySQL `datetime(3)` 与 Go 纳秒时间比较导致条件更新无法命中，清理队列失去失败原因记录。

## What Changes

- 统一消息图片清理 Job 的时间值为 UTC 毫秒精度。
- 规范化注册、重试预留、失败更新和终结更新使用的时间值。
- 保留现有条件预留 CAS：Job ID、状态、retry_count 和 next_retry_at 必须同时匹配。
- 保持初次对象失败的状态语义：`pending`、`retry_count=0`、稳定 `last_error_code`、保留下一次重试时间。
- 保持第 24 次失败转 `dead`、dead 不重入和迟到结果保护。
- 增加真实 MySQL 时间精度、失败状态和竞争重试回归测试。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `file-management`: 明确消息图片清理失败状态、重试时间和稳定失败码必须在 MySQL 毫秒精度下可靠持久化；不改变重试次数和 dead 语义。

## Impact

- 代码：`internal/files/adapters/gorm/cleanup.go`、Files 清理 adapter 测试及 MySQL integration 测试。
- 规格 delta：仅记录既有清理契约的实现修复；归档时使用 `--skip-specs`，不同步主规格。
- 数据：不回填、不扫描、不批量重写历史清理 Job；不重置 retry_count。
- 运行时：存储失败可恢复队列保留冻结位置、下一次调度和稳定失败码。
- 非目标：不改变对象存储重试周期、24 次上限、dead 处置流程、bucket、object key 或后台调度。
