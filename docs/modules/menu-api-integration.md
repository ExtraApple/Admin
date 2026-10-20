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
- 菜单绑定先校验原始 API ID 列表：`0` 整次拒绝，不能先过滤；合法重复 ID 仍去重处理，空列表不表示解绑。HTTP 和直接调用同一用例均遵守该规则，无效请求不留下菜单、API、关联、权限、角色授权或版本写入。C1定向验收见[去敏记录](../reviews/evidence/c1-menu-api-ids-fix-2026-10-16.json)。
- 一个按钮菜单可以绑定多个 API；同一动作建议使用同一个权限码。
- 指定权限码时，菜单和绑定 API 统一使用该权限码；未指定且 API 权限码一致时可自动推导。
- API 缺少权限码时，系统按 Method + Path 生成默认权限码并补齐权限记录。
- 普通菜单PUT修改权限码时，同样在一个最外层事务同步现有直接绑定API、确保权限并合并角色授权，绑定ID集合不变；普通字段和版本一起提交，真实变化时版本只处理一次。显式空码沿用既有绑定解析，不表示解绑。C3的四阶段真实失败回滚、成功后旧会话拒绝及引用清理见[验收记录](../reviews/evidence/c3-menu-api-code-fix-2026-10-16.json)；不据此声称S3复杂多菜单图已设计或全量验收。
- 修改已绑定 API 的权限码时，同步菜单、同组 API 和角色授权；不再被引用的旧权限才可清理。
- 绑定、权限记录、角色授权和受影响用户授权版本必须处于同一事务边界。
- 删除菜单或 API 时清理 `menu_apis` 关联。
- API启用、存在且需要认证是绑定提交前提，不使用锁前快照代替；先合法绑定后停用可保留配置，但真实路由访问仍拒绝，后续删除清当前关联并失效实际用户。
- 动态直接依赖先预发现、合并升序锁定，再current read复核；变化时拒绝本请求，Menu入口为`NAV_CONFLICT`，API入口由App保留cause映射为既有`API_META_CONFLICT`。清理检查用到的同码资源只加入锁定检查集，不扩大直接传播和用户集合，不新增S3政策。
- 可选外部缓存钩子通过事务Runner延后到最外层提交；外层回滚或失败重试丢弃该次钩子。真实交错、较低ID等待、胜方提交保留和SQLite外部副作用验证见[生命周期证据](../reviews/evidence/authorization-reference-lifecycle-2026-10-18.json)；未声称生产缓存接线。
- F2另有[整请求同步验收](../reviews/evidence/api-permission-sync-atomic-2026-10-18.json)：整轮锁集准备与原业务遍历分离，后项失败回滚前项事实及全部版本；响应只统计本次提交且最终保留的新权限。同用户多项实际联动保留逐API版本触发，不借此改为请求去重或选择S3传播政策。
- 菜单绑定先完成原始ID、启用／认证、引用与当前事实校验；目标API集合、最终实际码和既有规则维护的派生事实都不变时，成功但不重建关联、不增版本，也不触发可选失效钩子。缺失／软删权限、直接API码漂移、真实目标或码变化仍需修复／合并授权并处理版本；同码不会凭空引入额外授权或元数据更新政策。三类第二用户版本失败回滚、当前角色行交错及合法全skip／真实Runner重试见[配置no-op证据](../reviews/evidence/configuration-noop-2026-10-18.json)，不扩成API其他元数据字段或S3传播设计。
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
