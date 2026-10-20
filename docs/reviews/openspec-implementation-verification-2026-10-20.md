# OpenSpec 实施一致性核验报告：本批15个change

核验日期（UTC）：2026-10-20。范围由用户明确选择为本批全部15项。Schema均为`spec-driven`，contextFiles均指向本仓库。原核验的195/195及逐项分数卡保留历史快照；随后授权的10项补证已完成，当前205/205，见“后续执行跟踪”。本文核验当前组合实现，不做固定提交点diff审查，不把任务勾选、语法校验或历史测试通过单独当作正确性证明。

## 总体结论

**原核验未发现已证实的缺失实现或与已裁定规格冲突的产品行为，0项CRITICAL；当时的3类WARNING均为证据缺口。本次新增回归及原程序补证已关闭W1-W3，当前0项CRITICAL、0项未关闭WARNING。2项文档／发布SUGGESTION仍保留，不代表全C6或生产发布完成。**

| 维度 | 当前结果 |
|---|---|
| Completeness／完整性 | 205/205任务（原195项＋补证10项）；25份delta；原43/43 Requirement实现映射保留 |
| Correctness／正确性 | 原359个Scenario出现项、174个去重能力＋条款＋场景目录保留；W1-W3所列5个不同场景、9个重复出现项的缺失条件已补直接证据，不扩大为359项所有条件均实际重跑 |
| Coherence／一致性 | 模块owner、App唯一组合、事务与当前读、既有保护边界和后续裁定基本一致；发布合并及工件时间口径见S1/S2 |

“有实现映射”不等于“每个条件均已运行证明”。下方保留原核验发现及本次关闭依据；跨change重复的完整Requirement按出现次数统计，不冒充359种独立新能力。

## 分级问题

### CRITICAL：0

未定位到未勾选任务或完全缺失的Requirement实现。没有据不完整证据推断产品一定正确，也没有把本批之外的历史异常数据、S3/S4/S5/I3或整个C6审核当成本批必须实现的目标。

### WARNING：3类证据缺口，不是产品缺陷

以下W1-W3保留核验时的问题及建议；本次已分别取得直接证据并关闭，当前状态和新证据见每项关闭记录及后续执行跟踪。

#### W1：继承的列表排序／分页条件缺少直接断言

- **规格**：S1 `openspec/changes/archive/2026-10-20-support-explicit-authorization-field-clearing/specs/rbac/spec.md:14-16,70-72`要求角色／权限按排序和ID分页；保护目标`openspec/changes/archive/2026-10-20-enforce-protected-authorization-assignments/specs/rbac/spec.md:42-45`要求角色已分配权限按排序返回且空时为空列表。这些场景也出现在生命周期／配置no-op完整正文中，共7个出现项。
- **实现**：`internal/authorization/adapters/gorm/repository.go:24-38,446-458,541-550`确有排序与适用分页；没有观察到错误SQL或错误响应。
- **缺口**：角色列表未定位到成功的排序／翻页断言；权限列表`repository_test.go:41-44`只检查长度及total；角色权限`repository_test.go:59-62`为单权限，另有空列表支持，不能证明多项排序。原程序403停用路由记录不是成功列表证据；按map比对资源集合也不证明顺序。
- **建议行动**：复用确实断言顺序／页边界的现有证据，或在另行授权的验收／测试任务中构造乱序sort、相同sort不同ID及多页数据，断言精确ID顺序、total、跨页无重复；角色权限另断言多项sort顺序与空数组。无需据此修改当前产品SQL。
- **2026-10-20已关闭**：`repository_test.go`新增角色／权限精确分页与角色已分配权限排序三项真实持久化回归；[新原程序证据](evidence/authorization-followup-2026-10-20.json)的`W1-roles-pagination`、`W1-permissions-pagination`、`W1-assigned-permissions`记录64次角色／权限分页GET和2次角色权限GET、精确ID序列、边界及空数组。全部13项受保护表投影／授权版本前后相同；生命周期／配置no-op重复条款复用此结果。

#### W2：登录初始化版本失败未被实际验明

- **规格**：`openspec/changes/archive/2026-10-20-fix-access-version-auth-error-classification/specs/auth/spec.md:32-36`“登录初始化失败”。
- **实现**：`internal/identity/application/service.go:164-175`在Ensure失败、后续Snapshot读取失败或版本不一致时于Issue前返回内部错误。当前源码符合意图。
- **缺口**：已检查的登录fake总是Ensure成功；原程序真实1146失败是Access请求，不是登录初始化失败。成功登录和单独Ensure成功不能填补失败路径。
- **建议行动**：分别在真实登录Ensure及其后Snapshot读取处注入受控失败，保留成功对照；断言安全500、无Access/Refresh输出，并记录版本及凭据事实。版本状态遵守现有Ensure提交语义，不臆加登录整事务回滚要求。不要把这项继承场景缺证写成C6已发生的新缺陷。
- **2026-10-20已关闭**：`session_regressions_test.go`覆盖Ensure创建／读取失败及成功Ensure后Snapshot失败；[新证据](evidence/authorization-followup-2026-10-20.json)`W2-login-*`通过实际登录路由取得三类安全500／null data及三个撤故障后200对照。原程序观察到Ensure已提交1、Snapshot角色SELECT被真实表锁阻挡，再仅改名独占版本表、释放阻挡，最终版本SELECT实际1146；失败后1保留，身份凭据不变。不要求登录整事务回滚。

#### W3：Refresh拒绝矩阵仍不完整

- **规格**：同一C6 delta `specs/auth/spec.md:44-50`“Refresh Token 刷新失败”。
- **实现**：`internal/identity/application/service.go:199-220`处理解析失败、用途错误、黑名单、身份缺失／停用与版本缺失／失配，再决定是否Issue。
- **已验明**：版本缺行、失配、禁用用户等实际401；缺行不补版本。
- **缺口**：未定位到适用证据覆盖畸形／过期Refresh、Access用途提交Refresh以及用户不存在的Refresh。Refresh当Access被拒绝不能证明反方向消费者。
- **建议行动**：补上述拒绝类的受控调用，断言通用401 `AUTHN_REFRESH_TOKEN_INVALID`、null data、无新token对、无旧字段兜底或版本补行；当前不据此改产品规则。
- **2026-10-20已关闭**：[新证据](evidence/authorization-followup-2026-10-20.json)`W3-refresh-*`包含合法Refresh对照、畸形／确定性真实过期／有效Access用途、对应用户物理不存在且版本存在或缺失的五次拒绝观察；逐项相同401、`AUTHN_REFRESH_TOKEN_INVALID`、安全英文消息及`data=null`，13项投影和版本不变。永久回归另覆盖前述前三类在版本存在／缺失下的组合。缺行／失配复用C6原证据，停用用户复用昵称目标`explicit-toggle-old-refresh-while-disabled`；明确该旧Token亦已失配，不扩张原证据效力。

### 后续执行跟踪（2026-10-20，新增10项已完成）

用户授权将W1-W3补证写回既有change并执行。本次只增加消费者行为回归、独占原程序去敏证据及验收记录，不改产品代码或规格语义，也不把原证据不足追认为产品缺陷；历史JSON保持原样。

| 缺口 | 执行owner／任务 | 当前状态 |
|---|---|---|
| W1角色／权限列表排序、分页 | [S1 tasks第7节](../../openspec/changes/archive/2026-10-20-support-explicit-authorization-field-clearing/tasks.md)：7.1–7.3 | 3/3；两项精确分页回归＋64次原程序GET；已清理 |
| W1角色已分配权限排序 | [关联保护tasks第7节](../../openspec/changes/archive/2026-10-20-enforce-protected-authorization-assignments/tasks.md)：7.1–7.2 | 2/2；多项排序／排除未分配项回归＋有序／空数组原程序GET；已清理 |
| W2登录Ensure及其后Snapshot失败 | [C6 tasks第7节](../../openspec/changes/archive/2026-10-20-fix-access-version-auth-error-classification/tasks.md)：7.1、7.2、共享7.5 | 已完成；三处真实SQL故障和撤除对照，保留Ensure提交语义 |
| W3 Refresh拒绝矩阵 | 同一C6 tasks：7.3、7.4、共享7.5 | 已完成；真实JWT／HTTP与无存储fallback事实；C6新增5/5 |

新增10项均完成，当前205/205；三个owner分别15/15、18/18、16/16。原程序15组case记录78次目标HTTP响应（列表66次、认证12次），见[去敏证据](evidence/authorization-followup-2026-10-20.json)。本轮定向Go、全Go（131.10秒）、`-tags=mysql_integration -run ^TestMySQL`真实MySQL门禁及OpenSpec严格检查（15 change＋18主规格）均通过。独占原程序／Redis／MinIO进程、四个端口、原程序和门禁schema、私有配置／token／日志／数据／下载包目录均已清理；原MinIO二进制保留。未自动同步、归档、commit或push。重复继承条款复用owner证据，不重复计数为独立能力。

### SUGGESTION：文档／发布事项

#### S1：旧全文delta归档必须合并，不可覆盖已发布新增规则

原核验快照：8项已按依赖同步的前置，其Scenario名称均仍存在于当时主规格；7项其他delta保持独立未同步，符合原核验授权边界。另检出10个较早完整Requirement正文缺少后来已同步的场景名称，涉及C1/C2/C3、关联保护及引用生命周期。这是时序演进及**发布合并门禁**，不是已观察到的源码缺陷，也不指称主规格已回退。

建议按各design的发布基线逐Requirement合并；已同步旧change归档不得再次用旧全文抹掉R6／S1等新增场景。保留正文语义及场景，不只比较名称。详见后方重叠清单。此核验没有执行sync或archive。

**2026-10-20后续同步记录**：用户随后授权同步全部change，15项的25份delta现已智能合并到六份主规格，详见[整批同步记录](contract-readiness.md)。新增1个Requirement及41个Scenario，六份主规格原267个场景名称均保留，旧全文遗漏的后来规则没有被覆盖；最终18份主规格严格校验通过，二次语义合并dry-run无变更。此操作未归档、未改历史delta或JSON；本节保留的旧全文归档覆盖风险仍是后续流程的合并门禁，不代表当前仍有7项未同步。

**2026-10-20后续归档记录**：用户授权的15项均已归档到`openspec/changes/archive/2026-10-20-<name>/`。归档使用`--skip-specs`保留已验证的智能合并结果，不再用旧delta全文更新主规格；全部元数据、25份delta及205项已完成任务保留，仅迁移受影响的文档路径。本报告工件引用已指向归档位置，原核验／补证／同步阶段“未归档”表述保留为历史记录，不表示当前仍有活跃change。未执行commit或push，未修改产品代码或历史JSON。

#### S2：区分工件创建时描述与当前实施状态

例如`openspec/changes/archive/2026-10-20-fix-concurrent-organization-moves/design.md:5,56`仍写未来实施／当前仅创建工件，而当前tasks全勾选、审计页与新增证据已记录执行。建议在获准维护工件时将这些句子明确标为创建时历史前提，或补执行记录链接；不改历史JSON，也不为措辞建立永久测试。Seed的旧“两个COUNT”表述同理：后续R6把绑定存在性改成锁定Find，保持错误／空结果区分，不应为了旧措辞恢复COUNT。

## 原核验快照的执行与复用边界

| 检查 | 实际结果／效力 |
|---|---|
| `openspec list --json`及逐项status／apply instructions | 15项repo-local／spec-driven／all_done；195/195；所有可用工件均进入核验 |
| `go test -count=1 ./...` | 本轮全量通过，121.70秒；App包115.931秒。不是仅复用缓存结果 |
| 三项真实MySQL门禁 | 本轮通过，4.99秒；当前父图覆盖旧RR快照、角色／组织共享CAS、锁定版本覆盖旧RR快照。仅新建独占verification schema，结束已确认删除 |
| `openspec validate --changes --strict --json --no-interactive` | 本轮15/15通过，0失败 |
| `openspec validate --specs --strict --json --no-interactive` | 本轮18/18通过，0失败；internal-messaging长正文仅既存INFO，不列为本批问题 |
| 原程序HTTP／JWT／真实锁交错证据 | 复用各change已保存且适用的实际main/MySQL记录；本轮未重新搭建Redis／MinIO原程序环境，不冒充新运行 |

MySQL实际命令：`go test -tags mysql_integration ./internal/organization -run '^TestMySQL(LockedOrganizationParentsOverridesEarlierRepeatableReadSnapshot|RoleAndOrganizationWritesShareOneOptimisticVersion|LockedVersionOverridesEarlierRepeatableReadSnapshot)$' -count=1`。DSN只在私有子进程环境传递，报告不含凭据。

Fake／SQLite局部回滚和顺序注入不证明InnoDB并发；SIGNAL1213/1205证明既有Runner对应类别处理，不等同真实物理死锁／经过完整超时。历史失败、夹具修正、额外无效404/404 pair及证据丢失限制均保留，未重跑旧失败“确认”、未把失效证据改写成通过。

## 每项change分数卡（核验快照，新增补证前）

| Change | 已勾选任务 | Requirement出现项 | Scenario出现项 | 核验结论 |
|---|---:|---:|---:|---|
| `fix-admin-seed-query-error-handling` | 12/12 | 1 | 8 | 有实现映射；未发现范围内偏离 |
| `fix-admin-user-update-validation` | 10/10 | 1 | 6 | 有实现映射；未发现范围内偏离 |
| `fix-menu-api-binding-id-validation` | 10/10 | 1 | 8 | 有实现映射；未发现范围内偏离 |
| `fix-menu-create-status-preservation` | 11/11 | 1 | 11 | 有实现映射；未发现范围内偏离 |
| `fix-menu-api-code-consistency` | 13/13 | 1 | 12 | 有实现映射；未发现范围内偏离 |
| `fix-concurrent-organization-moves` | 13/13 | 1 | 5 | 有实现映射；未发现范围内偏离 |
| `fix-access-version-auth-error-classification` | 11/11 | 1 | 12 | 有实现映射；证据限制 W2,W3 |
| `enforce-protected-authorization-assignments` | 16/16 | 8 | 45 | 有实现映射；证据限制 W1 |
| `enforce-authorization-reference-lifecycle` | 18/18 | 10 | 81 | 有实现映射；证据限制 W1 |
| `make-api-permission-sync-atomic` | 15/15 | 1 | 5 | 有实现映射；未发现范围内偏离 |
| `support-explicit-authorization-field-clearing` | 12/12 | 3 | 33 | 有实现映射；证据限制 W1 |
| `preserve-sessions-on-nickname-updates` | 13/13 | 1 | 10 | 有实现映射；未发现范围内偏离 |
| `preserve-sessions-on-noop-configuration` | 13/13 | 7 | 73 | 有实现映射；证据限制 W1 |
| `invalidate-only-changed-memberships` | 14/14 | 4 | 39 | 有实现映射；未发现范围内偏离 |
| `invalidate-organization-hierarchy-dependents` | 14/14 | 2 | 11 | 有实现映射；未发现范围内偏离 |

## 当前实现索引

下面为父核验端复查的当前精确入口；附录对重复场景共享这些入口，不把中间检索的旧行号偏移当作新源码证据。

| 责任／边界 | 当前入口 |
|---|---|
| Seed存在性错误与启动上返 | `internal/identity/adapters/gorm/seed.go:25-102`；`internal/app/seeddata/seed.go:15-44`；`internal/app/seed.go:25-56`；`main.go:9-21` |
| 管理用户邮箱／昵称／实际身份变更 | `internal/identity/application/user_service.go:186-218,255-299` |
| 非登录缺行分类、Login／Refresh | `internal/identity/application/service.go:164-175,199-255`；`internal/app/identity.go:95-110` |
| 实际角色保护、完整目标与逐人CAS | `internal/authorization/application/service.go:242-320,321-400,405-458,923-977` |
| 角色／范围配置no-op与真实失效 | `internal/authorization/application/service.go:125-216,479-527,639-687` |
| 生命周期清理与实际owner事务 | `internal/authorization/application/service.go:219-239`；`internal/authorization/adapters/gorm/repository.go:133-144`；`internal/organization/service.go:247-311` |
| C4父图及合法根0 | `internal/organization/repository.go:133-156`；`internal/organization/service.go:85-244,297-311` |
| I4有限范围根依赖 | `internal/organization/hierarchy.go:87-105`；`internal/authorization/application/service.go:854-920`；`internal/organization/service.go:221-239`；`internal/app/organization.go:19-40` |
| 纯成员差量，不额外失效读取者 | `internal/authorization/application/service.go:242-400`；`internal/organization/service.go:323-401`；`internal/organization/user_memberships.go:60-141` |
| 菜单存在性／创建状态／绑定与联动 | `internal/navigation/service.go:34-185,386-590`；`internal/navigation/configuration.go:5-40` |
| 整请求API同步与逐API版本触发 | `internal/apimetadata/application/service.go:80-174`；`internal/navigation/api_permission_sync.go:49-282`；`internal/navigation/service.go:612-666` |
| 原始角色／权限配置与列表查询 | `internal/authorization/adapters/gorm/repository.go:24-38,58-85,203-265,446-458,515-550` |

