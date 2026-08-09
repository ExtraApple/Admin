## MODIFIED Requirements

### Requirement: 菜单 API 联动
系统 SHALL 允许管理员把菜单按钮和 API 元数据绑定，并保证菜单权限码、API 权限码、角色授权和相关用户会话状态一致。

#### Scenario: 菜单绑定 API
- **WHEN** 管理员向 `POST /api/admin/menus/:id/apis` 提交菜单 ID、API ID 列表和可选权限码
- **THEN** 系统校验菜单存在
- **AND** 系统校验 API 均存在、已启用且需要认证
- **AND** 系统 SHALL 在同一数据库事务中覆盖写入 `menu_apis`
- **AND** 系统 SHALL 同步菜单和绑定 API 的 `permission_code`
- **AND** 系统 SHALL 确保权限表存在对应权限码
- **AND** 新权限码已存在时系统 SHALL 合并并去重既有角色授权
- **AND** 系统 SHALL 在同一事务中使变更前后受影响用户的旧 token 失效

#### Scenario: 菜单绑定 API 失败
- **WHEN** 菜单、API、`menu_apis`、权限、角色授权或 token 版本任一步写入失败
- **THEN** 系统 SHALL 回滚本次绑定操作的全部数据库写入
- **AND** 菜单、API、权限关联和用户 token 版本 SHALL 保持事务开始前状态

#### Scenario: 查询菜单绑定 API
- **WHEN** 管理员调用 `GET /api/admin/menus/:id/apis`
- **THEN** 系统返回该菜单绑定的 API 元数据列表
- **AND** 如果没有绑定 API，系统返回空列表

#### Scenario: 删除菜单清理 API 关联
- **WHEN** 管理员删除叶子菜单
- **THEN** 系统清理该菜单对应的 `menu_apis` 关联

#### Scenario: 菜单权限变更实时生效
- **WHEN** 菜单绑定 API 或菜单权限码发生联动变更
- **THEN** 系统 SHALL 在保存权限关系的同一数据库事务中提升相关 token 版本
- **AND** 相关用户的旧 token SHALL 在事务提交后的下一次请求时失效
