# 菜单与 API 联动

## 当前状态

菜单与 API 联动已经落地为可测试功能，核心目标是让“前端按钮是否显示”和“后端接口是否允许访问”使用同一套 `permission_code`。

已完成内容：

- `menus.permission_code`：控制前端菜单/按钮可见性。
- `apis.permission_code`：控制后端 API 动态权限。
- `menu_apis`：保存菜单与 API 的绑定关系。
- 菜单绑定 API：支持一个按钮菜单绑定一个或多个 API。
- API 生成按钮菜单：支持从某个 API 一键生成 `type=3` 的按钮菜单。
- API 权限码同步菜单：修改已绑定 API 的权限码后，会同步更新关联菜单权限码。
- 删除清理：删除菜单或 API 时，会同步清理 `menu_apis` 关联。

## 数据模型

### menus

菜单表继续使用原有字段：

```text
id
parent_id
name
path
component
icon
permission_code
sort
type
status
```

其中：

- `type=1`：目录。
- `type=2`：页面菜单。
- `type=3`：按钮菜单。
- `permission_code`：前端判断菜单/按钮是否展示的权限码。

### apis

API 表继续使用：

```text
method
path
permission_code
status
need_auth
need_audit
```

其中：

- `permission_code`：后端动态权限中间件使用的权限码。
- `status=1`：接口启用。
- `need_auth=1`：需要认证和权限校验。

### menu_apis

新增菜单 API 关联表：

```text
menu_id
api_id
```

作用：

- 明确某个菜单按钮绑定了哪些 API。
- 方便后台页面做 API 选择器。
- 方便后续做菜单/API一致性检查。
- 删除菜单或 API 时可以清理关联关系。

## 联动规则

### 菜单绑定 API

接口：

```http
POST /api/admin/menus/:id/apis
Authorization: Bearer <admin_token>
Content-Type: application/json
```

Body：

```json
{
  "api_ids": [21, 22],
  "permission_code": "admin.users.save"
}
```

规则：

- `:id` 是菜单 ID。
- `api_ids` 是要绑定到该菜单的 API ID 数组。
- `permission_code` 可选。
- 如果传了 `permission_code`，菜单和绑定 API 都会同步使用这个权限码。
- 如果不传 `permission_code`，系统会尝试从 API 自动推导。
- 多个 API 权限码不一致时，必须手动传 `permission_code`。
- 只允许绑定 `status=1` 且 `need_auth=1` 的 API。

成功返回：

```json
{
  "code": 200,
  "msg": "绑定成功"
}
```

### 查询菜单绑定的 API

接口：

```http
GET /api/admin/menus/:id/apis
Authorization: Bearer <admin_token>
```

成功返回：

```json
{
  "code": 200,
  "data": [
    {
      "id": 21,
      "name": "删除用户",
      "method": "DELETE",
      "path": "/api/admin/users/:id",
      "group": "user",
      "permission_code": "admin.users.id.delete",
      "remark": "",
      "sort": 0,
      "status": 1,
      "need_auth": 1,
      "need_audit": 1
    }
  ]
}
```

### 从 API 生成按钮菜单

接口：

```http
POST /api/admin/apis/:id/menu-button
Authorization: Bearer <admin_token>
Content-Type: application/json
```

Body：

```json
{
  "parent_id": 2,
  "name": "删除用户",
  "sort": 1
}
```

规则：

- `:id` 是 API ID。
- `parent_id` 是按钮挂载的父级菜单 ID。
- 系统会创建 `type=3` 的按钮菜单。
- 按钮菜单 `path` 和 `component` 为空。
- 按钮菜单的 `permission_code` 使用 API 的 `permission_code`。
- 如果 API 没有权限码，系统会根据 `method + path` 自动生成。
- 系统会自动写入 `menu_apis` 关联。
- 系统会自动确保 `permissions` 表存在对应权限码。

成功返回：

```json
{
  "code": 200,
  "msg": "生成成功",
  "data": {
    "id": 10,
    "parent_id": 2,
    "name": "删除用户",
    "path": "",
    "component": "",
    "icon": "",
    "permission_code": "admin.users.id.delete",
    "sort": 1,
    "type": 3,
    "status": 1
  }
}
```

## 前端使用方式

前端进入后台后调用：

```http
GET /api/user/context
Authorization: Bearer <access_token>
```

响应中会返回：

```json
{
  "roles": ["editor"],
  "permissions": ["admin.users.get", "admin.users.id.delete"],
  "menus": []
}
```

前端按钮判断：

```text
如果 permissions 包含按钮菜单的 permission_code
  显示按钮
否则
  隐藏按钮
```

后端接口判断：

```text
请求进入 APIPermission 中间件
  -> 根据 method + c.FullPath() 查询 apis
  -> 获取 apis.permission_code
  -> 判断当前用户 permissions 是否包含该权限码
```

这样前后端都使用同一套权限码，不会出现“前端显示按钮但后端 403”或“后端有权限但前端按钮不显示”的错位问题。

## Apifox 测试流程

1. 管理员登录，拿到 `access_token`。
2. 调用 `POST /api/admin/apis/sync` 同步当前 Gin 路由。
3. 调用 `POST /api/admin/apis/sync-permissions` 同步 API 权限码到权限表。
4. 调用 `GET /api/admin/apis`，找到要绑定的 API ID。
5. 调用 `GET /api/admin/menus`，找到父级菜单 ID。
6. 调用 `POST /api/admin/apis/:id/menu-button` 从 API 生成按钮菜单。
7. 调用 `POST /api/admin/menus/:id/apis` 给按钮菜单绑定更多 API。
8. 调用 `POST /api/admin/roles/:id/menus` 给角色绑定菜单。
9. 调用 `POST /api/admin/roles/:id/permissions` 给角色绑定权限。
10. 测试用户重新登录。
11. 调用 `GET /api/user/context`，确认 `menus` 和 `permissions` 都返回。
12. 调用按钮对应 API，确认后端权限通过。

## 注意事项

- 新增路由后，需要重新执行 API 同步和 API 权限同步。
- 公开 API，即 `need_auth=0`，不应该绑定菜单按钮权限。
- 禁用 API，即 `status=0`，不允许绑定菜单。
- 一个按钮菜单可以绑定多个 API，但建议同一个按钮动作下的 API 使用同一个权限码。
- 如果多个 API 权限码不同，绑定时手动传 `permission_code`，系统会统一同步。
- 修改已绑定 API 的 `permission_code` 后，系统会同步更新关联菜单和同组绑定 API。