### 跨change裁定复核

- C5的邮箱可选／非空禁止保留；后来的纯昵称／合法同值豁免替代旧无条件昵称增版，不把历史CE的增版结果当当前要求。
- S1的遗漏／空／替换与null原解析保留；配置no-op只豁免真实等价，实际清空、残留custom清理、缺失权限与直接API码修复仍是变化。
- R6按实际admin关联保护，停用admin角色不解保护也不授运行权限；legacy用户role标签不是RBAC。当前关系锁定读取的Seed等价替代保留原错误cause，无历史孤儿自动修复。
- F2保留用户裁定的严格有序整轮准备／当前复核及逐API增版；同一用户两项实际API变更可增两次，不擅自请求级去重。
- 成员差量只处理对称差；逐人Ensure／CAS先于合法no-op，整组no-op不补版本。I4仅父边真实变化触发范围根依赖，不蔓延到纯成员变动。
- C6继承的“权限相关变更提升版本”已结合Identity、Authorization、Navigation、Organization各owner映射：真实身份／权限／菜单／关系及I4变化均按各自边界处理；纯昵称、合法no-op与保留成员是后来明确裁定的豁免，而非漏接版本能力。

## 主规格重叠与发布合并清单

以下只列当前主规格中存在、旧delta正文未包含的后来场景；不是说旧change原实施时就遗漏未来目标。

| 旧change | Capability／Requirement | 需保留的后来已同步场景 |
|---|---|---|
| `fix-menu-api-binding-id-validation` | menu-management / 菜单 API 联动 | API停用或删除先完成；未绑定菜单改码；空API集合不是解绑；绑定与删除的版本原子性；绑定先完成后停用保留配置；绑定菜单显式空码沿用既有解析；菜单侧修改已绑定权限码；菜单侧联动中途失败 |
| `fix-menu-create-status-preservation` | menu-management / 菜单 CRUD | 合法路径清空；菜单删除与子菜单创建交错；菜单可选属性清空与遗漏分开；非法清空不部分提交 |
| `fix-menu-api-code-consistency` | menu-management / 菜单 API 联动 | API停用或删除先完成；空API集合不是解绑；绑定与删除的版本原子性；绑定先完成后停用保留配置 |
| `enforce-protected-authorization-assignments` | menu-management / 菜单 API 联动 | API停用或删除先完成；绑定与删除的版本原子性；绑定先完成后停用保留配置 |
| `enforce-protected-authorization-assignments` | organization-management / 组织成员绑定 | 受控清理软删旧普通成员；软删用户不能作为新增成员 |
| `enforce-protected-authorization-assignments` | organization-management / 组织单位 CRUD | 残留custom引用阻止组织删除；解除引用后合法删除 |
| `enforce-protected-authorization-assignments` | rbac / 角色数据权限 | custom先提交阻止组织删除；两个入口离开custom均清理；组织先删除则custom拒绝 |
| `enforce-authorization-reference-lifecycle` | menu-management / 菜单 CRUD | 合法路径清空；菜单可选属性清空与遗漏分开；非法清空不部分提交 |
| `enforce-authorization-reference-lifecycle` | rbac / 角色 CRUD | 拒绝清空角色必填标识；角色说明显式清空；角色说明遗漏保持 |
| `enforce-authorization-reference-lifecycle` | rbac / 权限 CRUD | 拒绝清空权限必填名称；清空权限分组标签 |

## 未执行／不扩大结论

- 未执行生产停写、全实例切换、回滚部署、全调度压力测试、容量性能基准、全库历史坏数据修复；部署协议文档化不能写成生产已完成。
- 未重跑全部历史原程序case；当前Go／真实MySQL门禁与复用的native证据分别注明。尤其F2最终二进制只补跑过部分成功路径，不能称全部旧故障都在最终binary再次重演。
- 未把实际main的JWT／权限证明归给使用替代middleware的App测试；真实认证与授权效力只记适用原程序case。
- I4刻意孤儿角色引用触发现有成员前置500、13项业务投影回滚，只保留为历史异常关系适用承诺／错误分类待核实观察；不是已确认发布缺陷，未据此改规则或创建change。
- 原核验阶段只新增本报告；本次补证按授权修改回归、任务／验收说明及审计记录，不改产品代码、规格语义或历史JSON，未自动同步、归档、commit或push。

## 最终评估

**当前无CRITICAL实现阻断项，W1-W3三类WARNING已通过本次直接补证关闭；不是“359场景全部实际重跑”的全绿结论。** 三个owner新增任务均完成，具备进入独立归档核验流程的任务状态；本次未归档。S1/S2文档／发布建议仍保留，正式归档须按各design发布基线合并主规格，不能逐旧全文直接覆盖。

## 详细映射附录

附录是完整条款／场景目录与实际case支持。重复完整正文通过共享目录ID复用，未从359计数中删掉。对于关系切片的中间范围行号，已统一去掉未经父端确认的偏移，改指向文件与上方当前实现索引；主分级问题使用复查后的精确行号。附录中的局部“证据充分”仍受主报告W1（含角色权限排序子条件）、W2、W3统一限定，不解释为全部条件／所有并发调度已证明。

### 附录A：Identity／Seed



All source/test line references below refer to the current snapshot.

- **Seed implementation:** `internal/identity/adapters/gorm/seed.go`.
- **Seed orchestration:** `internal/app/seeddata/seed.go:15-44`; `internal/app/seed.go:25-56`; `internal/app/app.go:124-169`; `internal/app/build.go:40-50`; `main.go:9-21`.
- **Managed-user implementation:** `internal/identity/application/user_service.go:186-299`.
- **Managed-user persistence:** `internal/identity/adapters/gorm/repository.go:38-72,183-216`.
- **Auth implementation:** `internal/identity/application/service.go:135-255`; `internal/app/identity.go:95-110`.
- **Version owner:** `internal/authorization/adapters/gorm/access_version.go:14-35,76-135`; `internal/authorization/application/service.go:34-37,698-719`.
- **Seed evidence (SE):** `docs/reviews/evidence/id-seed-count-fix-2026-10-16.json`.
- **C5 evidence (CE):** `docs/reviews/evidence/c5-admin-user-update-fix-2026-10-16.json`.
- **C6 evidence (AE):** `docs/reviews/evidence/c6-access-version-auth-fix-2026-10-17.json`.
- **Nickname evidence (NE):** `docs/reviews/evidence/nickname-sessions-2026-10-18.json`.

Evidence references identify an actual case, not merely an overall passed-gate statement. Earlier failure evidence was retained as historical ground truth and was not rerun.

## 1. fix-admin-seed-query-error-handling

### Requirement and design adherence

Artifact requirement: `openspec/changes/archive/2026-10-20-fix-admin-seed-query-error-handling/specs/rbac/spec.md:3-46`, **超级管理员兜底初始化**.

- `seed.go:30-36` returns the active-admin read error before any username lookup, password hash, user creation/restoration, or relationship write. `seed.go:88-93` consumes the error on the COUNT's returned query object, not the original transaction's `Error`.
- `seed.go:60-68` separately returns the configured-user relationship read error before either the ordinary-name conflict or password restoration. The current helper is `seed.go:96-102`: `SELECT ... FOR UPDATE` into relationship records, returning `(bool,error)`, not a second COUNT. This is the later lifecycle cutover; successful empty results alone mean no binding.
- `seed.go:25-29,48-58` uses the actual enabled `admin` role and current user/role locks. `seed.go:75-87` creates/restores only after confirmed absence/binding facts. No version capability is imported or called.
- `seeddata.Finalize:28-44` returns module errors; App's separate Foundation, API/permission-sync, and Finalize stages remain distinct (`internal/app/seed.go:25-56`). A Finalize error aborts that transaction but does not undo earlier committed stages.
- Startup consumes the error: `internal/app/app.go:136-145` returns before engine registration, `internal/app/build.go:45-48` returns the build error, `main.go:11-14` exits instead of reaching `Run`. No global Seed transaction, new retry policy, or version-repair behavior was introduced.

### Complete scenario map (8/8)

| Artifact scenario | Current implementation | Deterministic and/or precisely applicable native evidence |
|---|---|---|
| **admin 角色不存在** (`spec.md:8`) | `internal/authorization/adapters/gorm/seed.go:26-40,51-89` creates the enabled default admin role; Foundation is called before Finalize | `internal/app/seeddata/seed_test.go:27-47` asserts default roles and admin binding after fresh initialization; SE `fresh-bootstrap`, `:53-60`, records original main creating the configured administrator |
| **已存在启用超级管理员** (`:13`) | Identity `seed.go:30-36` returns without password recovery when active association exists | `internal/identity/adapters/gorm/seed_lifecycle_test.go:138-156` checks password/status/version retention; SE `normal-restart-control`, `:62-68`, records original password 200, replacement password 401, unchanged hash/version |
| **没有启用超级管理员** (`:18`) | Identity `seed.go:37-79` validates configured fields and creates the user plus `user_roles`; an existing bound disabled/deleted identity uses the existing restoration branch, not a duplicate identity | `seeddata/seed_test.go:27-47`; SE `fresh-bootstrap` (`:53`); restoration/no-version assertions in `seeddata/seed_test.go:198-287` and rollback in `seed_lifecycle_test.go:43-78` |
| **配置用户名已存在但未绑定 admin 角色** (`:23`) | Identity `seed.go:60-68` rejects confirmed missing actual binding | `seeddata/seed_test.go:156-196` supplies `users.role=admin` without an actual relationship, checks refusal, unchanged credentials/version, and no promotion; `seed_lifecycle_test.go:80-109` checks cleared binding is not recreated |
| **管理员身份判断** (`:28`) | Identity `seed.go:25-30,88-102` uses `roles.code=admin` plus actual relationship, never the legacy user role tag | Same ordinary-name regression `seeddata/seed_test.go:156-196`; native NE `legacy-label-context-remains-rbac-derived` (`:3690`) and `legacy-label-does-not-grant-admin-route` (`:3845`) independently show legacy `role=admin` grants neither runtime roles nor permissions |
| **启用管理员存在性读取失败** (`:33`) | Identity `seed.go:30-33,88-92`, error propagated through the startup chain above | `seeddata/seed_test.go:69-154`, subcase `enabled administrator lookup`, checks original cause, identity/hash/binding/version preservation, Finalize menu rollback, and retained earlier API/permission writes; `seed_lifecycle_test.go:111-136`; SE `real-active-admin-count-interruption` (`:71-96`) records real MySQL 1317, exit 2, no HTTP listener, unchanged identity/hash/binding/version, prior-stage commit and Finalize rollback |
| **配置用户名角色绑定读取失败** (`:38`) | Identity `seed.go:60-63,96-102` propagates the actual locked relationship query error, rather than returning ordinary username conflict or restoring password | `seeddata/seed_test.go:69-154`, subcase `configured user role lookup`, disables the admin, injects one `user_roles` query error, checks `errors.Is`, no user/credential/version effects and Finalize rollback. SE `limits` (`:111-117`) explicitly limits the native 1317 run to active-admin COUNT; it does not claim a native run of this second branch |
| **错误消除后重新启动** (`:43`) | Successful retry follows the same existence/restore logic and stage boundaries | `seeddata/seed_test.go:50-67` checks table-count idempotency; SE `fault-removed-recovery` (`:98-109`) records same configuration recovering to HTTP, original password still 200, replacement 401, unchanged hash/version and successful Finalize restoration |

**Task conclusion:** 12/12 checked tasks have corresponding source, test, or delivery-note implementation. Applicable maintained notes are `docs/runbooks/database-migration-and-seeding.md:39-57` and `docs/modules/user-management.md:31`. The second native COUNT fault was never claimed; the deterministic current adapter-boundary test remains applicable after its replacement with a locked read.

## 2. fix-admin-user-update-validation (C5)

### Requirement and design adherence

Artifact requirement: `openspec/changes/archive/2026-10-20-fix-admin-user-update-validation/specs/user-management/spec.md:3-38`, **管理员修改用户**.

- The actual HTTP DTO is `internal/identity/dto.go:104-109`: `Email string` has no format validator. This follows the approved **all nonempty strings reach business prohibition** decision, rather than only adding `omitempty,email`.
- `internal/identity/adapters/http/routes.go:285-301` binds that DTO and passes `Nickname`, `Email`, `Role`, `Status`, operator ID, and target ID to the same Application use case. The route remains `PUT /api/admin/users/:id`, permission `admin.users.id.put` (`routes.go:65-67`).
- The business email error is evaluated before difference/no-op classification (`user_service.go:188-193`), after the existing managed-target safeguards. Neither accepted empty email nor rejected nonempty email builds an email/pending/verification update. The repository consumes only the actual `UserChanges` fields (`repository.go:183-208`).
- Managed-target checks retain caller transaction ownership and current protection/scope facts (`user_service.go:255-299`). Actual admin association is protected independent of role status via App `identity.go:159-170`, Authorization `service.go:923-937,426-492`; the legacy `users.role` string is not protection identity.
- Mutation commits before `service.Get` readback (`user_service.go:218`). Readback failures are not transaction rollback. Existing Get maps all repository read failures to `ErrUserNotFound` (`:47-54`); the HTTP error adapter maps that to token-invalid (`internal/identity/adapters/http/errors.go:149-150`). This is pre-existing and explicitly retained in the nickname design; it is not a C5/C6 regression.

### Complete scenario map (6/6)

| Artifact scenario | Current implementation | Applicable test/native evidence |
|---|---|---|
| **管理员修改普通用户** (`spec.md:8`) | `user_service.go:186-218`, repository `:183-208`, HTTP `routes.go:285-301`, safe response conversion `dto.go:135-141` | `internal/app/identity_http_test.go:88-194` exercises real composed PUT/profile/ownership projections with fixture middleware; `internal/identity/application/nickname_sessions_test.go:58-89` asserts role/status actual changes and exactly one increment. CE `omitted-email` (`:50-75`) / `empty-email` (`:78-104`) provide original-main HTTP and DB facts. The current narrower session policy is additionally exercised in NE `mixed-nickname-and-accepted-legacy-role-change` (`:3093`) / `mixed-nickname-and-real-status-change` (`:3985`) |
| **管理员尝试修改自己或其他管理员** (`:15`) | `user_service.go:255-258,273-298`; production protection uses actual association, not legacy string or active-role-only snapshot | CE guards `self` (`:164-175`) and `protected` (`:177-188`) show unchanged state/version. NE `self-noop-remains-rejected` (`:2190`) and `actual-admin-membership-noop-remains-protected` (`:2341`) establish no-op cannot bypass protection. `internal/identity/application/user_lifecycle_test.go:141-170` covers current membership in a disabled admin role |
| **管理员提交邮箱更新** (`:19`) | `user_service.go:188-190` returns the existing email field error before writes | `internal/identity/application/email_change_test.go:139-180` checks no submitted field/ownership/version effect; `identity_http_test.go:155-194` checks 422 plus exact field code and persisted state/version. CE `nonempty_email_refusals`, `:107-160`, contains valid and malformed address inputs with unchanged full supplied user projection/version |
| **无邮箱字段的合法资料更新** (`:25`) | DTO `dto.go:104-109`, HTTP `routes.go:291-301`, business `user_service.go:191-218` | `identity_http_test.go:131-153`, omitted-email subcase; CE `omitted-email` (`:50`) records success, updated safe user response and retained email/pending/verified facts; NE pure nickname cases establish the later applicable version rule |
| **空邮箱不表示清空邮箱** (`:30`) | Empty `request.Email` bypasses only the email prohibition and never populates an email change; common safeguards still execute | `identity_http_test.go:131-153`, empty-email subcase, checks retained verified/pending ownership; CE `empty-email` (`:78`) records nickname/status success without clearing ownership; same managed-target path handles protection |
| **非空邮箱与其他字段混合** (`:35`) | `user_service.go:188-190` returns before nickname/role/status differences or version writes | `identity_http_test.go:155-194` sends email plus changed nickname/status and checks all-or-nothing; CE `nonempty_email_refusals` (`:107-160`); NE `email-guard-before-difference-nonempty-valid-format` (`:1870`) and `...nonempty-invalid-format` (`:2030`) show even otherwise same-fact candidates cannot be exempted |

**Task conclusion:** 10/10 tasks have relevant source/tests/docs evidence. Empty update still returns validation failure; out-of-scope targets remain rejected (`user_service.go:191-193,288-297`; CE `empty-update` `:190`, `outside-scope` `:203`; the latter records the same route permission being present). The C5 requirement is already in the published main spec, `openspec/specs/user-management/spec.md:285-318`. Its historical evidence's missing-row→2 nickname increment was correct at that implementation date and must not be reused as a current expected value after the approved nickname change.

