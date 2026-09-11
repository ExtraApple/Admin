# 契约可信度调研 · api-management

> 方法与纪律见 [contract-readiness.md](contract-readiness.md)。本文件只记录本模块结果。

规格：`openspec/specs/api-management/spec.md`（249 行，7 个 Requirement）
代码：`internal/apimetadata/**`、`internal/navigation/service.go`（编排实现在 Navigation）
路由：`internal/apimetadata/adapters/http/routes.go:96-107`（10 条）

---

## 一、路由层核对

| # | 路由 | 规格场景 | 权限码 | 判定 |
| --- | --- | --- | --- | --- |
| 1 | `GET /api/admin/api-groups` | `:59-62` | `admin.api-groups.get` | ✅ 一致 |
| 2 | `GET /api/admin/api-methods` | `:64-66` | `admin.api-methods.get` | ✅ 一致 |
| 3 | `GET /api/admin/apis` | `:12-15` | `admin.apis.get` | ✅ 一致 |
| 4 | `GET /api/admin/apis/:id` | `:17-20` | `admin.apis.id.get` | ✅ 一致 |
| 5 | `POST /api/admin/apis` | `:22-26` | `admin.apis.post` | ✅ 一致 |
| 6 | `PUT /api/admin/apis/:id` | `:28-31` | `admin.apis.id.put` | ✅ 一致 |
| 7 | `DELETE /api/admin/apis/:id` | `:51-54` | `admin.apis.id.delete` | ✅ 一致 |
| 8 | `POST /api/admin/apis/:id/menu-button` | `:179-187` | `admin.apis.id.menu-button.post` | ✅ 一致 |
| 9 | `POST /api/admin/apis/sync` | `:106-114` | `admin.apis.sync.post` | ✅ 一致 |
| 10 | `POST /api/admin/apis/sync-permissions` | `:136-141` | `admin.apis.sync-permissions.post` | ✅ 一致 |

**结果：10/10 全部有规格场景，0 条 📄 未记录。**
与 dict-management（9 条中 4 条未记录）形成对比，见索引 X3。

### 规格提到但不在本模块路由表内的入口

| 入口 | 规格位置 | 实现位置 | 判定 |
| --- | --- | --- | --- |
| `GET /docs` | `:73` | `internal/app/http.go:24-27`（App 技术路由） | 见索引 X2/X2b |
| `GET /docs/openapi.json` | `:79` | 同上 | 见索引 X2/X2b |

两条均为设计如此（`route-catalog/spec.md:94`），不经过 Route Catalog 校验。

---

## 二、行为契约核对

### 契约 A｜权限码变更编排（规格 `:38-49`）

规格要求（逐字）：

> `:40` **AND** 系统 SHALL 在同一数据库事务中同步更新关联菜单和同一菜单绑定的其他 API 权限码
> `:41` **AND** 系统 SHALL 确保权限表存在新权限码
> `:42` **AND** 新权限码已存在时系统 SHALL 合并并去重旧权限对应的角色授权
> `:43` **AND** 旧权限 SHALL 仅在不再被菜单、API 或角色引用时清理
> `:44` **AND** 系统 SHALL 在同一事务中使变更前后受影响用户的旧 token 失效
> `:48` **THEN** 系统 SHALL 回滚本次权限码变更的全部数据库写入

实现：`internal/navigation/service.go:376-439`（`ChangeAPIPermissionCode`）

| 要求 | 实现 | 判定 |
| --- | --- | --- |
| `:40` 同事务更新菜单 + 同菜单绑定的其他 API | `:417-424` `UpdateMenusPermissionCode` + `apis.SetPermissionCode(linkedAPIIDs)` | ✅ |
| `:41` 确保权限表存在新权限码 | `:406-409` `authorization.EnsurePermission` | ✅ |
| `:42` 合并去重角色授权 | `:410` → `:639` `mergePermissionRoles` | ✅ |
| `:43` 仅在不再被引用时清理旧权限 | `:437` → `:655` `cleanupPermission` | ✅ |
| `:44` 同事务失效受影响用户 token | `:398,425-435` `affectedRoleIDs(before/after)` → `IncrementAccessVersions` | ✅ |
| `:48` 任一步失败全部回滚 | 整体包在 `service.transactions.Run` 内 | ✅ |

