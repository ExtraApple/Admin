## Context

internal/identity/dto.go:104把管理更新email标为email校验；Application UpdateByAdmin拒绝所有非空email。真实C5请求省略邮箱在绑定层失败，带合法邮箱到业务层又被禁写。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- 将邮箱可省略语义贯通HTTP绑定与Application；省略或空邮箱不构成邮箱修改。
- 任何非空邮箱仍由既有业务错误整次拒绝，不放宽管理员代改邮箱。
- 保留操作者范围、本人和管理员目标保护以及本轮之前的版本触发行为；昵称豁免由独立新目标处理。

**Non-Goals:**

不开放邮箱编辑、不重开已完成的邮箱所有权流程，不在此实现纯昵称或同值请求不失效。

## Decisions

### 1. 禁写优先于地址格式

AdminUpdateUserRequest保留string email供旧HTTP形状解析，但移除该管理入口的邮箱地址格式validator；Application既有非空禁写仍是唯一业务判定。

**理由与替代方案**：单用omitempty,email只能修省略值，却仍让非空错误格式绕过既有“所有非空邮箱禁写”字段错误；不新增邮箱写入路径。

### 2. 不改变本批其他行为

HTTP继续把合法字段交给同一UserService，操作者/目标范围与保护检查不变；空email不表示清空；非空email请求的其他字段和版本全不变。

**理由与替代方案**：不顺手修改昵称失效条件或邮箱所有权流程，保持与独立新目标分类及依赖清晰。

### 实施与规格发布依赖

- 功能/写入协议前置：无其他change功能前置。
- 共享Requirement发布基线：无更早change修改同名Requirement。
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- 只测DTO会漏业务互锁 → 使用真App路由、可管理普通用户和独立状态端点正向控制。
- 前置权限拒绝冒充字段保护 → 每个拒绝用例记录实际进入绑定/业务阶段，并读目标资料与版本。

## Verification

- 省略email与空email的合法昵称/状态请求经真实PUT成功，邮箱/待验证/验证状态保持。
- 合法地址和错误格式的非空email字符串均到业务禁写，HTTP 422并含IDENTITY_ADMIN_EMAIL_NOT_WRITABLE，其他字段与版本不变。
- 本人、受保护用户、范围外目标及无可更新字段按原规则拒绝；不把当前昵称版本提升误判为本change回归。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。当前只创建工件，未执行这些检查。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
