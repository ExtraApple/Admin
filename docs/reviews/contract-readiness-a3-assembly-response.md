# A3 装配与响应契约审计报告

## 1. 审计结论

本次 A3 是有限范围审计，不是产品修复或全端点验收。审计使用当前已发布规格、现行 runbook、真实 App 组合根、隔离 MySQL／Redis／MinIO／RabbitMQ、真实 HTTP／JWT，以及结构化 Route Catalog／OpenAPI 快照。

结论分为四类：

| 分类 | 结论 |
|---|---|
| 契约一致 | Route Catalog 与 Gin 业务路由集合一致；技术路由、业务 JSON、OpenAPI、头像原生响应、404／405、Access Level 静态边界和 required／optional 接线的代表性行为符合当前判据。 |
| 实现偏差 | 新建 API Metadata 对 7 条当前 Public 路由保存了 `need_auth = 1`，只有 `POST /api/refresh` 保存了 `need_auth = 0`；这与 `route-catalog` 和 `api-management` 的公共路由同步契约不一致。当前 Gin Public 静态中间件边界未被该字段降级或升级，因此未观察到匿名访问被阻断。 |
| 证据不足 | 429、500、503 没有通过生产式故障注入在真实隔离 App 中触发；使用 Descriptor、OpenAPI 和聚焦测试覆盖。浏览器 WebSocket 不能附加当前 Authenticated 中间件要求的 Authorization header，实时升级仅取得升级前 401；原生 Handler 的有效 ticket 测试通过。 |
| 语义／文档待裁决 | 当前浏览器对 `OPTIONS /api/admin/apis` 返回信封化 405，没有成功 CORS 预检协议；本审计不推断系统是否应启用 CORS。组织范围外查询当前返回 `422 ORG_VALIDATION_INVALID`，而不是专用 403；这被记录为当前实现语义，不在本 change 内改写。 |

未发现需要在本 change 内修复的安全降级。产品偏差只保留证据，后续应分别创建产品修复、规格裁决或文档修复 change。

## 2. 判据与证据

### 2.1 当前判据

- `openspec/specs/route-catalog/spec.md`
- `openspec/specs/modular-layered-architecture/spec.md`
- `openspec/specs/api-management/spec.md`
- `openspec/specs/api-response-contract/spec.md`
- `docs/runbooks/api-documentation.md`
- `docs/runbooks/configuration.md`
- `docs/runbooks/messaging.md`
- `docs/runbooks/audit-logging.md`

### 2.2 机器证据

- 汇总证据：`docs/reviews/evidence/a3-assembly-response.json`
- 完整结构快照：`docs/reviews/evidence/a3-route-catalog-snapshot.json`
- 代码基线：`7d28b1121ec5c28f673001b2b66e2e2bbdeb09a9`
- 隔离 App：`http://127.0.0.1:18080`
- 隔离资源：MySQL `13306`、Redis `16379`、MinIO `19000`、RabbitMQ `15673`

证据不保存 Authorization header、JWT、验证码、密码、DSN、签名 URL、对象 key 或原始内部错误。

## 3. 章节一：Route Catalog、Gin 与 OpenAPI

### 3.1 集合比较

真实组合代码构造了 126 条 Catalog 业务路由；Gin 实际注册了相同的 126 条业务路由，另有 3 条 App 技术路由：

- `GET /ping`
- `GET /docs`
- `GET /docs/openapi.json`

规范化规则：HTTP Method 大写；路径补前导 `/`；根路径外删除尾斜杠；Gin／Catalog 业务集合以 `Method + Path` 比较；OpenAPI 将 `:id` 转换为 `{id}`。结果：

```text
Catalog only: 0
Gin business only: 0
Gin technical extras: 3
```

数据库创建的 `GET /api/a3-database-only` 没有生成 Gin 路由，真实请求返回 `404 HTTP_NOT_FOUND`，测试记录随后已删除。

### 3.2 Descriptor 与协议完整性

快照中：

- Public：8
- Authenticated：20
- PermissionControlled：98
- PermissionControlled 缺默认 Permission Code：0
- JSON 请求缺 Schema：0
- JSON 响应缺 Schema：0
- 错误响应缺公开错误定义：0
- WebSocket 原生协议入口：1，`GET /api/user/messages/ws`

