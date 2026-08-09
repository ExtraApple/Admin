# API 管理

> 本文只保留 API 元数据和运行时策略边界。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/api-management/spec.md`](../../openspec/specs/api-management/spec.md) 为准。

## 模块边界

`internal/apimetadata` 维护 API 元数据、运行时访问策略、权限同步和菜单按钮生成。

API 元数据不会创建或替换 Gin 路由。真实路由由业务模块声明 Route Descriptor，App 通过 Route Catalog 统一注册。

## 关键规则

- `method + path` 唯一，创建和修改时会规范化方法与路径。
- API 元数据可配置展示名称、分组、权限码、启用状态、认证标记和审计标记。
- 路由同步消费 Route Catalog Snapshot，不扫描 Gin Engine。
- API 权限同步只为需要认证且存在权限码的 API 补齐权限记录。
- 未配置或已禁用的受保护 API 请求会被拒绝。
- 超级管理员可以绕过普通权限码检查，但不能访问已禁用 API。
- 修改已绑定菜单的权限码时，同步菜单、关联 API、角色授权和授权版本，并保持事务原子性。
- 公开 API 不得生成按钮菜单；生成按钮菜单时会建立 `menu_apis` 关联并确保权限记录存在。

## API 文档

自动化 API 文档由 `internal/apidoc` 根据 Route Catalog 和 API Metadata Snapshot 生成：

```text
GET /docs
GET /docs/openapi.json
```

具体导入和使用方式见 [api-documentation.md](../runbooks/api-documentation.md)。
