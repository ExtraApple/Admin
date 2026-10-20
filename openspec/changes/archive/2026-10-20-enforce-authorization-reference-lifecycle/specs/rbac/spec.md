## MODIFIED Requirements

### Requirement: 角色 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除角色。

普通角色硬删除 SHALL 在同一事务清理user_roles、role_permissions、role_data_scopes和Navigation持有的role_menus，并处理全部实际原成员版本；共享Menu与Permission资源不得随角色删除。删除与新增引用 SHALL 共享角色生命周期提交保障。

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

### Requirement: 角色数据权限
系统 SHALL 支持按角色配置数据范围，用于限制管理员可见的业务数据。

custom SHALL 至少包含一个合法存在组织；原始非法ID不得被过滤。非custom模式的organization_ids SHALL 省略或为空，非空列表必须整次拒绝；全局配置管理权限不被臆造的另一套组织数据范围政策替代。

普通角色修改与专用范围入口成功切换到非custom模式时 SHALL 清除旧custom组织关联，不保存非活动custom配置；切回custom必须重新提交合法非空组织集合。

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

#### Scenario: 拒绝空custom和原始零ID
- **WHEN** 角色配置custom但组织列表为空，或原始列表含0/不存在组织
- **THEN** 系统 SHALL 拒绝且保持范围、关联与版本不变

#### Scenario: 拒绝非custom的不适用列表
- **WHEN** 配置all/self/org/org_and_children且organization_ids非空，包括仅含0
- **THEN** 系统 SHALL 在过滤前整次拒绝，不静默忽略列表

#### Scenario: 停用普通角色可配置
- **WHEN** 普通角色处于停用状态，管理员拥有对应配置权限且输入满足全部前提
- **THEN** 系统 SHALL 允许合法配置，不把配置成功当成当前有效授权

#### Scenario: 两个入口离开custom均清理
- **WHEN** 普通UpdateRole或专用数据范围入口从custom成功切换到all/self/org/org_and_children
- **THEN** 系统 SHALL 在同一事务更新范围并清空该角色旧role_data_scopes，按现行触发规则处理用户版本

#### Scenario: custom先提交阻止组织删除
- **WHEN** 仍有普通角色当前custom配置引用组织，包括角色停用或没有成员的情况
- **THEN** 系统 SHALL 拒绝组织删除，不自动清空custom或降级/扩大范围

#### Scenario: 组织先删除则custom拒绝
- **WHEN** 组织已合法删除后请求新建指向该组织的custom配置
- **THEN** 系统 SHALL 拒绝整次配置，不留下范围或关联部分状态

### Requirement: 权限 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除权限记录。

权限ID关联写入与权限删除 SHALL 保持提交时引用合法；成功删除后不得再成功新增指向该已删除Permission实体的role_permissions。普通权限删除可按现行生命周期清理admin引用，但不得改写admin用户身份/固定范围。

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
