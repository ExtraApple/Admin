# docs 使用说明

`docs/` 保存实现说明、设计草案、测试步骤、运维说明和背景材料。

当前系统稳定行为以 `openspec/specs/` 为准；未来功能变更应先创建 `openspec/changes/`，实现完成后再同步回主规格和模块文档。

## 文档定位

| 文档 | 定位 | 对应 OpenSpec |
|---|---|---|
| `登录安全策略.md` | 登录锁定、验证码和安全策略说明 | `openspec/specs/auth/spec.md` |
| `配置管理.md` | 环境变量、敏感配置和超级管理员初始化说明 | `openspec/specs/auth/spec.md` |
| `用户管理.md` | 用户、个人中心、头像、强制下线等说明 | `openspec/specs/user-management/spec.md` |
| `角色管理.md` | 角色、角色用户、角色权限、角色菜单、数据范围说明 | `openspec/specs/rbac/spec.md` |
| `数据权限.md` | 角色数据范围和组织数据过滤说明 | `openspec/specs/rbac/spec.md`、`openspec/specs/organization-management/spec.md` |
| `权限管理.md` | 权限、权限分组、权限码同步说明 | `openspec/specs/rbac/spec.md` |
| `菜单管理.md` | 菜单树、菜单 CRUD、角色菜单、菜单 API 绑定说明 | `openspec/specs/menu-management/spec.md` |
| `菜单与API联动.md` | 菜单按钮和 API 权限码联动说明 | `openspec/specs/menu-management/spec.md`、`openspec/specs/api-management/spec.md` |
| `API管理.md` | API 元数据、路由同步、动态权限、生成按钮菜单说明 | `openspec/specs/api-management/spec.md` |
| `文件管理.md` | 文件上传、下载、浏览、轮转说明 | `openspec/specs/file-management/spec.md` |
| `Zap日志模块.md` | Zap 运行日志模块说明 | `openspec/specs/logging/spec.md` |
| `操作日志.md` | 审计日志、冷热归档、分类查询说明 | `openspec/specs/logging/spec.md` |
| `组织管理.md` | 组织 CRUD、组织树、成员绑定、数据范围说明 | `openspec/specs/organization-management/spec.md` |
| `字典管理.md` | 字典类型、字典条目、公开字典查询说明 | `openspec/specs/dict-management/spec.md` |

## modify 目录

`docs/modify/` 用来记录已经完成的跨模块重要修改，例如权限链路、配置迁移、数据权限、菜单与 API 联动。

当前主要记录：

```text
docs/modify/权限链路修改记录.md
```

## 维护规则

- 已实现行为：先更新 `openspec/specs/`，再更新对应模块文档。
- 新功能：先创建 OpenSpec change，再实现代码，最后同步主规格和文档。
- 接口测试步骤可以放在 `docs/`。
- 行为规则、接口约束、安全边界、错误处理必须沉淀到 OpenSpec。
- 不要把临时调试日志写入正式文档。
