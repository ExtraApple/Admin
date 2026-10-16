# 契约可信度调研 · menu-management / Navigation 授权事实写入

> 方法与分类以 [contract-readiness.md](contract-readiness.md):128-183、185-223、238-313 为准。本报告是 C6b 首个授权事实写入切片，不是 branch diff review，不宣布版本存储、运行时授权或完整 C6b 验收完成。

## 一、基线、来源与证据纪律

- 审计基线：`c4b96ce1d64daede23a8d867d0a990e1c5887c04`，Go 1.25.0 / Windows。产品代码、测试、主规格、runbook 未改，未创建 change。静态矩阵后已运行原程序、相关现存测试及隔离 MySQL / Redis / MinIO 的真实 HTTP；执行和局部补证见第五节与 [首轮总报告](contract-readiness-c6b-authorization-writes.md)，不将局部结果扩为完整 C6b。
- Governing：`openspec/specs/menu-management/spec.md:9-156`（7 Requirements / 25 Scenarios）；关联 `api-management/spec.md:9-249`（8 Requirements / 37 Scenarios）。原文快照见附录 A/B，附录收录不表示已验收。
- 追踪：菜单 25/25 场景均列出，14 写入场景纳入、11 读/运行时场景明确暂不覆盖；API 37/37 场景均列出，20 写入或关联元数据部分纳入、17 读/文档/运行时或无本切片写入的场景暂不覆盖。API 的「路径偏离真实路由」「消息与邮箱」部分场景只核对元数据/同步部分，不算整场景验收。7+8 个 Requirement 正文全部有原文和追踪；不将每个 SHALL 计为缺陷。
- 事实 owner：菜单、角色菜单、菜单 API 关联及其联动编排归 **Navigation**；API 记录归 **API Metadata**，在本切片记录实际写入和 Navigation 编排依赖。Permission/RolePermission/AccessVersion 存储能力属于 **Authorization**；本报告只引用依赖，不复制成第二份 Authorization 缺口。Identity 只提供全部用户目录，不把目录当授权版本来源。
- ADR 0003:3 要求同一权限码及同步处理角色授权/会话；ADR 0006:45-63 要求真实组合根、Context 共用最外层事务；0006:71-74 固定兼容边界、Seed 不创建版本及真实存储门禁；ADR 0008:25-44 不把未接线能力当生效能力；ADR 0012:9-11 区分用户定向归属写入和角色配置，不能用全量角色菜单替换模拟逐人归属。`CONTEXT.md:31-49,60-82,94-108` 是术语，不替代主规格；其中刚批准的保护身份/影响集合不能追溯替换旧承诺。
- 旧 `contract-readiness-api-management.md` 的勾选、全量一致结论不是本轮证据。其 header 写 7 Requirements，而当前实际 8；这里只复用路径线索，重新读取了源码/规格/测试/运行记录。
- 主会话已使用 LSP 追踪实际版本存储 `EnsureAndIncrement` 的 13 个引用，包括 Authorization 应用、批量 Increment 与 Organization 个体写入；跨 owner 事务能力仍按实际组合根和运行结果核对。

证据状态：**S**=当前源码静态支持；**T**=现存测试定义，本轮相关包已执行；**H**=实际历史记录，局部范围和兼容性见第四节；**R**=本轮原程序真实运行；**G**=精确待补断言。S/T 不自动等于完整通过，H/R 也不自动升级为整规格通过。

## 二、实际生产入口与写入矩阵

所有下列管理员路由的 Descriptor 均为 `PermissionControlled`（Navigation `routes.go:89-120`、API HTTP `routes.go:94-107`）；是否实际正确判权留给运行时切片，不能把中间件拒绝作为业务规则证明。

