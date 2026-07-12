# API 管理

## 模块定位

API 管理维护后端接口元数据，用于接口分组、启停、权限码绑定、路由同步、动态权限校验、菜单按钮联动和后续审计策略扩展。

它不会动态生成 Gin 路由，真实接口仍然必须在代码中注册。API 表只负责记录接口元数据和权限配置。

## 已完成功能

- API 列表。
- API 详情。
- API 分组列表。
- HTTP 方法列表。
- 手动创建 API。
- 修改 API。
- 删除 API。
- 自动扫描 Gin 已注册路由。
- API 权限同步到权限表。
- API 动态权限中间件。
- 自动生成 OpenAPI 文档。
- 从 API 生成按钮菜单。
- 修改 API 权限码后同步关联菜单。
- 删除 API 时清理 `menu_apis` 关联。

## 数据模型

表名：

```text
apis
```

核心字段：

| 字段 | 说明 |
|---|---|
| `name` | API 展示名称 |
| `method` | HTTP 方法，例如 `GET`、`POST` |
| `path` | Gin 路由路径，例如 `/api/admin/users/:id` |
| `api_group` | API 分组 |
| `permission_code` | 绑定权限码 |
| `remark` | 备注 |
| `sort` | 排序 |
| `status` | 状态，`1` 启用，`0` 禁用 |
| `need_auth` | 是否需要认证 |
| `need_audit` | 是否需要审计日志 |

## 字段约定

| 字段 | 可选值 |
|---|---|
| `method` | `GET`、`POST`、`PUT`、`PATCH`、`DELETE`、`OPTIONS`、`HEAD` |
| `status` | `1` 启用，`0` 禁用 |
| `need_auth` | `1` 需要认证，`0` 公开接口 |
| `need_audit` | `1` 记录审计日志，`0` 不记录审计日志 |

## 接口

所有接口都需要管理员 Token：

```http
Authorization: Bearer <admin_token>
```

### 获取 API 列表

```http
GET /api/admin/apis?page=1&size=10&keyword=user&group=user&method=GET&status=1&need_auth=1&need_audit=1
```

可选查询参数：

| 参数 | 说明 |
|---|---|
| `page` | 页码，默认 `1` |
| `size` | 每页数量，默认 `10` |
| `keyword` | 匹配名称、路径、权限码 |
| `group` | API 分组 |
| `method` | HTTP 方法 |
| `status` | 状态 |
| `need_auth` | 是否需要认证 |
| `need_audit` | 是否记录审计 |

成功返回：

```json
{
  "code": 200,
  "data": {
    "list": [
      {
        "id": 1,
        "name": "GET /api/admin/users",
        "method": "GET",
        "path": "/api/admin/users",
        "group": "user",
        "permission_code": "admin.users.get",
        "remark": "",
        "sort": 0,
        "status": 1,
        "need_auth": 1,
        "need_audit": 1
      }
    ],
    "total": 1,
    "page": 1,
    "size": 10
  }
}
```

### 获取 API 详情

```http
GET /api/admin/apis/:id
```

路径参数：

| 参数 | 说明 |
|---|---|
| `id` | API ID |

成功返回：

```json
{
  "code": 200,
  "data": {
    "id": 1,
    "name": "GET /api/admin/users",
    "method": "GET",
    "path": "/api/admin/users",
    "group": "user",
    "permission_code": "admin.users.get",
    "remark": "",
    "sort": 0,
    "status": 1,
    "need_auth": 1,
    "need_audit": 1
  }
}
```

### 获取 API 分组列表

```http
GET /api/admin/api-groups
```

用途：

- 给前端 API 管理页面的分组筛选下拉框使用。
- `count` 表示该分组下已有多少条 API 元数据。

成功返回：

```json
{
  "code": 200,
  "data": [
    {
      "group": "user",
      "count": 5
    },
    {
      "group": "role",
      "count": 4
    }
  ]
}
```

### 获取 HTTP 方法列表

```http
GET /api/admin/api-methods
```

用途：

- 给前端创建、修改、筛选 API 时的 HTTP 方法下拉框使用。

成功返回：

```json
{
  "code": 200,
  "data": [
    {
      "label": "GET",
      "value": "GET"
    },
    {
      "label": "POST",
      "value": "POST"
    }
  ]
}
```

### 手动创建 API

```http
POST /api/admin/apis
Content-Type: application/json
```

Body：

```json
{
  "name": "查询用户列表",
  "method": "GET",
  "path": "/api/admin/users",
  "group": "user",
  "permission_code": "admin.users.get",
  "remark": "管理员查询用户列表",
  "sort": 1,
  "status": 1,
  "need_auth": 1,
  "need_audit": 1
}
```

### 修改 API

```http
PUT /api/admin/apis/:id
Content-Type: application/json
```

Body：

```json
{
  "name": "查询用户列表",
  "group": "user",
  "permission_code": "admin.users.get",
  "remark": "管理员查询用户列表",
  "sort": 1,
  "status": 1,
  "need_auth": 1,
  "need_audit": 1
}
```

