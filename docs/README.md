# docs 使用说明

`docs/` 只保存实现说明、设计草案、测试步骤、运维说明和背景材料。

当前系统行为以 `openspec/specs/` 为准；未来功能变更以 `openspec/changes/` 为准。不要再把 `docs/` 当作需求或规格的唯一来源。

## 文档定位

| 文档 | 定位 | 对应 OpenSpec |
|---|---|---|
| `登录安全策略.md` | 已实现模块的实现说明 | `openspec/specs/auth/spec.md` |
| `用户管理.md` | 已实现模块的实现说明 | `openspec/specs/user-management/spec.md` |
| `角色管理.md` | 已实现模块的实现说明 | `openspec/specs/rbac/spec.md` |
| `权限管理.md` | 已实现模块的实现说明 | `openspec/specs/rbac/spec.md` |
| `菜单管理.md` | 已实现模块的实现说明 | `openspec/specs/menu-management/spec.md` |
| `文件管理.md` | 已实现模块的实现说明 | `openspec/specs/file-management/spec.md` |
| `Zap日志模块.md` | 待实现设计草案 | `openspec/specs/logging/spec.md` |
| `操作日志.md` | 待实现设计草案 | `openspec/specs/logging/spec.md`，后续应创建 `openspec/changes/add-operation-audit-logs/` |

## 维护规则

- 已实现行为：先更新 `openspec/specs/`，再按需要更新这里的实现说明。
- 新功能：先创建 OpenSpec change，再实现代码，最后归档回主规格。
- 长代码块、Apifox 测试步骤、实现过程记录可以放在 `docs/`。
- 行为规则、接口约束、安全边界、错误处理必须沉淀到 OpenSpec。
