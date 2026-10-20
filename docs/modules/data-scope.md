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

`custom`必须至少包含一个合法存在组织；原始0／不存在ID整次拒绝。其他模式的`organization_ids`必须省略或为空，非空列表（包括仅含0）不能被静默忽略。此配置入口按其全局管理权限判断，不额外套用操作者的组织数据范围；用户／组织业务操作仍分别应用自身Scope。普通停用角色允许合法预配置，运行时不因此启用该角色。

普通角色通过普通PUT或专用数据范围入口离开`custom`，都在同一事务清理旧`role_data_scopes`并处理实际成员版本。当前普通`custom`引用阻止组织删除，停用和无成员不豁免；不能为方便删除自动切`self/all`。自定义引用先进入C4当前父图协议，再锁角色并复核组织存在。R4-R6验收见[生命周期记录](../reviews/evidence/authorization-reference-lifecycle-2026-10-18.json)。

数据范围合法同值比较使用锁定当前模式与原始组织关联；custom目标先逐项校验再去重，等价集合不重建关联、不创建／提升版本。非custom字段即使同值，残留旧custom关系也必须清理并处理版本；普通PUT与专用入口均覆盖。保护角色、非法组织及普通PUT不接受custom的前提不因同值豁免。[配置no-op验收](../reviews/evidence/configuration-noop-2026-10-18.json)已完成，delta未同步、未归档。

管理写入使用显式current-read范围能力；普通认证、目录与授权上下文继续使用原有快照读取。不能将只锁关联表的JOIN当作角色元数据的通用当前读。

## 生效资源

当前主要应用于：

- 管理员用户列表及用户管理操作。
- 组织单位列表、组织树和组织成员查询。

组织树会保留可见节点所需的祖先节点。角色数据范围事实真实变化时，依赖该角色的用户旧Token失效。纯角色／组织成员变化只失效实际新增／移除用户；其他范围数据读取者不额外下线，并按当前数据读取最新成员。[成员差量验收](../reviews/evidence/membership-delta-2026-10-18.json)不以最终权限重复豁免真实归属变化。组织父边真实移动的I4也已独立验收：按逐角色／成员范围根的层级事实依赖失效，不做最终合并范围的最小差分；共同祖先及不变范围不扩入，详见[组织拓扑与切换](organization-management.md#组织拓扑写入与部署切换)及[I4证据](../reviews/evidence/organization-hierarchy-dependents-2026-10-18.json)。I4 delta未同步、未归档。

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
