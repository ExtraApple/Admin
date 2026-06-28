# Audit Log Archive Design

## Tables

| 表 | 说明 |
|---|---|
| `audit_logs` | 热日志表，保留最近 N 天 |
| `audit_log_archives` | 冷日志表，保存历史日志 |

## Archive Flow

```text
定时任务
  → 查询 audit_logs 中 created_at < now - retention_days 的记录
  → 按 batch_size 批量读取
  → 写入 audit_log_archives
  → 写入成功后删除 audit_logs 中对应记录
```

## Config

```yaml
audit_log_archive:
  enabled: false
  retention_days: 90
  batch_size: 1000
```

## Safety

- 归档任务默认关闭。
- 只有写入冷表成功后才删除热表记录。
- 每次只处理一批，避免长事务和大批量删除。
