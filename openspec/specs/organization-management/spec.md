# organization-management Specification

## Purpose

组织管理用于维护组织单位层级，为组织成员归属和数据范围提供基础，
并约束组织树、成员关系、循环防护以及数据范围过滤行为。

## Requirements

### Requirement: 组织单位 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除组织单位。

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

### Requirement: 组织树
系统 SHALL 支持管理员查询组织树。

#### Scenario: 查询组织树
- **WHEN** 管理员请求 `GET /api/admin/organizations/tree`
- **THEN** 系统返回按 parent_id 组织的树形结构
- **AND** 同级节点按 sort asc, id asc 排序

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

#### Scenario: 修改父组织
- **WHEN** 管理员修改组织的 parent_id
- **THEN** 系统 SHALL 拒绝将组织挂载到自身或自身后代下面

### Requirement: 组织成员绑定
系统 SHALL 支持管理员将用户绑定到组织。

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