**额外实现（规格未要求）**：`:378-397` 按"API → 菜单 → 关联 API"固定顺序加行锁，
是防死锁的正确做法。

**判定：✅ 一致。** 这是本次调研至今实现完整度最高的契约。

### 契约 B｜同步的幂等与例外（规格 `:106-131`）

| 规格要求（逐字） | 实现 | 判定 |
| --- | --- | --- |
| `:109` **AND** 只处理 `/api/` 前缀下的路由 | `sync.go:38-40`；`seed.go:72` | ✅ |
| `:110` **AND** 对已存在的 `method + path` 跳过创建 | `sync.go:45-61`；`seed.go:77-93` | ✅ |
| `:111` **AND** 为需要权限控制的接口生成默认权限码 | `sync.go:86-91`；`seed.go:101-104` | ✅ |
| `:112` **AND** 对 Public 路由以及当前兼容规则指定的公开字典接口标记为不需要权限码检查 | `sync.go:81-84` + `:86-91`（Public 时 `needAuth=0` 且 code 为空）；`seed.go:98-100` | ✅ |
| `:113` **AND** 对已存在但错误标记为需要认证的 `POST /api/refresh` 纠正 `need_auth` 并清空 `permission_code` | `sync.go:51-54`；`seed.go:83-86` | ✅ |
| `:114` **AND** 同步流程 SHALL NOT 扫描 Gin Engine 发现路由 | 路由来自 `RouteSource.Routes()`（`app/route_source.go:22-39` 读 Catalog 快照） | ✅ |
| `:125-126` 已存在记录保留管理员配置，不因 Catalog 默认值覆盖 | `sync.go:45-61` 对已存在记录只写 `deleted_at`/`status`（及 refresh 例外） | ✅ |
| `:130-131` 软删除记录按现有规则恢复，保持 `method + path` 唯一 | `sync.go:47-50` 恢复 `deleted_at=nil` + `status=1` | ✅ |

**判定：✅ 一致。**
注：`syncCatalogAPIs`（`internal/app/seed.go:65-119`）与 `syncRoutesInTransaction`
（`sync.go:30-78`）是两条并行实现的同一契约 —— 启动路径与管理员端点各一份。
两者当前行为一致，但这是重复实现（见下"实现观察"）。

### 契约 C｜动态权限校验（规格 `:143-174`）

实现：`internal/apimetadata/adapters/http/policy.go:19-66`

| 规格场景 | 实现 | 判定 |
| --- | --- | --- |
| `:146-150` 已启用 + 有权限 → 允许 | `:40` 查 `status`，`:59-63` 查权限码 | ✅ |
| `:152-155` API 未配置 → 拒绝 | `:26-38` 返回 `API_META_PERMISSION_NOT_CONFIGURED` | ✅ |
| `:157-159` API 已禁用 → 拒绝 | `:40-44` 返回 `API_META_DISABLED` | ✅ |
| `:161-165` 非超管缺权限码 → 拒绝 | `:53-58` 返回 `API_META_PERMISSION_MISSING` | ✅ |
| `:167-170` 超管兜底（非禁用 API 允许） | `:49` `contextContains(roles,"admin")` | ✅ |
| `:172-174` 首次同步例外 | `:28-31,45-48` + `isBootstrapRoute`（`:68-70`） | ✅ |

**判定：✅ 一致。** 且 `internal/app/http_access_policy_test.go:71-82` 有 6 个场景断言。

### 契约 D｜删除 API（规格 `:51-54`）

- `:53` "系统清理该 API 对应的 `menu_apis` 关联" → `navigation/service.go:440` `DeleteAPI` 编排
- `:54` "系统硬删除 API 记录" → `apimetadata/adapters/gorm/repository.go:179` `Unscoped().Delete`
- 测试：`internal/navigation/service_test.go:286` `TestServiceDeleteAPIHardDeletesMetadataAndRollsBackLinkedWrites`
- **判定：✅ 一致**

