## Context

HTTP与CreateInput的int零值丢失存在性；CreateMenu把所有非1变为1；MenuModel.Status又有gorm default:1。仅改Service会被ORM再次用默认覆盖。当前依赖gorm v1.31.1的create callback对struct零值应用默认，而对map保留值，且在模型schema下回填map的自增id。

依据[proposal](proposal.md)及其引用审计/裁决；本文件描述未来实施，不代表已改代码或执行验收。实际路径遵ADR0006的internal业务模块、caller-owned Contract与App唯一组合根，不沿用已登记为旧提示的顶层技术目录。

## Goals / Non-Goals

**Goals:**

- HTTP和CreateInput使用明确的状态存在性表示，合法0与1原样保存，遗漏或现行无效值使用默认启用。
- Adapter的单次INSERT必须真正写入0，不能只修响应或被GORM default重新覆盖。
- 迁移所有CreateInput调用并覆盖创建、Seed／同步／按钮生成的既有合法默认，不新增兼容输入旁路。

**Non-Goals:**

不改变菜单启停产品规则、无变化策略、批量状态更新或数据库历史数据，不以事后第二次UPDATE掩盖错误INSERT。

## Decisions

### 1. 存在性只保留到应用规范化

menuCreateRequest和CreateInput.Status改为*int；nil沿用默认，显式0/1原样，其余值沿现行无效值默认。20个已由LSP定位的CreateInput引用需清洁迁移，包括测试，其他生产构造路径保持明确合法状态。

**理由与替代方案**：不新增StatusProvided和Status双字段，不用int的0同时表示省略与停用，也不改变外部JSON字段形状。

### 2. Adapter单次显式INSERT

保留查询MenuModel及数据库default；CreateMenu的Adapter使用Model(MenuModel)的显式列值map插入，包含已规范化status和既有创建/更新时间。读取该版本GORM回填的id形成返回实体，避免struct默认路径覆盖0。

**理由与替代方案**：已核对本地固定版本callbacks/create.go:248-252,336-347及154-158；不新增持久化指针字段、数据迁移或事后第二次UPDATE。不假设Select字段能绕过默认。

### 3. 返回真实持久化事实

回填ID、状态与时间后才形成MenuDetail；整个创建和原版本处理仍在现有事务。其他Create/Seed/sync/button写入不靠新隐式兼容分支。

**理由与替代方案**：真实MySQL验证单次插入与返回，不以响应字段硬改为0掩盖存储。

### 实施与规格发布依赖

- 功能/写入协议前置：无其他change功能前置。
- 共享Requirement发布基线：无更早change修改同名Requirement。
- 本delta对重复Requirement使用包含前置增量的完整正文，不能乱序归档覆盖。功能依赖与发布依赖不是同一含义；无功能依赖的源码工作可独立，但发布须按此基线。每个前置必须先实施、验证并按流程同步到主规格；若主规格另有变化，合并保留其新增场景，不以旧全文覆盖。

## Risks / Trade-offs

- ORM map创建不会自动走同样的struct时间处理 → 显式维护当前时间列并验证真实行、ID和事务回滚。
- 修改公开输入漏内部调用 → 按LSP引用逐一迁移，不保留别名或旧输入签名。

## Verification

- 真实POST明确status0，返回ID可用于GET且存储status0；随后用户菜单按既有停用规则过滤。
- 省略、显式1与既有无效数值走正确默认；按钮/同步/Seed的默认保持。
- 版本写入中途失败时菜单行及全部本次版本回滚；不以只测DTO或生成SQL文本代替真实存储验证。

只为真实消费者行为/边界保留适用回归；HTTP权限、跨模块事务和并发需要真实存储/原程序场景，Fake或SQLite仅保留自身局部效力。当前只创建工件，未执行这些检查。

## Migration Plan

无新增表、列、数据回填或外部依赖。按既有发布流程部署；回滚程序不回退授权版本、不恢复已失效Token，也不自动重写历史业务数据。

实施前确认上述依赖及既有DDL/装配，不打印凭据。此变更不新增HTTP路径、Redis key、MinIO bucket或权限码；涉及的现有入口和表见proposal。实施后更新对应审计状态与去敏证据，不能把剩余C6风险一并标完成。

## Open Questions

本change范围内无阻塞实施的未决业务选择。未纳入的复杂传播、未知parent_path或其他候选设计保留原状态，不通过本change代为裁决。
