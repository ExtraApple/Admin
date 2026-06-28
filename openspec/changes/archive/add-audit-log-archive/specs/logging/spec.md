## ADDED Requirements

### Requirement: 审计日志冷热归档
系统 SHALL 支持将超过保留天数的审计日志从热表归档到冷表。

#### Scenario: 归档任务未启用
- **WHEN** `audit_log_archive.enabled` 为 false
- **THEN** 系统 SHALL NOT 启动审计日志归档任务

#### Scenario: 归档过期日志
- **WHEN** `audit_log_archive.enabled` 为 true
- **AND** `audit_logs` 中存在早于 `retention_days` 的记录
- **THEN** 系统按 `batch_size` 批量复制记录到 `audit_log_archives`
- **AND** 只有复制成功后才删除 `audit_logs` 中对应记录