Route Catalog 聚焦测试通过：重复 Method + Path、缺字段、未知 Method、未知 Access Level、缺 Schema、错误 envelope、错误状态、错误 owner 冲突和 WebSocket 协议字段均在 Gin 注册前阻断。

### 3.3 OpenAPI

真实 App 返回：

- HTTP 200
- `Content-Type: application/json; charset=utf-8`
- OpenAPI `3.0.3`
- 91 个规范化 paths
- 成功文档未包装成四字段业务 envelope

`/docs/openapi.json` 生成失败的聚焦测试确认：HTTP 500、`HTTP_INTERNAL_ERROR`、四字段 envelope、`data: null`，且不泄露 Metadata、数据库或生成器内部错误。

## 4. 章节二：Access Level、API Metadata 与 Permission

### 4.1 真实 HTTP 访问矩阵

代表性结果：

| Case | Route | HTTP | error_code |
|---|---|---:|---|
| 匿名 Authenticated | `GET /api/user/context` | 401 | `AUTHN_HEADER_MISSING` |
| 无效 Token | `GET /api/user/context` | 401 | `AUTHN_TOKEN_INVALID` |
| 有效身份 | `GET /api/user/context` | 200 | 空 |
| 匿名 PermissionControlled | `GET /api/admin/apis` | 401 | `AUTHN_HEADER_MISSING` |
| 无效 Token | `GET /api/admin/apis` | 401 | `AUTHN_TOKEN_INVALID` |
| 普通用户无权限 | `GET /api/admin/apis` | 403 | `API_META_PERMISSION_DENIED` |
| 超级管理员 | `GET /api/admin/apis` | 200 | 空 |
| 普通用户范围内 | `GET /api/admin/organizations/1/users` | 200 | 空 |
| 普通用户范围外 | `GET /api/admin/organizations/2/users` | 422 | `ORG_VALIDATION_INVALID` |

范围用户的组织树只返回组织 `1`，排除范围外组织 `2`。超级管理员的放行没有被推导为普通用户权限通过。

### 4.2 `need_auth=0`

对 `GET /api/admin/api-groups` 的隔离 Metadata 做了受控修改并在 Case 后恢复：

- 匿名请求：401 `AUTHN_HEADER_MISSING`
- 有效身份、无 Permission Code：200
- 说明 `need_auth=0` 只跳过动态 Permission Code 检查，没有把静态 PermissionControlled 降级为匿名。

### 4.3 Metadata 停用、路径漂移和旁路记录

真实 HTTP 结果：

- Metadata `status=0`：有效身份请求返回 403 `API_META_DISABLED`
- Metadata 路径改成 `/api/a3-diverged`：原真实路由返回 403 `API_META_PERMISSION_NOT_CONFIGURED`
- 数据库只有 Metadata、没有 Descriptor 的路由：真实 Gin 仍返回 404
- 普通用户不能执行 API 首次同步；超级管理员可执行并返回 200

### 4.4 实现偏差 A3-001：Public Metadata 的 `need_auth`

当前新建 Metadata 观察到：

| Public 路由 | 期望 `need_auth` | 实际 `need_auth` |
|---|---:|---:|
| `GET /api/dicts/:type_code/items` | 0 | 1 |
| `GET /api/captcha` | 0 | 1 |
| `POST /api/register` | 0 | 1 |
| `POST /api/login` | 0 | 1 |
| `POST /api/refresh` | 0 | 0 |
| `GET /api/avatars/default` | 0 | 1 |
| `GET /api/avatars/:user_id` | 0 | 1 |
| `GET /api/ready` | 0 | 1 |

代码中的 `syncCatalogAPIs` 对新 Descriptor 按 `Public` 设置 `needAuth = 0`，但真实 MySQL 记录仍以 `1` 保存；现象与 GORM 带默认值字段对零值写入的行为一致，但本审计不把该推测当作已修复根因。

影响边界：当前 Public 路由由静态 Access Level 直接注册，没有因为数据库 `need_auth=1` 被动态加上认证；偏差发生在 Metadata 投影和同步契约，不是本次观察到的匿名安全降级。

## 5. 章节三：App 组合根与依赖接线

### 5.1 组合根

