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

Route Catalog 会在启动注册前拒绝重复或不完整的描述。

## 使用建议

1. 启动服务并确认 `/docs/openapi.json` 返回 JSON。
2. 在 Swagger UI 查看或调试接口。
3. Apifox 使用 URL 导入并配置 `base_url`、登录 Token 等环境变量。
4. 接口变更后重新导入，不继续依赖旧集合中的请求或响应假设。
5. 跨接口业务流程、安全边界和错误语义仍以 OpenSpec 与专题文档为准。
