# organization-management Specification

## Purpose

组织管理用于维护组织单位层级，为组织成员归属和数据范围提供基础，
并约束组织树、成员关系、循环防护以及数据范围过滤行为。

## Requirements

### Requirement: 组织单位 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除组织单位。

组织删除若将连带移除操作者本人或受保护身份的组织归属 SHALL 整次拒绝；普通权限/菜单生命周期的受控引用清理例外不得扩大为组织成员删除豁免。

本规格的受保护身份 SHALL 依据现存实际admin角色关联判断，不得使用只包含启用角色的运行时有效授权投影；admin角色停用不解除其关联用户的管理保护。

只要普通角色当前custom范围仍引用组织，组织删除 SHALL 拒绝，角色停用或当前无成员不豁免。解除引用须由有相应配置权限者先完成；删除本身不得清空custom、改成self/all或留下空custom。

真正改变parent_id的合法组织移动 SHALL 在受保护的当前父图中识别旧新祖先范围根，仅将后代组织集合因本次移动发生改变的根用于org_and_children层级依赖查询；具有适用org_and_children角色且成员组织命中这些根的用户 SHALL 被去重纳入版本处理，依赖者可能在移动子树内外。移动、依赖事实稳定读取及用户版本 SHALL 在同一最外层事务完成；不得仅使用移动节点成员或逐用户最终有效范围差分，另一角色重复授权或all范围也不豁免已有层级事实依赖。

没有真实parent_id变化、纯名称/排序等展示变化及纯成员变化 SHALL NOT 触发本项层级失效；纯成员变化遵循各成员入口的差量规则，org/self/custom的固定组织ID也不因父边移动自动成为org_and_children依赖。

#### Scenario: 查询组织单位列表
- **WHEN** 管理员请求 `GET /api/admin/organizations`
- **THEN** 系统分页返回组织单位列表

#### Scenario: 创建组织单位
- **WHEN** 管理员请求 `POST /api/admin/organizations`
- **THEN** 系统创建组织单位
- **AND** 组织编码 SHALL 唯一

#### Scenario: 删除组织单位
- **WHEN** 管理员删除组织单位
- **THEN** 如果该组织存在子组织，系统 SHALL 拒绝删除

#### Scenario: 删除组织保护其成员归属
- **WHEN** 合法删除请求的目标组织含本人或受保护身份成员
- **THEN** 系统 SHALL 拒绝删除，组织、成员与授权版本保持不变

#### Scenario: 残留custom引用阻止组织删除
- **WHEN** 目标组织仍被普通角色当前custom配置引用，不论该角色启用状态或成员数量
- **THEN** 系统 SHALL 拒绝整次删除，组织、范围、关系与版本无本请求变化

#### Scenario: 解除引用后合法删除
- **WHEN** 有权限者已解除custom引用或切换范围，且其他组织删除前提满足
- **THEN** 系统 SHALL 允许按既有删除契约继续，不删除无关角色或配置

#### Scenario: 树外祖先范围依赖者失效
- **WHEN** 组织移动改变旧或新祖先根的org_and_children覆盖，某用户在移动子树外但通过其成员根和角色依赖该层级
- **THEN** 系统 SHALL 将该用户纳入版本处理，提交后旧Access/Refresh失效
- **AND** 不得因其另有all角色或重复授权使最终有效范围未变而遗漏该依赖

#### Scenario: 移动事实和多个用户版本共同回滚
- **WHEN** 组织移动后处理第一个或后续依赖用户版本失败
- **THEN** 系统 SHALL 回滚父级变化及本次全部用户版本，不留下新树配旧会话版本

#### Scenario: 相同父级及不依赖该层级的用户
- **WHEN** 请求没有真实parent_id变化，或某用户没有本次改变的层级范围依赖
- **THEN** 系统 SHALL 不因本目标提升其版本，不机械使用全部用户或移动节点全体成员

### Requirement: 组织树
系统 SHALL 支持管理员查询组织树。

#### Scenario: 查询组织树
- **WHEN** 管理员请求 `GET /api/admin/organizations/tree`
- **THEN** 系统返回按 parent_id 组织的树形结构
- **AND** 同级节点按 sort asc, id asc 排序

### Requirement: 组织树节点可管理标志
`GET /api/admin/organizations/tree` SHALL 在现有按 `parent_id` 组装、同级按 `sort asc, id asc` 排序的组织树中，为每个返回节点提供 `manageable: boolean`；操作者数据范围内的节点为 true，范围外仅为展示树结构保留的祖先为 false。仅用于定位的祖先 SHALL NOT 作为组织详情、移动或成员写入的目标。

