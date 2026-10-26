# 自动化 API 文档

> 本文只说明 API 文档入口和维护方式。接口契约以运行中的 OpenAPI JSON 为准；当前生成行为以 [`openspec/specs/api-management/spec.md`](../../openspec/specs/api-management/spec.md) 和 [`openspec/specs/route-catalog/spec.md`](../../openspec/specs/route-catalog/spec.md) 为准。

## 访问入口

启用 `api_docs.enabled` 后：

```text
Swagger UI:   http://localhost:8080/docs
OpenAPI JSON: http://localhost:8080/docs/openapi.json
```

OpenAPI JSON 可以直接导入 Apifox。

## 配置

```yaml
api_docs:
  enabled: true
  title: "Admin API"
  version: "1.0.0"
  description: "Admin 后台管理系统 OpenAPI 文档"
```

生产环境是否开放文档由部署策略决定；关闭后不会注册 `/docs` 和 `/docs/openapi.json`。

## 生成来源

```text
业务模块 Route Descriptor
→ Route Catalog 校验并生成 Snapshot
→ 合并必要的 API Metadata Snapshot
→ 生成 OpenAPI 3.0 JSON
```

API 文档、API 元数据同步、权限同步和启动 Seed 消费同一份 Route Catalog Snapshot，不扫描 Gin Engine。

## 维护规则

新增或修改接口时必须同时维护 Route Descriptor 中的：

- Method、Path 和最低访问等级。
- 名称、分组、默认权限码和审计分类。
- OpenAPI summary、请求体及响应 Schema。
- multipart 文件字段或二进制响应类型。
- Cookie 安全方案、必需请求 Header、响应 Header（包括 `Set-Cookie` 与 `Cache-Control`），以及真实 400／401／403／409／500 错误。

Route Catalog 会在启动注册前拒绝重复或不完整的描述，并校验 Cookie／Header 元数据；APIDoc 只投影 Catalog 事实，不按路径另建认证规则。`POST /api/user/logout` 精确声明 Public 是取消 Bearer 前置要求，不是匿名任意撤销；Identity Handler 仍强制 Cookie／Origin／JSON Header 控制。其他 `/api/user/**` 仍 Authenticated。Metadata／Seed 同步保留管理员配置和历史，不批量回填历史 Public `need_auth`，不引入新权限策略。

## 使用建议

1. 启动服务并确认 `/docs/openapi.json` 返回 JSON。
2. 在 Swagger UI 查看协议；控制端点需从已配置可信 Origin 调用，`http://localhost:8080/docs` 与默认 Vite Origin 不同，不能为调试把白名单改成任意来源。Header 中的 Origin 由浏览器控制，不能依赖 Swagger 手填绕过。
3. Apifox 使用 URL 导入；业务接口配置受保护的临时 Access 变量，控制接口使用 Cookie jar、精确 Origin、JSON 与 `X-Admin-Request: 1`，Refresh 再加不可预测的 attempt ID。不要把密码／Token／jar 导出、打印、记录或共享。
4. 接口变更后重新导入，不继续依赖旧集合；Login data 无 Refresh，Refresh／Logout 必须 `{}` 且不附加 Bearer，不能保留旧 JSON Refresh 调用。
5. 跨接口业务流程、安全边界和错误语义仍以 OpenSpec 与专题文档为准。

完整 Cookie 生命周期、固定 30 秒幂等窗口、错误恢复与安全 curl 示例见[登录安全策略](login-security.md#浏览器控制协议)。浏览器不能通过脚本读取 HttpOnly Refresh，也不能通过 Swagger 响应体复制它；Cookie 是会话凭据而非文档中的示例值。
