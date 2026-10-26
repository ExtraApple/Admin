## MODIFIED Requirements

### Requirement: 跨模块值类型保持稳定和最小
跨模块 Contract SHALL 使用业务值类型传递身份、授权快照和数据范围，不得泄漏持久化或 HTTP 实现。

#### Scenario: 创建请求 Principal
- **WHEN** Identity 完成 Token、活动登录会话、用户存在性和启用状态校验
- **THEN** 系统 SHALL 创建只携带用户 ID 的 Principal
- **AND** JWT 中的旧角色或权限快照 SHALL NOT 绕过运行时授权校验

#### Scenario: 返回 Access Snapshot
- **WHEN** Identity 请求用户初始化上下文或其他模块请求授权快照
- **THEN** Authorization SHALL 返回最小的角色、权限和授权版本值
- **AND** 返回值 SHALL NOT 包含 GORM Model、SQL 或基础设施客户端

### Requirement: 模块所有权保持分离
Authorization、Navigation、API Metadata、Identity 和 Organization SHALL 按各自变化原因维护业务能力。

#### Scenario: 授权和导航数据归属
- **WHEN** 系统维护角色、权限、用户角色、角色权限、数据范围、菜单、角色菜单或菜单 API 关联
- **THEN** 角色、权限、用户角色、角色权限和数据范围 SHALL 由 Authorization 拥有
- **AND** 菜单、`role_menus`、`menu_apis` 和可见菜单树 SHALL 由 Navigation 拥有

#### Scenario: 角色菜单路由归属
- **WHEN** 系统处理 `POST /api/admin/roles/:id/menus` 或 `GET /api/admin/roles/:id/menus`
- **THEN** 路由和用例 SHALL 由 Navigation 拥有
- **AND** URL 中出现角色资源 SHALL NOT 改变 `role_menus` 的数据所有权

#### Scenario: 身份和 API 元数据归属
- **WHEN** 系统处理 JWT、Refresh Token、用户状态、用户上下文或 API 运行时策略
- **THEN** JWT、Refresh Token、登录会话及其撤销、用户状态和用户上下文编排 SHALL 由 Identity 拥有
- **AND** `apis`、状态、认证标记、审计标记和 Permission Code 策略 SHALL 由 API Metadata 拥有

#### Scenario: 组织、文件和系统能力归属
- **WHEN** 系统维护组织树、组织成员、字典、管理员文件、用户头像、审计、OpenAPI 或上传验证
- **THEN** 组织树和成员关系 SHALL 由 Organization 拥有
- **AND** 字典类型和条目 SHALL 由 Dictionary 拥有
- **AND** 管理员文件 SHALL 由 Files 拥有，用户头像 SHALL 由 Identity 拥有
- **AND** 审计采集和记录 SHALL 由 Audit 拥有
- **AND** OpenAPI 生成 SHALL 由 API Doc 拥有，文件内容验证 SHALL 由 Upload Security 提供
