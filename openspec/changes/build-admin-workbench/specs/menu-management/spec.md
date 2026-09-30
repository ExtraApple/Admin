## MODIFIED Requirements

### Requirement: 角色菜单分配
系统 SHALL 允许管理员替换某个角色拥有的菜单列表，并 SHALL 在管理员查询分配结果时返回按 `sort asc, id asc` 排序的平铺已分配菜单项（含 `parent_id` 和 `status`），不以运行时用户菜单可见性或未分配父级的存在与否裁剪配置。

#### Scenario: 分配菜单到角色
- **WHEN** 管理员提交角色 ID 和菜单 ID 列表
- **THEN** 系统清除该角色已有的角色菜单记录
- **AND** 系统插入本次提交的关联记录

#### Scenario: 查询角色菜单
- **WHEN** 管理员请求 `GET /api/admin/roles/:id/menus` 查询某个角色拥有的菜单
- **THEN** 系统返回该角色所有显式分配菜单的平铺列表，包括已停用菜单及每项的 `parent_id`、`status`，不按角色是否拥有菜单要求的权限码过滤
- **AND** 如果没有已分配菜单，系统返回空列表

#### Scenario: 仅分配子菜单或停用菜单
- **WHEN** 某角色只分配了子菜单而未分配其父级，或某已分配菜单随后被停用
- **THEN** 管理员查询该角色菜单时仍能看到这些已分配菜单及其状态
- **AND** 未显式分配的父级 SHALL NOT 作为已分配菜单返回

## ADDED Requirements

### Requirement: 管理员菜单配置与用户可见菜单分离
系统 SHALL 保持 `GET /api/user/context` 的用户可见菜单规则不变；管理员角色菜单读取 SHALL NOT 使停用菜单自动对用户可见，也 SHALL NOT 绕过用户的有效权限码检查。

#### Scenario: 用户上下文过滤已停用菜单
- **WHEN** 用户的角色分配包含已停用菜单
- **THEN** `GET /api/user/context` 不在该用户的可见菜单树中返回该停用菜单
- **AND** 同一菜单仍可出现在管理员的角色已分配菜单结果中

#### Scenario: 用户上下文按有效权限过滤
- **WHEN** 普通用户的角色分配包含已启用但要求其未拥有的权限码的菜单
- **THEN** `GET /api/user/context` 不将该菜单作为用户可见菜单返回
- **AND** 管理员查询角色已分配菜单时仍能看到该菜单配置
