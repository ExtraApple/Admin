## 1. 范围与前置

- [x] 1.1 确认以下前置已实施、验证并完成相应规格同步后再实施本change：`fix-menu-api-binding-id-validation`、`fix-menu-api-code-consistency`；区分design中的功能与发布依赖，不能乱序用完整Requirement覆盖。

## 2. Model / DTO / Contract

- [x] 2.1 为需要操作者的批量用例显式贯通operatorID，按LSP迁移全部caller；新增安全的管理事实读取，不扩大公开目录到软删用户。
- [x] 2.2 统一保护身份来源为未按启用状态裁剪的实际admin关联，不能只删Identity局部status判断而继续用RolesForUser。

## 3. Application / Adapter实现

- [x] 3.1 角色完整替换验证原始目标及实际新增/移除旧对象，空数组同样保护本人/admin/范围，不静默部分替换。
- [x] 3.2 角色权限/菜单及data-scope逐入口做完整引用、admin关联保护和输入适用性；停用普通对象可预配置，Permission不新增状态。
- [x] 3.3 组织整组成员只对实际保护关系变更拒绝，保留不变关系可维护；删除组织连带移除保护成员拒绝；逐人组织仍保留不可管理关系。
- [x] 3.4 保留合法Seed、普通生命周期受控引用清理和共享非保护配置间接影响操作者的边界；不得引入operatorID=0管理旁路。

## 4. HTTP / App装配

- [x] 4.1 通过App显式装配各caller-owned事实能力，所有现有HTTP传正确操作者，保持具体模块拒绝语义而非全部409。

## 5. 行为回归与原程序验证

- [x] 5.1 角色批量目标或被移除者含本人/admin/越界/不存在/0，分别整次拒绝且关系和全部版本不变。
- [x] 5.2 admin的用户/权限/菜单直接编辑拒绝，停用admin角色下用户仍在Identity/Authorization/Organization受保护。
- [x] 5.3 组织保留本人/admin原关系可合法增删其他普通成员；实际增删保护关系或删组织连带移除则拒绝。
- [x] 5.4 停用普通对象预配置成功但运行时不因此授权；非法Permission ID拒绝但不新增Permission状态。
- [x] 5.5 custom空、非custom原始非空组织列表、菜单API空等保持其精确前提；合法共享配置允许操作者间接失效。

## 6. 文档与交付

- [x] 6.1 在产品行为实现后只保留能捕获消费者可见错误的确定性回归；运行本切片适用门禁及原程序smoke，保存去敏响应/事务前后状态/必要交错证据，清理自建资源，不把未运行检查标通过。
- [x] 6.2 依据实际验收更新现有实现说明、审计状态与必要changelog；保留历史JSON原样，记录本次新证据和局部覆盖，不把其他C6风险标完成。
- [x] 6.3 核对delta完整保留前置Requirement与新场景，并完成OpenSpec检查；规格同步/归档按独立流程及design发布依赖执行，不在apply中自动归档。

## 7. 核验补证：W1角色权限列表排序（2026-10-20）

以下为核验后新增的补证任务，已另行执行并记录于design及[新去敏证据](../../../../docs/reviews/evidence/authorization-followup-2026-10-20.json)，原产品验收保留。来源：[核验报告W1](../../../../docs/reviews/openspec-implementation-verification-2026-10-20.md#w1继承的列表排序分页条件缺少直接断言)。本change只负责“查询角色权限”；角色／权限分页列表由`support-explicit-authorization-field-clearing`第7节负责，配置no-op继承条款复用同一结果。

- [x] 7.1 为角色已分配权限补真实持久化／公共查询回归：建立多项不同sort且插入／分配顺序打乱的权限，断言返回精确排序及仅含实际分配项；复用适用的空列表回归，必要时补空数组行为。保留既有同sort处理，不增加Permission状态或新的授权／排序政策。
- [x] 7.2 通过原程序角色权限GET路由观察多项有序结果及无权限时的空数组，保存去敏响应、实际关联／排序事实，确认读取不改变关系或版本并清理自建资源；结果回链核验W1及配置no-op同名场景，取得证据后才关闭本补证任务。
