## 1. 时间精度与状态实现

- [x] 1.1 在 Files GORM cleanup adapter 增加 UTC 毫秒时间规范化，覆盖注册、重试预留、失败更新、dead 转换和终结条件。
- [x] 1.2 保持 `id + status + retry_count + next_retry_at` CAS 条件，不放宽迟到 Worker 防护。
- [x] 1.3 修复初次对象删除失败状态持久化：保留 `pending`、`retry_count=0`、`next_retry_at`，并可靠写入 `last_error_code`。
- [x] 1.4 保持第 24 次 dead、dead 不重入、NoSuchKey 成功终结和迟到失败不复活语义。

## 2. 回归测试

- [x] 2.1 增加 adapter 测试，使用带纳秒时间验证写入/读取/失败更新使用毫秒精度。
- [x] 2.2 增加真实 MySQL integration，确认对象失败后 `last_error_code` 实际落库且队列状态可恢复。
- [x] 2.3 保留并增强多 Worker 条件预留竞争测试，确认只有一个预留赢家且不重复递增。
- [x] 2.4 运行第 24 次、dead 不重入、迟到失败、NoSuchKey 和回滚测试。

## 3. 交付边界

- [x] 3.1 复核不新增数据库字段、不迁移或回填历史 Job、不重置 retry_count、不改变调度和对象位置语义。
- [x] 3.2 保留 `file-management` change-local delta，归档时使用 `--skip-specs`，不回写主规格。