## 3. fix-access-version-auth-error-classification (C6)

### Requirement, consuming-side dispatch, and design adherence

Artifact requirement: `openspec/changes/archive/2026-10-20-fix-access-version-auth-error-classification/specs/auth/spec.md:3-75`, **Token 实时失效**.

The full missing-version cause path is present:

1. `AccessVersions.Current`, `internal/authorization/adapters/gorm/access_version.go:20-28`: only GORM record-not-found becomes `ErrAccessVersionNotFound`; a real query failure returns its own error.
2. Authorization Snapshot `internal/authorization/application/service.go:698-719` reads roles/permissions/current version and wraps failures. `internal/authorization/application/errors.go:26-32,40-48` preserves `Unwrap`; classification does not destroy the sentinel.
3. App bridge `internal/app/identity.go:101-110` uses `errors.Is` for that exact sentinel and `errors.Join` with the Identity caller-owned `ErrAuthorizationVersionMissing`. Other failures are returned unchanged. Identity does not import Authorization's GORM adapter.
4. Identity `internal/identity/application/service.go:245-250` dispatches only that new reason to `CodeTokenInvalid`; all other Snapshot failures stay `CodeInternalError`.
5. The actual consuming middleware `internal/identity/adapters/http/middleware.go:30-42` explicitly routes internal errors to safe 500, disabled account to 403, other authentication rejection to safe 401. HTTP definitions `errors.go:32-36,60-61` provide stable public codes/messages. No introduced reason is silently dropped.
6. Refresh is a separate consumer: `service.go:199-220` returns the pre-existing generic refresh rejection for any failed snapshot, missing/current-version mismatch or wrong purpose. `routes.go:191-204` and `errors.go:106-109` preserve the refresh 401 envelope. Login remains the only authentication lifecycle path calling Ensure (`service.go:164-174`, App `identity.go:97-98`, Authorization `service.go:34-37`).

No Ensure, token issue, legacy field lookup, or alternative-storage fallback exists in Authenticate. Current production middleware is selected by `internal/app/app.go:162-168`, not an unused adapter.

### Complete scenario map (12 source-mapped)

| Artifact scenario | Current implementation | Applicable test/native evidence and limits |
|---|---|---|
| **Token 版本一致** (`spec.md:8`) | `service.go:223-255`, owner Current `access_version.go:20-28`, middleware `:30-48` | `internal/identity/application/service_test.go:219-235`; AE `valid-access-before-targeted-fault` (`:34`), restored-same-access controls (`:78,:116,:188`) each 200 |
| **Token 版本不一致** (`:14`) | `service.go:201-203,212-214,225-227,252-254` rejects mismatch/nonpositive versions and wrong purpose | `service_test.go:211-235`; AE `ordinary-version-mismatch` (`:128`), `ordinary-refresh-version-mismatch` (`:140`), `refresh-purpose-used-as-access` (`:152`) record stable 401 codes |
| **用户被禁用或删除** (`:20`) | `service.go:234-243` distinguishes missing identity from disabled established identity; repository `repository.go:38-46` distinguishes record absence from storage error | AE `disabled-existing-user` (`:164`) records 403 `IDENTITY_ACCOUNT_DISABLED`; `soft-deleted-user` (`:176`) records 401 `AUTHN_TOKEN_INVALID`; restored positive control confirms healthy token premise |
| **登录初始化新用户版本** (`:26`) | `service.go:164-177`; App `identity.go:97-98`; Authorization `service.go:34-37`; owner `access_version.go:31-35,120-135` creates1 and rereads; Snapshot comparison precedes issue | `internal/authorization/adapters/gorm/access_version_test.go:32-47` checks Ensure1 and repeat1; `service_test.go:110-137` checks issued version/current snapshot (local fake). NE `registered-baseline` (`:21`), `baseline-target-login` (`:29`), and before-projection of `baseline-pure-nickname-invalidates` (`:38`, user2 version1) supply actual registration→login initialization evidence; it is before nickname product edits, not before the already-completed C6 auth fix |
| **登录初始化失败** (`:32`) | `service.go:164-174` returns safe internal error before Issue for Ensure error, Snapshot error, or changed snapshot version; HTTP adapter `errors.go:151-152` maps internal to500 | **Coverage gap W1.** No deterministic test or inspected native case actually fails login Ensure/reread and asserts zero issued tokens plus safe500. AE's real missing-table failure exercises Access, not login. Successful login tests and separate Ensure tests are not failure-path evidence |
| **使用 Refresh Token 刷新** (`:38`) | `service.go:199-220`, HTTP `routes.go:191-204`, DTO response `dto.go:50-53` | `service_test.go:199-209`; AE `healthy-refresh-after-restoration` (`:200`) records200 and both token fields; NE `old-refresh-remains-valid-after-nickname-and-noop` (`:1259`) records complete success envelope with redacted pair |
| **Refresh Token 刷新失败** (`:44`) | `service.go:201-215` rejects parse error/wrong purpose, blacklist/storage refusal, missing/disabled user, snapshot error/missing row/mismatch; no issue until all pass | AE missing-row refresh (`:58`) and mismatch (`:140`); NE disabled-user old refresh (`:4287`) and other old-pair rejections (`:3387,:4873,:5730,:6161`) record401/null/generic refresh code. **Partial evidence W2:** inspected material does not exercise malformed/expired/wrong-purpose refresh or missing-user refresh; successful JWT purpose parsing (`internal/identity/adapters/jwt/service_test.go:11-34`) is not a rejection test |
| **认证时授权版本缺失** (`:52`) | Cause path steps1-6 above; non-login Authenticate/Refresh never Ensure | `service_test.go:244-265` tests joined missing reason vs infrastructure and same-text non-sentinel; AE missing Access (`:46`), missing Refresh (`:58`), row count0 after both (`:70`), exact-row restoration positive control (`:78`) |
| **权限相关变更提升版本** (`:58`) | Identity password `user_service.go:154-182`, real role/status `:198-211`, Delete/Toggle/Kick `:221-252`; App `identity.go:155-156`; owner `access_version.go:76-115` EnsureAndIncrement. Authorization current real role/relationship writes use `service.go:120-183,300-424,1003-1013` | Identity: `user_service_test.go:89-101`; `nickname_sessions_test.go:58-112`; lifecycle `user_lifecycle_test.go:120-138,173-216`; NE real role/status/Toggle/Kick old-pair observations. **Cross-slice boundary:** role permissions/menus, menu-record writes and organization member/hierarchy invalidation require parent's Authorization/Navigation/Organization maps; do not infer them all from Identity's increment adapter or this C6 classification fix |
| **Access缺版本不补行** (`:63`) | `service.go:245-254`, Current `access_version.go:20-28`, exact-sentinel App translation; no issue or Ensure in this path | AE `sole-version-row-missing` (`:46`) plus `both-non-login-rejections-did-not-recreate-row` (`:70`), original access still unexpired and user valid, row count0; same row restoration makes the same token200 |
| **Access读取真实基础设施失败** (`:68`) | App `identity.go:104-107` leaves non-sentinel errors; `service.go:250`; middleware `middleware.go:36-37`; safe fixed public definition `errors.go:60-61` | `service_test.go:244-265` includes real error identity vs matching text; AE `version-query-only-real-sql-failure` (`:90`) / `isolated-query-failure-state` (`:102`) report MySQL1146, HTTP500 `IDENTITY_INTERNAL_ERROR`, no SQL/schema/credentials in public response, preserved version table contents and restored-same-token200 control |
| **Refresh缺行保持既有结果** (`:73`) | Refresh `service.go:212-215`, route error consumer `routes.go:199-201`, fixed refresh definition `errors.go:29-30,106-109` | AE `refresh-with-same-missing-row` (`:58`) is401 `AUTHN_REFRESH_TOKEN_INVALID`, null data; post-both row-count0 (`:70`) proves no lazy repair; no success pair was returned |

**Task conclusion:** 11/11 checked tasks match the implemented targeted C6 change. The missing/infrastructure distinction is directly proven through original main plus real storage, not just mocked error identity. W1/W2 concern inherited full-requirement scenario completeness, not a claim that C6's named implementation tasks are unfinished. The full auth audit is expressly outside this change.

## 4. preserve-sessions-on-nickname-updates

### Requirement and design adherence

Artifact requirement: `openspec/changes/archive/2026-10-20-preserve-sessions-on-nickname-updates/specs/user-management/spec.md:3-57`, **管理员修改用户**. Its first six scenarios preserve C5; its last four introduce the approved nickname/no-op policy.

- The actual current locked target is passed into the mutation closure only after `ensureManagedTarget` obtains current identity/association/scope facts (`user_service.go:259-298`). Comparisons use that target, not the initial unlocked existence read.
- Managed protection/scope and nonempty-email prohibition run before no-op success. Empty request remains invalid (`:188-193`).
- Raw nickname/legacy-role strings are compared exactly (`:195-202`). This follows the design's explicit finding that this endpoint previously did not Trim; no new normalization/empty-string policy is silently introduced.
- Empty differences return success (`:204-205`); pure nickname persists via repository Update but never touches version owner (`:207-214`). Postcommit Get only reads the user and does not Ensure; a never-logged user therefore stays absent even while returning the successful profile (`:218`, `:47-54`).
- Any actual accepted legacy-role or status difference makes exactly one `IncrementVersions` call in the common transaction, even if both differ alongside nickname (`:198-211`). Repository `Update` consumes all supplied changes on the same transaction context; App translates IncrementVersions to the owner store (`identity.go:155-156`), which EnsureAndIncrement creates2 from missing or advances an existing version once (`access_version.go:76-115`).
- Delete, Toggle, and Kick remain separate mutation closures (`user_service.go:221-252`), so nickname/no-op exemption cannot accidentally suppress active lifecycle invalidation.

### Complete scenario map (10/10)

| Artifact scenario | Current implementation | Applicable test/native evidence |
|---|---|---|
| **管理员修改普通用户** (`spec.md:10`) | `user_service.go:186-218`; real differences select persistence/invalidation; HTTP safe conversion unchanged | `internal/app/nickname_sessions_test.go:15-70`; `internal/identity/application/nickname_sessions_test.go:15-89`; NE pure profile (`:653`), mixed role (`:3093`) and mixed status (`:3985`), safe responses and exact owner version transitions |
| **管理员尝试修改自己或其他管理员** (`:17`) | Shared guards `user_service.go:255-298`, production association protection as described under C5 | NE self-noop (`:2190`) and actual-admin-noop (`:2341`) stay refused with all13 business projections unchanged; disabled admin current-membership regression `user_lifecycle_test.go:141-170`; no legacy tag protection is introduced |
| **管理员提交邮箱更新** (`:21`) | `user_service.go:188-190` before no-op | `identity_http_test.go:155-194`; NE valid-format email guard (`:1870`), unchanged full13 business projections and exact field error |
| **无邮箱字段的合法资料更新** (`:27`) | DTO/route→Application path retained, accepted actual fields only | `identity_http_test.go:131-153`; CE omitted email (`:50`); NE pure nickname (`:653`) and mixed update cases supply later-policy native evidence |
| **空邮箱不表示清空邮箱** (`:32`) | Empty email is never converted into UserChanges.Email; managed guards still apply | `identity_http_test.go:131-153`, empty email subcase; CE empty email (`:78`), unchanged email/pending/verification ownership |
| **非空邮箱与其他字段混合** (`:37`) | Email prohibition precedes all difference/no-op branches | `identity_http_test.go:155-194`; NE both email classes (`:1870,:2030`) reject with all13 projections unchanged, even when the other fields match current values |
| **纯昵称变更保留会话** (`:42`) | `user_service.go:195-196,207-214,218`; no owner call/readback Ensure | Application `nickname_sessions_test.go:15-56` checks both existing7 and absent row while any attempted version mutation would fail; HTTP `internal/app/nickname_sessions_test.go:15-53,72-98` checks safe returned nickname, old access and absent-version case. NE pure change (`:653`) records2→2/raw spaces; old Access (`:805`) and old Refresh (`:1259`) remain200; never-logged nickname (`:1415`) plus absence summary (`:1721`) keep row count0 |
| **合法全同值更新** (`:47`) | `user_service.go:191-205` distinguishes accepted same facts from no fields, after guards | Same application and HTTP regressions; NE `legal-all-same-fields-preserve-version` (`:955`), old access (`:1109`), old refresh (`:1259`), never-logged all-same (`:1567`) show successful safe response, retained version and absent row; empty request stays422 (`:1730`) and out-of-scope same input stays403 despite real management grant (`:2941`) |
| **昵称与真实身份状态变化混合** (`:51`) | `user_service.go:198-211` jointly writes differences and increments exactly once using current locked user | Application `nickname_sessions_test.go:58-89` separately covers status, legacy role, both; `:91-113` proves changed current facts affect classification; HTTP `internal/app/nickname_sessions_test.go:54-69` checks exactly one increment for role+status+nickname. NE role2→3 (`:3093`) plus Access/Refresh rejection (`:3247,:3387`); status3→4 (`:3985`) plus disabled Access403/Refresh401 (`:4138,:4287`); true MySQL wait/current-winner case (`:6748`) distinguishes SQL winner0/version9 from resumed payload1/version10 |
| **主动Kick不适用资料豁免** (`:55`) | `user_service.go:221-252`: Toggle always flips current status; Kick always calls owner; Delete remains invalidating | `user_lifecycle_test.go:120-138,173-216` preserves atomicity/current-toggle/delete owner state; NE Toggle0→1/version4→5 (`:4591`), Toggle1→0/version5→6 (`:5024`), re-enable/version6→7 (`:5448`), Kick/version7→8 (`:5881`), old-pair stable refusals (`:4733,:4873,:5166,:5306,:5590,:5730,:6021,:6161`) |

### Additional design/task acceptance outside scenario headings

- **Protection priority / legacy role:** NE actual association no-op refusal (`:2341`), out-of-scope identical refusal with a real route grant (`:2941`), legacy tag set to admin (`:3093`) followed by still-manageable nickname (`:3529`), empty runtime roles/permissions (`:3690`) and denied admin route (`:3845`). This is not using `users.role` as RBAC.
- **Rollback of identity and versions:** `internal/identity/application/nickname_sessions_test.go:115-156` uses real persistence/transaction state to check profile failure and initialization+increment failure; `user_lifecycle_test.go:120-138` covers later version failures across common mutations. NE profile CHECK failure (`:6320`), existing version-write failure (`:6462`), missing-row initialization failure (`:6605`) each record HTTP500 and all13 named business projections unchanged. This is a business-projection claim, not rollback of audit logs/the whole database.
- **Current-read evidence boundary:** application test `nickname_sessions_test.go:91-113` performs a deterministic same-transaction interleaving; it is not MySQL concurrency proof. NE `current-locked-status-not-stale-pre-read` (`:6748-6902`) separately records a real identity lock wait and a native SQL winner. It does not claim a second application HTTP writer.
- **Committed mutation versus readback failure:** `user_service.go:218` occurs after managed transaction returns. NE `committed-profile-write-then-get-readback-fails` (`:6905-7052`) queues a private users-table rename after commit, records nickname committed, version10 unchanged, and existing401 mapping; fixture correction (`:7054-7068`), same operator token200 after restoration (`:7097`) and explicit conclusion (`:7247`) prevent a false rollback or invalidated-operator claim. No broader error-classification repair is required by this change.
- **Docs:** route OpenAPI description `internal/identity/adapters/http/routes.go:65-67`; native served description/safe-return case NE `:7071-7094`; `docs/modules/user-management.md:22-31` states policy and readback boundary. 13/13 tasks have source/test/evidence/delivery correspondence.


### 附录B：Navigation／API同步／S1

## Evidence key and limits

Exact JSON object/case names below identify recorded historical observations. None is a fresh run by this reviewer.

