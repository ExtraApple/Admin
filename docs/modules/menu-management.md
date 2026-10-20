# 菜单管理

> 本文只保留菜单模块边界和使用注意事项。接口路径、请求体、响应 Schema 和认证要求以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/menu-management/spec.md`](../../openspec/specs/menu-management/spec.md) 为准。

## 模块边界

菜单由 `internal/navigation` 拥有，负责：

- 菜单树和菜单记录 CRUD。
- 前端路由同步。
- 角色菜单分配与可见菜单树。
- 菜单与 API 元数据的绑定。

## 关键规则

- `type=1` 为目录，`type=2` 为页面菜单，`type=3` 为按钮菜单。
- 创建状态区分遗漏与显式值：`0` 保存为停用，`1` 保存为启用，遗漏或无效数值沿用默认启用；响应和存储一致，停用项仍可从配置树读取，但不进入已分配用户的可见菜单。
- 创建持久化保留数据库默认值，以单次显式INSERT保存规范化状态并返回真实ID、维护时间列；没有事后修复UPDATE。当前依赖GORM v1.31.1的map回填行为，升级须保留SQLite与原生MySQL的ID／状态回读验证。C2的真实HTTP、Seed／同步／按钮默认和中途版本失败回滚见[验收记录](../reviews/evidence/c2-menu-create-status-fix-2026-10-16.json)。
- 菜单路径必须唯一；按钮菜单可以没有路径和组件。
- 菜单PUT的`icon`／`component`／合法`path`区分遗漏、空字符串和非空替换；空path沿现有规范化存为SQL NULL，不新增类型／路径政策。`name`显式空或纯空白整次拒绝，路径冲突也拒绝其他同时提交的清空；`permission_code`仍走独立绑定码契约。JSON null沿原解析表示未提交。真实清空仍按现有菜单配置规则处理版本，后续用户版本失败全回滚，API码及绑定／授权关系不变；见[S1验收](../reviews/evidence/authorization-field-clearing-2026-10-18.json)，主规格已按依赖同步，未归档。
- 有子菜单的节点不能删除；删除与父引用创建共享菜单目标锁，删除入口将目标与当前子级合并按ID锁定后判断，不能先锁高ID父级再追加低ID子级。子级先提交则拒删，父级先删则拒绝新子级；合法删除清理`role_menus`和`menu_apis`。
- 普通角色菜单分配是全量替换，原始0／不存在菜单整次拒绝、合法重复ID去重；普通停用角色及菜单允许预配置，但直接编辑`admin`角色菜单关联拒绝（包括清空）。用户可见菜单仍从有效角色合并、去重并按权限码和菜单状态过滤。
- 超级管理员返回全部启用菜单；普通用户只返回角色已分配且有权限的菜单，并保留必要的父级目录。
- 菜单/API 绑定必须在同一事务中完成，失败时回滚关联、权限和授权版本变化。
- 管理员 `GET /api/admin/roles/:id/menus` 返回按 `sort asc,id asc` 排序的平铺已分配菜单数组，含 `parent_id,status`，保留停用项和没有分配父级的子菜单；未分配父级不是已分配项。
- 编辑器以 `/api/admin/menus` 完整树定位层级，只按平铺响应 ID 预选，不自动选父级。单角色缺权限码仅为配置风险，不能代表用户最终可见菜单数。
- `GET /api/user/context` 仍返回按用户全部角色、启用状态及有效权限过滤的运行时菜单树；配置读取变化不赋予可见性。此 breaking 读取变化见 [`菜单 delta`](../../openspec/changes/build-admin-workbench/specs/menu-management/spec.md)，尚未据此同步长期规格或宣称验收。
- 菜单创建、修改、角色菜单分配、同步、按钮生成及适用Seed都参与目标协议；锁后缺失返回既有不存在错误，非法角色菜单选择仍是校验错误。原程序发现的缺失父级误返500已修正并保留前后证据，见[R4-R6验收](../reviews/evidence/authorization-reference-lifecycle-2026-10-18.json)。
- 合法菜单字段同值、显式角色菜单集合等价及合法全skip同步均不提升版本；比较仍先满足全部现有校验，不以运行时可见性裁剪配置。真实名称／图标／排序等展示变化保留现行菜单更新的全用户触发，不在此实现I3最小集合。显式清空第一次是真实变化，重复已清空值可无变化；同步重试计数按attempt重置。[配置no-op验收](../reviews/evidence/configuration-noop-2026-10-18.json)已完成，delta未同步、未归档；未知parent_path政策未由此裁定。

## 接口入口

接口分组位于 Swagger UI 的 `menu` 标签，主要包括：

- 菜单查询、创建、修改、删除。
- 前端路由同步。
- 菜单绑定和查询 API。
- 角色菜单绑定和查询。

前端初始化菜单使用：

```text
GET /api/user/context
```

## 验证方式

1. 启动服务并访问 `/docs`。
2. 先完成登录和 API/权限同步，再按 Swagger Schema 调用接口。
3. 涉及菜单可见性时，重新登录并检查 `/api/user/context`。
4. 规则变化先修改 OpenSpec，再更新本页的边界摘要。
