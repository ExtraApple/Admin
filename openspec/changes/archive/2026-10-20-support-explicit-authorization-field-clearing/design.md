## Context

Role.Description与Permission.Group及Menu.Path/Component/Icon的HTTP/Application当前多用string且只在非空时更新。现行Repository map写入可保存空值，主要缺口是请求存在性在进入持久化前已丢失。

依据[proposal](proposal.md)及其引用审计/裁决；Context记录实施前问题，Decisions为采用方案，实际实施与验收见下文。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

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

三个PUT继续由请求DTO反射生成Route Catalog schema；为真实空必填拒绝规则投影既有string binding的min/max为minLength/maxLength，并用各操作说明区分遗漏、空字符串与保留的null解析。复用现有schema构造函数，不新增独立手写schema或全平台输入协议。

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

只保留真实消费者字段边界、资源／关系不变及失败回滚回归；SQLite效力不扩大为原生MySQL或生产部署证明。

### 实施验收（2026-10-18）

- 三个PUT和Application更新字段采用局部*string，全部LSP可见caller已迁移，不留旧签名。遗漏／null沿原解析保持，合法空字符串清空；必填空及纯空白拒绝，创建表示与默认不改。
- Role／Permission沿既有owner map更新显式保存空值；Menu空path规范化为NULL且不解引用nil，非空路径继续唯一性检查。清空展示属性不改独立permission_code、直接API码或授权／绑定关系。
- [新增去敏证据](../../../../docs/reviews/evidence/authorization-field-clearing-2026-10-18.json)保存原程序基线、五字段四态、非法整次拒绝、真实独立分组资源保留、Role及绑定Menu后续版本失败全回滚、正常版本与旧会话401；实际`/docs/openapi.json`三个PUT说明及长度／可选schema逐项核对。
- 全Go、纠正后SDK定向门禁及OpenSpec严格检查通过。首次SDK新夹具比较使用隐式status0而存储默认1，已改为显式Status1建立夹具，不修改或重钉产品创建默认；其他已完成原程序场景未重放。
- 自建MySQL schema、Redis／MinIO／原程序进程、端口及私有运行目录已清理。C3/R6前置已验收并同步；S1 delta未自动同步或归档，未抢做no-op及S3。

### 核验补证验收：W1列表排序与分页（2026-10-20，已完成）

[实施核验W1](../../../../docs/reviews/openspec-implementation-verification-2026-10-20.md#w1继承的列表排序分页条件缺少直接断言)发现本delta继承的角色／权限列表排序及分页条件缺少直接结果断言；当前owner查询有相应实现，尚未证实产品缺陷。这是既有场景补证，不修改proposal、delta或主规格语义，不撤销上方历史字段清空验收。

按tasks第7节，以真实存储建立乱序sort、同sort不同ID和跨页数据，经现有公共查询／HTTP入口核对精确ID序列、total和边界，不能以map集合、长度、非空、mock回声或SQL文本证明顺序。永久回归只捕获排序与分页的消费者可见错误；原程序成功响应另外记录真实认证／路由边界，不能复用停用路由403为成功列表证据。若补证暴露实际违约，先保存反例，再按既有条款限定根因修复，不自行增加排序或筛选政策。

角色已分配权限排序由`enforce-protected-authorization-assignments`第7节负责。本节结果供生命周期和配置no-op等同名继承场景复用；共享证据不成为产品功能依赖，不重复维护同路径测试。以下为本次实际补证结果，不能用原验收门禁代替。

- `repository_test.go`的`TestRepositoryListsRolePagesInSortAndIDOrder`、`TestRepositoryListsPermissionPagesInSortAndIDOrder`使用真实持久化，逐页比较精确ID、total、同sort跨页、尾页及空边界。
- [新增原程序证据](../../../../docs/reviews/evidence/authorization-followup-2026-10-20.json)记录7个角色、110个权限的64次实际成功分页GET；三项W1 case合计66次GET。fixture写入完成后，13项受保护表投影及授权版本前后相同。
- 定向／全Go、真实MySQL及OpenSpec严格门禁通过；独占运行资源及私有配置／token／下载包已清理。第7节3项完成，W1中本owner负责的条件已关闭；无产品源码或规格语义变更，未归档。


## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
