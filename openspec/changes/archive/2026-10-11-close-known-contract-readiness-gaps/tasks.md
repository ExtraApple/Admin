## 1. Service 与 Handler 行为保护

- [x] 1.1 删除 `UpdateByAdmin` 中不可达的邮箱查重／赋值分支及空更新判断中的失效邮箱条件，保留前置拒绝、共享邮箱字段和其他可达更新语义；不修改 Model、DTO 或生产路由。
- [x] 1.2 增强既有 `TestUpdateByAdminRejectsEmailField`，验证混合提交非空邮箱及其他资料字段时整次更新被拒绝，字段错误保持稳定，目标用户资料、邮箱状态和授权版本不变；使用行为状态断言，不检查源代码文本或调用次数。
- [x] 1.3 在 `internal/apidoc/service_test.go` 复用 Metadata fixture 注入可控失败，增加真实 `Service.OpenAPI` handler 回归，验证 HTTP 500、四字段信封、`code=500`、`HTTP_INTERNAL_ERROR`、`data=null` 及内部错误／敏感标记不泄露，保留成功文档验证。

## 2. 运行行为验收

- [x] 2.1 代码和永久回归完成后运行一次 `go test ./... -count=1`，确认 Identity 自助邮箱与管理员保护、认证锁定／登出及 API Doc 行为验证通过；记录实际结果，不新增文案或内部默认值快照测试。
- [x] 2.2 用临时本地 HTTP 入口挂载真实 OpenAPI handler，实际请求 `/docs/openapi.json` 的受控 Metadata 失败路径，记录状态、信封及不泄露注入错误的结果；不破坏真实依赖来制造失败，不把回归通过代替 HTTP 冒烟。

## 3. 运维说明与归档入口

- [x] 3.1 修正 `docs/runbooks/login-security.md` 的锁定档位、错误累计续期与清除说明，明确第 10 次和锁定到期不清零、每次记录失败续期 24 小时、成功登录清除。
- [x] 3.2 修正同一 runbook 的登出黑名单 TTL 为 `jwt.expire` 配置固定时长，保持既有生产装配和主规格，不改为 Token 剩余有效期。
- [x] 3.3 修正 README 中联合验收记录、响应基线、错误矩阵和 320px 截图的归档路径，保留原历史日期与结论。
- [x] 3.4 修正 `docs/module-navigation.md` 和 ADR 0011 中两个关联 change 的归档链接，不重写历史验收工件或 ADR 业务决策。

## 4. 规格同步与文档验证

- [x] 4.1 对照当前主规格确认三份完整 MODIFIED Requirement 保留所有既有 Scenario，只包含管理员邮箱边界、累计锁定澄清及 OpenAPI 稳定错误码的批准修改；登出 TTL 不重复提交 delta。
- [x] 4.2 在运行行为验收通过后，将三份 delta 同步到 `openspec/specs/`，不修改其他能力，不将剩余 C6b 内容并入本批。
- [x] 4.3 运行 `openspec validate close-known-contract-readiness-gaps --type change --strict` 并记录结果。
- [x] 4.4 运行 `openspec validate --specs --strict` 并记录主规格结果。
- [x] 4.5 逐项解析本批修正的相对链接，确认对应归档目录、任务记录、联合验收文件、响应基线、错误矩阵和截图实际存在；不建立别名、重定向或复制历史证据。

## 5. 已知遗留闭环记录

- [x] 5.1 在 `docs/reviews/contract-readiness.md` 记录本批五项的实际修复与验收证据，关闭对应遗留；保留剩余 C6b 未核状态，不把工件生成、静态源码核对或本批通过声明为全部审计完成。
