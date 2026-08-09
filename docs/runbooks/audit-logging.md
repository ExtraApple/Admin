# 操作日志

> 本文只保留审计日志的分类、脱敏和归档边界。查询接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/logging/spec.md`](../../openspec/specs/logging/spec.md) 为准。

## 与运行日志的区别

| 类型 | 用途 | 存储 |
|---|---|---|
| Zap 运行日志 | 启动、请求、后台任务和故障排查 | 标准输出与轮转文件 |
| 审计日志 | 谁在何时调用了什么 API，以及结果和耗时 | `audit_logs` / `audit_log_archives` |

审计日志不能用运行日志替代。

## 记录内容

`/api/*` 请求记录 method、path、query、status、duration、client IP、user agent、创建时间以及可用的用户 ID。

分类：

| category | 用途 |
|---|---|
| `login` | 登录请求 |
| `permission` | 权限、菜单、角色绑定关系的写操作 |
| `operation` | 其他业务写操作 |
| `data_access` | GET 查询 |
| `api` | 其他 API 请求 |

GET 查询类角色、权限和菜单接口仍属于 `data_access`，不属于权限变更。

## 脱敏与上传审计

- JSON 中的密码、验证码和 Token 字段写为 `***`。
- multipart 请求不读取或记录文件内容，Body 使用固定省略标记。
- 上传审计只记录用途、清洗后的文件名、大小、声明/检测 MIME、结果、策略版本和稳定原因码。
- 不记录二进制内容、原始客户端路径、object key、签名 URL、存储凭据或底层解析错误。
- 异步审计写入失败不改变业务响应，但必须写入受控运行日志。

## 查询入口

Swagger UI 的 `audit` 标签提供：

- 全部审计日志。
- 登录日志。
- 操作日志。
- 权限变更日志。
- 数据访问日志。

查询支持的分页和筛选参数以运行中的 OpenAPI 与处理器实现为准。

## 冷热归档

```yaml
audit_log_archive:
  enabled: true
  retention_days: 90
  batch_size: 1000
```

启用后，后台任务按批次把超过保留天数的日志及 metadata 从热表复制到冷表；只有复制成功后才删除热表记录。关闭时不启动归档任务。
