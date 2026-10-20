## Context

角色和菜单更新/配置替换多为接受合法输入后直接写并提升版本，菜单路径甚至使用AllUserIDs。N目标只改变“是否真实变化”这一触发判断，不完成I3全图最小集合。字段存在性依赖S1，联动事实依赖C3及生命周期规则。

依据[proposal](proposal.md)及其引用审计/裁决；本文件原为实施前设计，现已按下列边界实现并定向验收，见[新证据](../../../../docs/reviews/evidence/configuration-noop-2026-10-18.json)，不表示生产发布或自动归档。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- 角色/菜单字段及角色权限/菜单/custom/API绑定等配置集合，在完成权限、保护、引用、范围和现有并发前提后比较规范化的目标事实。
- 无字段请求仍拒绝；真正同值或等价配置成功且不提升版本；派生关系仍需变更时不得假no-op。
- 真实角色/菜单展示变化仍使现行依赖集合失效；不以另一角色重复授权或最终权限未变豁免。
- 同步正常skip且没有任何派生写入时不额外失效；不改变未知parent_path处理。

**Non-Goals:**

不覆盖成员关系差量或Identity资料入口，不新增全局最小失效/最终权限diff，不豁免真实展示变化、Kick或状态切换。

## Decisions

### 1. 比较完整业务目标而非SQL结果

在锁定当前配置并完成所有前提后，用现有规范化规则构造完整目标，比较字段和适用关系。等价才返回现有响应；实际清custom/补权限/改绑定码等派生变化不能只看DTO同值跳过。

**理由与替代方案**：不用RowsAffected、更新时间、字段是否非空或最终有效权限相同作为语义判定；不引入通用diff引擎。

### 2. 领域内小型比较与单次失效

角色字段、data-scope及组织集合、角色权限集合由Authorization比较；菜单字段、角色菜单、API绑定最终码/权限关系与同步写入由Navigation比较。通过已有最外层事务控制一次版本处理。

**理由与替代方案**：成员差量与Identity同值各由专门change持有；不搞跨所有模块的通用Noop中间件。

### 3. 真实展示变化依然是变化

角色名称/说明/排序和菜单名称/图标/排序真实变化仍按现行依赖用户失效，当前较宽覆盖不在此强制收紧。完全无合法字段仍拒绝。

**理由与替代方案**：保留已裁定N3而不是把不影响最终权限当豁免；不涉及Kick或状态切换。

### 实施与规格发布依赖

- 功能/写入协议前置：`enforce-authorization-reference-lifecycle`、`support-explicit-authorization-field-clearing`、`fix-menu-api-code-consistency`
- 共享Requirement发布基线：`enforce-authorization-reference-lifecycle`、`enforce-protected-authorization-assignments`、`support-explicit-authorization-field-clearing`
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- 提前return绕过admin/无效引用或CAS → 校验及当前读取在比较之前，加入相同集合带非法原始ID/保护对象的拒绝案例。
- 请求码同值但派生关系不同 → 比较规范化最终绑定事实，不只比较一字段；S1空值存在性要先落地。

## Verification

- 角色/菜单字段全同值、等价权限/菜单/custom/API配置成功且版本不变，真实展示变化仍升版本。
- 空更新、保护角色、非法引用/越权即使看似同值仍按原拒绝，无事实/版本残留。
- 需清旧custom、需补权限/授权或真实码变更不能no-op；第二用户版本失败仍全回滚。
- 合法菜单同步全skip不创建、不失效，未知parent_path处理不由此改变；不验证未实施全图精准集合。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。全Go门禁和原程序92条观察已完成，包含真实会话／缺行、派生修复、后次版本失败、禁止写入、重试计数及当前角色行交错；SQL胜方不是第二HTTP写者，SIGNAL1213不是物理死锁。七个Requirement的前置场景均保留；具体执行器恢复、夹具修正及清理边界见验收证据。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