| 行 / 不变量 | 实际入口 → Application / Source | 写入、事务与依赖 | 条款、证据与结论 |
| --- | --- | --- | --- |
| W1 菜单创建及默认值 / Navigation | `POST /api/admin/menus` → `routes.go:131-141` → `service.go:33-73`；内部 `CreateMenu` | 名称 Trim；PathExists 在事务外；无路径存 NULL；type 非 1..3 默认 1；status 所有非 1 置 1。事务写 Menu + AllUserIDs + AccessVersions | M2.a/b/c；T1；默认值主体 S，合法 status=0 见 C2 候选；并发冲突/实际 HTTP 错误 G。按钮可无路径并直接保存 code，不自动创建 Permission，M2.b 未要求后者 |
| W2 菜单部分更新 / Navigation | `PUT /api/admin/menus/:id` → `routes.go:143-157` → `service.go:74-137` | ParentID 自己拒绝；字符串用非空决定提交，指针字段可传 0/空权限码；无 updates 拒绝；PathExists 事务外；事务 LockMenu→Update→读取→全部用户版本 | M2.c/d/e；T2；字段应用/拒空 S，显式空 Path/Component/Icon 的语义 A1；普通 code 更新没有走 API 联动，A2；全部用户不是新批准精确集合（N3） |
| W3 叶子删除 / Navigation | `DELETE /api/admin/menus/:id` → `routes.go:159-168` → `service.go:139-159` → `repository.go:102-115` | 同事务 LockMenu→ChildCount→拒绝有子→删 RoleMenu/MenuAPI→Unscoped 硬删→全部用户版本；未调用 cleanupPermission，现行叶子删除未要求删 Permission | M2.f/g、M7.d；T2/T10；清理路径 S；并发子级新增/移动与删除前提稳定性、真实 DB 完整回滚 G |
| W4 角色菜单完整替换 / Navigation | `POST /api/admin/roles/:id/menus` → `routes.go:220-233` → `service.go:161-178` → `repository.go:117-131` | 事务前 RoleExists；事务删旧→uniqueUintIDs（去重且滤 0）→插新；查角色用户→版本。当前 Contract 只有 RoleExists，不能识别 admin 身份；没有显式查每个菜单或锁角色 | M3.a；T3、H2 只含删除角色后提交的拒绝；全量目标/移除对象合法性、保护 admin、提交一致性 N1/N2/G。停用菜单可配置方向与现行读取一致；不是运行时授予 |
| W5 菜单/API 绑定一致性 / Navigation | `POST /api/admin/menus/:id/apis` → `routes.go:189-202` → `service.go:292-364` | 先 uniqueUintIDs（过滤原始 0）且拒空；事务 LockMenu→锁旧关系 API→锁目标 API→数量/启用/NeedAuth 校验→resolve code→before roles→EnsurePermission/merge 菜单旧码→更新目标 API 与本菜单 code→覆盖 MenuAPI→after roles→用户版本→旧权限引用检查；返回 nil 后可选 cache invalidation | M7.a/b/e；T4/T5/T6；C1 原始 0 候选；A2 传播和旧 API code 授权范围；真实关系/权限/版本全成全败 G1；重复提交仍提升版本属 N3 |
| W6 API 常规创建/更新 / API Metadata | `POST /api/admin/apis`、`PUT /api/admin/apis/:id` → HTTP `routes.go:158-184` → Application `service.go:41-55`、`core.go:20-64,115-188` | method/path 规范化、冲突读；模型 unique(method,path)。创建直接 Core；Update 没提交 PermissionCode 直接 Core（非最外层事务）；提交 code 则调用 W7。HTTP 开关只允许 0/1 | A1.c/d/e；T11/T12；数据库唯一约束声明不是无保证，真实 schema 和错误映射 G；无 code 的 status/need_auth 改动不走 Navigation，不预设旧条款要求全用户失效 |
| W7 API 已绑定权限码变更 / Navigation 编排、API Metadata 写 API | HTTP PUT → Application `service.go:45-55` → App coordinator `navigation.go:151-153` → Navigation `service.go:381-443` → callback Core.Update | 同事务先锁 API→找关联菜单并逐个锁→锁同菜单 API；收集旧角色；trim/必要时按旧 API method/path derive；Ensure/merge API 旧权限角色；callback 更新提交字段；更新关联菜单与同菜单 API code；收集新角色→用户版本→cleanup | A1.f/g；T5/T6/T12；已发布的局部同步/merge/cleanup S；真实全部存储回滚 G1；空码与同次改 method/path 的生成取旧值及响应快照是否最终值 A3；多菜单图的传播范围 A2 |
| W8 API 硬删除与清引用 / Navigation 编排、API Metadata 生命周期 | HTTP DELETE → `service.go:58-63` → App `navigation.go:154-156` → Navigation `service.go:445-486` → Core.Delete `190-197` → GORM `repository.go:178-179` | 同事务锁 API→锁关联菜单→读角色→清 MenuAPI→delete callback 硬删→角色用户版本→cleanup；成功后可选 cache invalidation。保留菜单及其 code，引用存在时不删共享 Permission | A1.h；T7（真实 SQLite API+MenuAPI，但授权 fake）；硬删/清引用 S；影响集/真实 DB rollback G1/G2；删除不承诺同时删按钮 |
| W9 从 API 生成按钮 / Navigation | `POST /api/admin/apis/:id/menu-button` → HTTP `208-222` → API Application `73-77` → App `160-162` → Navigation `487-528` | 同事务先 Lock 父菜单（不允许 0 作为此 API 父级）→Lock API→启用/认证检查→沿用/derive code→EnsurePermission→API code→type3/status1/NULL path 菜单→MenuAPI→全部用户版本 | A7.a/b/c/d；T5/T8；创建和公开拒绝 S；失败每阶段完整事实/版本 G1；不存在/停用 API 与并发停用/删除 G2 |
| W10 前端路由同步 / Navigation | `POST /api/admin/menus/sync` → `routes.go:170-187` → `service.go:223-273` | 一个事务，顺序处理 path；存在跳过；parent_path 查询现有记录，不存在即 parentID=0；保存 permission_code/创建计数；即使 created=0 也调用全部用户版本 | M6.a/b；T9；已有父路径解析和跳过 S；父级未出现/后出现语义 A4；无变化版本 N3；事务重试的 created 闭包累计 G3 |
| W11 API 路由同步 / API Metadata | `POST /api/admin/apis/sync` → HTTP `225-238` → `application/sync.go:10-100`；RouteSource 是 App `route_source.go:22-38` | 读 Catalog 快照；一个事务只处理 /api/；Unscoped 查 method/path；恢复 deleted_at/status；Refresh 公开纠正；现有其他展示/策略字段保留；新记录由 Access 推导 NeedAuth/Code/Audit | A4.a/b/c/d、A1.e 同步半项；T13/T14/T15；当前管理员同步 S；启动路径不同（W13/A5），不能继承旧报告“完全同一实现”结论 |
| W12 API 权限同步 / Navigation+Authorization 依赖 | `POST /api/admin/apis/sync-permissions` → HTTP `240-248` → Application `service.go:80-112` → W7 或 App `157-158` → Navigation `service.go:740-748` | 遍历正常 API；NeedAuth!=1 跳过；空 code derive 并经 W7 更新；逐 API EnsureAPIPermission 单独事务。不是整个列表一个事务，现行 A5 没承诺整批回滚 | A5.a；T16；EnsurePermission 的 code 唯一/恢复及 merge 真存储引用见第三节；失败后的之前记录可能已提交不能凭此判旧条款违约；成功全部内容与幂等 G |
| W13 启动/Seed / Navigation & API Metadata，Authorization 初始化依赖 | App `seed.go:22-55,65-142` → Foundation→syncCatalogAPIs→syncCatalogPermissions→Finalize `seeddata/seed.go:28-43` → Navigation `seed.go:24-108` | Foundation / Finalize 各独立事务，API 逐记录同步不是单一全局事务；Permission 单独 Application。Menus Unscoped 恢复、补父/空 code、保留展示；SeedAdminRoleMenus 将所有正常菜单加入 admin 且 OnConflict DoNothing；此 Seed 不调用版本 | A4/关联 RBAC Seed/AccessStorage；T17/T18；初始化例外按发布条款，不套管理员保护/no-op目标；完整启动跨阶段原子性不是现行本报告条款承诺；Authenticated 默认码差异 A5、真实初始化环境 G |

