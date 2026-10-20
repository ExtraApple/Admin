## MODIFIED Requirements

### Requirement: 逐人修改角色归属
系统 SHALL 提供受 `admin.users.id.roles.put` 权限码保护的 `PUT /api/admin/users/:id/roles`，请求为 `{role_ids: number[], expected_access_version: number}`；该请求 SHALL 只替换指定用户的实际 `user_roles` 关系，不替换角色下的其他用户，不改变用户的组织关系。空 `role_ids` SHALL 表示显式清空该用户可修改的角色归属。授权版本的检查、关系更新和版本提升 SHALL 在锁定该用户版本记录的同一事务中完成；尚无版本记录的用户按既有授权版本初始化规则处理。

角色与用户身份 SHALL 在关系提交时仍满足存在/生命周期前提；角色删除先完成不得新建其引用，绑定先合法完成后合法删除须清理该关系并原子处理真实依赖用户版本。

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

#### Scenario: 用户软删与角色绑定交错
- **WHEN** 用户软删除先提交，或正常绑定先提交后用户合法软删
- **THEN** 系统 SHALL 拒绝软删后的正常新增；先前合法关系的保留/受控清理按已确认生命周期执行
- **AND** 版本记录保留，恢复身份不复活删除前Token或已清归属

## ADDED Requirements

### Requirement: 软删旧角色成员的受控清理
角色完整替换对软删普通旧成员 SHALL 允许在管理范围、本人/保护规则均满足时受控移除；清理不是正常新增，系统 SHALL NOT 重新加入软删用户或忽略断链身份事实。

#### Scenario: 移除软删旧角色成员
- **WHEN** 完整角色成员替换实际移除软删普通用户且所有管理/保护前提满足
- **THEN** 系统 SHALL 允许清理并原子处理关系和版本，不删除独立版本记录

#### Scenario: 恢复不还原被清除归属
- **WHEN** 受控清理后用户恢复身份
- **THEN** 系统 SHALL 保留清理后的关系事实，不自动恢复角色或组织归属
