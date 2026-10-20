## Why

管理端用户PUT对省略邮箱先拒绝、对非空邮箱又按业务禁止，导致现行规格允许的资料修改没有合法请求组合。C5已取得真实HTTP与目标状态证据，需要打通已有合法入口并保持禁止代改邮箱。

## What Changes

- 将邮箱可省略语义贯通HTTP绑定与Application；省略或空邮箱不构成邮箱修改。
- 任何非空邮箱仍由既有业务错误整次拒绝，不放宽管理员代改邮箱。
- 保留操作者范围、本人和管理员目标保护以及本轮之前的版本触发行为；昵称豁免由独立新目标处理。

**性质与来源**：现行承诺下已核实缺陷；C5。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不开放邮箱编辑、不重开已完成的邮箱所有权流程，不在此实现纯昵称或同值请求不失效。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `user-management`: 补充管理员用户更新的无邮箱合法请求及邮箱禁止组合场景。

## Impact

- 代码：`internal/identity/dto.go`、`internal/identity/adapters/http/routes.go`、`internal/identity/application/user_service.go`。
- 入口：PUT /api/admin/users/:id；既有admin.users.id.put权限码不变。
- 数据：`users`、`user_access_versions`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：无外部change前置；与其他独立修复可分别实施。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
