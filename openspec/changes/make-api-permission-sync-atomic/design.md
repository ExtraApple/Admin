## Context

API Metadata.Service已有transactions字段，经WithRouteSource注入，但SyncPermissions循环各项调用Navigation协调而无整轮Run。Platform.Run会加入ctx已有事务并对外层1205/1213按既有次数重试，返回统计若在callback外累积会不代表成功attempt。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

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

SyncPermissions在现有transactions.Run内读取候选API并遍历，所有Core/Navigation/Authorization调用都使用transactionContext。内层Run仅加入，不独立提交或fallback全局DB。

**理由与替代方案**：复用已装配Runner与协调端口，不增加第二种批处理事务或改Seed边界；缺必需装配应在组合根失败，不静默退回逐项提交。

### 2. attempt局部返回值

created、updated等在每次callback内初始化，只有Run最终成功才向HTTP发布该attempt结果；任何失败返回不带部分完成成功统计。协调中的缓存等外部副作用推迟至真正最外层提交后。

**理由与替代方案**：无需新增重试承诺，但必须正确支持现有Runner重执行；不能因为内层Run返回nil就认为已提交并做不可回滚副作用。

### 3. 维持用途与skip

NeedAuth不适用、已存在权限保持正常skip；权限码解析、正常授权合并沿已有协调路径。此同步不等于Route Catalog同步、API路由同步或菜单同步。

**理由与替代方案**：不统一所有同步策略或借机裁决S3/S5；不把旧D3业务计数疑点描述成已复现缺陷。

### 实施与规格发布依赖

- 功能/写入协议前置：`fix-menu-api-code-consistency`
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

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。当前只创建工件，未执行这些检查。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