- **E1:** `docs/reviews/evidence/c1-menu-api-ids-fix-2026-10-16.json`. Real main/MySQL8.0.19 RR/auth/permissions. `.refusals[]`: mixed zero, only zero, empty, missing, disabled, public; each records six complete projections unchanged. `.duplicate_success`: one stored relation and equal menu/API codes. Historical direct service check has SQLite/fake scope only.
- **E2:** `docs/reviews/evidence/c2-menu-create-status-fix-2026-10-16.json`. `.normal_creations[]`:0/omitted/1/invalid2, response/stored status/timestamps/returned-ID read. `.ordinary_user_visibility`: disabled exclusion with enabled positive control. `.other_defaults`: fresh Seed/sync/button enabled. `.mid_version_failure_and_recovery`: first-user progress observed explicitly uncommitted, later-user native failure, complete menu/version rollback, recovery. No available SQL_TEXT statement count; single INSERT/no repair UPDATE is source evidence.
- **E3:** `docs/reviews/evidence/c3-menu-api-code-fix-2026-10-16.json`. `.native_success`: consistent direct codes, unchanged binding IDs, grant merge/dedup, once-per-user versions, actual old Access/Refresh401. `.native_failure_observations.cases[]`: API, permission-create, grant and second-user-version faults. Per-fault in-memory hashes were lost; recorded observed assertions are not fabricated reconstructed digest bytes. `.preserved_boundaries[]`: omitted/bound-empty/unbound code and referenced/unreferenced old permissions.
- **EF:** `docs/reviews/evidence/api-permission-sync-atomic-2026-10-18.json`, `.post_change_native_observations[]`. Native SIGNAL1213/1205 + MyISAM attempt witness proves driver retry classes/execution, not a physical deadlock ring or elapsed timeout. Final source epoch reran derived creation and existing skip only, not every historical fault/interleaving. Baseline known failures were not rerun here.
- **ES:** `docs/reviews/evidence/authorization-field-clearing-2026-10-18.json`, `.observations[]` with after_product_edits. Five fields' omitted/empty/replacement/null states, actual SQL/response, invalid atomic rejection, separate group resource preservation, late-version rollback and sessions. Baseline/fixture records are not credited as post-S1 clearing acceptance.
- **ER:** `docs/reviews/evidence/authorization-reference-lifecycle-2026-10-18.json`, `.native_observations[]`. Inherited current-role/menu lifecycle and controlled legal commit orderings. Reverse-role exact-key probe limitation remains explicit: expectation failed/released barrier, while completed HTTP outcomes were recorded. Corrected menu parent-first case is used instead of the historical500. Disabled registered role/permission GET cases are403 and are NOT list-pagination proof.
- **EN:** `docs/reviews/evidence/configuration-noop-2026-10-18.json`, `.observations[]`. Later approved policy: equal clears become no-ops, empty/invalid updates still reject, real direct-code/permission/target repairs still write and version. Legitimate supersession of unconditional equal-input invalidation, not removal of real-clear/repair requirements.

SQLite and collaborator fakes are credited only for their actual asserted local behavior, never native MySQL locking/retry/real ACL or production deployment proof.

## C1 — fix-menu-api-binding-id-validation

**Requirement:** `openspec/changes/archive/2026-10-20-fix-menu-api-binding-id-validation/specs/menu-management/spec.md:3-49`, 菜单 API 联动. **Tasks10/10.** N-bind owns raw validation, N-input/HTTP does not prefilter, N-prepare retains existence/enabled/auth and current decision checks. Existing global helper semantics/root0 are not changed; empty input does not become unbind. All writes and versions share the transaction context.

| Every scenario (artifact line) | Current code | Deterministic/native evidence |
|---|---|---|
| 菜单绑定 API (8) | N-bind/N-prepare/N-apply/N-repository/Composition | `internal/navigation/service_test.go:340-371`: duplicate input→one binding/equal code/permission/version. E1 duplicate_success. Actual grant-owner merge covered by C3 App/native cases below |
| 菜单绑定 API 失败 (18) | N-bind + Transactions; versions are after binding facts, in same callback | `service_test.go:405-455` local relationship/menu rollback only, not fake-owner atomicity; `internal/app/configuration_noop_test.go:129-182` complete composed before/after facts on late binding version failure; EN late-second-binding-user-failure and healthy grant migration |
| 查询菜单绑定 API (23) | `internal/navigation/service.go:374-391` real bound API query/nonnil empty result | `service_test.go:304-339` nonempty and empty assertions; E1 duplicate_success GET |
| 删除菜单清理 API 关联 (28) | N-delete/N-repository | `internal/navigation/repository_test.go:48-123`: both relationship families absent after delete; `service_test.go:101-138` leaf rules |
| 菜单权限变更实时生效 (32) | N-bind final versions; N-update for PUT | `service_test.go:340-371`; E3 native_success.old_token_rejections; EN binding migration and late-version cases |
| 混合有效API与零ID (37) | N-bind raw scan before dedup | `service_test.go:373-403` direct mixed-zero call/readback; `internal/navigation/configuration_test.go:141-189` no equal-binding bypass; E1 mixed-valid-and-zero422/all6 unchanged |
| 合法重复API ID (42) | N-bind + membership composite persistence | `service_test.go:340-371`; E1 duplicate_success one relation |
| 内部调用不能绕过原始校验 (47) | Same public Application method, not HTTP-only validation | `service_test.go:373-403` invokes AssignAPIs directly. No claim of native direct-SDK execution |

Design: smallest boundary fix; no global unique-ID rewrite or empty-unbind policy. Checked tasks have source/entrypoint/regression/evidence mappings, not merely boxes.

## C2 — fix-menu-create-status-preservation

**Requirement:** `openspec/changes/archive/2026-10-20-fix-menu-create-status-preservation/specs/menu-management/spec.md:3-55`, 菜单 CRUD. **Tasks11/11.** N-input/HTTP preserves *int; N-create defaults only nil/invalid and retains0/1. N-repository map insertion preserves0, explicit timestamps, pinned GORM ID types and nonzero returned ID without a second UPDATE. Original DDL/default metadata remains (`internal/navigation/model.go:5-16`). Seed enabled defaults remain `internal/navigation/seed.go:168-184`; sync uses explicit Status1 (`internal/navigation/service.go:334-340`), generated buttons explicit Status1 (`:710-720`).

| Every scenario (artifact line) | Current code | Deterministic/native evidence |
|---|---|---|
| 创建菜单 (8) | N-create | `service_test.go:19-99`; E2 normal_creations |
| 创建按钮菜单 (13) | N-create + N-normalize retain empty path/Type3/code | `service_test.go:340-346,691-720`; E2 other_defaults.button_generation |
| 菜单路径冲突 (18) | N-create/N-update; nullable unique path `model.go:9` | `service_test.go:19-48,101-138`; ES path-conflict-rejects-other-clear |
| 父级菜单指向自己 (22) | N-update prevalidation | `service_test.go:101-118`; configuration_test.go:53-85 |
| 修改菜单 (26) | N-update presence map/empty-request rejection/current equality after validation | `service_test.go:101-138`; configuration_test.go:53-85; ES no-fields-menus |
| 删除存在子菜单的菜单 (31) | N-delete/N-repository current children barrier | `service_test.go:101-138`; ER menu-child-create-first-refuses-parent-delete |
| 删除叶子菜单 (35) | N-delete/N-repository cleanup/hard delete | `service_test.go:101-138`; repository_test.go:48-123; ER corrected parent-first deletion |
| 显式创建停用菜单 (41) | N-create/N-repository; enabled-only repository read `repository.go:48-57` | `service_test.go:50-99`; repository_test.go:13-46 returned-ID/full-entity round trip; E2 explicit-disabled + ordinary visibility |
| 创建状态遗漏与合法启用 (45) | N-create explicit status policy | Same table test; E2 omitted and explicit-enabled |
| 无效创建状态使用既有默认 (49) | N-create only0/1 accepted as explicit | Same test above-range; E2 invalid2→1 |
| 创建事务中版本失败 (53) | N-create + Transactions + actual owner version store | E2 mid_version_failure_and_recovery: real first progress, later native3819, full menu/version rollback, recovery; not merely DTO/generated SQL |

No compatibility input alias/second INSERT repair path observed. Full new LSP reference enumeration was not performed by this reviewer; parent owns LSP.

## C3 — fix-menu-api-code-consistency

**Requirement:** `openspec/changes/archive/2026-10-20-fix-menu-api-code-consistency/specs/menu-management/spec.md:3-70`, 菜单 API 联动. **Tasks13/13.** N-update owns one outer transaction, reads/locks actual direct dependencies, applies ordinary fields and N-apply, and reads final menu for response. No full nested AssignAPIs call or duplicated version trigger. N-apply has precisely two real use-case callers. N-merge-cleanup retains existing merge/owner SQL cleanup; no S3 transitive propagation or historical batch repair.

**All eight inherited scenario occurrences:** 菜单绑定 API(10), 菜单绑定 API 失败(20), 查询菜单绑定 API(25), 删除菜单清理 API 关联(30), 菜单权限变更实时生效(34), 混合有效API与零ID(39), 合法重复API ID(44), 内部调用不能绕过原始校验(49) each maps to its identically named C1 row above, including that row's current source/test/native refs. This explicitly maps all eight full-text occurrences.

| Four additional scenarios (line) | Current code | Deterministic/native evidence |
|---|---|---|
| 菜单侧修改已绑定权限码 (53) | N-update/N-prepare/N-apply; no code-only membership replacement; final response and once-per-user all-user trigger | `internal/app/authorization_http_test.go:172-369`: two bound APIs, old-only/overlap/destination-only grants, dedup, IDs and obsolete cleanup. E3 native_success actual codes/IDs/grants/versions/sessions |
| 菜单侧联动中途失败 (58) | All ordinary/direct-code/permission/grant/version work shares Context transaction | Same App test actual API SQL trigger/persisted HTTP reads. E3 native_failure_observations API/create/grant/second-version faults, all6 rollback with recorded hash-loss qualification |
| 未绑定菜单改码 (63) | N-update allows zero direct targets, normal ordinary field semantics, no synthetic relation | `service_test.go:232-271`; E3 unbound-code-replacement, unbound-code-clear, permission-code-omitted |
| 绑定菜单显式空码沿用既有解析 (67) | N-prepare + N-resolve pre/current resolved-code equality | `service_test.go:140-230`: existing code, derivation, conflicting existing API codes, rollback and unchanged target IDs. E3 bound-explicit-empty-reuses-existing-code / bound-empty-conflicting-existing-codes |

### Later no-op interaction

N-equality excludes bound code only from the ordinary field write, not from N-apply. N-apply checks **all direct API/menu codes plus live owner permission** before no-op; missing/tombstoned permission and code drift remain real repairs. `internal/app/configuration_noop_test.go:93-127` checks repair/version and subsequent no-op. EN same-inputs-repair-direct-api-code-PUT, same-binding-restores-missing-live-permission, same-binding-creates-missing-permission-resource corroborate. Empty request still rejects before equality; `configuration_test.go:141-189` retains invalid API-state/ref checks. Icon-only updates do not invent rebinding prerequisites. Equal legal empty clears may no-op under later approval; real clears do not.

## F2 — make-api-permission-sync-atomic

**Requirement:** `openspec/changes/archive/2026-10-20-make-api-permission-sync-atomic/specs/api-management/spec.md:3-34`, API 权限同步. **Tasks15/15.**

### Requirement/design trace

- API-sync contains candidate read and the entire original loop within the required Core outer Runner. Composition installs the same Runner in Core and Navigation; route-source option supplies only routes. API-owner, N-repository and A-code-facts use ctx-derived connections; Transactions joins inner Run instead of committing independently.
- New target/context boundary explicitly traced: Composition produces APIPermissionSyncTarget; API-preparation consumes every applicable target, retains roots and passes the prepared context into the original callback; N-dispatch explicitly selects prepared handling. No unmatched routing or silent new-value drop.
- API-preparation discovers candidate/direct dependencies plus cleanup-only possible old derived codes before acquisition, locks permission/API/menu unions in order, current-rechecks candidates/relations, and prelocks only actual potential users' existing versions. A-versions LockExisting does not initialize/increment. No full unrelated API mutex or generic graph engine.
- API-owner keeps original `api_group,sort,id` business order, independent of ascending-ID lock order. Prepared propagation/current checks only accept the attempt's bounded direct graph; inspection-only references do not become mutation or user cohorts. Current cleanup still uses owner SQL comparison facts, not Go strings as a substitute for MySQL collation.
- N-api-change preserves each actual API mutation's own user-version trigger. Same user may increment twice; no request-level dedup. Skip-only preparation cannot Ensure/Increment missing versions.
- API-sync creates codeStates/attemptCreated/attemptUpdated inside callback. A-code-facts before includes tombstones; after excludes deleted. Coordination-created permissions count, restores do not, created-then-cleaned permissions do not survive into created. Errors return nil/0. `internal/apimetadata/adapters/http/routes.go:237-248` exits on error before publishing stats; updated_api remains empty-code-candidate count, not indirect write count.
- Transactions retains existing retry count/classes and queues external hooks only for the successful outer attempt. Optional Navigation cache invalidation uses AfterCommit; production external cache wiring is not claimed.

| Every scenario (line) | Current deterministic evidence | Applicable native EF cases |
|---|---|---|
| 同步 API 权限码 (10) | `internal/app/api_permission_sync_test.go:57-121`: generated/explicit/shared/existing/restored/public, exact facts and repeat skip; current code API-sync/API-preparation/A-code-facts | mixed-success-with-existing-and-public: actual10/reported10, updated9. final-source-original-main-derived-path / final-source-original-main-existing-path are final-build smoke |
| 第二项失败回滚第一项 (17) | `api_permission_sync_test.go:252-303`: actual SQL terminal failures at Core/permission/menu/linked API/grant insertion/removal/later version/cleanup; actual intended error, whole owner projection equality, nil/0 result. Transactions/N-api-change | second-core-check-rolls-back-first-change; second-permission-trigger-rolls-back-first-change; second-real-grant-merge-rolls-back-earlier-propagation-and-versions; second-same-user-version-failure-rolls-back-whole-request |
| 正常skip与整请求成功 (22) | `api_permission_sync_test.go:57-121`: exact retained/restored resources/no invented users/repeat unchanged. API-sync + A-versions | mixed-success-with-existing-and-public; already-configured-sync-does-not-write-or-initialize-missing-version; missing-unrelated-version-skip |
| 成功attempt统计不重复 (26) | Callback-local API-sync publish-on-success and Transactions source; SQLite not credited as native retry proof | native-1213-first-attempt-rollback-then-success:2 attempts/final created2 updated2. native-1205-exhausted-retries-no-request-writes:3 attempts/500/no request writes. SIGNAL driver-class evidence, not physical deadlock/timeout |
| 同用户多项API联动保留逐项触发 (31) | `api_permission_sync_test.go:180-249`: same user10→12, applicable missing-version initialization/increments, unrelated unchanged, original shared-menu order/net final creations; later-version rollback case. N-api-change | two-actual-api-changes-retain-per-api-version-triggers; per-api-version-user-ruling10→12; second-same-user failure; old-user Access/Refresh401; shared-direct-menu-preserves-existing-business-order created1/updated2 |

R6 native bounded proof: EF whole-request-acquires-lower-api-before-higher-business-first records wait on143 before higher business-first144 held and preserves business output order. current-candidate-recheck-preserves-winning-api-change records lower145 wait, concurrent146 NeedAuth winner, loser409 preserving winner/no writes. Not proof of all optimizer/scheduler cases.

MenuAPI/RoleMenu membership is preserved, not written by F2; no invented membership-write fault demanded. No S3/S4/S5, Seed redesign, route/menu sync unification, new retry promise or minimal-user invalidation target inferred.

## S1 — support-explicit-authorization-field-clearing

**Requirements:** `openspec/changes/archive/2026-10-20-support-explicit-authorization-field-clearing/specs/menu-management/spec.md:3-77` 菜单 CRUD; `specs/rbac/spec.md:3-57` 角色 CRUD and `:59-96` 权限 CRUD. **Tasks12/12.**

### Requirement/design adherence

- Menu: N-input/HTTP forwards *string untouched; N-update explicit map preserves omitted/null, accepts legal empty Icon/Component/path, rejects empty/blank Name before writes, N-normalize maps empty path to nil, nonempty conflicts still checked. Independent PermissionCode keeps existing binding resolver; no new clear/unbind policy.
- Role: A-HTTP/schema forwards local pointers. A-role-update locks/protects/rejects empty/blank required identifiers and empty requests, retains uniqueness, applies present Description even empty through corrected owner map126-131. Real changes version actual members inside same transaction; later equality is after validation.
- Permission: A-permission-update uses corrected owner map493-497 to write name/group/sort only, never code or PermissionGroup deletion, and introduces no new label-based version trigger.
- Existing three route descriptions distinguish omitted/empty/null; A-HTTP/schema uses existing reflection to project optional/nullable string and min/max lengths. ES actual-route-catalog-put-schemas checks the real served document, not merely the generator implementation.
- No generic Optional/PATCH abstraction or unrelated Identity/API/dictionary/organization policy. Later approved no-op on already-empty field supersedes unconditional equal-input invalidation; **real clearing still mutates and versions**.

### All15 Menu scenarios

The first11 are repeated C2 clauses. Every row names its current mapping so repeated scenarios remain counted.

