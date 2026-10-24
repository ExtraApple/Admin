## Why

A2 发现已登记清理或不可访问的消息图片经过 Files adapter 后，错误没有被 Messaging 组合根翻译，最终从真实图片读取入口返回 `500 / MSG_INTERNAL_ERROR`。这既不符合消息资源不可见时的稳定 404 语义，也暴露了跨模块错误实现边界。

## What Changes

- 在 App 组合根的 Messaging-owned Files adapter 中翻译 Files 错误。
- access-invalid、对象不存在、未绑定或已登记清理统一映射为 Messaging `ErrNotFound`，返回 `404 / MSG_NOT_FOUND`。
- 存储不可用、数据库失败和未知基础设施错误映射为受控内部失败，返回 `500 / MSG_INTERNAL_ERROR`。
- 保持 Messaging 先执行消息可见性判断；不可见消息不调用 Files。
- 增加 adapter、Messaging service 和真实 HTTP 图片读取回归测试。
- 不新增公开错误码，不返回 Files 原始错误、对象路径或底层存储错误。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `internal-messaging`: 明确消息图片不可访问与基础设施失败的稳定 HTTP 错误边界；不改变可见性判定和二进制成功响应。

## Impact

- 代码：`internal/app/messaging.go`、Messaging HTTP/service 测试和错误分类测试。
- 规格 delta：仅记录已发布消息图片安全契约的实现修复；归档时使用 `--skip-specs`，不同步主规格。
- 数据：无迁移、无图片清理历史回填。
- 运行时：不可访问图片不再泄露 Files 错误实现，正常图片仍返回原生二进制。
- 非目标：不改变 Files domain 错误定义、不新增路由/权限码、不改变对象存储策略或消息可见性规则。
