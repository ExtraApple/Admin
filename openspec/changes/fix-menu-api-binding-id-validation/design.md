## Context

AssignAPIs在进入事务前调用uniqueUintIDs，而helper显式跳过0。HTTP required列表和内部[]uint调用都能把有效ID与0混合传入；C1已观察成功写入，而非只有静态猜测。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- 对原始api_ids逐项拒绝零ID，再对合法值去重；内部Application调用与HTTP遵守同一规则。
- 保留非空集合及API存在、启用、需要认证的既有前提。
- 无效输入不修改Menu/API/关系/Permission/角色授权/任何用户版本；合法重复ID仍可规范化。

**Non-Goals:**

不修改所有unique ID helper的全局语义，不将根parent_id=0判非法，不借此新增空数组解绑或所有批量入口政策。

## Decisions

### 1. 原始校验再合法去重

在AssignAPIs公开用例边界首先扫描原始apiIDs，遇0直接返回现有NAV校验错误；随后才调用合法去重与非空检查。

**理由与替代方案**：不能只加HTTP validator，因为内部生产调用也跨同一Interface；也不全局修改用于内部查询结果的uniqueUintIDs。

### 2. 保留现有事务与对象前提

原始输入通过后，继续锁菜单/旧新API并验证存在、启用、NeedAuth，复用原绑定事务。失败不做任何关系/权限/版本写入。

**理由与替代方案**：不引入通用集合校验框架，不把父级根节点0或空数组新解绑语义混入。

### 实施与规格发布依赖

- 功能/写入协议前置：无其他change功能前置。
- 共享Requirement发布基线：无更早change修改同名Requirement。
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- DTO提前403/400掩盖真实用例 → 以可管理菜单/合法API并混入0的真HTTP和Application调用验证。
- 去重与过滤非法混淆 → 覆盖重复合法ID、仅0、有效加0、空集合及不存在API；检查具体错误与完整持久化状态。

## Verification

- 有效ID加0返回既有Navigation校验拒绝且Menu/API/MenuAPI/Permission/RolePermission/版本全不变。
- 重复有效ID成功且关系无重复，证明没有禁止合法规范化。
- 不存在、停用、无需认证API与空集合仍按原前提拒绝；内部调用同样拒绝混合0。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。当前只创建工件，未执行这些检查。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
