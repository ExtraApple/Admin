## Context

Role.Description与Permission.Group及Menu.Path/Component/Icon的HTTP/Application当前多用string且只在非空时更新。现行Repository map写入可保存空值，主要缺口是请求存在性在进入持久化前已丢失。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- 角色description、权限group、菜单icon/component及合法path更新使用存在性感知的HTTP与Application输入。
- 遗漏保持原值，显式空字符串清空，非空替换；路径继续满足现有唯一性/类型等适用前提。
- 必填name/code的显式空值拒绝，不把不可清空字段或未提交字段算合法更新。
- 一次请求的属性/关系和版本按其既有事务契约提交；真实清空是配置变化，不提前实现no-op豁免。

**Non-Goals:**

不扩展Identity邮箱/昵称空值政策、API/字典/组织全部可选字段，不裁定已绑定permission_code清空或S3自动码新规则，不新增通用PATCH抽象。

## Decisions

### 1. 局部指针表达存在性

仅对本change触及的更新字段采用*string，HTTP到Application不丢存在性；必填name/code若需区分明确空与遗漏也保留存在性并拒空。创建输入默认不因此改造。

**理由与替代方案**：不引入Optional泛型/全平台PATCH；按LSP引用清洁迁移所有内部输入，不能保留旧签名或兼容别名。JSON空字符串是本次明确清空载体，不新增JSON null政策，保留既有解析语义。

### 2. 字段合法性优先

角色description、权限group、菜单icon/component和合法path分别显式写空；path为空沿现有normalizeMenuPath映射并保持唯一性/类型/父级等已存在前提。独立permission_code继续既有绑定解析，不引入S3新语义。

**理由与替代方案**：不把所有空值都当合法、不清必填标识；Permission.group是标签不是分组资源删除。

### 3. 只改存在性，不抢做no-op

构造显式更新map，遗漏不写、空值确实写入；Role/Menu仍按原/已批准的真实配置事务规则处理版本，Permission标签不凭本目标新增全新版本触发。

**理由与替代方案**：无变化比较由后续配置change消费该表示；真实清空是事实变化，失败仍按既有关系/版本事务回滚。

### 实施与规格发布依赖

- 功能/写入协议前置：`fix-menu-api-code-consistency`、`enforce-authorization-reference-lifecycle`
- 共享Requirement发布基线：`enforce-authorization-reference-lifecycle`
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- 只改DTO不改Application字符串条件仍无法清空 → 每字段从HTTP到真实行逐项观察。
- 空字段触发默认或破坏路径/绑定 → 测省略/空/非空/非法交叉值，不扩展未决code清空政策。

## Verification

- 角色description、Permission.group、菜单icon/component/path各有省略保持、显式空合法清空、非空替换的HTTP/真实存储断言。
- 仅提交合法空可选字段不被当成“无可更新字段”；明确空name/code或路径冲突等非法组合整次拒绝。
- 其他字段、关系及Permission资源不被误删；Role/Menu清空的版本处理失败回滚全部本次变化。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。当前只创建工件，未执行这些检查。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