说明：

- 修改 `method + path` 后仍然必须保持唯一。
- 如果该 API 已绑定菜单，修改 `permission_code` 后会同步更新关联菜单权限码。

### 删除 API

```http
DELETE /api/admin/apis/:id
```

说明：

- 删除 API 会硬删除 API 元数据。
- 删除 API 会同步清理 `menu_apis` 关联。

### 同步 Gin 已注册路由

```http
POST /api/admin/apis/sync
```

Body 不需要填写。

说明：

- 只同步 `/api/` 开头的接口。
- 已存在的 `method + path` 不会重复创建。
- 登录、注册、验证码、公开字典接口会标记为 `need_auth=0`。
- 需要认证的接口会自动生成默认 `permission_code`。

### 同步 API 权限

```http
POST /api/admin/apis/sync-permissions
```

Body 不需要填写。

说明：

- 从 `apis.permission_code` 同步到 `permissions.code`。
- `need_auth=0` 的公开接口不会同步成权限。
- API 没有权限码时，会根据 `method + path` 自动补一个。
- 已存在的权限码不会重复创建。

### 从 API 生成按钮菜单

```http
POST /api/admin/apis/:id/menu-button
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

说明：

- `:id` 是 API ID。
- `parent_id` 是按钮菜单挂载到哪个父菜单下面。
- 只允许基于 `status=1` 且 `need_auth=1` 的 API 生成按钮。
- 系统会自动创建 `type=3` 的按钮菜单。
- 系统会自动写入 `menu_apis`。
- 系统会自动确保权限表存在对应权限码。

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

## 动态权限中间件

管理员接口现在通过 `APIPermission` 中间件进行动态权限判断：

```text
请求进入 /api/admin
  -> JWTAuth 解析用户角色和权限码
  -> APIPermission 使用 method + c.FullPath() 查询 apis 表
  -> status != 1 时拒绝访问
  -> need_auth = 0 时放行
  -> admin 角色作为超级管理员兜底放行
  -> 普通管理员必须拥有 apis.permission_code
```

示例：

```text
GET /api/admin/users
  -> 查询 apis: method=GET, path=/api/admin/users
  -> permission_code=admin.users.get
  -> 当前用户 permissions 包含 admin.users.get 才能访问
```

## 初始化和同步

当前项目已经接入 Go 代码版 Seed 初始化。

服务启动时会自动执行：

```text
seed.Run
  -> 同步 Gin 路由到 apis 表
  -> 同步 apis.permission_code 到 permissions 表
  -> 给 admin 角色补齐全部权限
```

所以新环境首次启动后，通常不需要再手动调用：

```http
POST /api/admin/apis/sync
POST /api/admin/apis/sync-permissions
```

这两个接口仍然保留，适合下面场景手动使用：

- 开发时服务未重启，但需要主动补齐当前路由元数据。
- 数据库中误删了 API 元数据或权限码，需要手动恢复。
- 调试 API 管理模块时需要观察同步结果。

新增后端路由后，推荐流程是：

```text
1. 在 router 中注册真实路由
2. 重启服务
3. seed.Run 自动同步 API 和权限码
4. 普通管理员重新登录以刷新 token 权限
```

## 自动化 API 文档

当前项目会根据 Gin 路由和 `apis` 表元数据自动生成 OpenAPI 文档：

```http
GET /docs
GET /docs/openapi.json
```

其中 `/docs` 是 Swagger UI 页面，`/docs/openapi.json` 可以直接导入 Apifox。

JSON 请求体会根据路由对应的 DTO 生成字段 Schema；没有请求体的同步、退出登录、强制下线等接口不会显示空的通用 body。

详细说明见 [自动化API文档.md](自动化API文档.md)。

API 管理收尾新增了这些路由，重启服务后 Seed 会自动同步：

```text
GET /api/admin/apis/:id
GET /api/admin/api-groups
GET /api/admin/api-methods
```

## Apifox 测试顺序

### 1. 查询 API 列表

服务启动后，Seed 已经自动同步 API 元数据。可以直接查询：

```http
GET http://localhost:8080/api/admin/apis?page=1&size=10
Authorization: Bearer <admin_token>
```

### 2. 查询 API 详情

```http
GET http://localhost:8080/api/admin/apis/<api_id>
Authorization: Bearer <admin_token>
```

### 3. 查询 API 分组

```http
GET http://localhost:8080/api/admin/api-groups
Authorization: Bearer <admin_token>
```

### 4. 查询 HTTP 方法

```http
GET http://localhost:8080/api/admin/api-methods
Authorization: Bearer <admin_token>
```

### 5. 手动同步 API 路由

```http
POST http://localhost:8080/api/admin/apis/sync
Authorization: Bearer <admin_token>
```

说明：正常启动后一般不需要手动调用，仅用于调试或修复数据。

### 6. 手动同步 API 权限码

```http
POST http://localhost:8080/api/admin/apis/sync-permissions
Authorization: Bearer <admin_token>
```

说明：正常启动后一般不需要手动调用，仅用于调试或修复数据。
