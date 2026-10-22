## 1. 根因隔离与实现边界

- [ ] 1.1 在独占 MySQL、Redis 环境中重现两条 Access 故障路径：仅使 `user_access_versions` 查询失败，以及停止完整 MySQL；分别记录 HTTP 状态、错误码、`data`、Token 签发和版本行状态。
- [ ] 1.2 沿 `users.FindByID`、`authorization.Snapshot`、`AccessVersions.Current` 和 HTTP 认证中间件追踪错误身份，确认每个已复现故障只在明确的存储边界分类，不使用错误文本或空结果猜测。
- [ ] 1.3 根据 1.1 的真实结果确定最小受影响文件集合；未复现的路径保持不改，并将表级通过／整体故障失败等分支记录为证据结论而不是推测。

## 2. Access 错误分类修复

- [ ] 2.1 保留明确 GORM `record not found` 到用户不存在／`ErrAccessVersionNotFound` 的类型化哨兵映射，确保连接、查询、表不可用和驱动错误原样保留为可识别的内部错误。
- [ ] 2.2 修复已确认的 Access 身份或授权版本错误传播，使真实基础设施故障到达 `CodeInternalError`，而不会被映射为 `CodeTokenInvalid`；不得补版本、回退旧存储或签发 Token。
- [ ] 2.3 在认证 HTTP 中间件中保持稳定响应：Access 基础设施故障返回 HTTP `500`、`IDENTITY_INTERNAL_ERROR`、`data=null`，不回显数据库、GORM、连接或堆栈信息。
- [ ] 2.4 保持明确缺失版本、明确不存在用户、禁用用户和 Token 版本不一致的既有语义：分别为 `401 AUTHN_TOKEN_INVALID`、`401 AUTHN_TOKEN_INVALID`、`403 IDENTITY_*` 和 `401 AUTHN_*`。
- [ ] 2.5 保持 Refresh 既有统一拒绝契约：无效、缺版本、版本不一致或 Refresh 基础设施失败均返回 HTTP `401 AUTHN_REFRESH_TOKEN_INVALID`、`data=null`，不签发新 Token。

## 3. 回归验证

- [ ] 3.1 增加或调整服务层回归测试，覆盖明确缺失哨兵、用户／版本基础设施错误、禁用用户和 Token 版本不一致的应用错误码；禁止测试实现细节或字符串偶然值。
- [ ] 3.2 增加或调整 HTTP 层回归测试，验证 Access `500 IDENTITY_INTERNAL_ERROR` 与 Access `401 AUTHN_TOKEN_INVALID` 的状态、错误码、`data=null` 和安全消息；同时验证 Refresh 的统一 `401`。
- [ ] 3.3 运行真实 MySQL 表级故障验收：用户表可读、授权版本查询失败时 Access 返回 `500 IDENTITY_INTERNAL_ERROR`，无补行、无回退、无 Token。
- [ ] 3.4 运行真实 MySQL 完整故障验收：用户查询失败时只要该路径属于已确认的基础设施故障就返回 `500 IDENTITY_INTERNAL_ERROR`；若路径未被当前实现覆盖，记录证据缺口而不扩大修复范围。
- [ ] 3.5 恢复 MySQL 后通过真实登录、Access `/api/user/info` 和 Refresh 验证正常链路恢复；确认故障期间没有签发或持久化异常 Token。

## 4. 契约与证据同步

- [ ] 4.1 将修复前后去敏 HTTP 结果、故障注入阶段、错误分类、版本行和 Token 签发事实写入独立修复证据文件；不保存 JWT、密码、验证码答案、Authorization header、DSN 或底层错误原文。
- [ ] 4.2 更新修复说明，明确表级故障与完整 MySQL 故障的实际覆盖结果；未执行或未复现路径不得写成通过，并链接当前 auth delta 与原始审计缺陷证据。
- [ ] 4.3 复核本 change 未新增路由、权限码、模型、Redis key、MinIO bucket、配置项或第三方依赖；主规格同步与归档按独立 OpenSpec 流程处理。

## 5. 最终验证与清理

- [ ] 5.1 运行受影响包的 Go 单元／HTTP 测试和必要的真实 MySQL 集成门禁，确认测试结果与去敏证据一致。
- [ ] 5.2 运行 OpenSpec 严格校验，确认 auth delta 完整保留 Access 缺行、Refresh 401、禁用用户和 Token 版本失效场景。
- [ ] 5.3 清理本 change 创建的隔离服务、schema、Redis 数据、临时 Runner、Token、验证码、配置和凭据，确认未写入共享业务库。
