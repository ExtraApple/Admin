## ADDED Requirements

### Requirement: 管理员按 ID 查询角色详情
系统 SHALL 提供受 `admin.roles.id.get` 权限码保护的 `GET /api/admin/roles/:id`，返回指定角色的稳定 ID、名称、编码、描述、排序、状态和数据范围，供角色独立详情页使用；该读取 SHALL NOT 修改角色授权或用户会话。

#### Scenario: 读取存在的角色
- **WHEN** 具有 `admin.roles.id.get` 权限的管理员按 ID 查询一个已存在的角色
- **THEN** 系统返回该角色的身份、状态及数据范围详情，包括编码为 `admin` 的受保护角色

#### Scenario: 角色不存在
- **WHEN** 管理员按 ID 查询不存在的角色
- **THEN** 系统返回未找到错误，而不是用列表中的其他角色代替

#### Scenario: 缺少角色详情权限
- **WHEN** 已认证用户缺少 `admin.roles.id.get` 权限而请求角色详情
- **THEN** 系统拒绝读取该管理接口
