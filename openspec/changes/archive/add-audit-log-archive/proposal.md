# add-audit-log-archive

## Why

审计日志会持续增长。长期把所有记录留在 `audit_logs` 热表，会影响后台常用查询性能，也会增加表维护成本。

## What Changes

- 新增 `audit_log_archives` 冷表。
- 新增审计日志冷热归档配置。
- 新增归档任务，将超过保留天数的热日志迁移到冷表。
- 归档采用批量复制成功后删除热表记录的方式。

## Out of Scope

- 不做按月分表。
- 不做 MinIO 文件归档。
- 不做跨冷热表联合查询。
- 不做手动触发接口。
