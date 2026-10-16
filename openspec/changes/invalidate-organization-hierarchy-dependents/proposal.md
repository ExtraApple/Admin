## Why

已裁定组织移动需要使层级范围边界依赖者失效，即使成员关系没变，且这些用户可能位于移动子树之外。该有限目标应在C4安全拓扑上实现，不等于复活全局最小失效引擎或把当前静态差异追认为旧缺陷。

## What Changes

- 对真正parent_id变化读取锁定的旧新父关系，按org_and_children等当前层级依赖识别范围根变化及其用户。
- 包含旧/新祖先范围根的树外用户；另一角色重复/全量授权不豁免已有层级事实依赖。
- 组织移动与去重后的受影响用户版本同一最外层事务，失败全回滚；仅成员变化仍走独立I1规则。
- 不以最终合并权限差异或移动节点当前成员替代依赖计算；无结构变化不触发本目标。

**性质与来源**：已裁定、尚未发布的新目标；I4。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不实现I3复杂菜单权限图精度、不枚举逐用户最终有效权限，不引入全局依赖缓存或分布式事件，不把纯组织展示字段变化当成结构移动。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `organization-management`: 组织移动的层级依赖识别与版本原子性。
- `rbac`: org_and_children范围根依赖而非最终权限差分。

## Impact

- 代码：`internal/organization/service.go`、`internal/organization/hierarchy.go`、`internal/organization/repository.go`、`internal/authorization/application/service.go`、`internal/authorization/application/contracts.go`、`internal/app/organization.go`。
- 入口：PUT /api/admin/organizations/:id；admin.organizations.id.put不变。
- 数据：`organizations`、`user_organizations`、`roles`、`user_roles`、`role_data_scopes`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`fix-concurrent-organization-moves`、`enforce-authorization-reference-lifecycle`
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
