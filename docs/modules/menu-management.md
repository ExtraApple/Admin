# 菜单管理

> 本文只保留菜单模块边界和使用注意事项。接口路径、请求体、响应 Schema 和认证要求以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/menu-management/spec.md`](../../openspec/specs/menu-management/spec.md) 为准。

## 模块边界

菜单由 `internal/navigation` 拥有，负责：

- 菜单树和菜单记录 CRUD。
- 前端路由同步。
- 角色菜单分配与可见菜单树。
- 菜单与 API 元数据的绑定。

## 关键规则

- `type=1` 为目录，`type=2` 为页面菜单，`type=3` 为按钮菜单。目录的 `path`、`component` 必须为空；页面必须有唯一非空 `path`，`component` 可为空；按钮的 `path`、`component` 必须为空。
- `status` 只接受 `0` 或 `1`；显式非法 `type/status` 拒绝，既有非法历史记录只读展示，不在启动或普通读取时静默修复。
- 按钮只能挂在目录或页面下；目录、页面不能新建菜单 API 绑定。菜单 API 绑定目标必须存在、启用且 `need_auth=1`。
- 菜单路径必须唯一；创建、修改和同步会拒绝路径冲突、自引用、循环父级和未知 `parent_path`。同步先规范化完整批次，支持批次内新父级的稳定拓扑排序，任何失败整批回滚。
- 菜单PUT的 `icon`、`component` 和合法 `path` 区分遗漏、空字符串和非空替换；`name` 显式空或纯空白拒绝。JSON null 沿原解析表示未提交。
- 有子菜单的节点不能删除；合法删除清理 `role_menus` 和 `menu_apis`。菜单/API 绑定、权限联动、角色授权和适用用户 access version 保持同一事务边界。
- `POST /api/admin/menus/:id/apis` 的非清空模式要求非空 `api_ids`；空数组不是解绑。显式 `clear=true` 要求 `api_ids` 和 `permission_code` 为空，清理全部 menu/API 关系；菜单自身 `permission_code` 不因解绑被清空。已清空按钮再次清空是 no-op，不提升版本。
- 高影响菜单写入必须先调用影响预览并提交 `expected_fingerprint`；删除通过 `X-Expected-Fingerprint` 请求头提交。陈旧或非法指纹拒绝写入并返回 `NAV_CONFLICT` 或验证错误。
- `POST /api/admin/menus/impact-preview` 复用 `admin.menus.get`，只读返回规范化目标、影响菜单/API/角色/用户摘要和 64 位小写 SHA-256 fingerprint；预览不写菜单、权限、版本或缓存。

## 接口入口

接口分组位于 Swagger UI 的 `menu` 标签，主要包括：

- 菜单查询、创建、修改、删除。
- 前端路由同步。
- 菜单绑定和查询 API。
- 角色菜单绑定和查询。

前端初始化菜单使用：

```text
GET /api/user/context
```

## 验证方式

1. 启动服务并访问 `/docs`。
2. 先完成登录和 API/权限同步，再按 Swagger Schema 调用接口。
3. 涉及菜单可见性时，重新登录并检查 `/api/user/context`。
4. 规则变化先修改 OpenSpec，再更新本页的边界摘要。
