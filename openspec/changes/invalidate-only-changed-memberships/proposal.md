## Why

已裁定纯成员替换仅使实际增删用户失效，保持关系的成员和其他范围内数据读取者不额外下线；这改变现行新旧成员并集失效条款。需要同时兑现等价集合、隐藏归属、加入时间与共享版本冲突规则。

## What Changes

- 角色和组织整组成员按集合差分仅失效新增/移除用户，不重建保留成员或重置joined_at。
- 逐人角色/组织写入先检查当前expected_access_version，合法等价目标不提升，真实关系变化只提升目标用户。
- 逐人组织仅比较可管理关系，保留范围外关系和原时间；角色整组仍完整替换且越界删除拒绝。
- 两次合法no-op可成功；真实修改先提交后携旧版本的no-op仍冲突，两个真实修改最多一个提交。
- 保持受控软删旧关系清理、保护及失败原子性，不以最终有效权限是否相同计算关系变化。

**性质与来源**：已裁定、尚未发布的新目标；I1 / I5 / N1(membership) / N2 / R6.e5。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不计算共享授权配置或组织结构的影响集合，不因范围内人员数据变化额外失效旁观者，不改变完整替换/可管理部分替换差异。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `rbac`: 角色整组成员差量失效和等价集合。
- `organization-management`: 组织整组/逐人归属、隐藏关系和真实修改CAS。
- `user-management`: 逐人角色关系无变化与真实修改版本规则。

## Impact

- 代码：`internal/authorization/application/service.go`、`internal/organization/service.go`、`internal/organization/user_memberships.go`、`internal/organization/repository.go`、`internal/authorization/adapters/gorm/repository.go`。
- 入口：POST /api/admin/roles/:id/users、/organizations/:id/users；PUT /api/admin/users/:id/roles、/organizations。
- 数据：`user_roles`、`user_organizations`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：`enforce-authorization-reference-lifecycle`、`enforce-protected-authorization-assignments`；功能整合与同名Requirement发布基线的区别见design。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