- `main.go` 只调用 `app.BuildFromPath`、`Application.Run` 和 `Application.Close`。
- `internal/app` 构造基础设施、模块、Route Descriptor、Catalog、迁移、Seed、Gin 和后台任务。
- `internal/app/http.go` 是唯一业务 Gin 注册点。
- 模块 HTTP Adapter 返回 Descriptor，不直接调用 Gin 注册方法。
- API Metadata、RBAC、OpenAPI 和 Seed 均使用 Catalog Snapshot 或其派生事实。

### 5.2 Required／Optional 接线

`files.application.Dependencies`：

- required：`Repository`、`Storage`、`Validator`、`MessageImages`、`MessageImageValidator`、`Signer`
- optional：`Transactions`、`Clock`、`ObjectNames`、`DownloadURLExpireSeconds`

`messaging.application.Dependencies`：

- required：`Messages`、`Categories`、`Inbox`、`Notifications`、`Outboxes`、`ConsumerDeadLetters`、`ConsumerDeadLetterReplay`、`Identity`、`Organizations`、`Authorization`、`Files`
- optional：`Transactions`、`ContentLimits`、`Clock`

架构护栏 `go test ./testsupport -run TestArchitecture -count=1` 通过。真实隔离 App 启动完成 MySQL、Redis、MinIO、RabbitMQ 装配；MinIO bucket 确认完成；`/api/ready` 返回 RabbitMQ `ready`。

### 5.3 技术路由与错误 Method

真实结果：

- `/ping`：200，四字段 JSON 成功 envelope，`data.msg = pong`
- `/api/ready`：200，四字段 JSON 成功 envelope，包含 `status` 与受控 `rabbitmq` 状态
- `/docs`：200，原生 Swagger HTML
- `/docs/openapi.json`：200，原始 OpenAPI JSON
- 未匹配 `/api`：信封化 404 `HTTP_NOT_FOUND`
- 已匹配路径错误 Method：信封化 405 `HTTP_METHOD_NOT_ALLOWED`

## 6. 章节四：响应、错误 owner 与原生协议

### 6.1 业务 JSON

成功业务响应均观察到：

- `code` 与 HTTP 状态一致
- `error_code` 为空
- `msg = success`
- 顶层只有 `code`、`error_code`、`msg`、`data`

真实错误覆盖：400、401、403、404、405、409、422；Descriptor、OpenAPI 和聚焦测试覆盖 429、500、503 的错误 owner、状态和 Schema。未做生产式随机故障注入。

### 6.2 原生协议

- Swagger HTML：`text/html; charset=utf-8`
- OpenAPI JSON：未包装的 `application/json`
- 默认头像：`image/png`，真实 HTTP 200
- WebSocket Descriptor：`x-native-protocol = websocket`，101 无 body，升级前错误为四字段 JSON
- 浏览器 WebSocket 真实尝试因当前 Authenticated 中间件要求 Authorization header 而在升级前返回 401；有效 ticket 的 handler 测试通过，未把该浏览器限制改写为产品结论。
- CORS OPTIONS 当前返回 405 信封化错误；没有在本 change 中新增 CORS 能力或假定其应启用。

### 6.3 错误 owner 与恢复

Route Catalog 校验和 OpenAPI 生成测试通过：

- 错误码 owner、状态、消息和安全 Schema 冲突会启动前失败。
- 中间件错误被 App 按 Access Level 合并到 Descriptor。
- 原生响应提交前失败使用错误 envelope。
- 原生响应提交后 panic／失败不会追加 JSON。
- 运行日志与 HTTP 错误不输出原始 cause、凭据、Token 或路径。

## 7. 未覆盖边界与分流

本报告不宣称以下范围全量通过：

- 全部 126 条端点的所有身份／状态组合。
- RabbitMQ 断连、数据库故障、Redis 故障、MinIO 故障下的真实 429／500／503 故障注入。
- 生产部署、前端联合验收、容量和压力。
- 完整 API Metadata／Permission 事务和菜单联动审计。

A3-001 应另建产品修复或规格裁决 change；本审计 change 不修改 `seed.go`、模型默认值、主规格、路由或权限码。

## 8. 清理状态

已清理隔离容器、隔离数据库、Redis 数据、MinIO bucket、RabbitMQ 拓扑、临时 App 进程、临时配置和 A3 临时日志。完整结构快照、本汇总证据和本报告属于去敏审计证据，不作为运行时产品数据。