| Scenario (S1 menu line) | Current code/test/evidence mapping |
|---|---|
| 创建菜单 (12) | N-create; `service_test.go:19-99`; E2 normal_creations |
| 创建按钮菜单 (17) | N-create/N-normalize; `service_test.go:340-346,691-720`; E2 generated button |
| 菜单路径冲突 (22) | N-create/N-update; `service_test.go:19-48,101-138`; ES path-conflict-rejects-other-clear |
| 父级菜单指向自己 (26) | N-update prevalidation; `service_test.go:101-118`; configuration_test.go:53-85 |
| 修改菜单 (30) | N-update; `service_test.go:101-138`; ES no-fields-menus |
| 删除存在子菜单的菜单 (35) | N-delete/N-repository; `service_test.go:101-138`; ER child-first refusal |
| 删除叶子菜单 (39) | N-delete/N-repository; repository_test.go:48-123; ER corrected parent-first case |
| 显式创建停用菜单 (45) | N-create/N-repository; `service_test.go:50-99`; E2 explicit-disabled/visibility |
| 创建状态遗漏与合法启用 (49) | N-create; same test; E2 omitted/explicit1 |
| 无效创建状态使用既有默认 (53) | N-create; same test; E2 invalid2→1 |
| 创建事务中版本失败 (57) | N-create/Transactions; E2 late-version first-progress/failure/full rollback/recovery |
| 菜单删除与子菜单创建交错 (61) | N-lock-parent/N-delete/N-repository current barrier; `service_test.go:1063-1107` stale-parent references reject. ER menu-child-create-first-refuses-parent-delete and corrected menu-parent-delete-first-child-not-found-after-classification-fix |
| 菜单可选属性清空与遗漏分开 (66) | N-update pointer map; `internal/app/authorization_field_clearing_test.go:120-178` separate Icon/Component four-state response/SQL/relations/version checks; ES icon-* and component-* after-product cases |
| 合法路径清空 (70) | N-update/N-normalize; same test path case asserts SQL NULL plus replacement/null behavior; ES path-omitted/path-empty-alone/path-replacement/path-null-with-real-other-change and real-bound-menu-display-clear-keeps-relations-and-version-policy |
| 非法清空不部分提交 (75) | N-update validates before writes; `authorization_field_clearing_test.go:180-208` full state for empty/blank name/conflict/no fields, `:210-238` late-version rollback; `internal/navigation/field_clearing_test.go:11-33` direct Application rejection. ES corresponding illegal/all13-unchanged and bound-menu late-version cases |

### All11 Role scenarios — corrected source anchors

| Scenario (S1 RBAC line) | Current source | Deterministic/native evidence |
|---|---|---|
| 创建角色 (10) | A-role-create | `internal/app/authorization_http_test.go:49-82` actual create; ES fixture-role supports successful native creation fields only, not post-S1 clear |
| 查询角色列表 (14) | A-role-list | **Source-only; W1.** No current ordered Role-page assertion found. Native rejected GET403 is not successful list proof |
| 修改角色 (18) | A-role-update, owner map126-131 | `authorization_http_test.go:49-82` Name update; `internal/authorization/application/service_test.go:237-284` versions; ES real Description update/sessions; EN real display invalidation |
| 超级管理员角色受保护 (23) | A-role-update/A-role-delete protection independent of status | `service_test.go:286-303`; ES admin-role-remains-protected-from-description-clear; EN protected equal-role409 |
| 删除角色 (27) | A-role-delete: Service219-240, owner133-144, actual member read147-153 | `internal/authorization/application/reference_lifecycle_test.go:34-120`; ER role-delete-healthy-preserves-shared-menu-and-permission |
| 普通角色跨模块清理 (33) | A-role-delete invokes Navigation port before owner delete/version | `reference_lifecycle_test.go:34-120`: four mapping families/shared resources/second-version rollback; ER role-delete-second-version-3819-rolls-back-all-four-mappings, healthy deletion and ordinary-role-deletion-shared-resources-and-version-history |
| 绑定先提交后合法删除 (38) | A-role-delete locked role/current actual UserIDsByRole147-153 | `reference_lifecycle_test.go:229-262` controlled callback includes newly bound member/expected3; ER role-bind-first-delete-current-new-member both200/new actual member22 |
| 删除先提交后新增绑定 (43) | A-role-bind: bulk242-320/per-user starts321; role-menu path `internal/navigation/service.go:218-271` also locks role | `reference_lifecycle_test.go:229-262` reverse bulk/per-user refusal/unchanged version; ER role-delete-first-later-bind-refused200/404 with exact-key probe limitation |
| 角色说明显式清空 (47) | A-role-update presence setter/owner126-131 | `authorization_field_clearing_test.go:120-178` description four-state; ES description-empty-alone/replacement/real healthy clear; `:210-238` and ES role late-member failure rollback |
| 角色说明遗漏保持 (51) | A-role-update nil skips setter | Same four-state test; ES description-omitted and description-null-with-real-other-change |
| 拒绝清空角色必填标识 (55) | A-role-update required checks before persist | `internal/authorization/application/field_clearing_test.go:11-36` direct Name/Code empty+Description rejects/full role equal; App invalid tests180-208; ES emptyName/Code400, whitespace422/all13 unchanged |

### All7 Permission scenarios — corrected source anchors

| Scenario (S1 RBAC line) | Current source | Deterministic/native evidence |
|---|---|---|
| 创建权限 (66) | A-permission-create Service565-582 | `internal/authorization/adapters/gorm/repository_test.go:16-40` owner create; App C3 actual HTTP creates `authorization_http_test.go:210-215`; ES fixture-permission native creation |
| 查询权限列表 (70) | A-permission-list Service555-563/owner446-458 | `repository_test.go:41-44` page size+total only, **not order**; App C3 permission-list comparison is by map. W1 |
| 修改权限 (74) | A-permission-update Service584-616/owner493-497 excludes code | `authorization_field_clearing_test.go:120-178` Group/sort transitions/identity+Code unchanged; ES group transitions/owner SQL |
| 删除权限 (79) | A-permission-delete Service618-637/owner499-505 | `authorization/application/service_test.go:237-284` affected versions; reference_lifecycle_test.go:264-303 relationship/dependent versions |
| 权限删除与新角色引用交错 (85) | A-permission-delete/bind shared target lock; Service618-674 | `reference_lifecycle_test.go:264-303` controlled binding-before-delete includes current dependent; later missing permission assignment refuses/no version leftovers. Deterministic protocol evidence, not native simultaneous-lock proof |
| 清空权限分组标签 (90) | A-permission-update Group setter/owner493-497, no group-resource delete | `authorization_field_clearing_test.go:120-178`; ES group-empty-alone, label-clear-keeps-permission-and-group-resources and exact before/after group listing/permission identity |
| 拒绝清空权限必填名称 (94) | A-permission-update validates explicit empty/blank name before persistence | `authorization/application/field_clearing_test.go:38-54` direct Name empty+Group clear rejects/full entity equal; App invalid tests; ES empty400/blank422/all13 unchanged |

All five optional field four-state transitions, four required identifiers, legal path NULL, path conflict, real Role/Menu version changes and rollback have applicable evidence. S1 does not require a new Permission-label invalidation trigger or null semantics.


### 附录C：保护／生命周期／配置no-op／成员差量



Source aliases (all paths repo-relative):

- **A** = `internal/authorization/application/service.go`; **AR** = `internal/authorization/adapters/gorm/repository.go`.
- **N** = `internal/navigation/service.go`; **NR** = `internal/navigation/repository.go`; **NC** = `internal/navigation/configuration.go`.
- **O** = `internal/organization/service.go`; **OR** = `internal/organization/repository.go`; **U** = `internal/organization/user_memberships.go`.
- **I** = `internal/identity/application/user_service.go`; **IR** = `internal/identity/adapters/gorm/repository.go`.
- **AV** = `internal/authorization/adapters/gorm/access_version.go`; **TX** = `internal/platform/database/transaction.go`.

Test aliases:

- **AP** = `internal/authorization/application/protected_assignments_test.go`.
- **AL** = `internal/authorization/application/reference_lifecycle_test.go`.
- **AS** = `internal/authorization/application/service_test.go`.
- **AN** = `internal/authorization/application/noop_configuration_test.go`.
- **AD** = `internal/authorization/application/membership_delta_test.go`.
- **ARS** = `internal/authorization/application/reference_scope_test.go`.
- **ART** = `internal/authorization/adapters/gorm/repository_test.go`.
- **NT** = `internal/navigation/service_test.go`; **NCT** = `internal/navigation/configuration_test.go`.
- **OM** = `internal/organization/membership_test.go`; **UM** = `internal/organization/user_memberships_test.go`.
- **OL** = `internal/organization/reference_lifecycle_test.go`.
- **AH** = `internal/app/authorization_http_test.go`; **OH** = `internal/app/organization_http_test.go`.
- **OMH** = `internal/app/organization_membership_http_test.go`.
- **CH** = `internal/app/configuration_noop_test.go`; **FC** = `internal/app/authorization_field_clearing_test.go`.

Historical native-evidence aliases (not current-run results):

- **P** = `docs/reviews/evidence/protected-authorization-assignments-2026-10-17.json`, `.native_observations[]` selected by exact `case`.
- **L** = `docs/reviews/evidence/authorization-reference-lifecycle-2026-10-18.json`, `.native_observations[]` selected by exact `case`.
- **C** = `docs/reviews/evidence/configuration-noop-2026-10-18.json`, `.observations[]`; only `phase=after` or explicitly post-change session/invariant observations are acceptance evidence.
- **M** = `docs/reviews/evidence/membership-delta-2026-10-18.json`, `.observations[]`; only `phase=after` observations are acceptance evidence.
- **C2** = `docs/reviews/evidence/c2-menu-create-status-fix-2026-10-16.json`.
- **C3** = `docs/reviews/evidence/c3-menu-api-code-fix-2026-10-16.json`.

An exact named case/JSON selector is an evidence reference, not an assertion that every other case in that JSON applies. Tests were inspected, not executed here. SQLite fixtures establish persisted projections/rollback/local policy behavior, not InnoDB locking or production authentication.

## Per-change requirement/scenario manifests

### 1. enforce-protected-authorization-assignments

Artifacts: `openspec/changes/archive/2026-10-20-enforce-protected-authorization-assignments/{proposal.md,design.md,tasks.md}` and all three `specs/*/spec.md`.

| Capability / requirement (spec line) | Requirement implementation | Every scenario, in artifact order |
|---|---|---|
| rbac / 用户角色分配 (`rbac/spec.md:3`) | internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go; App authorization/identity facts | RU1,RU2,RU3,RU4,RU5 |
| rbac / 角色权限分配 (`rbac/spec.md:31`) | internal/authorization/application/service.go; AR permission target locks/current raw relation reads | RP1,RP2,RP3,RP4 |
| rbac / 角色数据权限 (`rbac/spec.md:55`) | internal/authorization/application/service.go; hierarchy/reference checks before comparison | RS1,RS2,RS3,RS4,RS5,RS6 |
| rbac / 管理端受保护身份与角色关联 (`rbac/spec.md:89`) | internal/authorization/application/service.go; internal/identity/application/user_service.go; App identity/organization adapters | RI1,RI2 |
| menu-management / 角色菜单分配 (`menu-management/spec.md:3`) | internal/navigation/service.go; App navigation role fact adapter:107-135 | RM1,RM2,RM3,RM4,RM5 |
| menu-management / 菜单 API 联动 (`menu-management/spec.md:32`) | internal/navigation/service.go; ordered lock/recheck:475-554,801-837 | MA1-MA13 |
| organization-management / 组织成员绑定 (`organization-management/spec.md:3`) | internal/organization/service.go; App organization policy:47-80 | OB1-OB6 |
| organization-management / 组织单位 CRUD (`organization-management/spec.md:35`) | internal/organization/service.go | OC1-OC4 |

Key design adherence: actual persisted existing `admin` role associations, without status filtering, feed management protection. Runtime `RolesForUser` remains status-filtered (internal/authorization/adapters/gorm/repository.go), so disabled admin protection does not grant runtime privilege. Legacy `users.role` is not used for RBAC protection. Operator IDs reach bulk owner use cases (`authorization/adapters/http/roles.go:165`, `organization/http.go:146`), zero is rejected by owners. Organization retained self/admin links are distinct from changed links; role full-replacement protection is intentionally stricter. Permission has no new state. Global shared configuration does not acquire an invented organization-management policy. P's old union-invalidation and soft-deleted-old-member refusal observations are superseded by the later approved changes, not current regressions.

### 2. enforce-authorization-reference-lifecycle

Artifacts: `openspec/changes/archive/2026-10-20-enforce-authorization-reference-lifecycle/{proposal.md,design.md,tasks.md}` and all four `specs/*/spec.md`.

| Capability / requirement (spec line) | Requirement implementation | Every scenario, in artifact order |
|---|---|---|
| rbac / 角色 CRUD (`rbac/spec.md:3`) | internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go; required Navigation role-menu capability | RC1-RC8 |
| rbac / 角色数据权限 (`rbac/spec.md:45`) | internal/authorization/application/service.go; internal/organization/service.go | RS1-RS9 |
| rbac / 权限 CRUD (`rbac/spec.md:91`) | internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go onward; permission/role locking barriers | PC1-PC5 |
| menu-management / 菜单 CRUD (`menu-management/spec.md:3`) | internal/navigation/service.go; internal/navigation/repository.go; first ordered target+child acquisition | MC1-MC12 |
| menu-management / 菜单 API 联动 (`menu-management/spec.md:64`) | internal/navigation/service.go; API Core:109-123,143-252 | MA1-MA16 |
| organization-management / 组织单位 CRUD (`organization-management/spec.md:3`) | internal/organization/service.go; internal/authorization/application/service.go | OC1-OC6 |
| organization-management / 组织成员绑定 (`organization-management/spec.md:35`) | internal/organization/service.go; internal/identity/adapters/gorm/repository.go; current role/management facts | OB1-OB8 |
| organization-management / 逐人修改组织归属 (`organization-management/spec.md:77`) | internal/organization/user_memberships.go; internal/organization/repository.go | OU1-OU9 |
| user-management / 逐人修改角色归属 (`user-management/spec.md:3`) | internal/authorization/application/service.go; internal/authorization/adapters/gorm/access_version.go | UR1-UR6 |
| user-management / 软删旧角色成员的受控清理 (`user-management/spec.md:37`) | internal/authorization/application/service.go; safe deleted facts and same transaction versions | SD1-SD2 |

Key design adherence: required caller-owned `RoleMenuLifecycle` is injected by App (`internal/app/authorization.go`), consumed by `DeleteRole` before entity deletion and version increments, and implemented through context-bound Navigation persistence (`internal/navigation/repository.go`). Four relation families/entity/versions therefore share TX, not a cross-module GORM model. Custom creation and organization deletion enter the hierarchy barrier; disabled/memberless custom roles still block deletion, while absent/noncustom residual roles are not treated as live custom. Identity mutations lock current identity and actual role dependencies before scope/protection checks (internal/identity/application/user_service.go, internal/identity/adapters/gorm/repository.go). Deleted users can only be actually removed through the controlled old-member path; selecting/retaining them remains invalid. Version records are never deleted by these cleanup paths.

All inspected production consumers are wired: role/menu facts in `internal/app/navigation.go`; managed-user facts in `internal/app/authorization.go`; Organization policy in `internal/app/organization.go`; Identity access policy in `internal/app/identity.go`; API lock adapter and conflict translation in `internal/app/navigation.go`. API Service dispatch (`internal/apimetadata/application/service.go`) routes permission writes/deletes to the coordinator; status-only writes route to Core, whose current API lock is the same barrier. No silent producer-only contract was accepted.

Strict dynamic API ruling: internal/navigation/service.go prediscovers direct/menu dependencies, internal/navigation/service.go prediscovers API-side dependencies, internal/navigation/service.go locks permission plus complete API/menu inspection unions then rechecks. `internal/apimetadata/adapters/gorm/repository.go` and internal/navigation/repository.go acquire each union ordered by ID. No new lower-ID API is appended after dependency-set changes; Menu returns NAV_CONFLICT and App maps the API cause to API_META_CONFLICT. Cleanup inspections do not enlarge propagation/invalidation. This is bounded by the native strict-order case described below, not a claim about every optimizer plan.

Seed participants were inspected: `authorization/adapters/gorm/seed.go:51-82,115-143`, `identity/adapters/gorm/seed.go:25-77,101-109`, `navigation/seed.go:25-66,70-139,143-169`; ordinary administrator protection is not used as a Seed bypass. Existing-target/link barriers are retained. No Seed whole-round atomicity or historical orphan repair is claimed.

internal/platform/database/transaction.go joins nested work and owns retries; internal/platform/database/transaction.go queues after-commit effects; `finish` and context activity prevent failed-attempt/outer-rollback effects and reuse of completed transactions. internal/navigation/service_test.go proves nested optional cache scheduling locally. This does not claim production optional cache wiring.

### 3. preserve-sessions-on-noop-configuration

Artifacts: `openspec/changes/archive/2026-10-20-preserve-sessions-on-noop-configuration/{proposal.md,design.md,tasks.md}` and both `specs/*/spec.md`.

