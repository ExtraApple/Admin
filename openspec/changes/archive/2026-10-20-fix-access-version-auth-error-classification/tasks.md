## 1. 范围与前置

- [x] 1.1 固定本change现行条款、审计来源与适用前提；复用已取得反例，不为重新确认已知失败重跑旧程序。

## 2. Model / DTO / Contract

- [x] 2.1 在Identity调用方Contract表达不可验证版本原因，保留Authorization已有缺行错误身份和Unwrap链，不让Application导入GORM Adapter。

## 3. Application / Adapter实现

- [x] 3.1 App身份授权桥接仅对已确认缺行cause转换分类，其他Snapshot故障保留。
- [x] 3.2 Authenticate把此分类映射CodeTokenInvalid/401/AUTHN_TOKEN_INVALID；禁止Ensure/fallback，保持真实错误5xx和Refresh结果。

## 4. HTTP / App装配

- [x] 4.1 保持JWT中间件公开错误定义和GET user/info/POST refresh路由；若OpenAPI认证错误说明受影响仅更新实际契约。

## 5. 行为回归与原程序验证

- [x] 5.1 有效Access先成功，仅删除自建版本行后请求返回401/AUTHN_TOKEN_INVALID且表仍缺行。
- [x] 5.2 同一用户Refresh保持401及既有码且不签发Token。
- [x] 5.3 只令版本读取发生实际SQL/连接故障时保留安全5xx，不泄漏DSN/SQL；普通失配/用途/禁用等稳定错误不回归。

## 6. 文档与交付

- [x] 6.1 在产品行为实现后只保留能捕获消费者可见错误的确定性回归；运行本切片适用门禁及原程序smoke，保存去敏响应/事务前后状态/必要交错证据，清理自建资源，不把未运行检查标通过。
- [x] 6.2 依据实际验收更新现有实现说明、审计状态与必要changelog；保留历史JSON原样，记录本次新证据和局部覆盖，不把其他C6风险标完成。
- [x] 6.3 核对delta完整保留前置Requirement与新场景，并完成OpenSpec检查；规格同步/归档按独立流程及design发布依赖执行，不在apply中自动归档。

## 7. 核验补证：W2登录初始化失败／W3 Refresh拒绝矩阵（2026-10-20）

以下为[核验报告W2/W3](../../../../docs/reviews/openspec-implementation-verification-2026-10-20.md#warning3类证据缺口不是产品缺陷)后新增且已完成的补证任务；原11项产品任务及历史证据保留。新增验收见design和[去敏证据](../../../../docs/reviews/evidence/authorization-followup-2026-10-20.json)。本节只补既有“登录初始化失败”“Refresh Token刷新失败”场景证据，不将缺证追认为产品缺陷，不新增业务语义。

- [x] 7.1 W2：在合法身份／密码／验证码等前提满足时，针对登录Ensure版本创建／读取注入受控失败，验证最终安全5xx且响应不含Access／Refresh；保留正常新用户初始化为1并成功登录的对照。断言真实响应及owner事实，不以token发行器mock调用次数代替“不签发”行为。
- [x] 7.2 W2：单独覆盖Ensure成功后Snapshot／版本重读失败，验证安全5xx、无token输出并记录前后版本与凭据事实；遵守现有Ensure提交语义，不要求已合法初始化的版本回滚，不新增登录全事务或重试策略。
- [x] 7.3 W3：补畸形Refresh、真实过期Refresh及有效Access用途Token提交`POST /api/refresh`的拒绝回归；隔离签名／用途／有效期前提，使用确定性过期夹具而非等待。逐项断言401、`AUTHN_REFRESH_TOKEN_INVALID`、相同安全英文拒绝消息、`data=null`且无新token对；不以Refresh当Access被拒绝证明反方向。
- [x] 7.4 W3：补用途／签名／有效期均正确但对应用户不存在的Refresh拒绝，断言同一通用401／空data／无新token、无版本补行或其他存储fallback；缺版本、失配及禁用用户的适用既有证据明确引用，不重复创建同路径断言。
- [x] 7.5 通过原程序现有登录／Refresh路由补W2/W3成功对照与失败smoke；实际故障只作用于目标版本创建／读取，不能拿Access版本SQL故障替代登录故障。保存去敏请求／响应、版本及身份前后事实和故障撤除对照，运行适用门禁、清理自建资源并更新核验W2/W3；取得真实证据后才勾选，不改写旧JSON或声称生产／全调度已覆盖。