### 主规格全部条款覆盖追踪

附录 A/B 中 Requirement/Scenario 正文逐字保留；下表是其位置到实际审计行的导航。任何“未覆盖”不是“无实现”或“通过”。

#### Menu（M）

| Requirement / 正文位置 | Scenario / 原文位置 | 本轮覆盖 / 理由 |
| --- | --- | --- |
| M1 菜单树查询 :9-10 | 管理员查询菜单 :12-15 | 未覆盖；读取排序/树输出属后续读取切片；Source MenuTree :25-30 / Repository :44-53 仅定位 |
| M2 菜单 CRUD :17-18 | a 创建菜单 :20-23；b 创建按钮菜单 :25-28；c 菜单路径冲突 :30-32；d 父级菜单指向自己 :34-36；e 修改菜单 :38-41；f 删除存在子菜单 :43-45；g 删除叶子菜单 :47-51 | 7/7 纳入 W1/W2/W3；默认状态候选与字段语义分别记录，不把 CRUD 整体称通过 |
| M3 角色菜单分配 :53-54 | a 分配菜单到角色 :56-59 | 纳入 W4 |
| 同上 | b 查询角色菜单 :61-64；c 仅分配子菜单或停用菜单 :66-69 | 未覆盖；配置读取契约，H2 有局部真实配置读取历史，只保留其效力，不算此次写入验收 |
| M4 管理员菜单配置与用户可见菜单分离 :71-72 | 用户上下文过滤停用菜单 :74-77；用户上下文按有效权限过滤 :79-82 | 未覆盖；运行时可见性与管理员读模型分离留待后续；不能用 W4 替换成功推出有效授权正确 |
| M5 用户菜单树 :85-86 | 拥有角色菜单 :88-92；按权限码过滤 :94-97；父级目录保留 :99-102；超级管理员菜单 :104-106；没有分配菜单 :108-110 | 未覆盖全部 5；UserMenus :205-221 只作为隐式 admin 依赖线索；H2 无角色空上下文仅证明当次前提 |
| M6 前端路由同步菜单 :112-113 | a 同步创建新菜单 :115-120；b 同步遇到已存在路径 :122-124 | 2/2 纳入 W10，父级未知/顺序 A4 |
| M7 菜单 API 联动 :126-127 | a 菜单绑定 API :129-137；b 菜单绑定 API 失败 :139-142；d 删除菜单清理 API 关联 :149-151；e 菜单权限变更实时生效 :153-156 | 4/4 写入纳入 W5/W3/W7；e 本轮只到同事务版本写入，提交后旧 token 请求拒绝 G4、后续版本/运行时切片 |
| 同上 | c 查询菜单绑定 API :144-147 | 未覆盖；GET 结果属读取；T9 直接准备关系不是生产绑定证明 |

#### API（A）

