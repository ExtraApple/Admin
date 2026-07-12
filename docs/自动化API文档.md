# 自动化 API 文档

## 模块定位

自动化 API 文档用于把 Gin 已注册路由和 `apis` 表中的接口元数据生成 OpenAPI 3.0 文档。

它主要服务于：

- 后端开发自查接口。
- 前端联调查看接口路径、方法和认证要求。
- Apifox 导入 OpenAPI JSON。
- 后续接口数量增加后的文档自动维护。

当前方案直接复用项目已有的 API 管理模块：

```text
Gin 路由
  -> /docs/openapi.json
  -> 结合 apis 表中的 name / group / remark / status / need_auth
  -> 生成 OpenAPI 3.0 文档
```

## 配置

配置位置：

```yaml
api_docs:
  enabled: true
  title: "Admin API"
  version: "1.0.0"
  description: "Admin 后台管理系统 OpenAPI 文档，由 Gin 路由和 API 元数据自动生成。"
```

| 字段 | 说明 |
|---|---|
| `enabled` | 是否开启自动 API 文档路由 |
| `title` | OpenAPI 文档标题 |
| `version` | OpenAPI 文档版本 |
| `description` | OpenAPI 文档说明 |

生产环境如果不希望暴露接口文档，可以设置：

```yaml
api_docs:
  enabled: false
```

## 访问地址

### Swagger UI 页面

```http
GET http://localhost:8080/docs
```

### OpenAPI JSON

```http
GET http://localhost:8080/docs/openapi.json
```

这个地址可以直接导入 Apifox。

## 生成规则

文档从 `gin.Engine.Routes()` 自动读取路由。

当前会导出：

- `/api/` 开头的业务接口。
- `/ping` 健康检查接口。

当前不会导出：

- `/docs`。
- `/docs/openapi.json`。

如果 `apis` 表中存在对应记录，OpenAPI 会优先使用：

| apis 字段 | OpenAPI 用途 |
|---|---|
| `name` | `summary` |
| `api_group` | `tags` |
| `remark` | `description` |
| `status` | 禁用接口标记为 `deprecated` |
| `need_auth` | 是否添加 Bearer Token 安全要求 |

## Apifox 导入

1. 启动后端服务。
2. 访问 `http://localhost:8080/docs/openapi.json`，确认能返回 JSON。
3. 打开 Apifox。
4. 选择 OpenAPI / Swagger 导入。
5. 填入 URL：

```text
http://localhost:8080/docs/openapi.json
```

6. 导入后配置环境变量：

```text
base_url=http://localhost:8080
admin_token=登录返回的 access_token
```

## 推荐使用流程

新增后端接口后：

```text
1. 在 router 中注册真实路由
2. 启动后端服务
3. seed.Run 自动同步 API 元数据和权限码
4. 打开 /docs 查看接口
5. 在 Apifox 重新导入 /docs/openapi.json
```

如果只新增了路由，OpenAPI 文档也能显示接口；服务重启并执行 Seed 后，文档中的分组、名称、认证状态会更准确。

手动同步接口仍然保留：

```http
POST /api/admin/apis/sync
POST /api/admin/apis/sync-permissions
```

它们主要用于调试、数据修复或不重启服务时主动补齐 API 元数据。

## 当前边界

当前自动文档能生成：

- 路径。
- HTTP 方法。
- 路径参数。
- 接口分组。
- 接口标题。
- Bearer Token 认证要求。
- JSON 请求体字段。
- DTO 中 `required`、`min`、`max`、`len`、`oneof`、`email` 等基础校验规则。
- 通用 JSON 响应结构。
- 上传接口的 `multipart/form-data` 文件字段。

当前请求体 Schema 通过“路由 + DTO”的显式映射生成。新增有 JSON body 的接口后，需要在 OpenAPI 生成器中补充对应 DTO 映射，否则 Swagger UI 不会显示正确请求字段。

当前不会自动推导每个接口的精确响应 DTO 字段。

后续如果要进一步细化字段级文档，可以继续补充：

- 响应体 Schema。
- 错误码枚举。
