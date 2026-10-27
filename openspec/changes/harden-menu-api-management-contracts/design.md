## Context

`internal/navigation` 已拥有菜单树、菜单 CRUD、角色菜单、菜单/API 绑定和权限码联动；`internal/apimetadata` 已拥有 API Metadata CRUD、Route Catalog 同步、权限同步和按钮生成。两者通过最小 Application Contract 协作，并由 App 统一组合 Route Catalog、权限中间件和授权版本事务。

当前实现仍有四类可观察缺口：菜单 PUT 没有完整校验 `type/status`，目录和页面可以进入 API 绑定路径；菜单 API 集合只能非空替换，没有显式清空操作；菜单同步无法把未知 `parent_path` 区分为配置冲突，可能按根节点落库；高影响写入只能依赖页面读取快照，保存前没有服务端影响摘要和并发指纹。后续菜单管理页和接口管理页如果直接使用这些入口，会把授权规则复制到前端，并可能覆盖竞争修改。

本 Change 只加固后端契约。Route Catalog 继续是 Method、Path、Handler 和静态 Access Level 的唯一事实来源；`apis` 只保存展示及运行时策略；`user_access_versions` 仍由 Authorization 独占。没有数据库迁移、Redis 状态或对象存储变化。

## Goals / Non-Goals

**Goals:**

- 在 Navigation Application 内集中校验目录、页面、按钮的字段和 API 绑定前提。
- 保留空数组的既有非法语义，并通过显式 `clear` 操作解除菜单全部 API 绑定。
- 让菜单同步在提交前解析完整父级图；未知父级、重复路径和循环父级整批拒绝。
- 提供菜单/API 只读影响预览，返回安全的影响摘要和无状态 SHA-256 快照指纹。
- 让高影响写入在同一事务中重新计算并校验指纹；事实变化返回既有 `NAV_CONFLICT` 或 `API_META_CONFLICT`，不写入、不自动重放。
- 在 API Metadata 列表、详情及同步预览中投影 Route Catalog 匹配状态，不持久化诊断结果。
- 保持统一四字段响应、OpenAPI、稳定权限码和现有事务/授权版本所有权。

**Non-Goals:**

- 不实现菜单管理 React 页面、接口管理 React 页面或新的前端 API Client。
- 不动态创建、删除或替换 Gin 路由；不把 API Metadata 变成路由注册来源。
- 不在本 Change 中取消后端既有 API Metadata 的手工 CRUD；后续页面首版不暴露手工创建和 Method/Path/NeedAuth 编辑。
- 不新增组织数据范围、审批、草稿、发布版本、配置回滚、编辑锁或新的权限模型。
- 不新增数据库表/列、Redis key、MinIO bucket、后台任务或缓存事实。
- 不新增业务 Permission Code；新增只读预览入口复用现有菜单/API 查询权限码。

## Decisions

### 1. 保持模块所有权，扩展最小 Contract

Navigation 继续拥有菜单类型、父级、路径、`menu_apis` 和菜单同步；API Metadata 继续拥有 API 元数据、Route Catalog 投影和同步。影响预览不建立新的一级模块：

- Navigation 定义菜单变更的规范化目标和影响计算输入，并通过既有 `Authorization`、`APIMetadataReader` 获取角色/用户及 API 事实。
- API Metadata 使用既有 `RouteSource` 计算 Catalog 匹配状态，并通过现有 `PermissionCoordinator` 委托权限码联动事实。
- 两侧各自暴露最小的 Application 输入/输出类型；HTTP Adapter 不导入另一模块的 ORM、Gin Context 或数据库事务对象。
- 需要跨模块一致性的写入仍由当前最外层 TransactionRunner 和 Navigation Coordinator 负责，预览只读不触发版本或缓存副作用。

备选方案：建立独立 `configuration` 一级模块。放弃，因为会复制 Navigation/Authorization/API Metadata 的所有权，并违反现有模块边界。

### 2. 菜单类型和绑定校验

定义稳定菜单类型常量：目录 `1`、页面 `2`、按钮 `3`；状态只接受 `0/1`。创建和修改统一拒绝超出范围的显式值，已有无效历史数据只读可见，不做数据迁移。

规范化交叉规则：

- 目录：`path`、`component` 必须为空，不能绑定 API。
- 页面：`path` 必须非空且全局唯一，`component` 可为空，不能绑定 API。
- 按钮：`path`、`component` 必须为空；可绑定启用且 `need_auth=1` 的 API。
- 按钮的父级必须是目录或页面；其他父级关系继续使用现有父引用和循环保护。
- `permission_code` 的具体联动仍由现有绑定协调器处理；已绑定按钮的权限码变化必须同步菜单、API、权限角色关系和适用用户版本。
- `GenerateMenuButton` 复用同一父级校验；生成的菜单固定为按钮、启用、无路径/组件。

这会拒绝此前可能被写入的模糊组合，属于已声明的 Breaking 行为；不在启动时回填或删除旧记录。

### 3. 显式解除菜单 API 绑定

不新增路由或 Permission Code，扩展现有 `POST /api/admin/menus/:id/apis` 请求为两种互斥模式：

- 绑定/替换：`clear=false` 或省略，`api_ids` 必须非空；保持现有原始 ID 校验、启用/认证校验和权限码联动。
- 清空：`clear=true`，`api_ids` 和 `permission_code` 必须省略/为空；在同一事务内删除 `menu_apis`、计算旧权限引用清理和受影响用户版本。菜单自身的独立 `permission_code` 不因解除关系而静默清空。

`clear=true` 的请求仍须携带 `expected_fingerprint`；当前事实已经没有绑定时成功为 no-op 且不提升版本。目录/页面上存在的历史绑定允许通过清空操作修复，但新的绑定仍拒绝非按钮类型。