| Requirement / 正文位置 | Scenario / 原文位置 | 本轮覆盖 / 理由 |
| --- | --- | --- |
| A1 API 元数据 CRUD :9-10 | a 查询 API 列表 :12-15；b 查询 API 详情 :17-20 | 未覆盖；分页/详情读取 |
| 同上 | c 创建 :22-26；d 修改 :28-31；e 路径偏离真实路由 :33-36；f 修改已绑定菜单权限码 :38-44；g 修改失败 :46-49；h 删除 :51-54 | 6 纳入 W6/W7/W8/W11；e 只审后续能重建元数据，真实路由拒绝属于运行时，T14 部分 HTTP 既有定义不是全部前提验收 |
| A2 API 辅助选项 :56-57 | 分组 :59-62；方法 :64-66 | 未覆盖；只读选项，不产生授权事实 |
| A3 自动化 API 文档 :68-69 | Swagger UI :71-75；OpenAPI JSON :77-91；生成失败 :93-97；关闭文档 :99-101 | 未覆盖全部 4；文档/原生输出。`http_metadata_test.go:51-111` 证明文档 DTO 的测试定义不等于实际按钮写入 |
| A4 API 路由同步 :103-104 | a 同步已声明路由 :106-114；b 公开 Refresh :116-121；c 已存在元数据 :123-126；d 软删除元数据 :128-131 | 4 纳入 W11/W13；b 静态路由不能被 DB 降级部分留待运行时，T15 可供父定点补证 |
| A5 API 权限同步 :133-134 | a 同步 API 权限码 :136-141 | 纳入 W12；仅 API 元数据形成的权限写入，不扩大为全部 RBAC 权限同步 |
| A6 API 动态权限校验 :143-144 | 启用且有权限 :146-150；未配置 :152-155；禁用 :157-159；普通管理员缺权限 :161-165；超级管理员兜底 :167-170；首次同步例外 :172-174 | 未覆盖全部 6；路由/主体决策不是授权事实写入，W11 的调用资格仍需运行时验证 |
| A7 API 生成按钮菜单 :176-177 | a 从 API 生成 :179-187；b 缺少权限码 :189-192；c 事务失败 :194-197；d 公开 API 拒绝 :199-201 | 4 纳入 W9 |
| A8 消息与邮箱接口元数据 :203-204 | a WebSocket 原生协议文档 :206-209 | 未覆盖；协议/响应不是写入 |
| 同上 | b 受保护入口同步 :211-214 | 部分纳入 W11/W13：App RouteSource :22-38 + Messaging HTTP admin_routes :18-43 / routes :34-69 + Identity routes :65-77,110；通用同步只接收 Descriptor，不消费 Gin Engine。稳定默认 Code/Access/Audit 全集合真实数据库断言 G |
| 同上 | c 死信 Outbox 权限 :216-220；d Consumer 死信权限 :222-225 | 各部分纳入 W11/W13：默认 code 来自 Messaging `application/admin_service.go:12-13`、HTTP `admin_routes.go:32-42`；超级管理员限制/只接受 dead 属运行时/消息状态，不在本切片裁决，不从 PermissionControlled 字段推出仅超管 |
| 同上 | e 消息动态授权 :227-231 | 未覆盖；访问权限/组织过滤 |
| 同上 | f WebSocket 成功帧不创建权限 :233-235 | 纳入 W11/W12/W13 输入边界：RouteFact 只有 Method/Path/Access/default code/audit；metadata permission sync 只读 apis，Catalog 输入无消息帧/Exchange/Queue；实际全部生成 code 集合仍 G。不误扩张为所有 Authenticated WS route 都不得有路由权限 |
| 同上 | g 邮箱验证接口契约 :237-241 | 部分纳入 W11/W13：register Public、resend/confirm POST Authenticated Descriptor；API Metadata 纳入与认证字段 G；响应字段、确认错误/不得 GET 改状态是 Identity/响应切片，不替该 owner 新建重复缺口 |
| 同上 | h 邮箱更新错误契约 :243-249 | 未覆盖；OpenAPI Schema/Identity 错误与脱敏，不属于此授权事实写入 |

## 三、问题性质与证据状态（严格分列）

### 现行契约实现不符（本报告 3 项已取得真实入口证据）

| ID / owner | 现行依据、适用输入和具体偏差路径 | 证据状态 / 核实或排除断言 |
| --- | --- | --- |
| C1 原始 API ID 0 被过滤 / Navigation | M7.a :132「系统校验 API 均存在、已启用且需要认证」。HTTP DTO `routes.go:40-43` 只有列表 required，handler :194-198 传原数组；Service :293 先归一化，Repository helper :246-259 丢 0，再以缩减集合校验；即既有 R1.a，不重复立项 | S+R：真实 admin JWT、合法菜单12、启用且需认证API127，POST `[127,0]` 返回200并实际改变Menu/API/MenuAPI/Permission；`[127]`正向控制也200。已确认该输入被忽略而非整次拒绝，不外推全部集合入口 |
| C2 创建合法停用菜单被置为启用 / Navigation | M2.a :21 接受可选状态，:23 仅「未提供或无效」默认；模型0禁用，HTTP传显式0；`service.go:52-55` 将所有非1改1 | S+R：明确status0的POST返回200，响应和库中均status1。合法0与省略字段不能混同；不是仅从int零值推断 |
| C3 普通菜单改码破坏既有直接绑定一致性 / Navigation | M7 Requirement :127、ADR0003:3 要求权限事实一致及改码同步；`UpdateMenu:102-134`只改本Menu和版本，不同步已绑定API | S+R：绑定同code成功后普通PUT新menu code返回200，库中Menu为新code、同一绑定API仍旧code。依据是现行正文，不因细化Scenario只列API改码而缩小承诺；多菜单图／旧API授权迁移另需核实 |

### 契约语义与本轮裁定（1组已裁定，4组仍须固定前提）

