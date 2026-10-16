## MODIFIED Requirements

### Requirement: 组织单位 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除组织单位。

组织删除若将连带移除操作者本人或受保护身份的组织归属 SHALL 整次拒绝；普通权限/菜单生命周期的受控引用清理例外不得扩大为组织成员删除豁免。

只要普通角色当前custom范围仍引用组织，组织删除 SHALL 拒绝，角色停用或当前无成员不豁免。解除引用须由有相应配置权限者先完成；删除本身不得清空custom、改成self/all或留下空custom。

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

### Requirement: 组织成员绑定
系统 SHALL 支持管理员将用户绑定到组织。

组织整组覆盖 SHALL 对原始目标及实际移除用户完整校验存在和适用范围；本人或受保护关系的真实新增/移除 SHALL 整次拒绝。满足其他前提的原有本人/受保护关系可以原样保留，不能仅因出现在目标集合而拒绝维护其他合法成员。

完整替换可受控移除软删除普通用户的旧组织关系，但 SHALL 取得身份/范围/保护事实并满足其管理前提；不得把软删用户重新作为正常新增目标，不得丢弃授权版本记录或在用户恢复时重建已清关系。

#### Scenario: 分配组织成员
- **WHEN** 管理员请求 `POST /api/admin/organizations/:id/users`
- **THEN** 系统用请求中的 user_ids 覆盖该组织当前成员
- **AND** 如果组织不存在，系统 SHALL 拒绝操作
- **AND** 系统使新旧组织成员用户的旧 token 失效

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

### Requirement: 逐人修改组织归属
系统 SHALL 提供受 `admin.users.id.organizations.put` 权限码保护的 `PUT /api/admin/users/:id/organizations`，请求为 `{organization_ids: number[], expected_access_version: number}`；该请求 SHALL 把提交的组织 ID 集合作为该用户可管理组织关系的完整目标集合，只对该用户的可管理关系做差量增删，保留该用户范围外现有关系及其他用户的全部成员关系。空数组 SHALL 仅清空该用户可管理的组织归属。

组织目标与用户身份的适用状态 SHALL 在本次关系提交时仍合法；新增组织关系与组织删除、用户软删除须共享对应生命周期保障。失败只撤销本请求，不撤销其他合法并发事务。

#### Scenario: 修改单个普通用户的组织归属
- **WHEN** 有权限的管理员为范围内非本人、且未拥有 `admin` 角色的用户提交完整可管理组织 ID 集合和当前 `expected_access_version`
- **THEN** 系统仅为该用户增加新选择的组织关系并移除未提交的可管理组织关系，保留所有范围外关系及其他用户成员关系
- **AND** 系统提升目标用户的授权版本，返回新的 `access_version`，使其旧 Access Token 和 Refresh Token 在后续请求失效

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
- **THEN** 两种写入均在各自数据库事务中锁定该用户的授权版本并先比较再变更，最多一项成功并返回新版本
- **AND** 另一项返回对应模块的 HTTP 409 冲突且不部分提交关系变更

#### Scenario: 组织删除先于归属写入
- **WHEN** 目标组织已合法删除，逐人归属请求随后提交
- **THEN** 系统 SHALL 拒绝该引用，不留下本次关系或版本部分变化

#### Scenario: 用户软删先于新增归属
- **WHEN** 目标用户在关系提交前已软删除
- **THEN** 系统 SHALL 拒绝正常新增归属，保留其授权版本防止旧会话复活
