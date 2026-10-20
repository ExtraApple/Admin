# C6b 首轮实际审计 · 授权事实写入

## 一、范围、基线与停止点

2026-10-14；代码 `c4b96ce1d64daede23a8d867d0a990e1c5887c04`，Go 1.25.0 windows/amd64。主规格／源码未改；此前 CONTEXT／审计索引中的讨论目标不是已发布行为。当前轮最终停在 **创建 change 之前**：不生成 proposal、design、tasks、delta spec，不实施产品修复。

这是已确认顺序中的第一个实际审计切片：Authorization、Navigation、Organization、版本存储的事实写入，以及触及的 Identity 用户生命周期和 Token 版本边界。不是整个 C6b／整份 Auth／API 管理或生产部署验收。资源生命周期、Catalog／装配／统一响应和完整运行时决策仍按索引后续切片核对，不因本报告通过局部场景而改成完成。

**后续执行以[C6当前计划](contract-readiness.md#c6-当前执行计划)为准**。本报告保留首轮发现、证据和讨论来源；第三节新目标与第八节根因分组不是C6审计结束前必须整体实施的清单。本文审计执行时停在创建change之前；此后用户另行授权生成独立工件并开始实施，进度见索引D节，不改变本报告的历史反例或分类。

**2026-10-15候选补证**：ID-SEED-COUNT已通过[原程序与真实MySQL单条故障](contract-readiness-id-seed-count.md)确认既有管理员密码被覆盖，当时尚未修复；2026-10-16的后续定向修复验收见该报告的实施状态及新增证据。第二节六项保留为2026-10-14首轮结果，不重编号；原日期JSON及条款快照不改写。下文对应候选／根因状态据此更新，不表示启动其余审计切片。

**2026-10-16 C5后续修复**：管理端PUT已打通省略／空邮箱的合法更新，任意非空字符串仍由业务邮箱禁写错误整次拒绝。相关包回归及原程序真实HTTP的成功、拒绝、保护／范围与版本状态已记录于[新增验收证据](evidence/c5-admin-user-update-fix-2026-10-16.json)。纯昵称豁免仍未实施；下表C5保留首轮失败事实，不把当时的审计结果改写成修复后通过。

**2026-10-16 C1后续修复**：菜单API绑定在去重前拒绝原始0；原程序对混合0、空集合、不存在／停用／无需认证API的拒绝保持六个相关表完整投影不变，重复合法ID仍成功且无重复关联。直接用例回归及真实HTTP证据见[新增记录](evidence/c1-menu-api-ids-fix-2026-10-16.json)。不扩成所有集合政策或完整联动验收，首轮表与历史JSON不改写。

**2026-10-16 C2后续修复**：创建输入保留状态存在性，显式0经单次显式INSERT实际保存为停用；返回ID、普通用户过滤、其他默认路径及首个版本已更新后第二个版本失败的整事务回滚均有[新增验收记录](evidence/c2-menu-create-status-fix-2026-10-16.json)。撤除隔离约束后同请求成功，保留原DDL默认；不把新目标或其余审计标完成。

**2026-10-16 C3后续修复**：普通菜单PUT已复用事务内直接绑定编排，两个绑定API与菜单最终同码、绑定集合不变、角色授权合并去重及旧权限引用检查通过；版本单次提升与真实未过期旧Access／Refresh拒绝已观察。API／Permission／RolePermission／第二用户版本四阶段真实故障均整体回滚，见[新增验收记录](evidence/c3-menu-api-code-fix-2026-10-16.json)。执行器中断及恢复证据边界单列，不冒充产品故障，也不重复已完成的故障检查；S3复杂图仍未纳入。

**2026-10-17 C4后续修复**：创建、带父级修改及删除已统一使用事务内升序锁定的当前父图；双根互移、三节点长环、深层跨支及创建／移动／删除并发的受控原程序HTTP检查通过，拒绝移动方完整行无残留。真实移动存储失败及成员版本失败后的删除整体回滚、定位祖先拒绝与可管理组织合法移根均有[新增验收记录](evidence/c4-organization-moves-fix-2026-10-17.json)。执行器丢失状态后核验并接管仍存活的原进程，未重启或重跑已完成场景；并发删除获胜的调度顺序未覆盖，不冒充全顺序或I4验收。部署需全实例停结构写入切换，首轮表及历史JSON保留原样。

**2026-10-17 C6后续修复**：App只翻译独立版本缺行的既有cause，Identity认证按调用方分类返回401 `AUTHN_TOKEN_INVALID`，其他Snapshot故障仍安全500。原程序已证明有效Access先200、仅删除版本行后Access／Refresh各自稳定401且不补行；只让版本表SELECT实际报MySQL 1146时仍500且不泄漏底层信息，恢复后同一未过期Token可用。失配、用途、禁用／软删边界及正常Refresh也有[新增记录](evidence/c6-access-version-auth-fix-2026-10-17.json)。此处C6是首轮缺陷编号，不表示整个C6审计或未发布目标完成。

**2026-10-17 关联保护新目标实施**：P2-P6／R1-R3已在三个事实owner按入口落地，并非追认旧差异为缺陷。原始目标／实际移除对象的整次拒绝、停用admin在Identity／Authorization／Organization持续受保护、保留本人／保护归属维护普通成员、停用普通对象预配置与运行时隔离、共享配置间接失效操作者均有[新增证据](evidence/protected-authorization-assignments-2026-10-17.json)。全Go门禁及真实MySQL CAS／当前父图门禁通过；4处新夹具的停用状态已显式建立，不改产品创建默认。引用生命周期、差量／no-op、I4及整轮审计仍未完成。

**2026-10-18 引用生命周期新目标实施**：R4-R6已完成18项任务及[定向验收](evidence/authorization-reference-lifecycle-2026-10-18.json)：四类角色关系／版本共同回滚、custom与删除双向顺序、两种组织归属入口、软删普通旧成员清理／恢复、真实注册API停用访问及动态API严格锁序／依赖冲突均有原程序证据。缺失父级误返500的新回归已修正为既有404并保留前后记录；一个复合关联键探针断言失败单列，已完成请求未重放。全Go、MySQL门禁和OpenSpec严格检查通过，自建运行资源已清理；不代表S3、F2、差量／no-op、I4或生产切换完成，历史JSON与首轮分类不改写。

**2026-10-18 F2新目标实施**：整请求同步已复用必需Runner、整轮严格目标准备及原业务遍历；保留用户裁定的逐API版本提升。后项Core／权限／真实角色授权／版本失败均无请求残留，混合统计包含内部创建且排除最终清理项，真实MySQL既有重试类不重复计数；原程序还证明低ID先锁而业务高ID先处理、候选变化时409保留胜方及401／403访问边界。[新增证据](evidence/api-permission-sync-atomic-2026-10-18.json)区分原缺口、SQLite局部回归、原程序、SIGNAL夹具与物理死锁／超时未覆盖；全Go／OpenSpec门禁通过，自建资源已清理。F2 delta未同步、未归档；不改历史JSON、S3/S4/S5、差量／no-op与I4状态。

**2026-10-18 S1新目标实施**：Role说明、Permission分组标签和Menu图标／组件／合法路径已区分遗漏、合法空字符串、替换，保留JSON null原解析；必填name及角色code的显式空／纯空白整次拒绝。真实HTTP／MySQL逐字段四态、路径冲突、独立分组资源与绑定保留、Role／Menu后次用户版本失败全回滚及原程序OpenAPI有[新增验收记录](evidence/authorization-field-clearing-2026-10-18.json)。全Go／SDK／OpenSpec检查通过，自建资源已清理；一处新SDK夹具状态默认比较错误已纠正为显式建立状态，不改产品创建策略。S1 delta未同步、未归档，不改变S3或抢做no-op，历史证据保留。

**2026-10-18 S2/N4/Identity N6新目标实施**：纯昵称及合法全同值更新不创建／提升授权版本；旧有效Access／Refresh真实调用继续200，未登录缺行用户仍无版本。管理／邮箱前置保留，真实旧role或状态变化只增版一次，旧role标签不是RBAC身份，Kick与Toggle不豁免。原MySQL当前行交错、资料／版本失败回滚及已提交后Get失败分列于[新增证据](evidence/nickname-sessions-2026-10-18.json)；回读故障的既有401不代表昵称回滚，禁用Access403与Refresh401也未统一改写。夹具错误预期已据实纠正，已完成请求不重放。全Go／定向／OpenSpec门禁通过，自建资源已清理；C5按需同步，S2 delta未同步、未归档，配置no-op／差量／I4继续待实施。

**2026-10-18 N1配置／N3／N6配置新目标实施**：Role／Menu字段、权限／菜单／custom／API配置的合法同值和全skip同步不增版本；旧普通／admin有效会话继续200，缺行用户不补版本，真实展示变化保留当前依赖触发。原始ID／保护／状态／权限前提仍先校验，旧custom清理、缺失／恢复权限、直接码修复及真实授权迁移仍处理版本；三类第二用户版本失败全13投影回滚。[新增92条观察证据](evidence/configuration-noop-2026-10-18.json)包括禁止写入触发器下七类合法no-op、真实Runner计数重试、当前角色行胜方交错及原程序OpenAPI。执行器中断按持久证据恢复，已完成请求未重放；MySQL夹具CHECK限制修正单列。全Go门禁通过、自建运行资源已清理；S1主规格已按依赖同步，配置no-op delta仍待独立同步、未归档，不关闭成员差量、I4、S3/S4/S5或整个C6。

**2026-10-18 I1／I5／N1成员／N2／R6.e5新目标实施**：角色／组织完整目标仅维护差量、只升对称差用户版本，保留成员与旁观数据读取者旧会话；逐人先Ensure／CAS再no-op，初始缺行合法无变化保留1、拒绝初始化回滚，隐藏关系及保留时间不重建。[新增72条观察](evidence/membership-delta-2026-10-18.json)含十对原程序HTTP交错（缺行／两次no-op／先no-op后真改／真改后旧版本相同目标／两个真改／跨维度双向），逐项记录真实层级／身份／版本等待点、返回码及胜方事实。第二实际变更用户失败全13投影回滚，整组／逐人真实Runner重试、保护与范围差异、HTTP软删及受控清理也已验证。全Go与隔离MySQL门禁通过，四个旧全删持久化接口干净切换，物理时间列映射夹具修正单列，自建运行资源已清理。delta未同步、未归档；不改历史JSON、不把I4或整个C6标完成。

**2026-10-18 I4有限层级依赖新目标实施**：同一C4锁定父图内比较新旧祖先链，只处理后代边界实际变化的范围根；共同祖先、移动子树自身根及固定范围排除，另一all／admin角色、重复角色或多个根抵消最终覆盖不豁免独立依赖。[新增90条观察](evidence/organization-hierarchy-dependents-2026-10-18.json)包括原程序前后行为、十九组事前未过期Access／Refresh对照、缺版本行、第二项版本失败全13投影回滚及真实MySQL1213两attempt仅一次提交；十二对有效HTTP交错覆盖成员、范围、角色启用／删除、父组织删除与双向C4成环竞争，并记录实际当前锁等待和精确版本总增量。全Go、真实MySQL当前父图门禁通过，自建服务／schema／私有目录及本批工具缓存已清理；无DDL、新路由或权限码。C4／R6前置已同步，I4 delta未同步、未归档；本批15项完成不代表整个C6或生产切换完成。

**I4验收中的异常观察与夹具边界**：刻意保留的软删除角色孤儿引用令既有组织成员前置报500，13投影全回滚；只移除该fixture引用后，同一正常成员请求仅使实际新增者失效。此项保留为历史异常关系适用承诺／错误分类待核实的观察，不直接判定为发布缺陷，不在本change修改产品策略。数字code与error_code探针错误、删除父级后fixture未重建导致的正确404/404均单列；完成请求未误重放，额外无效pair不计入十二对有效交错。历史JSON与首轮条款快照不改写。

**2026-10-20实施核验W1-W3补证完成**：S1／关联保护／C6既有change新增10项证据任务已完成，原195项及历史JSON保留，当前205/205。新增真实持久化回归及[原程序15组去敏case](evidence/authorization-followup-2026-10-20.json)覆盖角色／权限精确分页、已分配权限排序及空数组、Login Ensure创建／读取和成功提交后Snapshot重读失败、畸形／真实过期／Access用途／用户不存在Refresh；各自记录安全响应、真实JWT前提、版本及受保护投影，Snapshot失败保留已合法初始化的1。W1-W3在[核验报告](openspec-implementation-verification-2026-10-20.md)已关闭；全Go、真实MySQL及OpenSpec严格门禁通过，自建进程／schema／端口／私有目录及下载包已清理。仅补现有条款证据，未改产品代码或规格语义，不改I4异常观察、S3/S4/S5、生产发布或整个C6结论；未自动同步、归档、commit、push。


| 现行规格 | Requirement / Scenario 总量 | 本轮追踪范围 |
| --- | --- | --- |
| rbac | 11 / 32 | [RBAC 矩阵](contract-readiness-rbac.md)：20 写入／初始化；12读／运行时明确未覆盖 |
| menu-management | 7 / 25 | [Navigation 矩阵](contract-readiness-menu-management.md)：14写入；11读／运行时未覆盖 |
| organization-management | 8 / 24 | 下文 O1-O8 覆盖全部条款位置；写入及定位祖先拒绝纳入，列表／树／运行时范围明确未验收 |
| access-version-storage | 5 / 12 | 下文 V1-V5逐场景，静态／本轮运行／历史／精确缺证分列 |
| api-management | 8 / 37 | Navigation关联矩阵20个写入或部分元数据场景；17其他场景未覆盖，部分场景不算整场景完成 |
| user-management / auth | 16 / 55；9 / 31 | 仅触及逐人角色、管理员修改／删除／状态／Kick及实时版本；其他身份、头像、登录风控、登出TTL等不全量重审 |

所有 Requirement 正文和 Scenario 原文逐字收录于 [固定现行条款快照](contract-readiness-c6b-authorization-spec-snapshot.md)，包含原文件 SHA-256。收录不表示验收。问题性质与证据状态是两个维度，未执行或历史不适用不是自动缺陷；确认结果也仅覆盖记录的具体输入。

## 二、六项现行承诺下已核实的不符

运行使用未修改的原程序 `main.go`，独立配置／schema `admin_c6b_audit_0eb291a5cba6`，真实 MySQL 8.0.19 / InnoDB / REPEATABLE-READ，Redis 8.0.5和独立MinIO临时目录，真实验证码／登录／JWT／管理权限。共享 `admin` schema、原 MinIO 数据、用户工作树资产未作迁移／Seed／验收写入。RabbitMQ未配置、SMTP关闭，不宣称验证了这些异步能力。

去敏原始状态、响应和门禁输出见 [运行证据 JSON](evidence/c6b-authorization-writes-2026-10-14.json)。没有密码、JWT、captcha答案或凭据DSN。

| ID / owner | 明确现行依据及实际偏差路径 | 本轮实际证据／限界 |
| --- | --- | --- |
| C1 原始API ID 0被过滤 / Navigation | menu:132要求API均存在；HTTP `routes.go:40-43,194-198` 传原始数组，`service.go:293` 归一化调用Repo helper `246-259` 丢0后才校验。即既有R1.a，同项不重复立项 | 合法菜单12、启用且需认证API127，POST `[127,0]` 返回200，并实际改变 Menu、API、MenuAPI、Permission；随后`[127]`同权限正向控制也200。不是前置403或只含0的空数组。已核实该输入放行，不外推全部集合入口 |
| C2 显式合法status0被启用 / Navigation | menu:20-23只在省略／无效状态使用默认；模型定义0停用；HTTP接受int，Service `52-55` 将所有非1改1 | POST明确status0返回200，响应和库中均1；不是省略字段被零值表示的推断。DTO、Service与持久化默认的协作需按根因处理，不先指定实现修复 |
| C3 普通菜单改码破坏现有绑定一致性 / Navigation | menu Requirement:127保证菜单／API／角色／会话一致；ADR0003:3要求改码同步；`UpdateMenu:102-134` 只更新本Menu及版本，没有走既有绑定API／Permission编排 | 同一菜单/API先成功绑定code `audit.c6b.bound`，普通PUT菜单code返回200，库中Menu=`audit.c6b.edited`而绑定API仍`audit.c6b.bound`。依据是已发布Requirement正文，不因为Scenario标题只列API改码就缩窄正文。这里只确认直接绑定边不一致；跨多菜单传播／旧API授权迁移另列待核实 |
| C4 并发组织父级修改形成环 / Organization | organization:99-104明确防止父子循环；`UpdateUnit:127-150` 在无最外层事务保护的读取中校验后代，:175再独立Update | 初始组织1、2均根节点。独立事务先锁两行，两个真实HTTP更新均通过前置读取并在生产UPDATE上等待，performance_schema确认2个等待者后统一释放；两请求都200，最终1→2、2→1。不是随机压测、串行成功或仅推测缺锁。证据保存后已恢复仅自建夹具拓扑 |
| C5 管理员用户PUT的DTO与禁代改邮箱互锁 / Identity | user-management:281-286允许修改昵称／角色／状态，:292-296要求拒绝非空email；DTO `dto.go:104` 却只有`binding:"email"`、无omitempty；HTTP `routes.go:270-275`先绑定，UseCase `user_service.go:195-197`拒非空email | 同一可管理普通用户，昵称请求不带email返回400 HTTP_REQUEST_INVALID；带合法当前email进入业务层后422 IDENTITY_VALIDATION_INVALID，fields含IDENTITY_ADMIN_EMAIL_NOT_WRITABLE，用户／版本不变；独立状态切换端点正向控制200，排除用户／操作者权限前提错误。这是既有已实施邮箱禁止条款暴露的新HTTP组合缺口，不重复重开旧五项，也不混为N4昵称失效问题 |
| C6 Access校验缺版本行返回500而非401 / Identity认证错误边界 | auth:182-186明确非登录缺版本时401稳定认证错误；Storage:33-36拒绝且无fallback。Current返回缺行错误→Authorization Snapshot `475-489` wrap→Identity Authenticate `245-248`统一Internal→HTTP middleware `36-37`500 | 新签发有效JWT，先GET user/info200；仅在自建库删除用户3的独立版本行后，Access请求500 IDENTITY_INTERNAL_ERROR，Refresh401 AUTHN_REFRESH_TOKEN_INVALID，非登录请求未补行。Storage拒绝／不fallback部分成立，但Auth状态码不符；不把整个DB宕机误当缺行。观察后恢复原版本／时间字段 |

上表保留原程序在2026-10-14适用输入下已观察的不符，不表示当时已经修复或自动创建了change。C1、C2此前只有静态候选，首轮取得真实入口和持久化结果才升级。后续另行授权的修复进度与新增证据见本文顶部日期说明及索引D节，历史JSON不追写为修复后结果。

## 三、候选、规范变更、语义与证据缺口分列

### A. 首轮具体候选的后续核实

**ID-SEED-COUNT / Identity（2026-10-15已核实，2026-10-16已完成定向修复）**：以下保留缺陷核实时的事实：rbac:90-93保证已有active admin不重置密码。`seed.go:73-76`丢Count.Error返回false；当时只中断该Count，原程序实际收到MySQL 1317，count=0且原tx.Error=nil。释放外部锁后，配置用户名／绑定读取完成，:43-49进入:65-70恢复密码，外层事务提交并正常提供HTTP：原密码401、配置密码200，身份／绑定及版本7不变。对照正常重启保留原密码。见[定向报告及分日期证据](contract-readiness-id-seed-count.md)；不再列为缺少这一故障证明的候选。历史审计未向`hasUserRoleInTx:79-82`注入错误；后续实现用独立SQLite回归覆盖其错误传播，不扩为第二项历史已确认缺陷或第二次原生MySQL故障。

菜单／RBAC同步回调外created累积的重试结果按精确证据缺口登记（D3），未因源码变量位置就确认响应计数错误。

### B. 已批准、尚未发布的规范目标

| 根因轴 | 当前实现与新目标的差异 | 分类纪律 |
| --- | --- | --- |
| 原始集合与保护 / R1-R3 | 部分批量入口只查角色／归一化后集合，缺全量目标与实际删除对象合法性；admin关系编辑未统一保护；单用户角色／组织已有较完整校验 | 新目标入口分别落条款；不能将C1扩成所有过滤0都是同一已确认缺陷。根组织parent_id=0仍合法 |
| noop与精确影响 / N、I | 角色／组织成员用旧新并集；Menu创建／更新／删除／同步用全部用户；等价集合仍提升；隐式admin不总在显式关联集合中 | 当前“新旧用户”承诺与仅实际新增／移除用户不同。保留成员B失效、无关用户失效、noop提升不追认旧条款违规；不以最终有效权限差分替代事实依赖 |
| 生命周期与提交顺序 / R6 | role删除未清Navigation role_menus；组织删除未处理custom scope引用；预读与提交对象合法性需要持久化保证；停用API允许保留既有绑定、后续新绑定须检查提交前提 | 已批准拒绝残留custom引用、软删关系受控清理、停用普通对象预配置等按新目标；C4当前明确“不成环”仍是独立现行不符，不全塞入未来并发目标 |
| 整请求原子性 / F2-F3 | /apis/sync-permissions逐API事务；Catalog权限同步／菜单同步已有整轮事务；Seed保留阶段提交、错误向上返回 | API权限同步整请求全成全败是新目标。Seed各阶段提交不叫当前违规；失败终止启动／重跑幂等仍需按现行承诺取证 |
| 可选字段与纯资料 / 2026-10-14裁定 | 用户确认合法可选字段显式空值清空、遗漏不变；管理员纯昵称变更不失效。当前多处忽略空字符串，Identity UseCase统一提升版本且真实HTTP被C5阻断 | 已裁定、未发布的条款明确化／行为目标；未修改主规格，未因这次新裁定增加已确认旧缺陷数量 |

### C. 具体业务语义与本轮裁定

- **S1 已裁定：合法可选字段显式空值清空**。用户选择：遗漏字段保持原值；角色description、权限group、菜单icon/component等合法可选字段明确提交空值则清空，必填名称／code仍拒绝清空。当前非空更新路径与该目标有静态差异，未运行全部字段清空请求，不自动计为新确认旧缺陷。路径等有交叉约束的字段仍须先满足其合法性，不绕过引用／类型前提。
- **S2 已裁定：纯昵称不失效（既有N4）**。用户选择：管理员纯昵称变更不提升授权版本；角色、状态与显式Kick各按相应失效条款执行。此裁定补清未来条款范围，不追认原“当角色或状态”必然等于“仅当”。UseCase无条件提升仍是源码事实；本轮纯昵称HTTP被C5的DTO阻断返回400，未实际执行该业务路径，不能称新昵称策略已实现／已验证。
- **S3 复杂权限联动／空API码**：C3直接绑定边已确定不符，不再全部归歧义；多菜单图、绑定API原有授权的迁移范围、同请求改method/path/NeedAuth并清code时应使用哪个最终事实及响应快照另需固定前提。
- **S4 parent_path解析**：菜单同步父级尚未出现与真正未知路径如何处理（延迟解析／拒绝／根节点），现行未逐项明确；不能把组织循环要求移植成菜单旧条款。
- **S5 同步用途**：RBAC受保护RouteFact、API仅PermissionControlled的默认code、启动Seed处理Authenticated的策略不同。先按各现行用途核对，不因实现不同就要求统一；尚无批准的全局统一目标。

这些不阻塞已确认六项的分类；未裁定部分不生成修复设计或change。

### D. 精确证据缺口（含已经局部消除的范围）

| 缺口 | 本轮已取得 | 尚不能推出 |
| --- | --- | --- |
| D1 真实跨模块回滚 | 真App HTTP PUT已绑定API新code；两个已有版本用户；只在第二用户且第一用户版本已提升时强制版本CHECK失败；500 API_META_INTERNAL_ERROR；Menu/API/MenuAPI/Permission/RolePermission/全部版本6个投影均与事务前相同；撤注入后同请求200，同码／角色grant迁移／两用户version各+1 | 不是只有wrapper或fake版本。仍未涵盖菜单绑定、API删除、按钮生成每个失败阶段、缺版本行、commit失败、提交后回读或缓存失败 |
| D2 schema与并发 | fresh App AutoMigrate后11个相关表InnoDB、全库FK=0、版本CHECK>0；C4两个实际UPDATE的受控等待；真实角色／组织并发dispatch从version7返回200／409、version8 | 只代表自建当前schema，不代表生产DDL；普通CAS运行没有版本读取内部暂停点，不替代RR快照覆盖、双维度已有／未有版本与各生命周期交错 |
| D3 重试可观察结果 | 现存Runner真实MySQL1205/1213最多2次重试；独立门禁注入1213验证只提交第3attempt；本轮8并发增版本亦观察实际1213并最终通过 | 菜单sync的created、RBAC sync的created列表在回调外累计，尚无实际业务重试结果；不靠基础Runner通过确认业务计数正确 |
| D4 持久化版本到旧Token拒绝 | 本轮菜单创建后admin version1→2，旧Access401 AUTHN_TOKEN_INVALID、旧Refresh401 AUTHN_REFRESH_TOKEN_INVALID；C6明确缺行Access错误映射问题；普通注册无版本、首次登录1 | 不是每个写入都已用旧Access／Refresh验收，也不是完整运行时scope／menu可见性验收。Nav optionalcache未由组合根接线，不能声称实际cache/metrics生效 |
| D5 验证入口维护 | 非tag相关包全通过；独立testsupport MySQL3cases通过；已知组织tag测试NewService五参而现生产六参 | 未编译重现／修复已知入口，不运行全仓tag suite，不计为生产业务缺陷，不把未执行的定义写为“测试通过” |

## 四、Organization全部条款矩阵

| 行 / Requirement正文与Scenario原文位置 | 当前实现与事实owner | 证据与覆盖状态 |
| --- | --- | --- |
| O1 组织单位CRUD :10-11；列表:13-15、创建:17-20、删除:22-24 | Organization Service CreateUnit `76-113`、DeleteUnit `185-219`；CodeExists／模型unique；删除前ChildCount在事务外，事务清成员／硬删／版本 | 写入静态核对、真实创建用于C4夹具；全部值、重复code映射、并发子级与删除未验收。分页列表未覆盖。任何“在事务外”须结合具体条件，不单独叫缺陷 |
| O2 树 :26-27；树查询:29-32 | Repository有序记录／树组装，Hierarchy为跨owner范围提供后代 | 读取切片未验收；只定位依赖，不以C4发现推出排序／树结构全部错误 |
| O3 manageable :34-35；树标志:37-40、拒定位祖先:42-44 | `ensureVisible`用于修改、成员；`UserMembershipService.Set:47-49`校验新增组织处于scope；定位祖先仅在读模型补链，不扩大scope | 写入目标拒绝静态支持；本轮已有App HTTP SQLite定义执行，历史9/30真实定位祖先拒绝可局部复用；范围树整体留后续 |
| O4 单用户组织 :46-47；成功:49-52、空:54-56、joined_at:58-61、非法:63-65、CAS:67-70、保护:72-74、并发:76-79 | `user_memberships.go:34-59`原始0拒绝、Ensure/CAS、当前ManagedUser scope／保护、完整可管理目标；Repo差量保留隐藏和既有时间；同事务版本 | 本轮CAS真实HTTP组织409且仅角色赢家推进版本；既有3组织单用户SQLite及App个体写入定义执行。历史9/30真实隐藏joined_at／stale冲突可复用；全部拒绝输入及受控双维度MySQL内部交错未全部验收 |
| O5 组织数据范围过滤 :81-82；组织列表:84-87、树:89-92、用户列表:94-97 | Authorization Resolve scopes + Organization hierarchy/read filters | 3读／运行时场景未覆盖；写入用当前visibility不等于完整All/None/self/org/custom决策已验收 |
| O6 防循环 :99-100；父级:102-104 | `UpdateUnit:127-150`读后代，再`175`独立写 | C4具体真实并发不符；普通self／后代拒绝逻辑仍保留正向静态事实，不因为C4说没有防护实现 |
| O7 成员绑定 :106-107；覆盖:109-113、查询:115-117、删除清理:119-121 | `SetUsers:221-258`规范化/目录存在→事务ReplaceMemberships→旧新并集版本；`DeleteUnit`事务清成员 | 写入静态支持；范围/保护/原始全集更强要求列新目标。查询未覆盖；多用户、并发引用及真实失败回滚尚缺 |
| O8 joined_at :123-124；建立:126-129、移除:131-134、覆盖保留:136-139、迁移:141-144 | Membership.CreatedAt UTC；Repo ReplaceMemberships `224-237`只删移除并OnConflictDoNothing插新增；App migration `119-122`仅补NULL时间 | 既有membership、单用户差量SQLite测试执行；真实历史9/30保留时间可复用。本轮MySQLfresh无旧NULL行，不宣称真实旧数据回填已验收；时间/隐藏关系不因新noop而重置 |

组织字段更新不维护结构依赖者版本的现状与已批准I4组织范围边界失效目标分列；当前org CRUD不逐项写这些版本前提，不能凭新目标直接判旧违约。

## 五、版本存储全部条款矩阵

| 行 / Requirement正文与全部Scenario位置 | 当前实现链 | 证据／精确缺口 |
| --- | --- | --- |
| V1 唯一持久化来源 :10-11；表:13-17、软删保留:19-22 | GORM UserAccessVersion `models.go:48-52`无软删、PK user_id、version默认1/notnull/check；App Migrate；Identity Delete事务soft-delete+Increment | fresh MySQL版本DDL/CHECK及独立migrate门禁支持schema；Identity当前删除路径相对f43f518未改、历史真实删除及旧MySQL删除原子日志有局部事实。未在本轮实际软删／恢复完整JWT链路验收，不假定生产FK |
| V2 唯一读取 :24-26；存在:28-31、缺失:33-36 | AccessVersions.Current只读独立表；Authorization Snapshot→Identity Authenticate/Refresh，User无镜像列 | 当前门禁证明不恢复token_version旧列／migration state；真实有效JWT、版本失效支持读取，C6缺行请求拒绝且不懒初始化，但Auth401映射不符。不得把拒绝500解释成完整认证契约一致 |
| V3 懒初始化 :38-40；首次:42-46、并发首次:48-51、失败:53-56 | Identity Login认证凭据后EnsureAccessVersion→Snapshot回读一致→Issue；ensureAccessVersion用主键冲突DoNothing再锁回读 | 本轮两个真实注册用户无版本、各首次登录1；源码拒绝初始化/回读不一致；历史真实并发Ensure证明旧实现。当前并发首次登录全成功claims／初始化单步故障未执行 |
| V4 原子EnsureAndIncrement :58-60；已有:62-65、首次失效:67-71、并发:73-76、失败:78-80 | AccessVersions `63-83` FromContext、ensure+lock、version+1、RowsAffected必须1、锁回读；业务owner携同一最外层事务；源码LSP13引用已追到Role/个体组织/批量Increment | 当前SQLite Ensure/EnsureMany和真实MySQL8并发init+increment最终9通过；真实API更新第二版本失败全投影回滚。Ensure与Increment之间所有并发组合、每个业务入口缺行/故障/响应阶段仍缺，不把Store方法单独调用看作业务事务证明 |
| V5 Seed不维护版本 :82-83；超管Seed:85-89 | Seed Foundation/Finalize只维护事实；Identity Seed没有版本写；versions唯一owner明确 | 当前Seed SQLite no-version/哨兵不变测试执行；本轮正常启动存在，但直到admin登录后才记录版本1，不能伪称本轮已记录登录前版本表0。真实已有用户版本修复禁令／每阶段失败复跑仍须定向记录 |

## 六、Identity写入与Auth关联覆盖

| 触及条款 | 当前实际边界 | 结论 |
| --- | --- | --- |
| user-management密码 :188-202 | ChangePassword `155-184`校验、bcrypt、同事务更新与Increment；自助nickname/email不同流程不移植配置规则 | 现存fake事务测试已执行，真实密码／版本全失败原子与旧Token尚缺 |
| 逐人角色 :253-276 | 只替目标用户、同事务版本/CAS；RBAC B5 | 本轮真实角色200组织409的具体shared-version结果；其余条件分别S/T/H/G |
| 管理员修改 :278-296 | 目标guard→非空邮箱拒绝→Update+Increment→commit后回读；HTTP DTO在前 | C5明确普通字段修改组合失败；S2纯昵称失效未定；旧五项邮箱禁止不重开/不据Service单测称HTTP可用 |
| 管理员删除 :298-314 | soft-delete+同事务版本，关系保留；恢复不是本轮新增操作 | 保护与版本有源码／历史局部支持；删除／恢复／故障组合本轮未全验收。清软删用户旧关系属新生命周期目标 |
| 状态 :316-327；Kick :329-340 | status实际1/0切换+版本；Kick即显式主动失效命令 | 本轮同一普通用户状态端点正向控制200并恢复；完整旧JWT／保护／故障未全验收。Kick不套noop豁免 |
| auth实时Token Requirement :135-191 | 当前JWT/Identity状态/Snapshot版；不访问旧版本镜像 | 真实版本一致可请求，菜单变化后旧Access/Refresh401；缺行C6。登录锁定、JWT所有purpose/errors、disabled角色权限、Scope Snapshot、菜单过滤和完整Auth余项明确未审完 |

## 七、历史证据适用性与本轮执行

- `standardize-api-response-contract/evidence/frontend-integration-acceptance-2026-09-30.txt:10-24,36-42`确有隔离真实MySQL/Redis/MinIO、配置保存、删角色后菜单404、逐人角色／组织stale版本、隐藏joined_at和定位祖先限制记录。`build-admin-workbench/evidence/acceptance-2026-09-30.txt`只转引，不算第二次验收。记录未固定执行SHA/完整Case，不补造。
- 对发布提交`f43f518`到当前，定向历史diff显示 Organization、当前AccessVersions、Identity删除／状态／Kick相关路径未改；Identity用户修改只有邮箱禁止改动，不能复用旧邮箱编辑成功。Navigation写入相对该发布链未改，配置读取改动与写入分开。部分历史行为因此保留局部效力，C1-C6所需输入仍由本轮实际验证，不以旧“全绿”排除。
- `migrate-access-version-storage/evidence/2026-07-31-post-drop-validation/mysql-concurrency-transaction.log`确有8个旧admin/service MySQLCase通过，包含Ensure唯一、并发累加/锁等待、不倒退、实死锁重试、primary-write失败、软删/version失败回滚。其代码经历分层重构，不能直接宣称当前Adapter所有路径通过；本轮现存独立MySQL门禁重新取得当前路径证据。
- 2026-08-22 backend acceptance仅列全量34 packages及后端专项结果，不能证明未记录的raw0/status0/失败阶段；旧review的勾选不作证据。

本轮实际执行：

```powershell
go build -o <owned-temp>/admin.exe .
go test ./internal/authorization/... ./internal/navigation ./internal/organization ./internal/identity/application ./internal/app ./internal/app/seeddata ./internal/apimetadata/... -count=1
go test -tags mysql_integration ./testsupport/testutil -run 'TestMySQL(AppMigrationUsesOnlyCurrentAuthorizationSchema|ConcurrentAccessVersionTransactionsSerializeIncrements|TransactionRunnerRetriesDeadlockAndRollsBackAttempt)$' -count=1 -v
```

三项均通过。后一门禁要求自建隔离DSN，只通过环境注入未打印；其输出包含真实1213和最终8次increment→9，以及人工1213三次attempt只提交第三次。第一组11个有测试包通过、2个无测试包；未运行全仓普通／全仓tag suite、lint、formatter，不宣称它们通过。测试不是唯一证明：原程序的实际HTTP／JWT／真实库状态和可控锁交错见去敏证据。

## 八、只作根因分组，停止在change之前

| 分组 | 已核实问题／未核实问题 | 可进入后续决策的边界 |
| --- | --- | --- |
| 输入边界与业务契约一致 | C1原始列表校验；C5 DTO邮箱与非空禁写互锁 | 现行修复候选已取证；其他集合保护／范围是规范变更，不能附带扩scope |
| 合法零值与默认协作 | C2显式停用状态 | 保持默认与合法0区别，核对真实持久化；未指定修复实现 |
| 权限事实联动 | C3直接绑定边失配 | 先保持现行已绑定一致性；多菜单传播／授权迁移范围另固定，不伪装整个联动图已审完 |
| 拓扑提交不变量 | C4真实并发成环 | 当前防循环修复与新组织移动版本影响集合分开；先取可控交错事实再谈设计 |
| 认证错误所有权 | C6缺版本Access500 | 区分缺行不可验证会话和真实基础设施内部故障；不统一吞掉所有存储错误 |
| Seed读取失败与业务不存在混同 | ID-SEED-COUNT | 2026-10-15单条故障已确认违反既有凭据保留承诺；只登记根因，未修复、不指定实现方案 |

以上不是change列表，没有分配变更名称或生成工件。业务语义／证据缺口保持各自状态；不通过“全归缺陷”得到一张提前修复清单。

## 九、审计环境清理

取证后已停止仅本轮拥有的应用、MinIO、Redis进程（Redis不持久化），删除自建审计schema及全部临时配置、凭据、编译产物、下载包和MinIO数据目录。去敏JSON已解析核对C1-C6记录，新增报告链接可解析。没有产品代码、永久测试、主规格、runbook或change工件写入，也没有终止或清理用户已有服务／工作树资产。

