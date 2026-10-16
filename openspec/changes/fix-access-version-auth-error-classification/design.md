## Context

AccessVersions.Current已有ErrAccessVersionNotFound，Authorization.Error保留Unwrap因果；Identity Authenticate对Snapshot任何错误都Internal。App identityAuthorization正是调用方Contract桥接点，可分类而不让Identity导入GORM Adapter。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- 通过现有错误cause链保留缺版本的类型分类，在App桥接为Identity可消费的不可验证会话原因。
- Authenticate只将该原因映射为HTTP 401及AUTHN_TOKEN_INVALID；真实数据库故障仍安全5xx。
- 不初始化缺行、不回退用户旧字段、不签发Token；保持Refresh既有稳定拒绝行为。

**Non-Goals:**

不将所有AUTHZ内部错误转成401，不吞掉DB故障，不新增版本懒修复、其他存储fallback或认证绕过。

## Decisions

### 1. 桥接只翻译已确认缺行分类

在Identity调用方Contract定义不可验证授权版本原因，App桥接通过errors.Is识别已有缺行cause并转换；其余错误原样保留因果。

**理由与替代方案**：不比较中文错误字符串，不将所有AUTHZ_INTERNAL_ERROR视为缺行，也不要求Identity Application导入Authorization GORM。

### 2. Authenticate保留两种拒绝

仅不可验证会话原因映射既有CodeTokenInvalid/401/AUTHN_TOKEN_INVALID，真实DB/Redis或其他Snapshot故障仍Internal安全5xx。

**理由与替代方案**：不能一律降为401来隐藏基础设施失败；也不把Storage.Current的拒绝改成Ensure。

### 3. 其他认证契约不回退

Refresh继续原AUTHN_REFRESH_TOKEN_INVALID，登录才可Ensure；不读取users.token_version或跳过比较，不发Token。

**理由与替代方案**：最小修改错误边界，不重构全部认证错误或公开新底层信息。

### 实施与规格发布依赖

- 功能/写入协议前置：无其他change功能前置。
- 共享Requirement发布基线：无更早change修改同名Requirement。
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- 包之间错误身份丢失 → 覆盖真实Adapter→Authorization包装→App→Identity链，不只测一个mock返回。
- 全DB断开无法证明缺行分类 → 分开注入真实版本表缺行和版本读取SQL故障，先证明JWT/用户/权限前提有效。

## Verification

- 有效Access先成功，仅删除自建版本行后请求返回401/AUTHN_TOKEN_INVALID且表仍缺行。
- 同一用户Refresh保持401及既有码且不签发Token。
- 只令版本读取发生实际SQL/连接故障时保留安全5xx，不泄漏DSN/SQL；普通失配/用途/禁用等稳定错误不回归。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。当前只创建工件，未执行这些检查。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
