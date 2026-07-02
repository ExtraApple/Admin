# rbac Specification

## Purpose

RBAC 模块负责角色、权限、权限分组、用户角色关系、角色权限关系，以及供前后端授权使用的权限码生成。

## Requirements

### Requirement: 角色 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除角色。

#### Scenario: 创建角色
- **WHEN** 管理员提交唯一的角色名称和角色编码
- **THEN** 系统创建包含名称、编码、描述、排序和状态的角色

#### Scenario: 查询角色列表
- **WHEN** 管理员查询角色列表
- **THEN** 系统按排序和 ID 返回分页角色列表

#### Scenario: 修改角色
- **WHEN** 管理员修改角色，并且新名称或新编码不冲突
- **THEN** 系统应用提交的字段
- **AND** 系统使该角色下用户的旧 token 失效

#### Scenario: 超级管理员角色受保护
- **WHEN** 管理员尝试修改或删除编码为 `admin` 的角色
- **THEN** 系统拒绝操作

#### Scenario: 删除角色
- **WHEN** 管理员删除非保护角色
- **THEN** 系统删除该角色对应的用户角色关联
- **AND** 系统硬删除角色记录
- **AND** 系统使原属于该角色的用户旧 token 失效

### Requirement: 用户角色分配
系统 SHALL 允许管理员替换某个角色下的用户列表。

#### Scenario: 分配用户到角色
- **WHEN** 管理员提交角色 ID 和用户 ID 列表
- **THEN** 系统清除该角色已有的用户角色记录
- **AND** 系统插入本次提交的关联记录
- **AND** 系统使新旧关联用户的旧 token 失效

#### Scenario: 查询角色用户
- **WHEN** 管理员查询某个角色下的用户
- **THEN** 系统返回分配到该角色的脱敏用户列表
- **AND** 如果没有用户，系统返回空列表

### Requirement: 角色数据权限
系统 SHALL 支持按角色配置数据范围，用于限制管理员可见的业务数据。

#### Scenario: 配置角色数据范围
- **WHEN** 管理员为非 `admin` 角色提交 `all`、`self`、`org`、`org_and_children` 或 `custom`
- **THEN** 系统更新该角色的数据范围
- **AND** 如果数据范围为 `custom`，系统记录角色绑定的组织 ID 列表
- **AND** 系统使该角色下用户的旧 token 失效

#### Scenario: 查询角色数据范围
- **WHEN** 管理员查询某个角色的数据范围
- **THEN** 系统返回角色 ID、数据范围和自定义组织 ID 列表

#### Scenario: 超级管理员数据范围受保护
- **WHEN** 管理员尝试修改编码为 `admin` 的角色数据范围
- **THEN** 系统拒绝操作
- **AND** `admin` 角色固定拥有全部数据权限

### Requirement: 超级管理员兜底初始化
系统 SHALL 在启动时保证新 RBAC 体系中存在启用的超级管理员。

#### Scenario: admin 角色不存在
- **WHEN** 服务启动并完成数据库迁移
- **AND** `roles.code = admin` 不存在
- **THEN** 系统自动创建启用状态的 admin 角色

#### Scenario: 已存在启用超级管理员
- **WHEN** 服务启动时已经存在启用用户绑定到 `roles.code = admin`
- **THEN** 系统不创建新的超级管理员
- **AND** 系统不重置现有超级管理员密码

#### Scenario: 没有启用超级管理员
- **WHEN** 服务启动时不存在启用用户绑定到 `roles.code = admin`
- **THEN** 系统使用配置中的管理员用户名、邮箱、昵称和环境变量中的密码创建超级管理员用户
- **AND** 系统通过 `user_roles` 将该用户绑定到 admin 角色

#### Scenario: 配置用户名已存在但未绑定 admin 角色
- **WHEN** 配置中的管理员用户名已经存在
- **AND** 该用户未绑定 `roles.code = admin`
- **THEN** 系统拒绝启动，避免误提升普通用户权限

#### Scenario: 管理员身份判断
- **WHEN** 系统判断用户是否为超级管理员
- **THEN** 系统只依据 `users -> user_roles -> roles.code = admin`
- **AND** 系统不依据 `users.role = admin`

### Requirement: 权限 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除权限记录。

#### Scenario: 创建权限
- **WHEN** 管理员提交唯一的权限码
- **THEN** 系统创建包含名称、权限码、分组和排序的权限

#### Scenario: 查询权限列表
- **WHEN** 管理员查询权限列表
- **THEN** 系统按排序和 ID 返回分页权限列表

#### Scenario: 修改权限
- **WHEN** 管理员修改权限名称、分组或排序
- **THEN** 系统应用提交的字段
- **AND** 系统不修改权限码

#### Scenario: 删除权限
- **WHEN** 管理员删除权限
- **THEN** 系统删除该权限对应的角色权限关联
- **AND** 系统硬删除权限记录
- **AND** 系统使受影响角色下用户的旧 token 失效

### Requirement: 角色权限分配
系统 SHALL 允许管理员替换某个角色拥有的权限列表。

#### Scenario: 分配权限到角色
- **WHEN** 管理员提交角色 ID 和权限 ID 列表
- **THEN** 系统清除该角色已有的角色权限记录
- **AND** 系统插入本次提交的关联记录
- **AND** 系统使该角色下用户的旧 token 失效

#### Scenario: 查询角色权限
- **WHEN** 管理员查询某个角色拥有的权限
- **THEN** 系统返回该角色的权限列表，并按排序排列
- **AND** 如果没有权限，系统返回空列表

### Requirement: 用户有效权限码
系统 SHALL 从用户拥有的角色中推导该用户的有效权限码。

#### Scenario: 用户拥有角色权限
- **WHEN** 系统为用户生成登录 token 或授权上下文
- **THEN** 系统通过用户角色和角色权限关联收集权限码

#### Scenario: 用户没有权限
- **WHEN** 用户没有任何可用的角色权限关联
- **THEN** 系统不返回权限码

### Requirement: 权限码列表
系统 SHALL 提供所有权限码，用于前端按钮级权限控制。

#### Scenario: 请求权限码列表
- **WHEN** 管理员调用权限码列表接口
- **THEN** 系统按分组和排序返回全部权限码
- **AND** 如果没有权限码，系统返回空列表而不是 null

### Requirement: 路由权限同步
系统 SHALL 能够扫描已注册 API 路由，并创建缺失的权限记录。

#### Scenario: 同步创建缺失权限
- **WHEN** 同步流程接收到 `/api/` 下的已注册路由
- **THEN** 系统跳过公开路由
- **AND** 系统根据路径和 HTTP 方法生成权限码
- **AND** 系统只为不存在的权限码创建记录

### Requirement: 权限分组 CRUD
系统 SHALL 允许管理员维护权限分组。

#### Scenario: 创建权限分组
- **WHEN** 管理员提交唯一的分组名称
- **THEN** 系统创建包含名称和排序的权限分组

#### Scenario: 修改权限分组
- **WHEN** 管理员修改分组名称或排序
- **THEN** 系统应用提交的字段

#### Scenario: 删除权限分组
- **WHEN** 管理员删除权限分组
- **THEN** 系统硬删除该分组记录