#### Scenario: 范围内节点与范围外祖先
- **WHEN** 管理员读取组织树，数据范围内的组织有范围外祖先
- **THEN** 系统返回用于定位的祖先节点并标记 `manageable = false`，同时将范围内节点标记 `manageable = true`
- **AND** 系统不返回与可见节点无关的其他范围外组织

#### Scenario: 拒绝操作定位祖先
- **WHEN** 管理员对 `manageable = false` 的祖先节点请求读取组织成员、修改 `parent_id` 或写入成员归属
- **THEN** 系统拒绝这些操作，而不把树中的定位节点解释为可管理组织

### Requirement: 逐人修改组织归属
系统 SHALL 提供受 `admin.users.id.organizations.put` 权限码保护的 `PUT /api/admin/users/:id/organizations`，请求为 `{organization_ids: number[], expected_access_version: number}`；该请求 SHALL 把提交的组织 ID 集合作为该用户可管理组织关系的完整目标集合，只对该用户的可管理关系做差量增删，保留该用户范围外现有关系及其他用户的全部成员关系。空数组 SHALL 仅清空该用户可管理的组织归属。

组织目标与用户身份的适用状态 SHALL 在本次关系提交时仍合法；新增组织关系与组织删除、用户软删除须共享对应生命周期保障。失败只撤销本请求，不撤销其他合法并发事务。

逐人组织无变化判断 SHALL 只比较本入口承诺的可管理关系，保留隐藏归属及原joined_at；仍须先锁定/按既有规则初始化比较版本并完成全部校验。合法无变化返回当前版本，不执行EnsureAndIncrement；真实修改仅提升目标用户一次，关系差量及版本共同提交。

#### Scenario: 修改单个普通用户的组织归属
- **WHEN** 有权限的管理员为范围内非本人、且未拥有 `admin` 角色的用户提交完整可管理组织 ID 集合和当前 `expected_access_version`
- **THEN** 系统仅为该用户增加新选择的组织关系并移除未提交的可管理组织关系，保留所有范围外关系及其他用户成员关系
- **AND** 仅可管理关系真实变化时，系统提升目标用户的授权版本，返回新的 `access_version` 并使旧 Access Token 和 Refresh Token 失效；合法无变化返回当前版本

#### Scenario: 清空可管理组织但保留隐藏归属
- **WHEN** 管理员对有范围外组织归属的可修改用户提交空 `organization_ids` 和当前授权版本
- **THEN** 系统只移除该用户当前可管理的组织关系，保留所有范围外关系及其原有 `joined_at`

#### Scenario: 保留已有关系加入时间
- **WHEN** 请求仍选择目标用户已有的可管理组织，且同时新增另一个组织关系
- **THEN** 系统保留未移除关系原有的 `joined_at`，并仅为新增关系记录新的 UTC `joined_at`
- **AND** 如果后来移除的组织重新加入，新的关系使用新的加入时间

#### Scenario: 拒绝范围外或不存在的组织
- **WHEN** 请求包含不存在的组织、范围外组织或 `manageable = false` 的祖先节点作为新增归属
- **THEN** 系统拒绝整个请求，不修改任何组织关系或授权版本

#### Scenario: 过期组织写入
- **WHEN** 请求的 `expected_access_version` 不是提交时目标用户的当前授权版本
- **THEN** 系统返回 HTTP 409 和 `ORG_CONFLICT`
- **AND** 系统不修改任何成员关系或授权版本，客户端须重新读取详情后才能提交新选择，不得静默覆盖并发变更

#### Scenario: 组织写入受保护或越权
- **WHEN** 请求试图修改操作者本人、已有 `admin` 角色的用户或操作者数据范围外的用户，或操作者缺少 `admin.users.id.organizations.put` 权限
- **THEN** 系统拒绝整个请求，组织关系和授权版本保持不变

#### Scenario: 并发编辑两个归属维度
- **WHEN** 两次角色或组织归属写入基于同一目标用户的 `expected_access_version` 并发提交
- **THEN** 两种写入均在各自事务中锁定该用户版本并先比较再变更，基于同一旧版本最多一项真实修改成功提交；合法无变化不提升版本
- **AND** 真实修改已提升版本后，仍携旧版本的另一项返回对应模块HTTP 409且无部分提交；无变化先完成则不阻止随后合法真实修改

