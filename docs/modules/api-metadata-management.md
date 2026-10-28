# API 管理

> 本文只保留 API 元数据和运行时策略边界。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/api-management/spec.md`](../../openspec/specs/api-management/spec.md) 为准。

## 模块边界

`internal/apimetadata` 维护 API 元数据、运行时访问策略、权限同步和菜单按钮生成。

API 元数据不会创建或替换 Gin 路由。真实路由由业务模块声明 Route Descriptor，App 通过 Route Catalog 统一注册。

## 关键规则

- `method + path` 唯一，创建和修改时规范化方法与路径；API Metadata 不创建或替换 Gin 路由。
- 列表和详情只读投影当前 Route Catalog 状态：`matched`、`metadata_only`、`access_mismatch`；同步影响预览还会标注 `catalog_only`。状态不持久化，Route Catalog 是运行时路由唯一事实来源。
- API 权限同步继续处理现行 `NeedAuth=1` 候选，空码按 Method + Path 派生；公开项跳过，已有权限不重复创建。一次同步的 API、菜单码、权限、角色授权及适用用户版本共同提交或回滚。
- 路由同步只消费当前 Route Catalog Snapshot，不扫描 Gin Engine；已有管理员名称、分组、权限码、备注、排序和策略字段不被同步覆盖，返回实际创建统计。
- `created`/`created_count` 只包含最终提交且实际新增的权限，恢复既有权限不算新建；`updated_api` 统计补齐空码项。失败不返回部分成功统计。
- 修改已绑定菜单的权限码、API 状态或删除 API 时，菜单、关联 API、权限、角色授权和用户 access version 保持事务原子性；失败完整回滚。
- 公开、停用或不需要认证的 API 不得生成按钮菜单；按钮父级只能是目录或页面。生成按钮会确保权限记录并建立 `menu_apis` 关联。
- 高影响 API 写入先调用 `POST /api/admin/apis/impact-preview`，再提交 64 位小写 SHA-256 `expected_fingerprint`；删除使用 `X-Expected-Fingerprint`。陈旧指纹返回 `API_META_CONFLICT`，不自动重放或覆盖并发修改。
- `POST /api/admin/apis/impact-preview` 复用 `admin.apis.get`，覆盖更新、删除、路由同步、权限同步和按钮生成，返回规范化目标、安全影响摘要和 fingerprint；预览不修改元数据、权限、授权版本、缓存或 Route Catalog。
## API 文档

自动化 API 文档由 `internal/apidoc` 根据 Route Catalog 和 API Metadata Snapshot 生成：

```text
GET /docs
GET /docs/openapi.json
```

具体导入和使用方式见 [api-documentation.md](../runbooks/api-documentation.md)。
