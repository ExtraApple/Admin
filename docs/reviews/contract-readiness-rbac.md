# 契约可信度调研 · RBAC 授权事实写入

## 基线与边界

- 日期：2026-10-14；代码 `c4b96ce1d64daede23a8d867d0a990e1c5887c04`；以当前主规格判定，不把讨论中新目标倒替现行条款。
- RBAC 11 Requirements / 32 Scenarios：20 个写入或初始化场景纳入，12 个读／运行时场景在追踪表中明确未覆盖。管理员身份判断另作保护策略依赖核对，不因此宣称完整运行时通过。
- 逐人角色写入的 governing 在 `user-management:253-276`；不是把角色批量成员替换改成可管理部分替换。Identity Seed 由 Identity 持有，在这里登记交叉依赖，不重复立项。
- 全部 Requirement 正文和 Scenario 原文见 [现行条款快照](contract-readiness-c6b-authorization-spec-snapshot.md#rbac)。运行结果及全切片分类见 [首轮审计](contract-readiness-c6b-authorization-writes.md)；明细见 [去敏证据](evidence/c6b-authorization-writes-2026-10-14.json)。未创建 change，未修改产品／测试／主规格。
- 2026-10-15定向补证：[ID-SEED-COUNT](contract-readiness-id-seed-count.md)已从源码候选提升为已核实现行不符，尚未修复；仅更新对应B13与分类状态，不扩充原场景覆盖范围，不改写2026-10-14证据JSON。

## 一、入口、不变量、实际写入

所有管理员入口为 PermissionControlled Descriptor。是否有配置权限与业务层是否满足引用／保护规则分开；一次前置 403 不能排除业务候选。

| 行 / owner | 入口 → 当前实现 | 写入及版本边界 | 现行条款与结论 |
| --- | --- | --- | --- |
| B1 角色创建 / Authorization | POST roles → HTTP `roles.go:10-17` → Application `service.go:91-118` | 校验 scope、名称／code 唯一；custom 创建拒绝；保存角色。没有既有成员，不调用版本 | rbac:12-14；字段创建静态支持，唯一冲突与状态边界未作全面 HTTP 验收；真实审计夹具创建普通角色成功不等于所有值通过 |
| B2 角色修改 / Authorization | PUT roles/:id → `service.go:121-182` | 先 FindRole；现有 admin 拒绝；字符串非空才更新；唯一校验在事务外；同事务 UpdateRole→角色用户版本，提交后回读 | :20-27；保护和更新／版本有既有 SQLite 行为测试；可选 description 显式空值语义待澄清。提交后回读失败不意味着已提交事实回滚 |
| B3 角色删除 / Authorization | DELETE roles/:id → `service.go:185-202` → Repo `repository.go:84-95` | 同事务读取旧用户→删除 user_roles、role_permissions、role_data_scopes→硬删 Role→旧用户版本；不清 Navigation role_menus | :29-33 仅明确用户关联、硬删、旧成员失效，静态支持；role_menus 完整清理是已批准生命周期目标，不凭遗漏自动判旧条款违约。删除与后续重新绑定的提交合法性仍需真实交错 |
| B4 角色成员整体替换 / Authorization | POST roles/:id/users → `service.go:205-218` → Repo ReplaceRoleUsers | 事务外角色存在检查；事务读旧成员→清旧／归一化插新→旧新用户并集提升。无逐个 Identity 目录或 admin 保护检查 | :53-57 当前明确新旧成员失效；保留 B 仍失效符合旧条款方向。原始 0／所有目标及删除对象合法性、保护 admin、精确实际增删失效是待变更目标。新旧并集不是已确认缺陷 |
| B5 单用户实际角色 / Authorization，Identity 供目录 | PUT admin/users/:id/roles → `service.go:220-294` | 原始 0 先拒绝；同事务 Ensure 锁版本→CAS→当前操作者 scope→目标存在／保护→每个角色存在／admin拒绝→只替该用户关系→EnsureAndIncrement。空数组可清空 | user-management:253-276；既有真实 SQLite+App HTTP 定义与本轮真实 JWT/MySQL CAS 支持局部行为。真实 HTTP 同一旧版本的角色／组织写入取得 200、409 ORG_CONFLICT，版本只增一次；未控制双方版本读取内部暂停点，RR 快照覆盖不能据此宣布通过 |
| B6 角色数据范围 / Authorization，Organization 供目录 | POST roles/:id/data-scope → `service.go:314-344` → Repo `186-202` | ParseDataScope、admin拒绝；custom 非空／存在检查在事务外；同事务 scope字段+清旧scope映射+仅custom插映射+角色用户版本 | :67-80；不套用运行时 Scope All/None 的策略判断。非custom提交非空组织列表目前被忽略并清旧，要求拒绝是新目标；组织删除／移动交错、范围重算及精确影响集未证 |
| B7 权限创建 / Authorization | POST permissions → `service.go:383-399` | 合法 PermissionCode、唯一检查、创建；无成员关联则无失效 | :113-115 静态支持；未以创建一条夹具权限外推所有 code 边界／并发冲突 |
| B8 权限展示修改 / Authorization | PUT permissions/:id → `service.go:402-423` | 更新 name/group/sort，不提供 code 修改；无版本调用 | :121-124 与不改 code 的承诺静态一致；可选 group 清空语义待澄清，不移植 Role/Menu 展示变更裁决到 Permission |
| B9 权限删除 / Authorization | DELETE permissions/:id → `service.go:425-439` → Repo DeletePermission | 同事务读原角色→清 role_permissions→硬删权限→原角色用户版本 | :126-130 静态支持；真实该入口各阶段失败及多个用户完整回滚待证，不把 Navigation API 更新的运行证明扩到此入口 |
| B10 角色权限整体替换 / Authorization | POST roles/:id/permissions → `service.go:441-451` | 角色存在在事务外；同事务清旧／插新→角色全部用户版本；无每个权限存在／admin检查 | :136-139；精确引用与 admin 关联保护属于新目标；同集合仍失效不能以新 noop 目标判旧违规；API 联动真 Permission merge 的局部运行证据不是此入口全部验收 |
| B11 权限分组 / Authorization | POST/PUT/DELETE permission-groups → `service.go:628-684` | name 唯一；修改非空name或sort；硬删除；无版本维护 | :185-195；无 PermissionGroup ID 关系（Permission.Group 是字符串），不凭名称变更新增级联承诺；可选／必填字段空值分别待明确 |
| B12 Route Catalog 权限同步 / Authorization + App 装配 | POST permissions/sync → HTTP `permissions.go:141-156`；App `authorization.go:41-66`；Application `688-722` | 一个事务遍历受保护 RouteFact；跳 Public；用默认码或 method/path；只创建缺失；不扫描 Gin。created 闭包在事务回调外累积，实际重试返回计数待核实 | :169-180；与逐 API 的 /apis/sync-permissions 是不同入口，不能混认全请求事务或测试覆盖。App 快照／幂等已有定义并本轮执行；Authenticated 码策略不同用途不直接叫违约 |
| B13 启动兜底 / Authorization 基础、Identity 用户、App 编排 | auth GORM `seed.go:26-84,111-138`；Identity `seed.go:25-82`；App seeddata `28-43` | Foundation 建立／恢复启用admin角色；Finalize 维护 admin 权限／菜单，再 SeedAdminUser。正常识别已有active admin时直接返回；配置普通用户名不提升；无版本维护 | :82-108；正常初始化及Seed no-version有既有SQLite执行，历史真实初始化可局部复用。ID-SEED-COUNT于2026-10-15以原程序／真实MySQL单条1317故障确认既有密码被重置；只证明该具体异常路径 |

## 二、全部 RBAC 条款覆盖追踪

| Requirement 正文位置 | Scenario 原文位置 | 本轮覆盖及明确未覆盖 |
| --- | --- | --- |
| 角色 CRUD :9-10 | 创建:12-14、列表:16-18、修改:20-23、admin保护:25-27、删除:29-33 | 4 写入纳入 B1-B3；列表未覆盖 |
| 按 ID 角色详情 :35-36 | 存在:38-40、不存在:42-44、缺权限:46-48 | 3 读取场景未覆盖；只定位现有 App 读取定义，不挪用为写入通过 |
| 用户角色分配 :50-51 | 分配:53-57、查询:59-62 | 分配纳入 B4；脱敏查询未覆盖 |
| 角色数据权限 :64-65 | 配置:67-71、查询:73-75、admin保护:77-80 | 配置／保护纳入 B6；查询及 all/self/org/children/custom 的运行时决策未在此验收 |
| 超管兜底 :82-83 | 缺角色:85-88、已有active:90-93、无active:95-98、已有普通用户名:100-103、身份:105-108 | 前4初始化纳入 B13；身份为保护策略依赖静态核对，完整 token 权限判断未覆盖 |
| 权限 CRUD :110-111 | 创建:113-115、列表:117-119、修改:121-124、删除:126-130 | 3写入纳入 B7-B9；列表未覆盖 |
| 角色权限分配 :132-133 | 分配:135-139、查询:141-144 | 分配纳入 B10；查询未覆盖 |
| 用户有效权限码 :146-147 | 有权限:149-151、无权限:153-155 | 2运行时场景未覆盖 |
| 权限码列表 :157-158 | 列表:160-163 | 读取未覆盖 |
| 路由权限同步 :165-166 | 管理端:168-174、启动:176-180 | 2纳入 B12/B13；只核对实际输入、持久化与幂等，不等于所有 Route 权限已验收 |
| 权限分组 CRUD :182-183 | 创建:185-187、修改:189-191、删除:193-195 | 3纳入 B11 |

## 三、严格分类

### 已发布承诺下的实现问题（候选补证）

**ID-SEED-COUNT / Identity（2026-10-15已核实，尚未修复）**：rbac:90-93不重置现有active admin密码；`identity/adapters/gorm/seed.go:73-76`忽略Count.Error，将本次MySQL 1317后count=0当成无管理员，原tx.Error仍为nil。后续配置用户名／绑定读取完成，:43-49 → :65-70重写密码；原程序事务提交并提供HTTP。正常重启原密码200／配置密码401；故障重启原密码401／配置密码200，身份／绑定及版本7不变。见[定向核实与去敏证据](contract-readiness-id-seed-count.md)。`hasUserRoleInTx:79-82`另有同根因丢错线索，但其失败未注入；不把这一处源码或其他数据库故障扩为确认，也不重复登记Owner。

### 需修改规格的新目标

- 原始集合完整引用／范围／保护校验，包含真实被删除的旧对象；单用户 SetUserRoles 的已有拒绝不能证明角色侧批量入口满足它。
- admin 的用户、权限、菜单关联直接编辑保护；普通配置变更合法间接影响操作者和 admin 不能被保护规则吞掉。
- 合法等价集合／同值字段不失效；旧成员集合改 `{A,B}->{B,C}` 只失效 A/C；属于与旧“新旧用户”条款不同的目标。
- 最小事实依赖影响集合、隐式admin依赖、组织移动的范围读者失效；不是最终权限列表 diff，也不是无关用户全失效。
- role 删除所有 cross-owner 引用的完整生命周期、并发删除与绑定的合法提交顺序；不得仅因当前模型裸 ID 就推断数据库结果。

### 本轮已裁定的字段语义

2026-10-14用户确认：合法可选description／group等显式空值清空，遗漏字段保持原值；用户名、角色名、code等必填标识不因此允许清空。当前非空更新路径与已裁定目标有静态差异，仍未发布／未实现，不因本次裁定新增已确认旧缺陷。Identity纯昵称不失效也已裁定并在总报告单列；不能从B2的Role展示变更失效外推到用户资料。

### 精确证据缺口

B2-B6/B9/B10 真实多个版本及未初始化版本每阶段失败回滚；Role/Permission/Org 生命周期可控交错；scope 输入对象与操作者 scope 到提交的稳定性；初始化 Count 单步故障；带真实 JWT 的普通管理员保护／范围拒绝。当前模型裸 ID 与真实 fresh MySQL schema 的 FK=0 已核实，但不等于生产 schema 无约束，也不是单独缺陷。

## 四、证据适用性

- 本轮 `go test ./internal/authorization/... ./internal/navigation ./internal/organization ./internal/identity/application ./internal/app ./internal/app/seeddata ./internal/apimetadata/... -count=1` 通过。Authorization service fixture 使用真实 SQLite Role/Permission/Version + fake Identity directory；这能证明局部行为，不能证明 MySQL FOR UPDATE 或全部 HTTP 前提。
- `TestRolePermissionAndDataScopeChangesInvalidateAccessVersion`、`TestSuperAdministratorRoleIsProtectedByApplicationUseCases`、`TestGetRoleAndSetUserRolesChangeOnlyTargetAndAdvanceVersionOnce`、`TestSetUserRolesRejectsProtectedTargetsAndInvalidAssignments`、`TestSetUserRolesRevalidatesScopeAfterConcurrentOperatorRevocation` 均在上述包执行。最后一项是注入事务边界的 SQLite 交错，不伪称生产 MySQL 并发全证明。
- 2026-09-30 frontend integration acceptance :36-42 的真实配置保存、单用户版本冲突与隐藏 joined_at 保留可复用；相对发布提交 `f43f518`，Organization、当前版本存储和 Identity 删除路径没有行为改写。历史执行未固定 SHA，不补造当时未知条件或完整 Case。
- 2026-07-31 version-storage 的原始 MySQL 日志确有 Ensure 唯一／累加／锁等待及删除回滚通过，但包为旧 `admin/service`，之后经过 layered-monolith 迁移，不能直接当成当前新 Adapter 的运行证明。本轮独立现存 MySQL 门禁重新覆盖 schema／8并发累加／Runner重试，范围见总报告。
- 已知 `organization/user_memberships_mysql_test.go:53` 的五参／六参签名不匹配未编译重现、未修复，也不混算生产业务缺陷；没有执行全仓库 tagged suite。
