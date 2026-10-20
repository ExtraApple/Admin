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

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。本段保留创建工件时的验收要求；原产品执行见[原验收证据](../../../../docs/reviews/evidence/c6-access-version-auth-fix-2026-10-17.json)，本次补证见下。

### 核验补证验收：W2／W3（2026-10-20，已完成）

[核验报告](../../../../docs/reviews/openspec-implementation-verification-2026-10-20.md#warning3类证据缺口不是产品缺陷)已定位现有登录与Refresh拒绝分支，但继承场景缺少部分直接行为证据。新增tasks第7节只关闭这些证据缺口；不扩大原错误分类修复范围，不修改proposal、delta或主规格，也不将尚未复现的失败认定为产品缺陷。

**W2登录初始化失败**：在合法用户、凭据和验证码前提下，分别令Ensure创建／读取失败、Ensure成功后的Snapshot重读失败，经真实owner和最终HTTP响应证明安全5xx且无Access／Refresh输出，并保留成功初始化／登录对照。版本及凭据前后事实须记录；若Ensure已合法提交版本，后续读取失败不被重新定义为必须回滚，不能为补证新增整轮登录事务、重试或版本修复。原Access读取1146记录不替代登录消费路径。

**W3 Refresh拒绝矩阵**：补畸形、过期、Access用途提交Refresh及用户不存在四类；对后两类建立签名和有效期等其他前提，避免一个更早失败掩盖目标条件。过期使用确定性真实过期Token，不sleep等待。验证通用401／`AUTHN_REFRESH_TOKEN_INVALID`／相同安全英文消息／`data=null`，无新token、无版本补行或其他存储fallback。缺行、失配及禁用已有证据按实际适用范围复用，不把Refresh作为Access的拒绝当反向证据。

永久回归只保留能捕获上述消费者可见错误的结果断言，不测试mock调用次数或源码文本。原程序定向故障与成功对照单独保存去敏证据；发现实际违约须先固定反例，再按既有条款限定根因修复，不自行统一其他错误或引入新认证政策。历史产品任务和JSON保留，以下记录新增补证的实际效力。

- `session_regressions_test.go`通过真实SQLite版本owner／身份／bcrypt／JWT覆盖初始化1、已有版本保持、Ensure创建／读取和成功后Snapshot故障；测试局部授权桥接不冒充App装配证据。最终HTTP／App桥接由[新增原程序证据](../../../../docs/reviews/evidence/authorization-followup-2026-10-20.json)证明。
- W2三次实际Login版本SQL故障均安全500／`IDENTITY_INTERNAL_ERROR`／`data=null`，撤故障后均成功签发有效双Token。Snapshot场景实际观察到Ensure提交1、角色SELECT等待表锁，再改名独占版本表并释放阻挡，最终Current读取1146；失败后1保留、身份凭据不变，不新增登录整事务或重试。
- W3合法Refresh成功对照后，畸形、确定性真实过期、有效Access用途以及用户物理不存在的两个版本状态均为相同通用401／空data，无新Token输出或版本fallback。缺行／失配和停用旧Token的适用历史case逐项引用，保留停用旧Token亦已失配的边界，不新增重复同路径回归。
- 全Go（131.10秒）、定向回归、真实MySQL及OpenSpec严格门禁通过；自建原程序／Redis／MinIO进程、schema、端口及私有配置／token／日志／下载包已清理。第7节5项完成，W2/W3已关闭；无产品代码或规格语义修改，未归档，不扩成整个C6或生产发布已完成。


## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
