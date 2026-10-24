## 1. Body 脱敏

- [x] 1.1 在 `internal/audit` 增加路径感知 Body 清洗入口，Messaging JSON 请求统一返回固定 `[message omitted]`。
- [x] 1.2 保持 multipart `[multipart omitted]` 和 UploadAuditMetadata allowlist 行为不变。
- [x] 1.3 确认消息 Body 不保留 title、Markdown、HTML、图片引用、正文或外链 URL。

## 2. Query allowlist

- [x] 2.1 将审计 Query 改为显式 allowlist：`page`、`size`、`user_id`、`method`、`path`、`status`、`category`、`start_time`、`end_time`。
- [x] 2.2 丢弃 `signature`、`expires`、ticket、Token、Authorization 及其他未批准参数。
- [x] 2.3 保持审计查询接口的分页、筛选字段和历史记录读取行为不变。

## 3. 回归测试

- [x] 3.1 增加 sanitizer 单测，覆盖消息 Body 固定省略、multipart 固定省略和普通 JSON 敏感字段遮蔽。
- [x] 3.2 增加 Query allowlist 单测，确认签名下载参数和未批准参数不进入结果，批准筛选参数仍保留。
- [x] 3.3 增加 HTTP middleware/最终 Audit Log 测试，确认请求、Query、Body 和 metadata 均满足脱敏边界。
- [x] 3.4 运行审计、上传、消息和日志定向测试，确认 RuntimeLogger 事件名/字段白名单不变。

## 4. 历史与交付边界

- [x] 4.1 确认修复只影响新写入 Audit Log，不删除、不回写历史 `audit_logs` 或 `audit_log_archives`。
- [x] 4.2 保留 `logging` change-local delta，归档时使用 `--skip-specs`，不回写主规格。
