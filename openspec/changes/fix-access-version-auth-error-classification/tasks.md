## 1. 范围与前置

- [ ] 1.1 固定本change现行条款、审计来源与适用前提；复用已取得反例，不为重新确认已知失败重跑旧程序。

## 2. Model / DTO / Contract

- [ ] 2.1 在Identity调用方Contract表达不可验证版本原因，保留Authorization已有缺行错误身份和Unwrap链，不让Application导入GORM Adapter。

## 3. Application / Adapter实现

- [ ] 3.1 App身份授权桥接仅对已确认缺行cause转换分类，其他Snapshot故障保留。
- [ ] 3.2 Authenticate把此分类映射CodeTokenInvalid/401/AUTHN_TOKEN_INVALID；禁止Ensure/fallback，保持真实错误5xx和Refresh结果。

## 4. HTTP / App装配

- [ ] 4.1 保持JWT中间件公开错误定义和GET user/info/POST refresh路由；若OpenAPI认证错误说明受影响仅更新实际契约。

## 5. 行为回归与原程序验证

- [ ] 5.1 有效Access先成功，仅删除自建版本行后请求返回401/AUTHN_TOKEN_INVALID且表仍缺行。
- [ ] 5.2 同一用户Refresh保持401及既有码且不签发Token。
- [ ] 5.3 只令版本读取发生实际SQL/连接故障时保留安全5xx，不泄漏DSN/SQL；普通失配/用途/禁用等稳定错误不回归。

## 6. 文档与交付

- [ ] 6.1 在产品行为实现后只保留能捕获消费者可见错误的确定性回归；运行本切片适用门禁及原程序smoke，保存去敏响应/事务前后状态/必要交错证据，清理自建资源，不把未运行检查标通过。
- [ ] 6.2 依据实际验收更新现有实现说明、审计状态与必要changelog；保留历史JSON原样，记录本次新证据和局部覆盖，不把其他C6风险标完成。
- [ ] 6.3 核对delta完整保留前置Requirement与新场景，并完成OpenSpec检查；规格同步/归档按独立流程及design发布依赖执行，不在apply中自动归档。
