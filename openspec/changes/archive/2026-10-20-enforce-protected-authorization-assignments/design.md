## Context

多个批量入口只接roleID/IDs，缺操作者与完整旧关系判断。Identity桥接不仅检查role.Status，还调用本身已过滤roles.status=1的RolesForUser，因此只删局部状态判断不足。新目标来源是P/R裁决，不是本轮已复现漏洞。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- 完整验证原始目标以及实际新增/移除的旧关系对象，任一非法、越界或受保护变更整次拒绝。
- 直接编辑admin角色的用户/权限/菜单关联拒绝；现存admin关联形成保护身份，不因角色停用解除。
- 组织整组成员允许原样保留受保护关系，实际增删本人/受保护归属拒绝；删除组织不得连带移除这类归属。
- 保留停用普通用户/角色/菜单预配置；custom及菜单API集合仍非空，非custom非空organization_ids拒绝。
- 角色完整替换与逐人组织可管理部分替换分开，Seed和普通对象生命周期引用清理按各自明确例外执行。

**Non-Goals:**

不把新前提全称旧漏洞，不增加Permission状态或组织范围之外另一套配置管理范围，不把共享非保护配置对操作者的间接影响当成直接本人写入。

## Decisions

### 1. 保护与有效授权使用不同事实

管理保护消费现存实际角色关联摘要，不走只返回启用角色的运行时RolesForUser；App统一桥接到各caller-owned最小Contract，返回安全的存在/删除/保护/可管理事实。

**理由与替代方案**：不改变Authorization有效权限过滤，不依赖users.role；不用给所有角色查询加includeDisabled布尔旁路。

### 2. 操作者与整次替换前提到达owner

需要范围判断的角色批量用例显式接operatorID，HTTP传当前身份并迁移全部内部调用；Application验证原始目标和实际新增/移除旧对象后才写。全局角色/权限/菜单配置只用其管理契约，不发明另一套组织范围。

**理由与替代方案**：不只依赖中间件或前端，不新增通用校验引擎；合法Seed继续专用初始化入口，不开放operatorID=0绕过管理保护。

### 3. 按入口区分保护与状态

角色完整替换按全量旧新边界；组织整组仅实际变更保护关系拒绝、保留不变可维护；逐人组织保留不可管理关系。custom非空/非custom输入适用性、菜单API非空及API特有启用/认证规则分别判断。

**理由与替代方案**：停用普通对象仍存在可预配置，Permission没有状态。受控软删旧清理由生命周期change补足，不能因为目录查不到就默认允许。

### 4. 对共享配置不错误应用本人规则

直接本人身份/归属禁止不等于普通角色权限/菜单配置不能影响操作者；按实际依赖失效，不跳过本人。

**理由与替代方案**：避免统一保护helper把合法配置阻断；admin直接关联保护与普通生命周期引用清理明确分开。

### 实施与规格发布依赖

- 功能/写入协议前置：`fix-menu-api-binding-id-validation`
- 共享Requirement发布基线：`fix-menu-api-code-consistency`
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- 原始集合提前过滤、只查新目标漏删旧关系 → 在owner事务内保留原始ID并读旧集合验证实际移除，空数组同样执行。
- 角色停用后保护失效 → 使用未按status裁剪的关联事实并给三个owner同一真实存储场景。
- 全局配置权限与用户scope混用 → 每条路由写明实际管理前提；不向所有配置用例机械注入组织scope。

## Verification

- 角色批量目标或被移除者含本人/admin/越界/不存在/0，分别整次拒绝且关系和全部版本不变。
- admin的用户/权限/菜单直接编辑拒绝，停用admin角色下用户仍在Identity/Authorization/Organization受保护。
- 组织保留本人/admin原关系可合法增删其他普通成员；实际增删保护关系或删组织连带移除则拒绝。
- 停用普通对象预配置成功但运行时不因此授权；非法Permission ID拒绝但不新增Permission状态。
- custom空、非custom原始非空组织列表、菜单API空等保持其精确前提；合法共享配置允许操作者间接失效。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。本段保留创建工件时的验收要求；原产品执行见[原验收证据](../../../../docs/reviews/evidence/protected-authorization-assignments-2026-10-17.json)，本次补证见下。

### 核验补证验收：W1角色权限列表排序（2026-10-20，已完成）

[核验报告W1](../../../../docs/reviews/openspec-implementation-verification-2026-10-20.md#w1继承的列表排序分页条件缺少直接断言)确认“查询角色权限”已有按sort查询及非空／空结果证据，但单权限断言不能证明多项排序。新增tasks第7节仅补现有条款的行为证据，不把证据不足认定为已确认产品缺陷，不修改保护、分配、无权限空数组或Permission无启停字段的业务规则。

采用真实owner持久化，打乱多项权限的插入／分配顺序，以精确返回序列而非长度／map集合／SQL文本断言排序；适用的既有空集回归直接复用。原程序成功GET及前后事实用于补充HTTP边界证明，不用mock调用计数替代结果。补证发现实际违约时先保留反例，再限定既有契约内的根因修复。

角色／权限分页列表由`support-explicit-authorization-field-clearing`第7节承担，本节只处理角色已分配权限列表；配置no-op的重复场景复用本证据，不重复创建相同测试或功能依赖。历史验收不改写，以下单列新增补证结果。

- `TestRepositoryListsAssignedPermissionsInSortAndIDOrder`以真实持久化断言多sort／同sort的精确ID序列、其他角色的未分配项排除及清空后空集。
- [新增原程序证据](../../../../docs/reviews/evidence/authorization-followup-2026-10-20.json)`W1-assigned-permissions`经真实权限GET得到`[6002,6005,6004,6001]`，另一角色得到`[]`；插入／分配顺序与返回次序不同。W1全部读取前后13项受保护投影及版本相同。
- 定向／全Go、真实MySQL及OpenSpec严格门禁通过，自建运行资源已清理。第7节2项完成，W1对应条件已关闭；未改保护、分配、Permission状态或排序政策，未归档。


## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
