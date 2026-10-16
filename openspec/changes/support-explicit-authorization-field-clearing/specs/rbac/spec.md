## MODIFIED Requirements

### Requirement: 角色 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除角色。

普通角色硬删除 SHALL 在同一事务清理user_roles、role_permissions、role_data_scopes和Navigation持有的role_menus，并处理全部实际原成员版本；共享Menu与Permission资源不得随角色删除。删除与新增引用 SHALL 共享角色生命周期提交保障。

角色description更新 SHALL 区分遗漏与显式空字符串：遗漏保持，显式空清空，非空替换。name/code的显式空值不是合法清空，系统 SHALL 拒绝整次请求；清空不绕过现有角色保护、唯一性及事务约束。

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

#### Scenario: 普通角色跨模块清理
- **WHEN** 合法普通角色删除存在用户、权限、自定义组织与菜单映射
- **THEN** 系统 SHALL 原子清理全部四类映射、删除角色并处理实际原成员版本
- **AND** 不得删除共享菜单或权限；任一步失败回滚本请求全部事实与版本

#### Scenario: 绑定先提交后合法删除
- **WHEN** 角色绑定先成功提交，后续删除仍满足保护/范围等前提
- **THEN** 系统 SHALL 允许两次成功，删除包括新关联在内的适用关系并处理其真实用户版本
- **AND** 不得因删除早期预读漏掉新成员的版本影响

#### Scenario: 删除先提交后新增绑定
- **WHEN** 角色删除已经成功，另一个关联请求随后进入提交判断
- **THEN** 系统 SHALL 拒绝新增已删除角色引用，不重建角色或留下关系/版本部分写入

#### Scenario: 角色说明显式清空
- **WHEN** 合法普通角色请求明确提交description为空字符串
- **THEN** 系统 SHALL 清空角色说明，未提交的其他字段保持原值

#### Scenario: 角色说明遗漏保持
- **WHEN** 合法更新其他字段但省略description
- **THEN** 系统 SHALL 保持原说明，不因DTO零值意外清空

#### Scenario: 拒绝清空角色必填标识
- **WHEN** 更新明确提交空name或code，即使同时有合法description
- **THEN** 系统 SHALL 拒绝整次请求，不部分清空说明或改变版本

### Requirement: 权限 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除权限记录。

权限ID关联写入与权限删除 SHALL 保持提交时引用合法；成功删除后不得再成功新增指向该已删除Permission实体的role_permissions。普通权限删除可按现行生命周期清理admin引用，但不得改写admin用户身份/固定范围。

权限group标签更新 SHALL 区分遗漏、显式空字符串和非空值；遗漏保持，显式空清空。此处group是权限标签而非PermissionGroup资源ID，系统 SHALL 保持现有不可修改权限码及其他校验规则。

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

#### Scenario: 权限删除与新角色引用交错
- **WHEN** 权限删除先提交或权限绑定先合法提交再删除
- **THEN** 系统 SHALL 分别拒绝删除后的新引用，或按合法先后清理已提交引用并原子处理实际影响用户
- **AND** 不因事务时间重叠就要求两次请求必有一方失败

#### Scenario: 清空权限分组标签
- **WHEN** 合法权限更新显式提交group为空字符串
- **THEN** 系统 SHALL 清空该权限标签，未提交字段保持不变，不删除分组或权限资源

#### Scenario: 拒绝清空权限必填名称
- **WHEN** 请求明确提交空权限名称
- **THEN** 系统 SHALL 拒绝整次更新，不部分应用其他字段