### 契约 E｜OpenAPI 生成（规格 `:68-101`）

| 规格要求 | 实现 | 判定 |
| --- | --- | --- |
| `:69` 由 Route Catalog + Metadata Snapshot 生成，不扫描 Gin Engine | `internal/apidoc/service.go`；`internal/app/apidoc.go:12-26` `Snapshot` | ✅ |
| `:74-75` `/docs` 返回 Swagger UI，HTML 不包装 | `service.go:127-129` `c.Data(..., "text/html")` | ✅ |
| `:80` 返回未包装的 OpenAPI 3.0 JSON | `service.go:145` `c.JSON(http.StatusOK, document)` | ✅ |
| `:85-88` 四字段信封、逐状态 error_code enum、422 字段级 enum | `buildResponses` + `routecatalog` 错误定义 | ✅ |
| `:89` 含 App 组合的中间件错误 | `internal/app/app.go:193-228` `addMiddlewareErrorDefinitions` | ✅ |
| `:90` 原生成功响应保留 Content-Type 与原生 Schema | `buildResponses` 对 `BinaryBody` 分支 | ✅ |
| `:93-97` 生成失败 → HTTP 500 四字段信封 + 稳定码 + 不泄漏内部错误 | `service.go:141-144` | 🧪 见索引 X2 |
| `:100-101` `api_docs.enabled = false` → 不注册这两个路由 | `internal/app/app.go:142-151` + `internal/app/http.go:24-27` | ✅ |

**判定：1 条 🧪（见索引 X2），其余 ✅。**

---

## 三、本模块判断

| 判定 | 数量 | 明细 |
| --- | --- | --- |
| ✅ 一致 | 10 条路由 + 契约 A/B/C/D + 契约 E 的 7/8 条 | — |
| 📄 未记录 | 0 | — |
| 🧪 未验证 | 1 | OpenAPI 生成失败路径（索引 X2） |
| ⚠️ 冲突 | 0 | — |
| ❌ 未实现 | 0 | — |

**本模块是迄今契约质量最高的一个。** 未发现规格与代码的行为冲突。

### 实现观察（非缺陷，但值得记录）

1. **同一契约两份实现**：`syncCatalogAPIs`（`internal/app/seed.go:65-119`）与
   `syncRoutesInTransaction`（`internal/apimetadata/application/sync.go:30-78`）
   实现同一组同步语义。两者当前一致，但权限码推导在四处出现
   （`seed.go:101-104`、`sync.go:80-93`、`service.go:87-95`、`seeddata/seed_test.go:283`），
   属重复实现风险。
2. **编排归属跨模块**：`apimetadata.Service.Update` 依赖 `navigation.Service.ChangeAPIPermissionCode`，
   而 `apimetadata` 自身的测试用 fake
   （`internal/apimetadata/application/service_test.go:47`）。
   真实编排的测试在 `internal/navigation/service_test.go:243`
   `TestServiceChangesLinkedAPIPermissionAndMergesRoleGrants`。契约完整覆盖，但跨模块。

---

## 四、证据缺口（未验证，不得据此下结论）

1. **`need_auth` 可被管理员置 0 从而降级受保护路由**。
   `policy.go:49` 在 `NeedAuth == 0` 时直接放行，而路由的
   `Authenticated` 中间件仍按 Descriptor 挂载（`internal/app/http.go:43-55`）——
   即"匿名被拒但任何登录用户通过"。
   该行为由规格 `:86` 显式允许（"API 元数据关闭权限码检查"），
   并由 `internal/app/http_access_policy_test.go:73` 断言固化。
   本次未判定其是否为预期能力，故不列为发现。

2. **唯一性检查与写入的事务边界**：`apimetadata` 的 `Update` 对
   `method + path` 冲突的检查（`core.go:133-141`）在事务外，
   写入在事务内。与字典模块同类，本次未做并发验证。
