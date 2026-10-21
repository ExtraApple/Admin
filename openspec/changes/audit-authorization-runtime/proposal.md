## Why

C6b 已完成若干已确认缺陷修复和局部授权目标验收，但授权运行时仍有未闭合的证据缺口：Access Snapshot、版本失效、角色／用户生命周期变更、数据范围和确定性并发尚未按现行契约形成一套可复核的真实 HTTP 证据。现在需要先验证现有承诺，而不是继续扩大授权策略或把候选设计当成当前缺陷。

## What Changes

- 建立授权运行时的有限审计边界，覆盖 Token／授权版本、授权关系变更、数据范围、保护对象、事务回滚和确定性并发。
- 使用固定隔离身份、角色、组织和 Token 夹具，通过真实 HTTP、真实 JWT、独占 MySQL／Redis 运行 32 个最小 Case。
- 固定现有 HTTP 状态码、错误码、授权版本、关系持久化和旧 Token 结果作为判定依据。
- 将每个风险域标记为通过、发现缺陷或证据缺口，并保存去敏总报告及机器可读证据。
- 审计期间不修改产品代码、主规格或永久测试；确认产品偏差后另行创建修复 change。
- 明确菜单是授权投影而非安全真值；不把文件、消息、日志、Route Catalog、S3/S4/S5、I3 或全局最小失效算法纳入本 change。

## Capabilities

### New Capabilities

无。本 change 是授权运行时证据审计，不引入新的产品能力。

### Modified Capabilities

- `auth`：补齐跨入口授权运行时的统一判定边界，明确真实授权变化、合法 no-op、旧 Token 失效、版本冲突和事务失败的可观察结果；不新增路由或改变既有授权策略。

## Impact

- 修改 `openspec/specs/auth/spec.md` 对 `Token 实时失效` 的跨入口契约表达；其他 RBAC、用户和组织细则继续以现有主规格为准。
- 新增 `docs/reviews/contract-readiness-c6b-authorization-runtime.md` 及去敏审计证据文件。
- 可能使用临时审计 Runner、独占 MySQL／Redis、临时 schema 和配置；所有自建资源在审计后清理。
- 不新增生产路由、权限码、数据模型、Redis key、MinIO bucket 或第三方依赖。
- 若发现明确实现偏差，记录证据并停止在本 change 内修复；后续修复通过独立 change 处理。
