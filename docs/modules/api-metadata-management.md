# API 管理

> 本文只保留 API 元数据和运行时策略边界。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/api-management/spec.md`](../../openspec/specs/api-management/spec.md) 为准。

## 模块边界

`internal/apimetadata` 维护 API 元数据、运行时访问策略、权限同步和菜单按钮生成。

API 元数据不会创建或替换 Gin 路由。真实路由由业务模块声明 Route Descriptor，App 通过 Route Catalog 统一注册。

## 关键规则

- `method + path` 唯一，创建和修改时会规范化方法与路径。
- API 元数据可配置展示名称、分组、权限码、启用状态、认证标记和审计标记。
- 路由同步消费 Route Catalog Snapshot，不扫描 Gin Engine。
- API 权限同步继续处理现行 `NeedAuth=1` 候选，空码沿既有 Method + Path 规则派生；公开项跳过、已有权限不重复创建，不统一路由同步、权限目录同步和 Seed 的用途。
- 一次 `sync-permissions` 的全部 API／菜单码、权限、角色授权及适用用户版本共同提交或回滚；失败不返回部分成功统计。
- `created`／`created_count` 包含协调内部实际新增且最终保留的权限，恢复既有权限不算新建；`updated_api` 仍统计补齐空码项，不扩大为间接传播行数。返回值只取最终成功 attempt。
- 整轮先准备严格有序目标并 current read 复核，业务遍历不按锁顺序重排；依赖变化返回既有 `API_META_CONFLICT`，保留并发胜方。版本只预锁，不为 skip 初始化；实际联动仍逐 API 触发，同用户两项可提升两次，不做请求级去重。
- F2 的原程序混合成功、后项／后续版本失败回滚、既有 MySQL 重试类和严格锁序见[定向验收](../reviews/evidence/api-permission-sync-atomic-2026-10-18.json)；未扩展 S3 或宣称所有 C6 场景完成。
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
