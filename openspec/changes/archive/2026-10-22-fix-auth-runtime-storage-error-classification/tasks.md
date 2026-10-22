## 1. 根因隔离与实现边界

- [x] 1.1 在独占 MySQL、Redis 环境中重现两条 Access 故障路径：仅使 `user_access_versions` 查询失败，以及停止完整 MySQL；分别记录 HTTP 状态、错误码、`data`、Token 签发和版本行状态。
- [x] 1.2 沿 `users.FindByID`、`authorization.Snapshot`、`AccessVersions.Current` 和 HTTP 认证中间件追踪错误身份，确认每个已复现故障只在明确的存储边界分类，不使用错误文本或空结果猜测。
- [x] 1.3 根据 1.1 的真实结果确定最小受影响文件集合；当前没有可确认的生产修复文件，表级通过、完整故障通过和连接时序传输失败均记录为证据结论，历史 `401` 保留为不可复现证据缺口。

## 2. Access 契约回归保护

- [x] 2.1 核对现有 GORM `record not found` 到用户不存在／`ErrAccessVersionNotFound` 的类型化哨兵映射；确认当前基线无需修改生产仓储逻辑。
- [x] 2.2 核对 `users.FindByID`、`authorization.Snapshot`、`AccessVersions.Current`、`Authenticate` 的错误传播；确认当前基线无需修改 Access 生产逻辑。
- [x] 2.3 核对认证 HTTP 中间件的稳定响应：Access 基础设施故障为 HTTP `500`、`IDENTITY_INTERNAL_ERROR`、`data=null`，不回显底层错误。
- [x] 2.4 运行现有测试验证明确缺失版本、明确不存在用户、禁用用户和 Token 版本不一致语义；只有发现具体覆盖缺口才增加最小回归保护。
- [x] 2.5 运行现有 Refresh 测试验证无效、缺版本、版本不一致和基础设施失败继续返回 HTTP `401 AUTHN_REFRESH_TOKEN_INVALID`，不签发 Token。

## 3. 回归验证

- [x] 3.1 运行服务层错误分类回归测试；若现有测试已覆盖全部边界，不新增实现性或重复性测试。
- [x] 3.2 运行 HTTP 层认证错误回归测试，验证 Access `500`／`401`、`data=null` 和安全消息；只在真实覆盖缺口存在时补测试。
- [x] 3.3 复核并归档 1.1 的真实 MySQL 表级故障结果：用户表可读、版本表不可用时 Access 返回 `500 IDENTITY_INTERNAL_ERROR`，无补行、无回退、无 Token。
- [x] 3.4 复核并归档 1.1 的完整 MySQL 与连接时序结果：当前均未复现 `401`；传输失败不写成业务认证通过或产品缺陷。
- [x] 3.5 复核 MySQL 恢复后的真实登录、Access `/api/user/info` 和 Refresh 正常结果；确认故障期间没有签发或持久化异常 Token。

## 4. 契约与证据同步

- [x] 4.1 将当前基线的去敏 HTTP 结果、故障注入阶段、错误分类、版本行和 Token 签发事实写入独立证据文件；不保存 JWT、密码、验证码答案、Authorization header、DSN 或底层错误原文。
- [x] 4.2 更新证据说明，明确表级故障、完整 MySQL 故障、连接时序探针和历史不可复现 `401`；未执行或未复现路径不得写成通过。
- [x] 4.3 复核本 change 未新增路由、权限码、模型、Redis key、MinIO bucket、配置项或第三方依赖；主规格同步与归档按独立 OpenSpec 流程处理。

## 5. 最终验证与清理

- [x] 5.1 运行受影响包的 Go 单元／HTTP 测试和必要的真实 MySQL 集成门禁，确认测试结果与去敏证据一致。
- [x] 5.2 运行 OpenSpec 严格校验，确认 auth delta 完整保留 Access 缺行、Refresh 401、禁用用户和 Token 版本失效场景。
- [x] 5.3 清理本 change 创建的隔离服务、schema、Redis 数据、临时 Runner、Token、验证码、配置和凭据，确认未写入共享业务库。
