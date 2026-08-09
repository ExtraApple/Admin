# 数据权限

> 本文只保留数据范围的概念和使用边界。角色数据范围接口以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/rbac/spec.md`](../../openspec/specs/rbac/spec.md) 和 [`openspec/specs/organization-management/spec.md`](../../openspec/specs/organization-management/spec.md) 为准。

## 与 API 权限的区别

- API 权限决定用户能否访问接口。
- 数据范围决定接口允许用户看到或操作哪些组织、用户数据。
- 数据范围不能替代 API 动态权限。

## 支持的数据范围

| 值 | 含义 |
|---|---|
| `all` | 全部数据 |
| `self` | 仅当前用户数据；组织范围为空 |
| `org` | 当前用户所属组织 |
| `org_and_children` | 当前用户所属组织及下级组织 |
| `custom` | 角色绑定的自定义组织单位 |

编码为 `admin` 的角色固定拥有 `all`，不能通过数据范围接口修改。

## 生效资源

当前主要应用于：

- 管理员用户列表及用户管理操作。
- 组织单位列表、组织树和组织成员查询。

组织树会保留可见节点所需的祖先节点。配置角色数据范围或组织成员关系后，受影响用户的旧 Token 会失效。

## 验证入口

使用 Swagger UI 完成以下顺序：

```text
创建组织单位
→ 绑定组织成员
→ 创建非 admin 角色
→ 配置数据范围
→ 分配角色和 API 权限
→ 重新登录
→ 检查用户与组织查询结果
```
