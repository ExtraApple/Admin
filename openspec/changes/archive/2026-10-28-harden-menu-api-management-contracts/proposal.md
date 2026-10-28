## Why

菜单和 API 配置是全局授权事实，但当前契约仍允许几类高风险歧义：菜单类型与 API 绑定关系缺少完整交叉校验，最后一个 API 没有显式解绑入口，菜单同步遇到未知父级可能错误落到根节点，保存前也没有由服务端确认授权影响的统一预览。现在先补齐这些边界，才能安全承载后续菜单管理页和接口管理页，避免前端复制授权规则或把过期快照当成成功事实。

## What Changes

- 为菜单创建、修改、绑定和生成按钮补充目录、页面、按钮之间的类型交叉校验；统一校验 `type`、`status`、父级和路径约束。
- 新增菜单 API 关系的显式“解除全部绑定”操作；不改变空数组的既有非法语义。
- 菜单同步遇到未知 `parent_path` 时返回稳定冲突，不再静默创建根菜单。
- 为菜单/API 高影响写操作增加服务端影响预览，返回权限码、关联对象、角色、受影响用户和授权版本影响摘要。
- 预览返回无状态快照指纹；正式写入必须复核指纹，事实变化时返回既有冲突错误，不自动覆盖或重放。
- API Metadata 管理提供 Route Catalog 匹配状态诊断，明确区分代码路由事实与数据库策略记录；不允许 Metadata 创建或替换 Gin 路由。
- 保持现有 `admin.*` 权限码、Route Catalog、Permission Code、`user_access_versions` 和事务所有权；不新增 Redis key、MinIO bucket 或独立授权来源。
- **BREAKING**：非法菜单类型/状态组合、目录或页面绑定 API、未知同步父级等请求将被拒绝，而不是继续写入模糊配置。
- **BREAKING**：依赖预览指纹的高影响写请求在服务端事实变化后返回冲突，不能继续使用旧页面快照提交。

非目标：本 Change 不实现菜单管理页、接口管理页、角色授权页、审批/草稿/版本回滚、组织数据范围、动态 Gin 路由、手工创建运行时 API、API Method/Path 编辑或新的前端权限模型。

## Capabilities

### New Capabilities

- `menu-api-impact-preview`: 定义菜单/API 配置变更的服务端影响预览、无状态快照指纹、保存前复核和冲突语义。

### Modified Capabilities

- `menu-management`: 修改菜单类型校验、菜单/API 绑定、显式解绑和路由同步父级错误语义。
- `api-management`: 修改 API 管理的 Route Catalog 漂移诊断、菜单按钮关系和高影响写入预览语义。

## Impact

- 领域/应用：`internal/navigation`、`internal/apimetadata`，以及两者之间现有的权限码、菜单/API 绑定和授权版本协调事务。
- HTTP：增加显式解除菜单 API 绑定和只读影响预览管理入口；现有菜单/API CRUD、同步和绑定错误需要补充稳定错误定义与 OpenAPI 描述。不会删除现有管理入口。
- 数据：不新增表、列、Redis key、MinIO bucket 或权限码；继续使用 `menus`、`apis`、`menu_apis`、权限关系和 `user_access_versions`。
- Route Catalog：继续作为真实 Method、Path、Handler 和静态 Access Level 的唯一来源；Metadata 只保存展示和运行时策略事实。
- 前端：本 Change 只提供后端契约，菜单页和接口页在后续独立 Change 中实现；现有 Client 不自动重放写请求。
- 验证：需要覆盖菜单类型/父级/绑定边界、显式解绑、同步冲突、预览指纹失效、事务回滚、授权版本影响和真实 HTTP/OpenAPI 契约。