- **A1（可选字段清空 / W2，已裁定）**：2026-10-14用户选择合法可选字段显式空值清空、遗漏不变；菜单icon/component等适用，必填名称／code仍不允许清空。DTO非指针string与Service非空更新路径有静态差异，未实际执行全部清空字段，不直接升级为本次已确认旧缺陷。Path等有类型或引用前提的字段仍须先满足合法性；按钮创建允许空路径不自动绕过修改前提。裁定尚未同步主规格或实现。
- **A2（复杂联动作用域 / W5/W7）**：普通PUT造成直接绑定边失配已升为C3，不再全部归歧义。仍需固定共享图／旧授权前提：API X已绑定菜单A，再给B绑定同X且手工新码；W7只走直接菜单及其API；W5 merge菜单旧码而非每个目标API旧码，已有旧API授权角色如何迁移和失效。静态路径已定位，但多菜单完整图、角色／权限及版本实际结果未取得；不假设用户永不这样配置，不据“传播范围待明确”抹去C3已确定的直接不一致。
- **A3（空 API code / W7）**：M7/A1.f 同步承诺下，`ChangeAPIPermissionCode:407-409` 基于锁到的旧 NeedAuth/method/path derive；callback Core.Update 同时可改 method/path/NeedAuth/PermissionCode，并把查回的 API 暂存为响应对象（API Application :49-55）。随后 :423-426 又写 linked APIs；显式空 code 时响应可能仍是 callback 的空码而最终库为 derive code。是否允许请求空码作为自动生成、同时改真实路由后的 code 应取哪个事实，以及返回记录应否等于最终值，需要 governing 前提；G 比较 HTTP响应/最终存储，不假装实际观察过响应错配。
- **A4（未知 parent_path / W10）**：M6.a 要求存在 parent_path 时「根据它解析父菜单」；实现只查当前数据库/此前已处理项目，查不到即根节点。父级后出现与真正未知路径应拒绝、延迟解析还是允许根节点，现行未逐项明确；无菜单完整循环/祖先禁止条款，不凭组织循环规则给菜单新增旧缺陷。0 是合法层级根哨兵，不套 API ID 0 候选。
- **A5（启动与管理员同步 / W11/W13）**：管理员 `routeAccessValues` 仅 PermissionControlled 生成 code，Authenticated 记录 code 空；Seed `app/seed.go:97-104` 对所有非Public都 derive code。当前 `rbac/spec.md:165-180` 明确 Authenticated 在受保护 RouteFact 同步范围，API route-sync 默认码承诺侧重需权限控制；不能把两条不同策略一概写“行为相同”。G 固定 Authenticated（WS/邮箱等）Descriptor，对比启动/API端点/RBAC同步/API权限同步的最终记录，并明确各用途。是否要统一属于规范裁决，不凭重复代码定缺陷。

### 已批准但尚未发布目标（4 组；不追认旧契约违规）

| ID | 目标来源与当前静态差异 | 证据状态 |
| --- | --- | --- |
| N1 完整集合引用/提交一致性 | 审计索引 :208,219,262,274-305：角色菜单须全量合法目标及实际增删旧对象；W4 只 RoleExists 且事务外、Repository 去重/滤0；停用菜单允许预配置。集合存在性/管理语义未写为当前 M3 每项场景；模型/约束也需核对 | S 差异线索；实际 invalid/stale refs、软删除菜单、角色并发删除与FK终态 G。不将 app 未显式检查推成数据库无保证 |
| N2 保护 admin 关联配置 | 索引 :216,218：管理员直接 admin 菜单配置拒绝，合法普通菜单生命周期清引用允许；W4 contract 只有 RoleExists，App :90-96 普通 FindRole，无 role code 返回/检查；SeedAdminRoleMenus 是初始化例外 | 静态方向与新目标不同；现行 rbac “修改/删除admin角色”不能自动证明历史关联配置条款范围已包含本入口。G HTTP授权后 admin role menu拒绝/状态不变，普通叶子删除清 admin 引用成功且版本原子 |
| N3 无变化与精确影响集合 | 索引 :210-212：合法等价集合不提升；覆盖直接/间接依赖并含admin，不失效无关用户。W1/2/3/9/10 调 AllUserIDs，W10 created=0也提升；W4/5/7重提交无语义比较。W5/7/8 affectedRoleIDs只显式RoleMenu与RolePermission，未补隐式admin依赖；新未引用code时admin不一定被该集合纳入。auth现行只是影响登录态变更提升受影响用户，没有明确“仅当/最小集合” | S；全部用户失效不是已确认旧缺陷，遗漏隐式admin是否违反当前「相关用户」范围先明确语义；N3目标 G分别测noop、无关用户、admin依赖、操作者本人间接依赖；不是逐用户最终权限diff目标 |
| N4 并发合法顺序与API提交前提 | 索引 :274-305：删除/停用先完成后新绑定拒绝；绑定先提交随后合法停用可保留配置，删除须清新引用；不用请求重叠推双方不可成功，不新增停用自动解绑。W5/9有API锁/状态检查，W8删除反向锁序 | 已发布 API启用/存在/事务部分 S；新增明确的顺序目标与锁/约束/真实冲突 G。并发结果不预设HTTP409；胜方提交不随失败方回滚 |

### 数据库、Context 与失败原子性证据缺口

