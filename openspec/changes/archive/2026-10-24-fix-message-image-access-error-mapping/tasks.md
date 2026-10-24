## 1. 跨模块错误翻译

- [x] 1.1 在 `messagingFilesReader.OpenMessageImage` 增加 Files → Messaging 错误翻译，不让 Files 具体错误类型穿过模块边界。
- [x] 1.2 将 access-invalid、对象不存在、未绑定和已登记清理映射为 `messagingapplication.ErrNotFound`。
- [x] 1.3 将存储不可用、数据库失败和未知错误映射为受控内部失败，保持 `MSG_INTERNAL_ERROR`，不暴露原始错误或对象定位。
- [x] 1.4 保持 Messaging 先执行消息可见性检查；不可见消息不得调用 Files。

## 2. 回归测试

- [x] 2.1 增加 adapter 错误翻译测试，覆盖 access-invalid、object-not-found、storage/persistence error。
- [x] 2.2 增加 Messaging service 测试，确认不可见消息直接返回 `ErrNotFound` 且 Files 不被调用。
- [x] 2.3 增加 HTTP 测试：已登记清理/不可访问图片返回 `404 / MSG_NOT_FOUND`，基础设施失败返回 `500 / MSG_INTERNAL_ERROR`。
- [x] 2.4 增加正常图片二进制读取回归测试，确认 Content-Type 和原生流响应不变。

## 3. 交付边界

- [x] 3.1 运行 Messaging、App HTTP 和文件图片相关定向测试，确认无新增公开错误码、路由、权限码或存储策略。
- [x] 3.2 保留 `internal-messaging` change-local delta，归档时使用 `--skip-specs`，不回写主规格。