#### Scenario: 组织删除先于归属写入
- **WHEN** 目标组织已合法删除，逐人归属请求随后提交
- **THEN** 系统 SHALL 拒绝该引用，不留下本次关系或版本部分变化

#### Scenario: 用户软删先于新增归属
- **WHEN** 目标用户在关系提交前已软删除
- **THEN** 系统 SHALL 拒绝正常新增归属，保留其授权版本防止旧会话复活

#### Scenario: 隐藏归属不妨碍合法无变化
- **WHEN** 请求目标与当前可管理组织关系相同，但用户另有不可管理的隐藏归属，当前版本匹配
- **THEN** 系统 SHALL 成功返回当前access_version，隐藏及保留关系时间均不变

#### Scenario: 同旧版本两次合法无变化
- **WHEN** 两次请求均满足保护/引用/范围前提，且各自在提交判断时仍无真实关系变化
- **THEN** 系统 SHALL 允许两次成功并保持版本

#### Scenario: 真实修改后旧版本无变化仍冲突
- **WHEN** 真实角色或组织修改先提升版本，随后请求虽与当前集合相同但携旧expected_access_version
- **THEN** 系统 SHALL 返回既有对应模块HTTP 409，失败请求不写入关系或版本

#### Scenario: 无版本行按既有初始版本比较
- **WHEN** 用户尚无独立版本行，合法逐人写入按既有Ensure初始化规则形成可比较初始版本且目标关系不变
- **THEN** 系统 SHALL 返回该当前版本而不调用EnsureAndIncrement；若请求被拒绝，本请求初始化须随事务回滚
- **AND** 非登录Token认证仍不得借此初始化缺行

### Requirement: 组织数据范围过滤
系统 SHALL 基于当前用户角色的数据范围过滤组织和用户查询结果。

#### Scenario: 查询组织列表时应用数据范围
- **WHEN** 管理员请求 `GET /api/admin/organizations`
- **THEN** 系统只返回当前用户数据范围内的组织
- **AND** 如果用户拥有全部数据范围，系统返回全部组织

#### Scenario: 查询组织树时应用数据范围
- **WHEN** 管理员请求 `GET /api/admin/organizations/tree`
- **THEN** 系统只返回当前用户数据范围内的组织节点
- **AND** 系统保留可见节点的祖先节点，保证树形结构可展示

#### Scenario: 查询用户列表时应用数据范围
- **WHEN** 管理员请求 `GET /api/admin/users`
- **THEN** 系统只返回当前用户数据范围内的用户
- **AND** `self` 数据范围至少允许用户看到自己

### Requirement: 防止组织循环
系统 SHALL 防止组织父子关系形成循环。

防循环 SHALL 对并发提交同样成立；父级合法性校验与父级变更必须基于同一受保护的当前拓扑，不能以请求早期读取替代提交前判断。合法根节点parent_id=0保持有效。

创建、真正parent_id修改及删除 SHALL 参与同一数据库拓扑串行化协议，并在各自最外层事务使用锁定当前父图复核管理范围、父级存在性及删除子级前提；不得只依赖进程锁、两个目标节点或锁前RR快照。

#### Scenario: 修改父组织
- **WHEN** 管理员修改组织的 parent_id
- **THEN** 系统 SHALL 拒绝将组织挂载到自身或自身后代下面

#### Scenario: 两个根节点交叉并发移动
- **WHEN** 两个现存根节点的请求分别尝试把对方设为父级，且各自已通过早期前置读取
- **THEN** 系统 SHALL 不允许两次变更共同提交为循环
- **AND** 成功终态必须无环；被拒绝或事务失败的请求不得留下自身写入

#### Scenario: 并发三节点长环
- **WHEN** 多个真实请求试图在不同分支间形成长度大于二的循环
- **THEN** 系统 SHALL 在提交保障下拒绝导致循环的变更，不仅处理固定双节点特例

#### Scenario: 等待后重读当前拓扑
- **WHEN** 移动请求等待其他拓扑写入完成后继续
- **THEN** 系统 SHALL 基于等待后受保护的当前父关系重新校验，而非旧RR快照

#### Scenario: 合法移动到根节点
- **WHEN** 管理员在既有管理前提下提交parent_id=0
- **THEN** 系统 SHALL 允许合法移到根节点，不将层级根哨兵套成成员目标非法ID

### Requirement: 组织成员绑定
系统 SHALL 支持管理员将用户绑定到组织。