1. **G1 真存储跨模块回滚**：Navigation/API/Authorization/AccessVersions均通过`FromContext`，组合根注入同一DB。既有T5/T7仍有fake授权，不能证明全存储。本轮原程序真实HTTP修改已绑定API的新code：只在第二用户、且第一用户版本已经提升时注入CHECK失败，返回500 API_META_INTERNAL_ERROR，Menu/API/MenuAPI/Permission/RolePermission/全部版本六个投影与事务前相同；撤注入后同请求200，同码及grant迁移、两用户版本各+1。此局部缺口已消除；W5/W8/W9各阶段、未初始化版本和全部共享引用清理尚缺，不扩成完整失败原子性通过。
2. **G2 锁与约束**：模型有path unique、API method+path unique、关联pair PK；本轮fresh App迁移的11个相关表均InnoDB，实际FK计数0，版本CHECK>0，隔离REPEATABLE-READ。这是自建当前schema，不是生产DDL，也不是单独缺陷。W5锁Menu→API、W7/W8反向；父/子创建移动删除、角色删除与绑定、API停用先提交后的新绑定仍需可控交错。组织C4已有两实际UPDATE等待的受控反例，不自动外推菜单拓扑。
3. **G3 重试状态**：真实 Runner 已有 MySQL 1205/1213 最多2次重试（:14,30-63），不是本轮新增建议。W10 的 `created` 声明在回调外且每创建++（:224,261），回调重跑未重置。它可能与仅最终提交的计数不同；当前无命中该业务路径的MySQL结果，归提交结果/重试证据缺口并关联M6.a计数承诺，不能把基础Runner注入错误测试当菜单计数通过。W11 created由每次sync返回覆盖，Seed没有套这个重试Runner；不同入口分别判断。
4. **G4 提交后旧token与外部副作用**：W5/W8在Run成功后可选postCommitInvalidate，但组合根没传cache能力，不能声称真实cache/metrics接线。本轮真实菜单创建令admin版本1→2，旧Access下次请求401 AUTHN_TOKEN_INVALID、旧Refresh401 AUTHN_REFRESH_TOKEN_INVALID；这是局部真实JWT证明，不是所有W行通过。Auth缺版本Access500另记总报告C6；其余提交/回读/缓存失败仍须分阶段取证。
5. **G5 输入与错误/fixture限制**：HTTP需要认证且有正确配置写权限，正常API目标启用且NeedAuth=1；Raw0必须混合有效API。HTTP绑定数组的 required是否接受显式[]应以真实Adapter结果为准，空集合不作为解除绑定目标（索引R3 :246）。现有MySQL gate只覆盖版本/基础事务，不覆盖菜单/API全部表或锁序。已知 `internal/organization/user_memberships_mysql_test.go:53` 调Authorization NewService五参而当前六参，使直接 `-tags mysql_integration ./...` 存在入口维护限制；不运行重现、不修复、不把它算Navigation生产缺陷，优先限制到现有独立testsupport门禁。

## 四、测试定义与历史运行记录适用性

### 当前定义（T）：相关包本轮已执行，局部证明范围不变

