# API管理文档

## 当前状态

> **状态**: 已实现基础 CRUD、Gin 路由同步、API 权限同步、API 动态权限中间件。

API 管理用于维护系统接口元数据，方便后续把接口、权限码、菜单、操作日志策略串起来。它不是微服务网关，也不会动态生成后端接口；前端添加 API 记录只是维护数据库里的接口档案。

## 已实现功能

- [x] API 列表
- [x] 手动创建 API
- [x] 修改 API
- [x] 删除 API
- [x] 自动扫描 Gin 已注册路由
- [x] 从 API 表同步权限码到权限表
- [x] 根据 API 表动态校验接口权限

## 数据模型

表名：`apis`

```go
type API struct {
    ID             uint
    Name           string
    Method         string
    Path           string
    Group          string
    PermissionCode string
    Remark         string
    Sort           int
    Status         int
    NeedAuth       int
    NeedAudit      int
}
```

字段说明：

| 字段 | 说明 |
|---|---|
| `name` | API 名称，给后台页面展示 |
| `method` | 请求方法，例如 `GET`、`POST` |
| `path` | Gin 路由路径，例如 `/api/admin/users/:id` |
| `group` | API 分组，例如 `user`、`role`、`permission` |
| `permission_code` | 绑定权限码，例如 `admin.users.get` |
| `status` | 状态，`1` 启用，`0` 禁用 |
| `need_auth` | 是否需要登录认证，`1` 是，`0` 否 |
| `need_audit` | 是否记录审计日志，`1` 是，`0` 否 |
| `sort` | 排序 |
| `remark` | 备注 |

## 接口列表

所有接口都需要管理员 Token：

```http
Authorization: Bearer <admin_token>
```

### 1. 获取 API 列表

```http
GET /api/admin/apis?page=1&size=10&keyword=user&group=user&method=GET&status=1&need_auth=1&need_audit=1
Authorization: Bearer <admin_token>
```

可选查询参数：

| 参数 | 说明 |
|---|---|
| `page` | 页码，默认 `1` |
| `size` | 每页数量，默认 `10` |
| `keyword` | 关键词，匹配名称、路径、权限码 |
| `group` | API 分组 |
| `method` | 请求方法 |
| `status` | 状态，`1` 启用，`0` 禁用 |
| `need_auth` | 是否需要认证 |
| `need_audit` | 是否记录审计日志 |

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

### 2. 同步 Gin 已注册路由

```http
POST /api/admin/apis/sync
Authorization: Bearer <admin_token>
```

Body 不需要填写。

说明：

- 只同步 `/api/` 开头的接口。
- 已存在的 `method + path` 不会重复创建。
- 登录、注册、验证码、公开字典接口会标记为 `need_auth = 0`。
- 需要认证的接口会自动生成 `permission_code`。

成功返回：

```json
{
  "code": 200,
  "msg": "同步成功",
  "data": {
    "count": 2,
    "created": [
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
    ]
  }
}
```

### 3. 同步 API 权限

```http
POST /api/admin/apis/sync-permissions
Authorization: Bearer <admin_token>
```

Body 不需要填写。

说明：

- 从 `apis.permission_code` 同步到 `permissions.code`。
- `need_auth = 0` 的公开接口不会同步成权限。
- 如果 API 没有权限码，会根据 `method + path` 自动补一个。
- 已存在的权限码不会重复创建。

成功返回：

```json
{
  "code": 200,
  "msg": "同步成功",
  "data": {
    "created": [
      "admin.users.get"
    ],
    "created_count": 1,
    "updated_api": 0
  }
}
```

### 4. 手动创建 API

```http
POST /api/admin/apis
Authorization: Bearer <admin_token>
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

成功返回：

```json
{
  "code": 200,
  "msg": "创建成功",
  "data": {
    "id": 1,
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
}
```

### 5. 修改 API

```http
PUT /api/admin/apis/1
Authorization: Bearer <admin_token>
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

成功返回：

```json
{
  "code": 200,
  "msg": "修改成功",
  "data": {
    "id": 1,
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
}
```

### 6. 删除 API

```http
DELETE /api/admin/apis/1
Authorization: Bearer <admin_token>
```

成功返回：

```json
{
  "code": 200,
  "msg": "删除成功"
}
```

## Apifox 测试顺序

1. 启动后端服务，让自动迁移创建 `apis` 表。
2. 调用 `POST /api/admin/apis/sync`，同步当前 Gin 路由。
3. 调用 `GET /api/admin/apis`，确认接口记录已经写入。
4. 按需要修改某个 API 的名称、分组、权限码、审计配置。
5. 调用 `POST /api/admin/apis/sync-permissions`，把 API 权限码写入 `permissions` 表。
6. 到角色管理里给角色分配这些权限。

## 动态权限中间件

管理员路由现在通过 `APIPermission` 中间件进行动态权限判断：

```text
请求进入 /api/admin
  → JWTAuth 解析用户角色和权限码
  → APIPermission 使用 method + c.FullPath() 查询 apis 表
  → status != 1 时拒绝访问
  → need_auth = 0 时放行
  → admin 角色作为超级管理员兜底放行
  → 普通管理角色必须拥有 apis.permission_code
```

示例：

```text
GET /api/admin/users
  → 查询 apis: method=GET, path=/api/admin/users
  → permission_code=admin.users.get
  → 当前用户 permissions 包含 admin.users.get 才能访问
```

首次初始化注意：

- `POST /api/admin/apis/sync`
- `POST /api/admin/apis/sync-permissions`

这两个同步接口允许 `admin` 角色在 API 表未完整配置时先执行，避免新库第一次启动后无法初始化 API 元数据。

## 注意事项

- API 记录不等于真实接口。真实接口必须已经在 `router/router.go` 中注册。
- `method + path` 组合唯一，重复创建会失败。
- 删除 API 使用硬删除，后续可以重新同步或重新创建相同路径。
- 当前版本的 `status` 和 `permission_code` 已接入动态权限中间件。
- `need_audit` 目前仍只是元数据字段，尚未控制审计日志中间件是否记录。
