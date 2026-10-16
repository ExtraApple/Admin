## Why

显式创建停用菜单会被Service和ORM默认协作变成启用，C2已有响应与真实存储反例。需要区分状态遗漏与合法零值，保留已发布的默认行为。

## What Changes

- HTTP和CreateInput使用明确的状态存在性表示，合法0与1原样保存，遗漏或现行无效值使用默认启用。
- Adapter的单次INSERT必须真正写入0，不能只修响应或被GORM default重新覆盖。
- 迁移所有CreateInput调用并覆盖创建、Seed／同步／按钮生成的既有合法默认，不新增兼容输入旁路。

**性质与来源**：现行承诺下已核实缺陷；C2。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不改变菜单启停产品规则、无变化策略、批量状态更新或数据库历史数据，不以事后第二次UPDATE掩盖错误INSERT。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `menu-management`: 明确菜单创建的状态存在性、合法停用和真实持久化保证。

## Impact

- 代码：`internal/navigation/routes.go`、`internal/navigation/types.go`、`internal/navigation/service.go`、`internal/navigation/repository.go`、`internal/navigation/model.go`。
- 入口：POST /api/admin/menus；admin.menus.post不变。
- 数据：`menus`、`user_access_versions（维持既有创建事务失效行为）`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：无外部change前置；与其他独立修复可分别实施。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
