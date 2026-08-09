# 菜单管理

> 本文只保留菜单模块边界和使用注意事项。接口路径、请求体、响应 Schema 和认证要求以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/menu-management/spec.md`](../../openspec/specs/menu-management/spec.md) 为准。

## 模块边界

菜单由 `internal/navigation` 拥有，负责：

- 菜单树和菜单记录 CRUD。
- 前端路由同步。
- 角色菜单分配与可见菜单树。
- 菜单与 API 元数据的绑定。

## 关键规则

- `type=1` 为目录，`type=2` 为页面菜单，`type=3` 为按钮菜单。
- 菜单路径必须唯一；按钮菜单可以没有路径和组件。
- 有子菜单的节点不能删除；删除叶子菜单时清理 `role_menus` 和 `menu_apis`。
- 角色菜单分配是全量替换；用户可见菜单从全部角色合并、去重并按权限码过滤。
- 超级管理员返回全部启用菜单；普通用户只返回角色已分配且有权限的菜单，并保留必要的父级目录。
- 菜单/API 绑定必须在同一事务中完成，失败时回滚关联、权限和授权版本变化。

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