| 证据 | Case / Source | 实际覆盖与局限 |
| --- | --- | --- |
| T1/T2 | `TestServiceCreatesAndReturnsOrderedMenuTree` :18-47 / `TestServiceUpdatesAndDeletesOnlyLeafMenus` :48-85，Navigation service_test | SQLite Menu+fake授权；默认/重复路径/自身父/展示修改/拒删父/删除普通叶子；没有显式status0、全链条引用/权限/真实版本断言 |
| T3 | `TestServiceAssignsRoleMenusAndPreservesVisibleParentChain` :86-117；`TestRoleMenusReturnsDisabledAndOrphanAssignmentsWithoutChangingUserMenus` :692-709 | 角色存在fake；重复菜单去重及角色用户fake版本；后者读配置/运行时树是后续切片的局部定义；无admin/非法ID/真实version |
| T4/T5/T6 | `TestServiceAssignsAPIsWithOnePermissionAndInvalidatesAffectedUsers` :154-185；`TestServiceRollsBackLinkedWritesWhenAuthorizationVersionFails` :187-238；`TestServiceChangesLinkedAPIPermissionAndMergesRoleGrants` :239-281 | SQLite MenuAPI/Menu事务+内存 API/Auth。T5只断言关系空/旧菜单code/没有新按钮，未断言fake API、Permission、版本恢复；T6角色merge和用户枚举只是fake实现，不是Production MergePermissionRoles运行 |
| T7 | `TestServiceDeleteAPIHardDeletesMetadataAndRollsBackLinkedWrites` :282-401，3 subcases `commits relation cleanup and hard delete` / `rolls back relation cleanup when delete callback fails` / `rolls back hard delete when access version update fails` | SQLite真实API+Navigation；正常与Unscoped计数/关系清理和两失败点；AccessVersion/Permission仍fake，不扩张为完整跨存储回滚 |
| T8/T9/T10 | `TestServiceGeneratesButtonMenuFromAuthenticatedAPI` :449-478；`TestServiceSyncsFrontendRoutesAndListsBoundAPIs` :118-153；`TestRepositoryOwnsMenuAndRelationshipPersistence` repository_test :12-87 | derive按钮/公开API拒绝；菜单同步2条+重复/先父后子；Repo关系CRUD。需要真实HTTP/不存在停用API/raw0/失败阶段/并发补证；T9的菜单API关系直接Repo准备不是生产AssignAPIs |
| T11/T12/T16 | API Application `TestCoreCreatesNormalizedAPIMetadataAndQueriesPolicy` core_test :12-35；`TestCoreListsUpdatesAndDeletesAPIMetadata` :36-77；`TestServiceDelegatesPermissionCodeChangesToNavigation` service_test :56-73；`TestServiceDelegatesAPIDeleteToNavigation` :75-90；`TestServiceSynchronizesPermissionsFromAPIMetadata` :11-43 | API Repo真SQLite、coordinator fake；规范化/唯一冲突/Update/delete、委托与公开跳过，不等于真Navigation联动 |
| T13 | `TestServiceSyncRoutesPreservesExistingMetadataAndReturnsOnlyCreated` sync_test :23-68；`TestServiceSyncRoutesFiltersNonAPIPathsAndRepairsRefresh` :70-110 | SQLite+RouteSource fake；保留/restore/Public/nonAPI/Refresh。不是生产Catalog装配/全部真实route集合 |
| T14/T15 | App `http_metadata_test.go:123-159` `TestApplicationResynchronizesRouteAfterMetadataPathDiverges`；:161-217 `TestApplicationSynchronizersShareRouteCatalogSet`；:219-266 `TestApplicationStaticAccessPrecedesMetadataBootstrap` | 真实App HTTP+SQLite，认证用自定义中间件；前者从直接DB预制偏离，不经HTTP PUT产生偏离；同步集合测试第二端点是 `/permissions/sync`，不是 `/apis/sync-permissions`；不能误认两条同一入口或JWT端到端 |
| T17/T18 | App `TestSeedCatalogUsesValidatedSnapshotAndRemainsIdempotent` seed_test :22-56；`TestSeedCatalogPreservesDescriptorMetadataForNewAPI` :103-134；seeddata `TestRunIsIdempotent` :49-66；`TestRunDoesNotMaintainUserAccessVersions` :68-157；`TestRunRestoresMissingAndSoftDeletedData` :159-190；`TestRunDoesNotOverwriteEditableMenuFields` :192-221 | SQLite Seed/计数/恢复/展示字段/不创建或改哨兵版本；不是实际MySQL启动全阶段故障保证或新admin管理端规则 |
| T19 | Navigation `TestServiceInvalidatesCacheAfterCommitAndExposesRetry` :479-539 | orderingTransactionRunner/fake cache；只证明自定义Runner回调返回后调用，无真实commit/生产cache装配 |
| T20 | `testsupport/testutil/mysql_behavior_gate_test.go:28-45,47-94,96-133` 3 MySQL cases | migration版本schema、并发版本累加、注入1213后基础行只留第3次；不检查menu/API FK、实际死锁交错或业务created重试计数 |

### 真实历史记录（H）与当前兼容范围

- **H1**：`openspec/changes/archive/2026-10-01-standardize-api-response-contract/evidence/backend-acceptance-2026-08-22.txt:5-10` 记录全量34 packages与模块专项通过，以及结构化错误。它没有逐Case/事务状态/raw0/status0/实际MySQL写入记录；当时 frontend/integration 尚未执行（:17）。只保留其后端测试/响应改造当时执行事实，不能据“Navigation included”确认每个写入前提。
- **H2**：同 archive `frontend-integration-acceptance-2026-09-30.txt:10-24,35-42` 记录隔离MySQL/Redis/MinIO、现有AutoMigrate/Seed、Go全量测试；:36-38 有真实角色菜单配置展示、父菜单单选、已删临时角色后提交菜单返回404且未显示保存成功；:30-32 记录无角色空上下文、旧会话强制下线；:40-41 是用户角色/组织版本和隐藏归属，不是菜单/API失败注入。`build-admin-workbench/evidence/acceptance-2026-09-30.txt:3-6` 仅转引H2，不是第二次独立验证。
- **历史适用性核对**：读取相关路径 `git log --since=2026-08-20` 和具体历史变更。`f43f518` 在本报告相关写入路径只改 Navigation `RoleMenus` 的启用过滤/树→配置平铺（`git show f43f518 -- ...`）；`d394528` 在这些路径新增 Repository.RoleMenuAssignments读模型、types字段，并给Seed Authorization NewService补第6个参数，未改既有menu/API写入函数。当前W1..W12写入路径相对这两个提交未出现该历史链条里的行为改写；不是用archive名称断兼容。H2的发布工件提交是f43f518，但记录没固定执行时的commit/hash或逐Case清单，不能据后续无改动补造当时未记录断言；H2局部404/配置读行为与当前相同路径可保留局部效力。版本存储/Identity目录是跨模块依赖，本报告只核对当前实际实现，未将H2“全绿”扩成真跨存储失败证明。
- **可复用结论**：H2保留真实入口局部效力，不覆盖raw0/status0/复杂图/全部失败阶段。C1/C2/C3及G1/G2/G4的本轮补证见第五节；剩余精确断言继续G，不从旧报告“0冲突”排除新问题。

