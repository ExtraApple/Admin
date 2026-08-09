# 菜单与 API 联动

> 本文只保留联动规则。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/menu-management/spec.md`](../../openspec/specs/menu-management/spec.md) 和 [`openspec/specs/api-management/spec.md`](../../openspec/specs/api-management/spec.md) 为准。

## 目标

按钮菜单和后端 API 使用同一个 `permission_code`，使前端按钮可见性与后端接口鉴权保持一致。

## 关联关系

```text
menus.permission_code
        ↕
menu_apis
        ↕
apis.permission_code
        ↓
角色权限与用户授权版本
```

## 联动规则

- 只能绑定已启用且需要认证的 API。
- 一个按钮菜单可以绑定多个 API；同一动作建议使用同一个权限码。
- 指定权限码时，菜单和绑定 API 统一使用该权限码；未指定且 API 权限码一致时可自动推导。
- API 缺少权限码时，系统按 Method + Path 生成默认权限码并补齐权限记录。
- 修改已绑定 API 的权限码时，同步菜单、同组 API 和角色授权；不再被引用的旧权限才可清理。
- 绑定、权限记录、角色授权和受影响用户授权版本必须处于同一事务边界。
- 删除菜单或 API 时清理 `menu_apis` 关联。
- 公开 API 不得生成按钮菜单或绑定菜单权限。

## 典型验证流程

```text
同步 API 元数据和权限码
→ 创建或生成按钮菜单
→ 绑定 API
→ 给角色分配菜单和权限
→ 用户重新登录
→ 检查 /api/user/context
→ 调用按钮对应 API
```

详细操作入口见 Swagger UI；事务、权限码和 Token 失效行为以 OpenSpec 为准。
