## Context

A2 真实 MySQL/MinIO 场景中，消息图片对象删除失败后，`FailMessageImageCleanup` 的条件更新没有命中。清理表使用 MySQL `datetime(3)`，而 Runner 传入的 Go 时间可能包含纳秒；注册返回的内存 Job 时间与数据库截断值不相等，导致 `last_error_code` 丢失。

现有清理状态机、CAS 预留和 0–24 retry 边界已经由规格和测试定义。本 change 只修复时间表示的一致性。

## Goals / Non-Goals

**Goals:**

- 所有清理 Job 持久化和条件更新使用 UTC 毫秒精度。
- 首次对象失败可靠写入稳定 `last_error_code`，保持 pending 和 retry_count=0。
- 保留 retry reservation、迟到结果和 dead 终态的 CAS 保护。
- 用真实 MySQL 覆盖纳秒输入、失败更新和竞争预留。

**Non-Goals:**

- 不改变 retry 次数、调度间隔、dead 处置或对象存储错误分类。
- 不迁移、扫描、补写或重置历史清理 Job。
- 不新增 processing 状态、租约、清理状态字段或人工 HTTP 接口。
- 不改变 bucket、object key、队列 schema 或后台调度。

## Decisions

### 1. 在 GORM cleanup adapter 统一规范化时间

新增内部时间规范化逻辑，将进入 `MessageImageCleanupJob` 或 CAS 参数的时间转为 `UTC().Truncate(time.Millisecond)`。注册、retry reservation、failure update、dead transition 和 completion 使用同一精度。

备选方案是放宽 `WHERE next_retry_at` 条件，仅按 Job ID/status/retry_count 更新；这会削弱迟到 Worker 防护，因此不采用。修改数据库精度也不必要，因为现有 schema 已是 `datetime(3)`。

### 2. 保持原有状态机

- 初次失败：pending、retry_count=0、last_error_code 有值、next_retry_at 保留。
- 预留成功后 retry_count 增加一次；失败更新只匹配当前预留时间。
- 第 24 次失败转 dead；dead 不再次预留。
- 成功或 NoSuchKey 仍条件删除 File Record 和 Job。

### 3. 以真实 MySQL 作为精度门禁

SQLite 测试继续覆盖状态机边界；MySQL integration 使用未截断纳秒时间，验证真实 `datetime(3)` 写入、读取和条件更新。并发测试继续验证只有一个条件预留赢家。

## Risks / Trade-offs

- 历史 Job 如果已经处于异常状态，不会被本 change 自动修复；需要独立数据处置。
- 毫秒截断会丢弃纳秒信息，但与现有 MySQL schema 精度一致，且不影响小时级调度。
- 如果调用方未来传入非 UTC 时间，adapter 仍统一转 UTC，避免跨时区 CAS 不一致。
