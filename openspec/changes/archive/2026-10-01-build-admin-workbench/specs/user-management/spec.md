## MODIFIED Requirements

### Requirement: 管理员用户列表
系统 SHALL 允许管理员通过 `GET /api/admin/users?page=&size=&keyword=&status=` 在其数据范围内分页查询脱敏用户；每项 SHALL 返回从实际关系读取的 `roles[]`（角色 ID、编码、名称）、当前操作者可管理的 `organizations[]`（组织 ID、名称）和 `has_unmanaged_organizations`，不得以旧 `users.role` 字段推断用户的角色归属。组织数组为空而标志为 true 时，不得将其解释为用户无组织归属。

#### Scenario: 管理员查询用户列表
- **WHEN** 管理员调用 `GET /api/admin/users`
- **THEN** 系统返回数据范围内分页后的脱敏用户列表，每项附有真实的角色数组、可管理组织数组和范围外组织归属标志
- **AND** 响应包含与筛选结果一致的总数、页码和每页数量

#### Scenario: 按用户名昵称和状态筛选
- **WHEN** 管理员提交 `keyword` 和 `status` 并指定页码及每页数量
- **THEN** 系统在操作者的数据范围内按用户名或昵称及状态筛选后分页，`total` 表示筛选后符合条件的用户数
- **AND** 系统 SHALL NOT 返回范围外用户

#### Scenario: 返回真实多角色和组织归属
- **WHEN** 用户拥有多个角色，且同时属于可管理组织和范围外组织
- **THEN** 对该用户的列表项，系统返回每个实际角色的 ID、编码与名称，只列出可管理组织的 ID 和名称，并设置 `has_unmanaged_organizations = true`
- **AND** 系统 SHALL NOT 将范围外组织的标识或名称泄露到 `organizations[]`

#### Scenario: 无匹配用户
- **WHEN** 指定筛选条件在操作者的数据范围内没有匹配的用户
- **THEN** 系统返回空用户列表和 `total = 0`

## ADDED Requirements

### Requirement: 管理员按 ID 查询用户详情
系统 SHALL 提供受 `admin.users.id.get` 权限码保护的 `GET /api/admin/users/:id`；对数据范围内的用户返回脱敏资料、真实 `roles[]`、当前操作者可管理的 `organizations[]`、`has_unmanaged_organizations` 和该用户当前的 `access_version`，以支持安全的逐人编辑。

#### Scenario: 读取可见用户详情
- **WHEN** 有 `admin.users.id.get` 权限的管理员查询数据范围内的用户，包括本人或已有 `admin` 角色的受保护用户
- **THEN** 系统返回该用户的真实多角色、范围内组织、隐藏组织标志及当前授权版本，且不返回密码哈希或范围外组织身份

#### Scenario: 用户有不可见组织归属
- **WHEN** 目标用户的某个组织归属在操作者可管理范围之外
- **THEN** 系统将 `has_unmanaged_organizations` 设为 true，并且只在 `organizations[]` 中返回可管理关系

#### Scenario: 详情不可见或缺少权限
- **WHEN** 目标用户不在操作者数据范围内，或操作者缺少 `admin.users.id.get` 权限
- **THEN** 系统拒绝详情读取，不暴露目标用户的角色、组织归属或授权版本

#### Scenario: 用户不存在
- **WHEN** 管理员按 ID 请求不存在的用户
- **THEN** 系统返回未找到错误，不返回其他用户的归属或授权版本

### Requirement: 逐人修改角色归属
系统 SHALL 提供受 `admin.users.id.roles.put` 权限码保护的 `PUT /api/admin/users/:id/roles`，请求为 `{role_ids: number[], expected_access_version: number}`；该请求 SHALL 只替换指定用户的实际 `user_roles` 关系，不替换角色下的其他用户，不改变用户的组织关系。空 `role_ids` SHALL 表示显式清空该用户可修改的角色归属。授权版本的检查、关系更新和版本提升 SHALL 在锁定该用户版本记录的同一事务中完成；尚无版本记录的用户按既有授权版本初始化规则处理。

#### Scenario: 修改单个普通用户的多个角色
- **WHEN** 有权限的管理员为范围内非本人且未拥有 `admin` 角色的用户提交有效、已存在的角色 ID 集合和该用户当前 `expected_access_version`
- **THEN** 系统只将该用户的角色归属替换为所提交的集合，其他用户的角色关系保持不变
- **AND** 系统提升目标用户的授权版本，返回新的 `access_version`，使其旧 Access Token 和 Refresh Token 在后续请求失效

#### Scenario: 显式清空角色归属
- **WHEN** 管理员对可修改用户提交空 `role_ids` 和当前授权版本
- **THEN** 系统清除该用户的角色归属而不清除其他用户的归属

#### Scenario: 过期角色写入
- **WHEN** 请求的 `expected_access_version` 不是提交时目标用户的当前授权版本
- **THEN** 系统返回 HTTP 409 和 `AUTHZ_CONFLICT`
- **AND** 系统不修改任何角色关系或授权版本，客户端须重新读取详情后才能提交新选择，不得静默覆盖并发变更

#### Scenario: 角色写入受保护或越权
- **WHEN** 请求试图修改操作者本人、已有 `admin` 角色的用户或操作者数据范围外的用户，或请求试图通过添加／移除 `admin` 角色改变管理权限
- **THEN** 系统拒绝写入，角色关系和授权版本保持不变

#### Scenario: 角色 ID 无效或缺少写入权限
- **WHEN** 请求包含不存在的角色 ID，或操作者缺少 `admin.users.id.roles.put` 权限
- **THEN** 系统拒绝整个请求，不部分更新角色归属，也不提升目标用户的授权版本
