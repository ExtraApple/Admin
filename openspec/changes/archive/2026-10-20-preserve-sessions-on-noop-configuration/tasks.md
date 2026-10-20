## 1. 范围与前置

- [x] 1.1 确认以下前置已实施、验证并完成相应规格同步后再实施本change：`enforce-authorization-reference-lifecycle`、`enforce-protected-authorization-assignments`、`fix-menu-api-code-consistency`、`support-explicit-authorization-field-clearing`；区分design中的功能与发布依赖，不能乱序用完整Requirement覆盖。

## 2. Model / DTO / Contract

- [x] 2.1 消费S1存在性输入和生命周期的锁定事实，按owner定义小型字段/集合比较，不新增全局diff引擎。

## 3. Application / Adapter实现

- [x] 3.1 Authorization在全部前提满足后比较角色字段、权限集和数据范围/组织集，同值合法成功不增版本，空更新仍拒绝。
- [x] 3.2 Navigation比较菜单字段、角色菜单、API最终码/权限/授权事实；真实派生关系变化不能只凭DTO同值返回。
- [x] 3.3 真实角色/菜单展示变化保留依赖失效，合法全skip菜单同步不额外增版；不收紧成I3最小集合或涉及Identity/Kick。

## 4. HTTP / App装配

- [x] 4.1 保留现有请求/响应和权限码，确保返回持久化当前事实；内部Runner重执行不复用上一attempt的变化标记。

## 5. 行为回归与原程序验证

- [x] 5.1 角色/菜单字段全同值、等价权限/菜单/custom/API配置成功且版本不变，真实展示变化仍升版本。
- [x] 5.2 空更新、保护角色、非法引用/越权即使看似同值仍按原拒绝，无事实/版本残留。
- [x] 5.3 需清旧custom、需补权限/授权或真实码变更不能no-op；第二用户版本失败仍全回滚。
- [x] 5.4 合法菜单同步全skip不创建、不失效，未知parent_path处理不由此改变；不验证未实施全图精准集合。

## 6. 文档与交付

- [x] 6.1 在产品行为实现后只保留能捕获消费者可见错误的确定性回归；运行本切片适用门禁及原程序smoke，保存去敏响应/事务前后状态/必要交错证据，清理自建资源，不把未运行检查标通过。
- [x] 6.2 依据实际验收更新现有实现说明、审计状态与必要changelog；保留历史JSON原样，记录本次新证据和局部覆盖，不把其他C6风险标完成。
- [x] 6.3 核对delta完整保留前置Requirement与新场景，并完成OpenSpec检查；规格同步/归档按独立流程及design发布依赖执行，不在apply中自动归档。
