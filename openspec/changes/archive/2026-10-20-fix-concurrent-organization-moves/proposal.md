## Why

两个合法前置读取后的组织移动可并发提交为循环，C4已有受控真实UPDATE等待及最终拓扑反例。现行防循环要求必须在跨实例并发提交时仍成立。

## What Changes

- 将拓扑读取、范围/父级校验与父级写入置于同一最外层事务与数据库串行化协议内。
- 使用Organization拥有的当前父关系锁定读取重新校验，不能沿用锁前RR快照或仅依赖进程mutex。
- 创建、移动、删除参与同一拓扑协议，保留根节点0及既有管理边界。

**性质与来源**：现行承诺下已核实缺陷；C4。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不在此实现I4组织层级依赖失效、不新增组织乐观锁API字段、全局分布式锁服务或自动重试策略。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `organization-management`: 补充并发移动仍不得形成组织循环的现行保证。

## Impact

- 代码：`internal/organization/service.go`、`internal/organization/repository.go`、`internal/organization/hierarchy.go`、`internal/organization/contracts.go`、`internal/app/organization.go`。
- 入口：POST /api/admin/organizations、PUT /api/admin/organizations/:id、DELETE /api/admin/organizations/:id；现有权限码不变。
- 数据：`organizations`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：无外部change前置；与其他独立修复可分别实施。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