| Capability / requirement (spec line) | Requirement implementation | Every scenario, in artifact order |
|---|---|---|
| rbac / 角色 CRUD (`rbac/spec.md:3`) | internal/authorization/application/service.go; raw persisted current role/explicit fields and residual scope compare | RC1-RC12 |
| rbac / 角色数据权限 (`rbac/spec.md:66`) | internal/authorization/application/service.go; raw current stored scope relation comparison | RS1-RS11 |
| rbac / 角色权限分配 (`rbac/spec.md:124`) | internal/authorization/application/service.go; raw permission references, not filtered effective permissions | RP1-RP6 |
| menu-management / 菜单 CRUD (`menu-management/spec.md:3`) | internal/navigation/service.go; internal/navigation/configuration.go; linked-code repair still runs | MC1-MC17 |
| menu-management / 角色菜单分配 (`menu-management/spec.md:89`) | internal/navigation/service.go; explicit configuration set, not UserMenus | RM1-RM6 |
| menu-management / 菜单 API 联动 (`menu-management/spec.md:126`) | internal/navigation/service.go; API IDs, final resolved codes and live permission fact compared | MA1-MA18 |
| menu-management / 前端路由同步菜单 (`menu-management/spec.md:239`) | internal/navigation/service.go; callback-local created reset and all-skip early return | MS1-MS3 |

Key design adherence: guard checks precede equality success; no RowsAffected/effective-permission global diff is introduced. Role structs and scope/permission sets compare current stored facts. `applyAPIBinding` does not call same DTO a no-op if direct API codes differ or the live Permission is missing/deleted. Same-code grants are not invented: the existing merge is inapplicable when old and new code are equal. Real menu/role display changes retain the existing broader invalidation cohorts. `SyncMenus.created`, binding `changed`/affected IDs, and field-change maps are recalculated per attempt. API F2's per-API increments are not deduplicated by this change; no S3/S4/S5/I3 final-graph policy was inferred.

### 4. invalidate-only-changed-memberships

Artifacts: `openspec/changes/archive/2026-10-20-invalidate-only-changed-memberships/{proposal.md,design.md,tasks.md}` and all three `specs/*/spec.md`.

| Capability / requirement (spec line) | Requirement implementation | Every scenario, in artifact order |
|---|---|---|
| rbac / 用户角色分配 (`rbac/spec.md:3`) | internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go; membershipChanges:1171-1194 | RU1-RU7 |
| organization-management / 组织成员绑定 (`organization-management/spec.md:3`) | internal/organization/service.go; internal/organization/repository.go | OB1-OB10 |
| organization-management / 逐人修改组织归属 (`organization-management/spec.md:55`) | internal/organization/user_memberships.go; internal/organization/repository.go | OU1-OU13 |
| user-management / 逐人修改角色归属 (`user-management/spec.md:3`) | internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go | UR1-UR9 |

Key design adherence: bulk requests validate raw targets plus old references before delta exemption; only added/removed users enter increments. Bulk equality does not initialize versions. Single-user requests Ensure+lock and compare expected version before equality; legal equality returns current without EnsureAndIncrement, rejection rolls initialization back. Organization single-user equality compares only manageable existing membership IDs; hidden rows/physical created_at remain untouched. Delta inserts alone receive a new UTC timestamp. No member-write path calls `HierarchyDependentUserIDs`; the I4 topology consumer is confined to actual changed parent writes at internal/organization/service.go. Thus integrated I4 does not add range-reader invalidation to pure membership changes.

## Shared complete scenario catalog

Each entry gives exact artifact scenario title followed by current code and inspected deterministic/native support. Ranges in a manifest (e.g. MA1-MA13) are inclusive, not omitted scenarios.

### RU — 用户角色分配 (7 unique scenarios)

1. **RU1 分配用户到角色** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go. internal/authorization/application/protected_assignments_test.go; internal/authorization/application/membership_delta_test.go. M case `bulk-role-ab-to-bc`. Old P/L union wording is superseded: retained B is neither rewritten nor invalidated.
2. **RU2 查询角色用户** — internal/authorization/application/service.go and directory projection; internal/app/authorization_http_test.go asserts populated safe avatars and non-null empty list. internal/authorization/adapters/gorm/repository_test.go supports ordered raw IDs.
3. **RU3 批量分配包含禁止目标** — internal/authorization/application/service.go. internal/authorization/application/protected_assignments_test.go covers zero/missing/deleted/self/admin/outside, selected, removed, retained and equal variants, plus zero operator. P cases `role-users-raw-zero`, `role-users-clear-removed-outside-scope`; M `equal-role-group-still-rejects-protected-old-target` and `complete-role-removal-cannot-filter-outside-old-user`.
4. **RU4 直接编辑admin角色成员** — internal/authorization/application/service.go, preceding no-op. internal/authorization/application/protected_assignments_test.go; P `admin-role-users-same-existing`, `admin-role-users-clear`.
5. **RU5 停用普通用户可预配置** — internal/authorization/application/service.go does not impose Status; runtime internal/authorization/adapters/gorm/repository.go filters roles. internal/authorization/application/protected_assignments_test.go; P `disabled-role-and-disabled-user-membership-preconfiguration`, `disabled-user-remains-unable-to-login-after-preconfiguration`.
6. **RU6 角色成员差量失效** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go. internal/authorization/application/membership_delta_test.go; M `bulk-role-ab-to-bc`, `retained-b-old-pair-remains-valid`, `retained-reader-old-pair-remains-valid`.
7. **RU7 角色成员等价集合** — internal/authorization/application/service.go. internal/authorization/application/membership_delta_test.go (writes forbidden, absent version stays absent, repeat equality); M `equivalent-bulk-role`, `bulk-role-noop-does-not-initialize-never-logged`.

### RP — 角色权限分配 (6)

1. **RP1 分配权限到角色** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go onward. internal/authorization/application/protected_assignments_test.go; internal/authorization/application/noop_configuration_test.go asserts raw deleted-reference removal/second-user rollback; C `real-role-permission-addition`, `real-role-permission-removal`. Equality invalidation wording is superseded by N.
2. **RP2 查询角色权限** — internal/authorization/application/service.go; AR PermissionsByRole returns sort/id-ordered live permissions. internal/authorization/adapters/gorm/repository_test.go populated projection; internal/authorization/application/reference_lifecycle_test.go empty application response. The tests exercise populated/empty cases; ordering itself is established by inspected owner query, not a multi-row ordering assertion.
3. **RP3 非法权限ID整次拒绝** — internal/authorization/application/service.go. internal/authorization/application/protected_assignments_test.go; internal/authorization/application/noop_configuration_test.go includes a deleted same-looking reference; P `permission-zero`, `permission-missing`.
4. **RP4 admin权限关联受保护** — internal/authorization/application/service.go; internal/authorization/application/protected_assignments_test.go. P `admin-permission-clear`; controlled Permission deletion/Seed exceptions are separate paths (internal/authorization/application/service.go; auth seed:115-143).
5. **RP5 等价角色权限集合** — internal/authorization/application/service.go raw current IDs. internal/authorization/application/noop_configuration_test.go (including empty set); internal/app/configuration_noop_test.go; C `equivalent-role-permissions`, `equivalent-reordered-permissions`, `write-forbidden-equal-permissions`.
6. **RP6 等价请求仍不得绕过保护** — internal/authorization/application/service.go before 665. internal/authorization/application/protected_assignments_test.go; internal/authorization/application/noop_configuration_test.go; C `protected-equivalent-permission-set`, `invalid-zero-in-same-looking-permissions`.

### RS — 角色数据权限 (11)

1. **RS1 配置角色数据范围** — internal/authorization/application/service.go, internal/authorization/adapters/gorm/repository.go. internal/authorization/application/protected_assignments_test.go; internal/authorization/application/reference_scope_test.go; internal/authorization/application/service_test.go. P `scoped-actor-global-data-scope-configuration`; C `enter-custom-scope`. Only true fact changes now invalidate.
2. **RS2 查询角色数据范围** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go. internal/authorization/adapters/gorm/repository_test.go and internal/authorization/application/reference_lifecycle_test.go inspect exact mode/IDs after both exits; this is a deterministic public service/repository fact check, not a native ACL claim.
3. **RS3 超级管理员数据范围受保护** — internal/authorization/application/service.go; internal/authorization/application/protected_assignments_test.go; internal/authorization/application/service_test.go. P `admin-data-scope`.
4. **RS4 拒绝空custom和原始零ID** — internal/authorization/application/service.go. internal/authorization/application/protected_assignments_test.go; P `custom-empty`, `custom-zero`, `custom-missing`; C `empty-custom-does-not-become-noop`.
5. **RS5 拒绝非custom的不适用列表** — internal/authorization/application/service.go before normalization. internal/authorization/application/protected_assignments_test.go; P `noncustom-nonempty`, `noncustom-only-zero`; C `noncustom-same-mode-invalid-org-list`.
6. **RS6 停用普通角色可配置** — internal/authorization/application/service.go has no role-status rejection; internal/authorization/application/protected_assignments_test.go; internal/authorization/application/reference_lifecycle_test.go; P `disabled-role-permission-preconfiguration` and `scoped-actor-global-data-scope-configuration`. Runtime remains filtered.
7. **RS7 两个入口离开custom均清理** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go. internal/authorization/application/reference_lifecycle_test.go covers both entries/all four noncustom modes; internal/authorization/application/reference_scope_test.go rollback. L `ordinary-role-update-leaves-custom-and-clears-reference`, `dedicated-scope-exit-clears-reference`.
8. **RS8 custom先提交阻止组织删除** — internal/authorization/application/service.go; internal/organization/service.go. internal/authorization/application/reference_scope_test.go includes inactive/memberless and noncustom/deleted/protected residual distinctions; L `custom-assignment-first-current-reference-blocks-organization-delete` and `inactive-memberless-custom-role-still-blocks-org-delete`.
9. **RS9 组织先删除则custom拒绝** — internal/authorization/application/service.go; internal/organization/repository.go; ScopeReader locked hierarchy below. L `organization-delete-first-current-graph-refuses-custom-binding` (200 delete,422 custom, zero refs and self mode retained).
10. **RS10 等价数据范围配置** — internal/authorization/application/service.go. internal/authorization/application/noop_configuration_test.go populated/reordered custom and noncustom equality; C `equivalent-custom-deduplicated-set`, `same-noncustom-scope`, `write-forbidden-equal-scope`.
11. **RS11 相同字段但需清理旧custom关联** — internal/authorization/application/service.go. internal/authorization/application/noop_configuration_test.go both entries with second-user failure; C `same-noncustom-cleans-residual-ordinary-role`, `same-noncustom-cleans-residual-dedicated-scope` and each subsequent no-op case.

### RI — 管理端受保护身份与角色关联 (2)

1. **RI1 停用admin角色不解除身份保护** — internal/authorization/application/service.go, internal/authorization/adapters/gorm/repository.go do not filter status; internal/identity/application/user_service.go; `internal/app/identity.go`; `internal/app/organization.go`. internal/authorization/application/protected_assignments_test.go. P `identity-disabled-admin-protection`, `role-disabled-admin-protection`, `per-user-role-disabled-admin-protection`, `org-new-disabled-admin-protection`, `org-delete-disabled-admin-protection`, `per-user-org-disabled-admin-protection`, and `disabled-admin-association-is-protection-not-runtime-authorization` directly exercise all three owners and runtime distinction.
2. **RI2 允许非保护共享配置间接影响操作者** — internal/authorization/application/service.go and internal/navigation/service.go deliberately do not apply direct-person self prohibition. internal/authorization/application/protected_assignments_test.go covers protected members indirectly affected. P `ordinary-shared-role-permission-may-invalidate-operator`, `ordinary-shared-role-menu-may-invalidate-operator`, plus each old-actor-session case.

### RM — 角色菜单分配 (6)

1. **RM1 分配菜单到角色** — internal/navigation/service.go; internal/navigation/repository.go. internal/navigation/service_test.go; internal/navigation/configuration_test.go includes actual removal; C `real-role-menu-addition-including-disabled`.
2. **RM2 查询角色菜单** — internal/navigation/service.go; internal/navigation/repository.go sorted menus. internal/navigation/service_test.go asserts sorted flat assigned menus with status/parent and excludes unassigned parent; internal/navigation/configuration_test.go empty after removal.
3. **RM3 仅分配子菜单或停用菜单** — internal/navigation/service.go vs UserMenus:284-305. internal/navigation/service_test.go; P `disabled-role-flat-menu-configuration-read`, `enabled-permission-does-not-make-disabled-menu-visible`.
4. **RM4 admin角色菜单直接编辑拒绝** — internal/navigation/service.go before equality. internal/navigation/service_test.go; internal/navigation/configuration_test.go; P `admin-menu-clear`; C `protected-equivalent-menu-set`.
5. **RM5 菜单非法目标与停用目标分开** — internal/navigation/service.go; internal/navigation/service_test.go; internal/navigation/configuration_test.go. P `menu-zero`, `menu-missing`, `disabled-role-enabled-and-disabled-menu-preconfiguration`.
6. **RM6 等价角色菜单配置** — internal/navigation/service.go raw explicit IDs. internal/navigation/configuration_test.go; internal/app/configuration_noop_test.go; C `equivalent-role-menus`, `equivalent-reordered-role-menus`, `write-forbidden-equal-role-menus`.

### MA — 菜单 API 联动 (18)

1. **MA1 菜单绑定 API** — internal/navigation/service.go; internal/navigation/service_test.go local projection; internal/app/configuration_noop_test.go real grant merge/dedup; C `real-code-and-grant-migration-after-fault-removal`; L `api-binding-commits-first-later-disable-retains-configuration`. Equality invalidation is superseded by N, not required unconditionally.
2. **MA2 菜单绑定 API 失败** — internal/navigation/service.go in same TX; internal/app/configuration_noop_test.go exact all13 table rollback after second-user failure; C `late-second-binding-user-failure`; C3 `native_failure_observations.cases` separately covers API-code, Permission-create, role-grant, second-version stages (historical fingerprints caveat retained).
3. **MA3 查询菜单绑定 API** — internal/navigation/service.go; internal/navigation/service_test.go asserts populated and non-null empty responses.
4. **MA4 删除菜单清理 API 关联** — internal/navigation/service.go; internal/navigation/repository.go; internal/navigation/service_test.go for parent/leaf lifecycle plus `internal/navigation/repository_test.go` relation persistence/removal; native leaf deletion in L parent-deletion cases. Menu entity, role menus, and menu APIs share transaction.
5. **MA5 菜单权限变更实时生效** — internal/navigation/service.go; internal/app/configuration_noop_test.go repair increments,129-176 grant change; C `old-ordinary-pair-survives-equivalent-configurations` vs `real-role-display-invalidates-dependent-old-pair-not-unrelated-admin`; actual code/token native contract also C3. Only true changes now invalidate.
6. **MA6 混合有效API与零ID** — internal/navigation/service.go before dedup. internal/navigation/service_test.go verifies existing codes/relations unchanged; C `invalid-zero-in-same-looking-apis`.
7. **MA7 合法重复API ID** — internal/navigation/service.go; internal/navigation/service_test.go creates one binding from repeated ID; internal/app/configuration_noop_test.go; C `equivalent-reordered-api-target-set`.
8. **MA8 内部调用不能绕过原始校验** — same public Application method internal/navigation/service.go, independent of HTTP DTO; internal/navigation/service_test.go directly calls it with mixed IDs and verifies no mutation.
9. **MA9 菜单侧修改已绑定权限码** — internal/navigation/service.go; internal/app/authorization_http_test.go observes two bound API IDs/codes, old/overlap/destination grants and response; C3 native grant migration; internal/app/configuration_noop_test.go current composed integration.
10. **MA10 菜单侧联动中途失败** — internal/navigation/service.go outer TX; internal/app/authorization_http_test.go API-write failure; internal/app/authorization_field_clearing_test.go second version rollback; C `late-second-menu-user-failure`; C3 stage failures.
11. **MA11 未绑定菜单改码** — internal/navigation/service.go excludes binding when no APIs; internal/navigation/service_test.go explicitly verifies replace and clear without fabricated links; C3 case `unbound-code-replacement`, `unbound-code-clear`.
12. **MA12 绑定菜单显式空码沿用既有解析** — internal/navigation/service.go onward binding-code resolution. internal/navigation/service_test.go existing-code reuse/derive/conflict and rollback; C3 `bound-explicit-empty-reuses-existing-code`, `bound-empty-conflicting-existing-codes`.
13. **MA13 空API集合不是解绑** — internal/navigation/service.go; P `menu-api-empty-not-unbind` (422/all captured facts unchanged); it never reaches ReplaceMenuAPIs.
14. **MA14 API停用或删除先完成** — internal/navigation/service.go validates locked current API; Core shared lock. internal/navigation/service_test.go; L `api-disable-commits-after-bind-discovery-before-current-validation` (422 binding/200 disable, no updater writes), `deleted-api-cannot-be-rebound`.
15. **MA15 绑定先完成后停用保留配置** — API Core:143-233 shared target lock; internal/navigation/service_test.go; L `api-binding-commits-first-later-disable-retains-configuration`, followed by `disabled-real-registered-roles-handler-fails-closed` (403).
16. **MA16 绑定与删除的版本原子性** — internal/navigation/service.go; internal/navigation/service_test.go persisted API-delete commit/rollback; L `legal-api-deletion-clears-current-links-and-invalidates`, `api-deletion-invalidates-actual-bound-user`, plus native winner/dependency-conflict cases.
17. **MA17 等价绑定还须比较派生事实** — internal/navigation/service.go compares menu/API final code and live Permission in addition to internal/navigation/service.go relation IDs. internal/app/configuration_noop_test.go; C `equivalent-menu-api-binding`, `restored-live-permission-now-noop`, `migrated-code-and-grants-now-noop`, `write-forbidden-equal-binding`.
18. **MA18 相同API集合仍有真实码变更** — internal/navigation/service.go and internal/navigation/service.go. internal/app/configuration_noop_test.go missing/deleted Permission/direct-code repair,129-176 grant migration/rollback; C `same-inputs-repair-direct-api-code-POST`, `same-inputs-repair-direct-api-code-PUT`, `same-binding-restores-missing-live-permission`, `same-binding-creates-missing-permission-resource`.

