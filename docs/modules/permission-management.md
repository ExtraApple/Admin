# 权限管理

> 本文只保留权限模块边界和同步规则。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/rbac/spec.md`](../../openspec/specs/rbac/spec.md) 为准。

## 模块边界

`internal/authorization` 负责角色、权限、权限分组、角色权限关系和访问快照。API 元数据由 `internal/apimetadata` 负责，菜单由 `internal/navigation` 负责。

## 关键规则

- 权限码必须唯一；创建后权限码不可通过普通修改接口变更。
- 权限分组、权限记录和角色权限关系支持管理接口。
- 权限PUT的`group`只是标签：遗漏保持、显式空字符串清空、非空替换，不删除Permission或独立分组资源；`name`显式空／纯空白整次拒绝，权限码仍不可修改。JSON null沿原解析表示未提交；标签清空不新增授权版本触发。[S1字段验收](../reviews/evidence/authorization-field-clearing-2026-10-18.json)包含真实分组资源与权限身份保留，主规格已按依赖同步，未归档。
- 普通角色权限分配是全量替换，空列表表示清空；原始0或不存在的权限ID整次拒绝，合法重复ID去重。直接替换`admin`角色权限拒绝，包括清空。Permission本身没有启停状态，不为预配置添加该前提；全部前提满足后，实际原始配置集合等价则不重建关系、不创建／提升用户版本，不能用过滤后的有效权限掩盖旧关联清理。
- 用户有效权限码从全部可用角色权限关系推导，并在授权上下文中去重。
- 启用的`admin`角色拥有超级管理员兜底能力，但禁用API仍然拒绝访问；停用admin关联只保留管理保护身份，不授予当前有效权限。
- API 权限同步、启动 Seed 和 API 文档都消费同一份 Route Catalog Snapshot，不扫描 Gin Engine。
- 角色权限、菜单/API联动的真实配置变化使当前依赖用户旧Token失效；合法等价配置的豁免见[配置no-op验收](../reviews/evidence/configuration-noop-2026-10-18.json)，不改变普通权限标签更新的版本策略。
- 权限分配与删除共享当前权限目标锁；关联清理和实际用户版本同事务。API/Menu新建权限码引用与旧码清理参与相同权限资源屏障，正常目录及有效权限快照不因此改为锁定读。[生命周期证据](../reviews/evidence/authorization-reference-lifecycle-2026-10-18.json)不代替F2整请求验收，后者另见[F2证据](../reviews/evidence/api-permission-sync-atomic-2026-10-18.json)。

## 接口入口

Swagger UI 的 `authorization` 标签提供：

- 权限和权限分组 CRUD。
- 权限码列表。
- Route Catalog 权限同步。
- 角色权限分配和查询。

新增或修改权限行为时，先更新 OpenSpec，再更新本页摘要。
