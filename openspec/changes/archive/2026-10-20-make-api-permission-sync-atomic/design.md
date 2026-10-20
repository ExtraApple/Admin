## Context

API Metadata.Service原有可选transactions字段经WithRouteSource注入，而R6已为Core要求同一事务Runner。SyncPermissions仍循环逐项协调，没有整轮Run。此change让Service复用Core的必需Runner，移除重复的可选事务注入；WithRouteSource只提供路由事实，不改变各同步用途。Platform.Run加入已有ctx，对外层1205/1213按既有次数重试。

依据[proposal](proposal.md)及其引用审计/裁决；Context记录实施前问题，Decisions记录采用方案，实际实施与验收见下文记录。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- API Metadata拥有整请求最外层事务，Navigation/Authorization通过Context加入，不独立提交。
- API权限码、权限、授权关联及全部受影响用户版本一起提交或回滚。
- created/created_count/updated_api基于已提交成功attempt；内部重试不累加已回滚attempt结果。
- 保留不需认证与已存在权限的正常skip以及现有受控公开错误。

**Non-Goals:**

不合并Route Catalog权限同步、API路由同步和菜单同步，不重设计Seed，不新增重试次数/部分完成响应；旧业务计数问题未复现，不冒称已确认缺陷。

## Decisions

### 1. API Metadata拥有整请求事务

SyncPermissions在API Metadata必需Runner的Run内读取候选API并遍历；Core/Navigation/Authorization使用同一transactionContext，内层Run只加入，不独立提交或fallback全局DB。Service与Core不保留两个可选Runner来源。

**理由与替代方案**：复用已装配Runner与协调端口，不增加第二种批处理事务或改Seed边界；缺必需装配应在组合根失败，不静默退回逐项提交。

### 2. attempt局部返回值

created、updated等在每次callback内初始化，只有Run最终成功才向HTTP发布该attempt结果；任何失败返回不带部分完成成功统计。协调中的缓存等外部副作用推迟至真正最外层提交后。

**理由与替代方案**：无需新增重试承诺，但必须正确支持现有Runner重执行；不能因为内层Run返回nil就认为已提交并做不可回滚副作用。

原程序前置观察已证明协调内部创建的权限会被旧统计遗漏（响应created_count=1，实际新增10），这不是D3重试累加疑点的复现。本change按既定“统计对应实际提交”目标，以owner当前事实覆盖协调内部创建：created仅列本attempt新增且最终保留的权限，既有记录恢复不算新建；updated_api仍沿原用途统计补齐空码项，不改成所有间接传播写入行数。

### 3. 维持用途与skip

NeedAuth不适用、已存在权限保持正常skip；权限码解析、正常授权合并沿已有协调路径。此同步不等于Route Catalog同步、API路由同步或菜单同步。

**理由与替代方案**：不统一所有同步策略或借机裁决S3/S5；不把旧D3业务计数疑点描述成已复现缺陷。

### 4. 整轮复用R6严格目标顺序

新增仅供本用例的准备能力：候选及既有直接关系先预发现，合并请求实际API／菜单／权限资源与清理检查资源，统一升序锁定后current read复核，再执行原有业务遍历。不得用“每项内部有序”替代同一外层事务的R6顺序，不额外锁无关全量API。可能失效用户的既有版本目标先有序锁定但不初始化、不递增；正常skip不得因此产生版本写入。

业务遍历仍保留原排序和派生，不以锁顺序重排共享API传播政策；同码检查集不扩大传播或失效集合。准备状态只属于本次事务attempt，不形成通用关系框架。

### 5. 逐API触发保持不变

**2026-10-18用户裁定**：同次同步的两项实际API联动影响同一用户时，保留逐API触发，授权版本可提升两次；全部提升与整请求共同提交／回滚。不增加请求级失效去重，也不借F2提前实现no-op／成员差量或S3。

### 实施与规格发布依赖

- 功能/写入协议前置：`fix-menu-api-code-consistency`、`enforce-authorization-reference-lifecycle`；后者是R6严格锁序裁定落地后整轮事务整合的新增技术前置。
- 共享Requirement发布基线：无更早change修改同名Requirement。
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- 候选读取/协调漏传ctx造成局部提交 → 真实第二项和第二版本故障检查API/Menu/Permission/RolePermission/全部版本投影。
- 嵌套协调提前缓存清理 → 分离数据库编排与最外层提交后副作用，不用缓存失败解释数据库全回滚。
- 事务较长导致锁竞争 → 合理固定处理顺序并保留现有Runner行为，不新增配置重试。

## Verification

- 全新/已存在/无需认证混合请求仅适用项提交，返回created/created_count/updated_api与实际库对应。
- 第二项Core/权限/关联失败和后续用户版本失败时整个请求回滚，不返回部分成功。
- 在真实存储按既有可重试错误触发重执行，只计最终成功attempt；最终失败时全部本请求变化无残留。
- 原程序HTTP验收保持权限码、公开错误和现有同步用途分离。

只保留真实消费者行为/边界回归；HTTP权限、跨模块事务和并发另以真实存储/原程序验收，SQLite效力不扩大为MySQL锁序或重试证明。

### 实施验收（2026-10-18）

- API Service复用Core必需Runner；候选读取、准备及原序遍历同属整请求事务。App唯一装配有界准备／owner事实端口，删除重复可选Runner注入；attempt局部统计覆盖内部创建与最终清理。
- Navigation按候选／直接依赖和潜在旧码检查资源合并锁定，current read复核，attempt局部允许集只接受已准备目标和自己的码传播；清理继续用owner SQL当前事实，不以Go字符串缓存替代MySQL比较语义。不新增全局API互斥或通用关系框架。
- Authorization owner只预锁版本，不初始化；实际逐API提升保留用户裁定。真实同用户10→12、后次版本约束失败全回滚及旧Access／Refresh拒绝均已观察。
- [去敏证据](../../../../docs/reviews/evidence/api-permission-sync-atomic-2026-10-18.json)包含实施前部分提交和统计遗漏、原程序后项Core／权限／真实授权关联／版本失败、混合统计、原序／锁序分离、并发胜方后的409及最终构建smoke。`menu_apis`／`role_menus`成员表不是此同步的写点，未伪造其写入失败。
- 原MySQL通过SIGNAL 1213后第二次成功、SIGNAL 1205总三次失败，MyISAM夹具仅记录attempt；不声称复现真实死锁环或实际等待超时，也未增加重试次数。全Go及OpenSpec检查通过，自建schema／进程／端口／私有目录已清理。
- 按授权同步已验收R6前置；F2自身delta未同步、未归档。S3/S4/S5、no-op／成员差量及I4仍未据此完成。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