### OB — 组织成员绑定 (10)

1. **OB1 分配组织成员** — internal/organization/service.go; internal/organization/repository.go. internal/organization/membership_test.go; internal/app/organization_membership_http_test.go actual composed membership/session mutation; M `bulk-org-ab-to-bc`. Later M narrows invalidation from old/new union to symmetric difference.
2. **OB2 查询组织成员** — internal/organization/service.go; internal/app/organization_membership_http_test.go verifies safe directory fields/populated/empty; M `reader-sees-original-members`, `reader-sees-updated-members` uses original main.
3. **OB3 删除组织时清理成员关系** — internal/organization/service.go; internal/organization/repository.go. internal/organization/reference_lifecycle_test.go; internal/app/organization_membership_http_test.go; L `organization-bulk-binding-first-delete-includes-new-member`, `organization-per-user-binding-first-delete-includes-new-member`.
4. **OB4 保留受保护关系维护普通成员** — internal/organization/service.go and validateMember changed flag:407-423; internal/organization/membership_test.go; P `org-retain-self-and-disabled-admin-maintain-ordinary`; M `equal-org-group-keeps-existing-self-and-protected`. Retained rows' original created_at/joined_at survives.
5. **OB5 拒绝实际改变受保护成员** — internal/organization/service.go; internal/organization/membership_test.go selected/removed/clear/deletion variants; M `full-org-replacement-cannot-remove-protected-old-target`, `full-org-replacement-cannot-remove-existing-self`.
6. **OB6 整组覆盖校验实际删除对象** — internal/organization/service.go; internal/organization/membership_test.go outside/missing/deleted-protected cases; P `org-clear-actual-removed-missing`, `org-target-outside-user-scope`; whole-set semantics are not replaced by hidden-part semantics.
7. **OB7 受控清理软删旧普通成员** — internal/organization/service.go allowDeleted only for removed IDs, same validation; internal/organization/reference_lifecycle_test.go increments preserved independent version 7->8; L `controlled-old-deleted-organization-member-removal`; M `controlled-deleted-old-org-member-removal`.
8. **OB8 软删用户不能作为新增成员** — internal/organization/service.go; internal/organization/membership_test.go deleted selected/retained cases; L `normal-bulk-retention-refuses-soft-deleted-user`; M `deleted-old-org-member-cannot-be-retained-normal-target`.
9. **OB9 组织成员差量与时间保留** — internal/organization/service.go; internal/organization/repository.go; internal/organization/membership_test.go exact B version/time, A/C increments, observer unaffected; M `bulk-org-ab-to-bc`, retained B/reader old-session cases.
10. **OB10 组织成员集合无变化** — internal/organization/service.go; internal/organization/membership_test.go (self/protected retained or empty, no version initialization); M `equivalent-bulk-org`, `bulk-org-noop-does-not-initialize-never-logged`.

### OC — 组织单位 CRUD (6)

1. **OC1 查询组织单位列表** — internal/organization/service.go; internal/organization/repository.go; internal/app/organization_membership_http_test.go exercises scoped successful list through App (not merely tree).
2. **OC2 创建组织单位** — internal/organization/service.go current topology, uniqueness; internal/app/organization_http_test.go creates root/child and asserts duplicate409. L fixture organization HTTP observations confirm native create, not all uniqueness branches.
3. **OC3 删除组织单位** — internal/organization/service.go child guard; internal/app/organization_http_test.go parent delete422; `internal/organization/topology_test.go` invalid topology cases. Successful leaf deletion does not bypass current child barrier.
4. **OC4 删除组织保护其成员归属** — internal/organization/service.go; internal/organization/membership_test.go delete self/admin/outside/missing cases; P `org-delete-disabled-admin-protection`.
5. **OC5 残留custom引用阻止组织删除** — internal/organization/service.go; internal/authorization/application/service.go; internal/authorization/application/reference_scope_test.go; L `inactive-memberless-custom-role-still-blocks-org-delete`.
6. **OC6 解除引用后合法删除** — internal/authorization/application/service.go releasing links then internal/organization/service.go; L `org-delete-after-ordinary-role-scope-exit`, `org-delete-after-dedicated-role-scope-exit`. No unrelated resource removal or automatic scope fallback occurs.

### OU — 逐人修改组织归属 (13)

1. **OU1 修改单个普通用户的组织归属** — internal/organization/user_memberships.go; internal/organization/repository.go. internal/organization/user_memberships_test.go other-user and hidden rows unchanged with one version increment; M `real-per-user-org-addition`, `repeat-per-user-org-addition`.
2. **OU2 清空可管理组织但保留隐藏归属** — internal/organization/user_memberships.go; internal/organization/user_memberships_test.go; M `clear-only-manageable-orgs-retains-hidden-time`.
3. **OU3 保留已有关系加入时间** — internal/organization/repository.go inserts only added and never rewrites retained; internal/organization/user_memberships_test.go explicitly checks retained/hidden/rejoin newer UTC time; M snapshots for `real-per-user-org-addition` and `repeat-per-user-org-addition`.
4. **OU4 拒绝范围外或不存在的组织** — internal/organization/user_memberships.go; internal/organization/user_memberships_test.go includes locating ancestor; M `hidden-id-not-valid-managed-target`, `locating-ancestor-not-manageable`, `invalid-org-after-ensure-rolls-back-initialization`.
5. **OU5 过期组织写入** — internal/organization/user_memberships.go before equality; internal/organization/user_memberships_test.go; M `stale-same-org-target-still-conflicts` (409 ORG_CONFLICT).
6. **OU6 组织写入受保护或越权** — internal/organization/user_memberships.go plus static route permission; internal/organization/user_memberships_test.go; P `per-user-org-disabled-admin-protection`; M `reader-cannot-use-noop-to-bypass-write-permission`, `empty-after-loss-of-target-visibility-still-denied`.
7. **OU7 并发编辑两个归属维度** — internal/organization/user_memberships.go and internal/authorization/application/service.go share internal/authorization/adapters/gorm/access_version.go current version lock; M ten controlled CAS observations listed below. Old L wording '最多一项' is superseded to at-most-one *real mutation* by M.
8. **OU8 组织删除先于归属写入** — internal/organization/user_memberships.go; L `organization-delete-first-both-binding-entries-refuse-recreation` (delete200, bulk404, per-user422, no new relation).
9. **OU9 用户软删先于新增归属** — internal/organization/user_memberships.go; internal/identity/adapters/gorm/repository.go and App policy obtain current deleted fact; L `normal-per-user-org-update-refuses-soft-deleted-user` (404), and identity-first role race proves current shared identity barrier rather than directory silence.
10. **OU10 隐藏归属不妨碍合法无变化** — internal/organization/user_memberships.go; internal/organization/user_memberships_test.go includes retained+hidden, hidden-only, empty manageable scope; M `scoped-equal-orgs-ignore-hidden-membership`.
11. **OU11 同旧版本两次合法无变化** — internal/organization/user_memberships.go; internal/organization/user_memberships_test.go two repetitions; M `cas-two-noops-cross-dimension` and `cas-two-noops-missing-initial-version` both return200 with unchanged current version.
12. **OU12 真实修改后旧版本无变化仍冲突** — internal/organization/user_memberships.go; internal/organization/user_memberships_test.go; M `cas-real-role-before-old-org-noop` (200/409 ORG_CONFLICT), plus stale same-org target case.
13. **OU13 无版本行按既有初始版本比较** — internal/organization/user_memberships.go; internal/authorization/adapters/gorm/access_version.go. internal/organization/user_memberships_test.go covers missing->1, rejection rollback, increment-failure rollback. M `legal-missing-version-organizations-noop-initializes-only-one`, `invalid-org-after-ensure-rolls-back-initialization`; token authentication does not call this owner entry (separate access-version authentication change owned by parent).

### UR — 逐人修改角色归属 (9)

1. **UR1 修改单个普通用户的多个角色** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go. internal/authorization/application/service_test.go and internal/authorization/application/membership_delta_test.go verify only target links/one increment; M `real-disabled-role-membership-still-invalidates` confirms even ineffective role membership is a true relationship change.
2. **UR2 显式清空角色归属** — internal/authorization/application/service.go (empty is valid, delta removes current); internal/authorization/application/membership_delta_test.go empty/current-equal path plus internal/authorization/application/service_test.go target-only semantics; native M lifecycle cleanup/clear outcomes retain other users. Empty clear is not an implicit admin-role bypass because protection precedes delta.
3. **UR3 过期角色写入** — internal/authorization/application/service.go; internal/authorization/application/service_test.go; internal/authorization/application/membership_delta_test.go. M `stale-same-role-target-still-conflicts` returns409 AUTHZ_CONFLICT.
4. **UR4 角色写入受保护或越权** — internal/authorization/application/service.go; internal/authorization/application/service_test.go; internal/authorization/application/protected_assignments_test.go; internal/authorization/application/membership_delta_test.go. P `per-user-role-disabled-admin-protection`.
5. **UR5 角色 ID 无效或缺少写入权限** — internal/authorization/application/service.go; internal/authorization/application/membership_delta_test.go zero/missing/deleted; internal/authorization/application/service_test.go; M `reader-cannot-use-noop-to-bypass-write-permission` for the real outer ACL boundary.
6. **UR6 用户软删与角色绑定交错** — internal/authorization/application/service.go, protected role barriers; internal/identity/application/user_service.go/269-303 identity delete participates. L `identity-soft-delete-first-normal-role-binding-rechecks-current-user`, `normal-per-user-role-update-refuses-soft-deleted-user`, restore/session case. M `actual-identity-soft-delete-retains-membership-cleanup-state` starts from previously valid membership.
7. **UR7 逐人角色等价集合** — internal/authorization/application/service.go; internal/authorization/application/membership_delta_test.go; M `equivalent-per-user-roles`, `repeat-initialized-roles-noop`.
8. **UR8 过期等价角色请求** — internal/authorization/application/service.go before389; internal/authorization/application/membership_delta_test.go; M `cas-real-org-before-old-role-noop` (200/409 AUTHZ_CONFLICT), `stale-same-role-target-still-conflicts`.
9. **UR9 无变化与真实修改合法先后** — internal/authorization/application/service.go; internal/authorization/application/membership_delta_test.go; M `cas-role-noop-before-real-org`, `cas-org-noop-before-real-role` both200, only actual mutation increments.

### SD — 软删旧角色成员的受控清理 (2)

1. **SD1 移除软删旧角色成员** — internal/authorization/application/service.go distinguishes target retention from removed old facts. internal/authorization/application/reference_lifecycle_test.go covers allowed removal, outside/protected/self/missing refusals; M `controlled-deleted-old-role-member-removal`; L corresponding controlled cleanup cases.
2. **SD2 恢复不还原被清除归属** — no restoration write in membership cleanup; internal/authorization/application/reference_lifecycle_test.go toggles safe identity fact live and asserts absent roles; L `restore-does-not-recreate-cleared-links-or-revalidate-old-session`; M `sql-identity-restoration-does-not-revive-old-pair-or-cleaned-memberships`. Restoration is an owned SQL fixture, not a claimed product restore endpoint.

### RC — 角色 CRUD (12)

1. **RC1 创建角色** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go; internal/app/authorization_http_test.go successful public create/update with nonzero ID; internal/authorization/application/noop_configuration_test.go consumes full persisted field create. No additional role-create status policy was inferred from the configuration-only change.
2. **RC2 查询角色列表** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go implements offset/limit, total and `sort asc,id asc`. **Source-only evidence gap:** no deterministic successful ListRoles/GET roles page+ordering assertion located in reviewed tests or assigned native observations. See coverage warning below; this is not a source defect.
3. **RC3 修改角色** — internal/authorization/application/service.go; internal/authorization/application/service_test.go; internal/authorization/application/noop_configuration_test.go real name/code/description/sort/status/scope mutation and second-user rollback. Later N requires true change before invalidation.
4. **RC4 超级管理员角色受保护** — internal/authorization/application/service.go; internal/authorization/application/service_test.go; internal/authorization/application/noop_configuration_test.go equal protected candidate; C `protected-same-role-field`.
5. **RC5 删除角色** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go; internal/authorization/application/reference_lifecycle_test.go; L `role-delete-healthy-preserves-shared-menu-and-permission`.
6. **RC6 普通角色跨模块清理** — internal/authorization/application/service.go plus required capability; internal/authorization/application/reference_lifecycle_test.go four families + shared resources + second-user rollback; L `role-delete-second-version-3819-rolls-back-all-four-mappings`, `ordinary-role-deletion-shared-resources-and-version-history`.
7. **RC7 绑定先提交后合法删除** — same role barrier internal/authorization/application/service.go/218 plus locked relationship reread:226; internal/authorization/application/reference_lifecycle_test.go sequential controlled caller; L `role-bind-first-delete-current-new-member` proves actual native two-request order/waits and two successes/new member version.
8. **RC8 删除先提交后新增绑定** — internal/authorization/application/service.go and361-369 refuse missing target; internal/authorization/application/reference_lifecycle_test.go; L `role-delete-first-later-bind-refused` 200/404 with no recreated links. Native exact-key wait-probe limitation retained below.
9. **RC9 角色说明显式清空** — internal/authorization/application/service.go pointer presence; internal/app/authorization_field_clearing_test.go role-description steps; C `clear-role-description-once` plus repeat no-op.
10. **RC10 角色说明遗漏保持** — internal/authorization/application/service.go applies only present pointer; internal/app/authorization_field_clearing_test.go omitted step verifies stored/response value and other owner facts.
11. **RC11 拒绝清空角色必填标识** — internal/authorization/application/service.go; `internal/authorization/application/field_clearing_test.go`; internal/app/authorization_field_clearing_test.go exact no partial facts/version mutation.
12. **RC12 角色同值与真实展示变化分开** — internal/authorization/application/service.go; internal/authorization/application/noop_configuration_test.go; C `same-role-fields`, `real-role-display-change-despite-another-role-grant`, `empty-role-update`, and old-session contrast.

### PC — 权限 CRUD (5)

1. **PC1 创建权限** — internal/authorization/application/service.go permission-code validity/existence/create; internal/authorization/adapters/gorm/repository_test.go full persisted Permission creation/pagination; internal/authorization/application/noop_configuration_test.go uses public application create; no state field introduced.
2. **PC2 查询权限列表** — internal/authorization/application/service.go; internal/authorization/adapters/gorm/repository.go onward sorted pagination. internal/authorization/adapters/gorm/repository_test.go asserts page count/total and internal/app/authorization_http_test.go reads actual permission listing after grant migration. Multi-row sort tie semantics rely on inspected query.
3. **PC3 修改权限** — internal/authorization/application/service.go only Name/Group/Sort writes; code not mutable. internal/app/authorization_field_clearing_test.go group omission/clear/replacement and immutable resource code; `authorization/application/field_clearing_test.go:38-60` rejects empty Name atomically. Valid name specifically is not isolated in those tests; group/sort mutation exercises the scenario's allowed update path.
4. **PC4 删除权限** — internal/authorization/application/service.go; AR permission hard delete and relation cleanup; internal/authorization/application/service_test.go checks dependent increment; internal/authorization/application/reference_lifecycle_test.go proves current dependents/absence/new-bind rejection.
5. **PC5 权限删除与新角色引用交错** — internal/authorization/application/service.go uses same Permission lock; internal/authorization/application/reference_lifecycle_test.go controlled before-delete callback proves bind-then-delete and reverse-refusal semantics locally. The assigned native lifecycle evidence does not contain a separate Permission-delete HTTP interleaving case; no InnoDB ordering claim for this particular pair is made beyond the inspected shared barrier.

