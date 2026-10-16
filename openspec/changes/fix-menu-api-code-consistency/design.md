## Context

UpdateMenu仅写menus及AllUserIDs版本；AssignAPIs已经拥有绑定码解析、EnsurePermission、MergePermissionRoles、API码写入和引用清理。ChangeAPIPermissionCode是API侧已有编排，本change不另定它的S3复杂图政策。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- 菜单侧修改权限码时，在同一最外层事务复用既有绑定码解析、权限确保、授权合并及清理能力。
- 同步现有直接绑定API，保留关系集合；字段、权限授权和用户版本共同提交，失败整体回滚。
- 提取事务内共享编排，避免嵌套用例重复提升版本或单独提交；响应读取最终持久化事实。

**Non-Goals:**

不设计S3复杂多菜单图传播、旧API授权迁移新政策或method/path/NeedAuth与空code组合的新优先级；不做全库历史不一致修复或最小失效集合算法。

## Decisions

### 1. 提取真实共享的事务内绑定编排

从AssignAPIs抽出私有事务内能力，消费已锁定菜单、现有/目标API和请求码，执行resolveBindingCode、权限确保/合并、直接API与菜单码写入及cleanup。AssignAPIs和UpdateMenu两个实际调用者复用，不新增跨模块总协调器。

**理由与替代方案**：避免在菜单PUT里复制半套API侧逻辑；不从一个用例循环调用另一个完整版本提升用例。

### 2. 菜单PUT拥有一次外层事务

锁菜单后读取实际直接绑定API集合并锁定；普通字段与码联动在同一Context事务完成，只读最终菜单形成响应。仅改码保持绑定集合。收集所需失效用户后去重，一次处理，不双重增版本。

**理由与替代方案**：本change不承诺I3最小集合；保留现行安全失效覆盖，不能为追求精度漏掉已有依赖。

### 3. 空码沿既有绑定解析而非新清空政策

有绑定时显式空码通过原resolveBindingCode消费当前API码/既有自动派生与冲突校验；无绑定保留原普通更新语义。

**理由与替代方案**：不新增空码解绑、撤权、图扩散或method/path/NeedAuth同时变化的优先级；已有复杂多菜单行为不被本单跳反例宣称全量验收。

### 实施与规格发布依赖

- 功能/写入协议前置：`fix-menu-api-binding-id-validation`
- 共享Requirement发布基线：`fix-menu-api-binding-id-validation`
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- 只同步字符串会漏角色grant或旧权限清理 → 复用完整现有绑定步骤，核对权限ID与RolePermission合并去重。
- 私有编排仍调用版本导致同请求重复提升 → 将事实编排与最终版本处理分开，在最外层仅一次调用。
- 复杂图历史状态超出反例 → 不批量修历史，保留S3为独立候选，不用特殊输入拒绝来掩盖已支持直接绑定。

## Verification

- 复用C3一致初始菜单/API，普通PUT非空新码后直接绑定全同码，权限/授权合并正确，关系ID集合不变。
- 新码已有权限时角色授权合并去重，旧权限清理遵原引用检查，不删除仍被使用对象。
- 在API/Permission/RolePermission及第二用户版本阶段故障，真实库全部投影回滚；成功后旧Access/Refresh按既有规则失效。
- 未绑定菜单、无permission_code字段与既有空码解析成功/失败均保持适用行为。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。当前只创建工件，未执行这些检查。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