组织整组覆盖 SHALL 对原始目标及实际移除用户完整校验存在和适用范围；本人或受保护关系的真实新增/移除 SHALL 整次拒绝。满足其他前提的原有本人/受保护关系可以原样保留，不能仅因出现在目标集合而拒绝维护其他合法成员。

完整替换可受控移除软删除普通用户的旧组织关系，但 SHALL 取得身份/范围/保护事实并满足其管理前提；不得把软删用户重新作为正常新增目标，不得丢弃授权版本记录或在用户恢复时重建已清关系。

纯组织成员完整替换 SHALL 在全部原始目标、实际移除对象及生命周期前提校验后，差量删除移除关系、插入新增关系，仅使实际新增/移除成员失效；保持成员关系及其joined_at不得重建。合法等价集合成功不提升版本，其他读取者不会仅因范围内人员数据变化而被额外失效。差量关系和去重后的实际变更用户版本 SHALL 在同一最外层事务提交。

#### Scenario: 分配组织成员
- **WHEN** 管理员请求 `POST /api/admin/organizations/:id/users`
- **THEN** 系统用请求中的 user_ids 覆盖该组织当前成员
- **AND** 如果组织不存在，系统 SHALL 拒绝操作
- **AND** 系统仅使实际新增或移除组织成员用户的旧 token 失效

#### Scenario: 查询组织成员
- **WHEN** 管理员请求 `GET /api/admin/organizations/:id/users`
- **THEN** 系统返回该组织下的用户列表

#### Scenario: 删除组织时清理成员关系
- **WHEN** 管理员删除组织
- **THEN** 系统 SHALL 删除该组织对应的用户关联记录

#### Scenario: 保留受保护关系维护普通成员
- **WHEN** 目标集合原样保留已有本人或受保护成员，只有其他合法普通成员关系变化
- **THEN** 系统 SHALL 允许满足全部存在与范围前提的请求
- **AND** 不得改写保留关系的joined_at

#### Scenario: 拒绝实际改变受保护成员
- **WHEN** 整组覆盖或清空会实际新增/移除本人或受保护身份归属
- **THEN** 系统 SHALL 拒绝整次请求，关系与版本无本请求变化

#### Scenario: 整组覆盖校验实际删除对象
- **WHEN** 完整目标省略一个范围外或其他不允许移除的旧成员
- **THEN** 系统 SHALL 整次拒绝，不静默改成仅可管理部分替换

#### Scenario: 受控清理软删旧普通成员
- **WHEN** 请求实际移除软删普通旧成员，且不存在本人/受保护/范围违规
- **THEN** 系统 SHALL 允许受控清理并按同事务规则处理版本，其他普通成员可合法维护

#### Scenario: 软删用户不能作为新增成员
- **WHEN** 请求把已软删除用户加入正常目标集合
- **THEN** 系统 SHALL 整次拒绝，不能因普通目录查不到而跳过判断或直接插入断链关系

#### Scenario: 组织成员差量与时间保留
- **WHEN** 合法完整目标从A/B变为B/C
- **THEN** 系统 SHALL 只使A/C失效，保留B的版本及原joined_at，只有C新增加入时间

#### Scenario: 组织成员集合无变化
- **WHEN** 请求完整集合合法且与当前集合等价
- **THEN** 系统 SHALL 成功不提升版本，并保留所有既有关系时间

### Requirement: 组织成员加入时间
系统 SHALL 持久化每个当前组织成员关系的加入时间，供内部消息首次可见公告判定历史已读状态。

#### Scenario: 成员关系建立
- **WHEN** 用户首次加入组织或组织成员覆盖操作新增用户
- **THEN** 关系 SHALL 写入 UTC `joined_at`
- **AND** 保留成员已有的 `joined_at`，不得因重复绑定重置加入时间

#### Scenario: 成员关系移除
- **WHEN** 用户从组织成员关系移除
- **THEN** 关系 SHALL 删除
- **AND** 后续重新加入 SHALL 生成新的加入时间

#### Scenario: 覆盖成员时保留既有加入时间
- **WHEN** 管理员用 `POST /api/admin/organizations/:id/users` 覆盖组织成员
- **THEN** 系统 SHALL 保留仍属于该组织用户的原始 `joined_at`
- **AND** 仅为新增成员写入新的加入时间

#### Scenario: 迁移既有成员关系
- **WHEN** 系统升级包含既有 `user_organizations` 记录的数据库
- **THEN** 系统 SHALL 为缺失 `joined_at` 的当前成员补写迁移执行时间
- **AND** 后续成员覆盖 SHALL 保留该补写时间