### MC — 菜单 CRUD (17)

1. **MC1 创建菜单** — internal/navigation/service.go; internal/navigation/repository.go; internal/navigation/service_test.go ordered persisted tree/default behavior.
2. **MC2 创建按钮菜单** — internal/navigation/service.go normalize empty path to nil; internal/navigation/service_test.go create button without path and use permission code; native L child/create fixtures also use type3.
3. **MC3 菜单路径冲突** — internal/navigation/service.go; internal/navigation/service_test.go duplicate create/update rejection; internal/navigation/configuration_test.go ensures no-op-looking invalid path still fails.
4. **MC4 父级菜单指向自己** — internal/navigation/service.go; internal/navigation/service_test.go; internal/navigation/configuration_test.go; C `same-parent-is-self-invalid`.
5. **MC5 修改菜单** — internal/navigation/service.go; internal/navigation/service_test.go; internal/navigation/configuration_test.go empty and invalid guard; internal/app/authorization_field_clearing_test.go exact field persistence/omission.
6. **MC6 删除存在子菜单的菜单** — internal/navigation/service.go; internal/navigation/repository.go; internal/navigation/service_test.go; L `menu-child-create-first-refuses-parent-delete`.
7. **MC7 删除叶子菜单** — internal/navigation/service.go; internal/navigation/repository.go; internal/navigation/service_test.go; `internal/navigation/repository_test.go` verifies relation deletion owner behavior.
8. **MC8 显式创建停用菜单** — internal/navigation/service.go; internal/navigation/repository.go explicit map; internal/navigation/service_test.go; C2 explicit disabled original-main observations; P disabled menu visibility cases.
9. **MC9 创建状态遗漏与合法启用** — internal/navigation/service.go; internal/navigation/service_test.go omitted/enabled rows; C2 create/default cases.
10. **MC10 无效创建状态使用既有默认** — internal/navigation/service.go; internal/navigation/service_test.go above-range case; C2 `other_defaults`.
11. **MC11 创建事务中版本失败** — internal/navigation/service.go; C2:229-295 `.mid_version_failure_and_recovery`: observed second-user version wait, dirty checkpoint new menu status0 and earlier increment, MySQL3819/500, complete final menu/version equality. This is historical native rollback proof, not a current execution claim.
12. **MC12 菜单删除与子菜单创建交错** — internal/navigation/service.go and188-206; internal/navigation/repository.go shared menu/child barrier; internal/navigation/service_test.go; L `menu-child-create-first-refuses-parent-delete`, corrected `menu-parent-delete-first-child-not-found-after-classification-fix`.
13. **MC13 菜单可选属性清空与遗漏分开** — internal/navigation/service.go; internal/app/authorization_field_clearing_test.go icon/component steps and persisted current response; C `clear-menu-icon-once`, repeat case.
14. **MC14 合法路径清空** — internal/navigation/service.go; internal/navigation/configuration.go compares nil/string target; internal/app/authorization_field_clearing_test.go button path empty -> SQL NULL; C `clear-menu-path-once`, `normalized-empty-path-repeat`.
15. **MC15 非法清空不部分提交** — internal/navigation/service.go before writes; internal/app/authorization_field_clearing_test.go; `internal/navigation/field_clearing_test.go` direct owner name rejection. No new path/type policy is demanded.
16. **MC16 菜单同值与真实展示变化** — internal/navigation/service.go; internal/navigation/configuration.go; internal/navigation/configuration_test.go; C `same-menu-fields`, `real-menu-display-retains-current-all-user-trigger`, old-admin-session rejection.
17. **MC17 仅DTO同值不能掩盖派生修复** — internal/navigation/service.go; internal/app/configuration_noop_test.go code/Permission repair with actual versions; C `same-inputs-repair-direct-api-code-PUT`, `same-binding-restores-missing-live-permission`.

### MS — 前端路由同步菜单 (3)

1. **MS1 同步创建新菜单** — internal/navigation/service.go; internal/navigation/service_test.go parent_path and duplicate route create count/tree; C `real-runner-retry-resets-created-count` creates exactly2 and commits one attempt's increments.
2. **MS2 同步遇到已存在路径** — internal/navigation/service.go; internal/navigation/service_test.go duplicate; internal/app/configuration_noop_test.go all-existing created0; C `all-skip-menu-sync`.
3. **MS3 同步仅正常跳过不失效** — internal/navigation/service.go reset +365-367 early return; internal/navigation/configuration_test.go final retry becomes all-skip, no versions changed; internal/app/configuration_noop_test.go; C `write-forbidden-all-skip-sync`, `retry-created-paths-next-request-all-skip`. `existing-unknown-parent-handling-not-redefined` shows old root-resolution behavior retained, not approval of a new policy.

## Native proof bounds and current composition

### Lifecycle concurrency

L is MySQL8.0.19 REPEATABLE-READ original main, not SQLite concurrency. Inspected full selected controlled records (including outcomes, wait SQL, final projections):

- `role-bind-first-delete-current-new-member`: bind waits on target version; delete waits on role; both200; final role/relations absent and new member version affected.
- `role-delete-first-later-bind-refused`:200/404 and final absence. Its exact-key probe failed because relationship scan waited at `(1,1)` rather than requested user; finally released barrier. Outcome is genuine, but not a precise requested-key pause proof and was not replayed.
- `custom-assignment-first-current-reference-blocks-organization-delete`:200/409 ORG_CONFLICT; delete waits on locked hierarchy. Reverse `organization-delete-first-current-graph-refuses-custom-binding`:200/422; no custom references; scope remains self.
- Bulk and per-user organization bind-first/delete cases each200/200; hierarchy wait and final membership absence/new user versions recorded. Reverse delete-first rejects both entries404/422.
- `identity-soft-delete-first-normal-role-binding-rechecks-current-user`:200/422; soft-deleted identity retains version. A relationship scan wait, not a claim that both HTTP writers paused directly at identity lock.
- Menu child-first delete409; corrected parent-first child404. The earlier500 is a historical implementation correction, not a current passing result.
- API bind-first/disable both200 with retained binding and registered handler403; disable wins after discovery then binding422 with winner retained/no rejected-request writes; later API delete200 invalidates actual user session, rebind404.
- `strict-api-union-locks-lower-dependencies-before-higher-entry-target`: entry129 waited on lower128, no higher129 record lock observed before release; inspection127 and indirect menu stayed old code; inspection-only user72 unchanged. EXPLAIN used index_merge/filesort: the record establishes that controlled optimizer plan, not a universal lock-order theorem.
- Menu dependency set changed ->409 NAV_CONFLICT; API set changed ->409 API_META_CONFLICT; legal API deletion winner remains and rejected request contributes no writes. No S3 propagation expansion inferred.

### Membership CAS concurrency

M's ten **actual two-HTTP-request** cases were inspected with request bodies, first/both wait rows, granted version locks, results, exact final projections, and before/after versions:

| Exact M case | Observed outcome |
|---|---|
| cas-two-noops-cross-dimension | 200/200, both version11 |
| cas-two-noops-missing-initial-version | 200/200, exactly missing target initialized to1 |
| cas-role-noop-before-real-org | 200(version11)/200(version12) |
| cas-org-noop-before-real-role | 200(version12)/200(version13) |
| cas-real-org-before-old-role-noop | 200(version14)/409 AUTHZ_CONFLICT |
| cas-real-role-before-old-org-noop | 200(version15)/409 ORG_CONFLICT |
| cas-two-real-role-writes | 200(version16)/409 AUTHZ_CONFLICT |
| cas-two-real-org-writes | 200(version17)/409 ORG_CONFLICT |
| cas-real-role-before-real-org | 200(version18)/409 ORG_CONFLICT |
| cas-real-org-before-real-role | 200(version19)/409 AUTHZ_CONFLICT |

First role operations were paused at hierarchy after owning version; organization-first operations at identity after owning hierarchy/version; observed second waits vary (version vs hierarchy). These are specific bounded orders, not 'start first means win', lock-free progress, or proof of no deadlocks. Native retry observations cover all four owner/granularity paths; source attempt-local data and recorded final increments/retained rows agree. SIGNAL1213 is an injected actual Runner retry, **not a physical InnoDB deadlock**. SQLite delta fixtures separately prove preserved rows/timestamps and rollback, not these native interleavings.

### No-op native evidence

C after-observations show seven equivalent entrypaths leave all captured facts unchanged; explicit missing-version invariant says never-logged user5 remains absent; ordinary/admin old session pairs survive. Real role display invalidates dependents despite duplicate grant; real menu display retains all-user trigger. Same-looking prohibited requests still401/403/409/422 without writes. Residual custom, missing/deleted Permission, direct API code, actual API target set, and code/grant migration cases change facts/versions once, then repeated requests become no-ops. Second-user failures leave all captured facts equal. Current-role recheck was a SQL winner plus one HTTP waiter, **not two HTTP mutations**. Native sync retry returns2 with one final set of increments; NCT final-attempt-all-skip fixture proves count reset when a later attempt creates0. Neither evidence is promoted to arbitrary optimizer plans or production deployment.


### 附录D：C4／I4

## fix-concurrent-organization-moves

- Schema: spec-driven; current CLI all_done; tasks13/13;1 Requirement/5 scenario instances.
- Completeness: all proposal/design/tasks/spec artifacts read; no missing capability identified.
- Current implementation: `internal/organization/service.go:85-126` CreateUnit, `:129-244` explicit-parent update, `:247-294` DeleteUnit all enter `:297-311` same outer Runner/current-parent context. `internal/organization/repository.go:148-156` ordered ID FOR UPDATE current read; `:133-143` consumes transaction-local graph. `service.go:175-195` checks target parent existence/self/ancestor chain from one graph;0 remains sentinel. `internal/app/organization.go:19-28` sole composition, existing routes/codes retained. `docs/modules/organization-management.md:40-68` documents whole-instance stop-write cutover and expanded later R6/I4 participants; production cutover not claimed executed.

| Requirement / Scenario | Current code | Test or precisely scoped native proof |
|---|---|---|
| 防止组织循环 / 修改父组织 | service.go:175-195 | topology_test.go:55-105 self/descendant stored-state rejection; current full Go passed |
| 两个根节点交叉并发移动 | service.go:134-138,297-304; repository.go:148-156 | c4-organization-moves-fix-2026-10-17.json controlled_native_races case two-root-cross-move:422/200, final no cycle, refused own rows unchanged. I4 newer c4-first-old-branch-under-new and c4-first-new-branch-under-old both200/422 with current locks |
| 并发三节点长环 | service.go:187-194 and shared graph | C4 JSON case three-root-long-cycle:200/422/200; case deep-cross-branch-cycle422/200; both final graph checks true |
| 等待后重读当前拓扑 | repository.go:148-156, service.go:297-304 | topology_mysql_test.go:23-114 TestMySQLLockedOrganizationParentsOverridesEarlierRepeatableReadSnapshot, rerun current with real MySQL; C4 JSON seven controlled graph-lock races |
| 合法移动到根节点 | service.go:175-196 only validates nonzero parent, scope still enforced | topology_test.go:107-153; C4 JSON additional_native_checks case real-scoped-actor-positioning-ancestor-and-root-zero; newer I4 case scoped-actor-can-move-managed-unit-to-root-zero |

Design adherence: database-owned current graph, all three topology writers, one parent map/chain traversal, no new mutex/lock service/DDL/retry policy. Later I4 version work is approved supersession, not C4 scope creep. Storage rollback tested topology_test.go:155-210 and C4 native-move-storage-failure / native-delete-fails-after-unit-and-membership-deletion. Older C4 delete-first schedule absent is explicitly bounded and supplemented for current composition by I4 delete-parent-then-move200/404. No functional CRITICAL/WARNING identified in this slice. C4 design.md:5,56 still says future/unexecuted; at most documentation-timeline SUGGESTION, not test failure.

## invalidate-organization-hierarchy-dependents

- Schema spec-driven; current CLI all_done; tasks14/14;2 Requirements/11 scenario instances (includes6 inherited CRUD scenarios).
- Requirement1组织单位 CRUD: existing List/Create/Delete plus new finite root dependency move.
- Requirement2组织层级范围根依赖失效: individual applicable role/member-root facts; not final merged scope delta.
- Current code: `internal/organization/hierarchy.go:87-105` old/new ancestor symmetric difference, common ancestors0/moved node excluded; `service.go:221-239` only actual parent change calls dependency port, sorted unique user versions in same transaction. `internal/authorization/application/service.go:854-920` batch current root members -> raw references -> sorted all candidate metadata locks incl disabled -> current recheck -> eligible org_and_children users; no all-role early return or identity-status filter. `internal/authorization/adapters/gorm/repository.go:58-85` ordered current metadata/references; `internal/organization/repository.go:250-265` batch current member locking read avoids DISTINCT relation-lock ambiguity. `internal/app/organization.go:35-40` translates owner conflict to existing ORG_CONFLICT. Unknown storage cause remains safe internal. No new route/schema/version field/cache.

| Requirement / Scenario | Current implementation | Test / reused native proof |
|---|---|---|
| CRUD / 查询组织单位列表 | organization/service.go:47-61, repository.go:159-181 count/filter/limit/offset | scope_repository_test.go:12-40 real stored total/scope projection; inherited read path, not claimed newly implemented by I4 |
| 创建组织单位 | service.go:85-126, repo CodeExists/Create | topology_test.go:107-118 creates root+child; unchanged unique-code guard at104-108 |
| 删除组织单位（有子拒绝） | service.go:269-272 | topology_test.go:55-105; native move-then-delete-restored-parent200/422 |
| 删除组织保护其成员归属 | service.go:255-285/current policy | inherited Protection/R6 implementation and protected-authorization-assignments / authorization-reference-lifecycle native evidence; no I4 exemption |
| 残留custom引用阻止组织删除 | service.go:259-267, Auth LockOrganizationReferences | inherited R6 reference protocol, native lifecycle custom-blocks-delete incl disabled/memberless role; no cleanup by I4 |
| 解除引用后合法删除 | same; only current references block | inherited R6 controlled scope-delete order; I4 delete-parent-then-move200/404 confirms ordinary valid leaf deletion |
| 树外祖先范围依赖者失效 | hierarchy.go:87-105; auth service.go:854-920; org service.go:221-239 | hierarchy_invalidation_test.go:33-102, auth hierarchy_dependencies_test.go:30-138; real App hierarchy_dependencies_test.go:73-179. Native sibling-parent-move-boundary-dependencies exact IDs1,2,3,7,8 once;19 old unexpired pairs observed |
| 移动事实和多个用户版本共同回滚 | same outer context service.go:134-138,221-244,297-304 | hierarchy_invalidation_test.go:154-207; App hierarchy_dependencies_test.go:127-145; native second-dependent-version-failure500/all13 facts same; actual-1213-at-second-version-recomputes-attempt2 attempts exact one commit |
| 相同父级及不依赖该层级的用户 | service.go:130-132,222; auth eligibility887 | hierarchy_invalidation_test.go:104-152; native equal-parent, display-only, equal-parent-with-display; common/moved/descendant/fixed/disabled/deleted/all-only exclusions, never-logged rows unchanged unless actual dependent |
| 根依赖 / 另一角色授权不豁免层级依赖 | auth service.go:885-918 no finalScope, per-role boolean | auth hierarchy_dependencies_test.go:30-138; App :89-125,160-175 proves all+child and unchanged multi-root union still+1. Native root/admin+explicit child and all+duplicate child each+1 |
| 成员变化不等于层级边界变化 | dependency call only org parent actual change | hierarchy_invalidation_test.go:104-152 persisted membership delta and reader versions; native pure-membership-valid-lifecycle-facts only added user18+1,4 reader old pairs valid, old reader sees new member |

Concurrency: native organization-hierarchy-dependents-2026-10-18.json has12 VALID pairs/24 actual main HTTP requests with first wait captured before launching second, exact final graph and per-user totals: membership,scope,parent delete,role delete,role enable,mutual parent cycles, both orders. Invalid extra404/404 fixture pair explicitly excluded. Historical orphan-role member guard500 retained as unclassified out-of-scope corrupted-data observation; no product fix or defect label. Current three MySQL gates passed; current full Go also passed. Source/scenario coherence holds; C4/R6 main prerequisites retain published scenarios, I4 main sync/archive intentionally not performed.

Limits: native original-main observations reused, not rerun under current verification; current Go+real MySQL gates executed. SQLite storage-mutation tests don't prove concurrent locks. No production switch, full scheduler coverage, S3/S4/S5/I3 decision, benchmark or historical-data remediation claimed. No source/spec/task changes made by verifier.