### 4. 同步先解析完整图，再整批提交

菜单同步仍使用现有 `POST /api/admin/menus/sync` 输入，但在事务写入前完成规范化计划：

- 规范化路径和父级路径；数据库已有路径归类为 skip。
- 同一请求重复路径、未知父级、父级循环或类型字段非法归类为 `NAV_CONFLICT`/`NAV_VALIDATION_INVALID`，不创建任何记录。
- 新提交项可以引用同一批次中其他项；服务按稳定输入序号进行拓扑排序，循环不允许。
- 预览和正式同步使用同一计划构建器；正式同步在最外层事务中一次提交，失败整批回滚。
- 既有管理员字段不被同步覆盖；正常全 skip 仍返回零创建且不提升版本。

同步预览返回新增、跳过和冲突分类，但不返回内部 SQL 错误或授权凭据。

### 5. Route Catalog 匹配状态只读投影

API Metadata 的 DTO 增加非持久化 `route_state`，由当前 Route Catalog Snapshot 与规范化 `method + path` 比较得到：

- `matched`：Metadata 与 Catalog 路由存在且 Method/Path 一致；
- `metadata_only`：数据库记录存在，但 Catalog 没有对应路由；
- `catalog_only`：仅在同步预览中出现，表示待创建 Metadata；
- `access_mismatch`：Catalog 的静态 Access 与 Metadata 的 `need_auth`/权限策略组合不一致。

状态只作为列表、详情和同步预览诊断返回，不写入数据库，不改变真实 Handler。现有 `need_auth` 的静态最低认证保护继续由 Route Catalog/App 执行；本 Change 不把数据库字段变成匿名路由开关。

### 6. 无状态影响预览和指纹

新增两个只读管理入口，复用现有查询 Permission Code，不新增权限码：

- `POST /api/admin/menus/impact-preview`，默认 Permission Code `admin.menus.get`；
- `POST /api/admin/apis/impact-preview`，默认 Permission Code `admin.apis.get`。

请求包含模块内 operation、规范化候选字段和必要的目标 ID；支持菜单创建/修改/删除/绑定/清空/同步，以及 API 修改/删除/同步/权限同步/按钮生成的预览。响应包含：

- `operation`；
- `preview_fingerprint`，固定 64 位小写 SHA-256 十六进制；
- 规范化目标事实摘要；
- 受影响菜单/API 数量及 ID（只返回业务 ID，不返回秘密或原始 SQL）；
- 受影响角色数量和用户数量；
- `access_version_change` 与 `requires_confirmation`；
- 同步项的新增、跳过、冲突摘要。

指纹是对“当前相关配置事实 + Route Catalog 版本化快照摘要 + 规范化候选操作”的稳定排序序列计算 SHA-256。预览不存储指纹，不使用 Redis/数据库/浏览器持久状态，也不把指纹当作授权凭据。

所有会改变菜单/API/绑定/权限/启停或同步事实的正式写请求增加 `expected_fingerprint`。写入在现有事务内按确定顺序锁定目标，重新构建同一规范化事实并比较指纹；不一致返回对应冲突错误。普通只读查询和不改变授权/运行时策略的字段更新不强制预览，但 UI 可以统一先预览。

### 7. 统一 HTTP 和 OpenAPI 契约

预览入口、`clear` 模式和新增指纹字段都必须在 Route Catalog Descriptor 中声明请求/响应 Schema、`NAV_CONFLICT`/`API_META_CONFLICT`、验证错误和内部错误。预览失败不产生任何写入或授权版本变化。HTTP Adapter 只负责绑定 DTO、调用 Application 和安全映射错误，不暴露底层原因。

不新增 Redis key、MinIO bucket 或权限码；新预览路由使用现有 `admin.menus.get`/`admin.apis.get` 权限，写入口沿用原有 `admin.menus.*`/`admin.apis.*` 权限。

### 8. 验证策略

- Go 单元测试覆盖菜单类型矩阵、父级/路径规则、clear 请求互斥条件、指纹确定性和过期冲突。
- Navigation/API HTTP 测试覆盖 Route Catalog Descriptor、OpenAPI Schema、稳定错误码、预览无副作用和原有权限码。
- SQLite/隔离存储测试覆盖整批同步回滚、绑定清空、权限引用清理和版本变化；MySQL 集成测试覆盖真实锁序、并发胜方和提交后旧 Token 失效。
- 真实 HTTP 验证覆盖预览→写入、陈旧指纹、未知父级、目录/页面绑定拒绝、显式清空和 Route Catalog 漂移。
- 不新增测试来锁定未改变的前端视觉或原始错误文本；后续页面 Change 负责真实浏览器 UI 验收。

## Risks / Trade-offs

- 预览读取和正式写入之间仍可能发生竞争；指纹复核只能保证不覆盖已观察事实，不能阻止另一个合法写请求在本请求提交后立即发生。
- 影响摘要需要读取角色和用户关系，配置规模大时会增加查询成本；只返回计数/有限 ID，不引入缓存事实，优先保证一致性。
- 拒绝历史上可接受的非法菜单组合可能让旧脚本失败；这是为了防止新页面继续生成无法解释的授权关系，发布前必须更新所有仓库内调用方和契约测试。
- `metadata_only` 记录仍可能保留用于诊断，不能被误解为真实路由；页面 Change 必须明确显示漂移状态，不提供修改 Method/Path 的 UI。
- 预览入口复用查询权限，拥有查询权限的操作者可看到配置影响计数；响应不得包含用户明细、Token、Cookie、数据库错误或其他敏感数据。
- 不做数据迁移意味着旧的非法类型/绑定组合不会自动修复；需要通过显式清空或后续管理页面处置，不能在启动时静默改变授权事实。