## 五、本轮实际执行与剩余断言

以下专项列表保留为既有Case定位。本轮实际运行的是总报告记录的更宽相关包命令、原程序build和独立testsupport MySQL门禁，均通过；不是逐行重跑以下全部命令。真实HTTP／数据库结果见 [去敏证据](evidence/c6b-authorization-writes-2026-10-14.json)。C1/C2/C3已核实，G1仅API联动第二版本失败、G2仅fresh schema、G4仅菜单创建旧Token结果取得局部证明；下表列剩余边界，不声明缺失场景已覆盖。

```powershell
go test ./internal/navigation -run 'Test(ServiceCreatesAndReturnsOrderedMenuTree|ServiceUpdatesAndDeletesOnlyLeafMenus|ServiceAssignsRoleMenusAndPreservesVisibleParentChain|ServiceSyncsFrontendRoutesAndListsBoundAPIs|ServiceAssignsAPIsWithOnePermissionAndInvalidatesAffectedUsers|ServiceRollsBackLinkedWritesWhenAuthorizationVersionFails|ServiceChangesLinkedAPIPermissionAndMergesRoleGrants|ServiceDeleteAPIHardDeletesMetadataAndRollsBackLinkedWrites|ServiceGeneratesButtonMenuFromAuthenticatedAPI|ServiceInvalidatesCacheAfterCommitAndExposesRetry|RepositoryOwnsMenuAndRelationshipPersistence|RoleMenusReturnsDisabledAndOrphanAssignmentsWithoutChangingUserMenus)$' -count=1
go test ./internal/apimetadata/... -count=1
go test ./internal/app -run 'Test(ApplicationResynchronizesRouteAfterMetadataPathDiverges|ApplicationSynchronizersShareRouteCatalogSet|ApplicationStaticAccessPrecedesMetadataBootstrap|SeedCatalogUsesValidatedSnapshotAndRemainsIdempotent|SeedCatalogPreservesDescriptorMetadataForNewAPI)$' -count=1
go test ./internal/app/seeddata -run 'TestRun(IsIdempotent|DoesNotMaintainUserAccessVersions|RestoresMissingAndSoftDeletedData|DoesNotOverwriteEditableMenuFields)$' -count=1
go test -tags mysql_integration ./testsupport/testutil -run 'TestMySQL(AppMigrationUsesOnlyCurrentAuthorizationSchema|ConcurrentAccessVersionTransactionsSerializeIncrements|TransactionRunnerRetriesDeadlockAndRollsBackAttempt)$' -count=1
```

独立MySQL命令使用自建隔离库和环境注入DSN，未打印密码。未扩大到已知Organization签名不匹配的全仓tag测试；SQLite与fake各自局部范围保留，不替代以下未覆盖部分。

| 优先级 / 断言 | 前提与应记录的事实 |
| --- | --- |
| 已核实 C1-C3 | 真实HTTP／最终库结果已见总报告；保留原始输入与正向控制，不重复提出同项待运行，也不把当前事实写为“已修复” |
| P1 G1 | API联动第二已有版本失败已证明完整六投影回滚；仍需W5/W8/W9各阶段、未初始化版本、共享旧权限各种引用和commit/回读边界 |
| P2 G2/N4 | 固定实际MySQLschema/engine/FK/index/隔离；可控暂停而非随机并发：API删除/停用先提交→新绑定拒绝且无新增残留；绑定先提交→合法删除清新引用并提升实际依赖；Menu父删除和子创建/移动交错；W4角色删除交错；锁失败最终无部分事实/版本。不把两次成功一律判错 |
| P2 N2/N3 | 新目标验证单列：admin菜单直接编辑拒绝、Seed例外；合法noop不失效、重复ID集合等价；精确依赖含隐式admin/操作者本人但排除无关用户。没有已发布判据的目标差异不升级旧缺陷 |
| P2 G3 | 实际W10写入中触发可重试事务失败后最终成功：response.created等于最终实际新菜单数，不含已回滚attempt；最终仅一个事实/版本增量。T20只证明基础Runner重试，不是此业务Case |
| P3 A2/A3/A4/A5 | 先裁定联动/清空/parent_path/Authenticated默认码语义，再按固定图、同次字段更新、输入顺序和启动/端点分别验证。现有HTTP doc schema、默认sync与fake委托不能替代待澄清承诺 |
| 后续 G4 / runtime | 对相关旧Access/Refresh token提交后下一次请求失效（M7.e、auth:188-191）；明确JWT真实流程而非custom认证中间件，cache不是版本存储替代。此项交后续版本/运行时切片，不将本报告写入静态矩阵标完整审完 |

## 附录 A：menu-management 现行 Requirement / Scenario 原文

`openspec/specs/menu-management/spec.md:9-156` 的逐字原文和SHA-256见 [固定条款快照](contract-readiness-c6b-authorization-spec-snapshot.md#menu-management)；第二节追踪全部25场景，收录不代表验收。

## 附录 B：api-management 现行 Requirement / Scenario 原文

`openspec/specs/api-management/spec.md:9-249` 的逐字原文和SHA-256见 [固定条款快照](contract-readiness-c6b-authorization-spec-snapshot.md#api-management)；第二节追踪全部37场景，部分覆盖不代表整场景验收。

