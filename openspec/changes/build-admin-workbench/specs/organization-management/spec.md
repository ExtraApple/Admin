## ADDED Requirements

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
