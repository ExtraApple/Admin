# C6b 授权事实写入审计 · 现行条款快照

> 2026-10-14；代码基线 `c4b96ce1d64daede23a8d867d0a990e1c5887c04`。只保留已读取现行主规格的 Requirement 正文与 Scenario 原文，不修改主规格。收录不是审计通过；覆盖状态见各审计矩阵。SHA-256 为原文件字节散列。

## rbac

来源：`openspec/specs/rbac/spec.md`；SHA-256：`11e08caeea1980c7ace8ffb8449b8785189658745a33e499123a0dc46e9d0a64`。

```markdown
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
系统 SHALL 能够从 Route Catalog 的已校验路由快照同步缺失的权限记录，而不是扫描已注册的 Gin 路由。

#### Scenario: 同步创建缺失权限
- **WHEN** 管理员调用 `POST /api/admin/permissions/sync`
- **THEN** 系统读取 Route Catalog Snapshot
- **AND** 系统跳过 `Public` 路由
- **AND** 系统根据受保护 Route Descriptor 的默认 Permission Code 创建缺失权限码；没有默认值时按既有 `method + path` 规则生成
- **AND** 系统只为不存在的权限码创建记录
- **AND** 系统 SHALL NOT 通过扫描 Gin Engine 发现路由

#### Scenario: 启动 Seed 同步权限
- **WHEN** App 启动执行权限 Seed
- **THEN** Seed SHALL 消费与 API Metadata 和 API Doc 相同的 Route Catalog Snapshot
- **AND** Seed SHALL 保持幂等，不重复创建已有权限码
- **AND** Route Catalog 收集和 Gin 路由注册 SHALL NOT 隐式创建权限记录

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
```

## menu-management

来源：`openspec/specs/menu-management/spec.md`；SHA-256：`79ec4eda413bdf52597b4805f4b762fff9a4b938bb8fbd6b50c703ac80e99209`。

```markdown
### Requirement: 菜单树查询
系统 SHALL 按排序和 ID 返回树形菜单。

#### Scenario: 管理员查询菜单
- **WHEN** 管理员调用菜单列表接口
- **THEN** 系统返回按 `parent_id` 组装的所有菜单
- **AND** 子菜单嵌套在父菜单的 `children` 中

### Requirement: 菜单 CRUD
系统 SHALL 允许管理员创建、修改和删除菜单记录。

#### Scenario: 创建菜单
- **WHEN** 管理员提交菜单名称，以及可选的路径、组件、图标、权限码、父级、排序、类型和状态
- **THEN** 系统创建菜单
- **AND** 当类型或状态未提供或无效时，系统使用默认值

#### Scenario: 创建按钮菜单
- **WHEN** 管理员创建 `type = 3` 的按钮菜单
- **THEN** 系统允许菜单路径为空
- **AND** 系统可保存该按钮菜单绑定的 `permission_code`

#### Scenario: 菜单路径冲突
- **WHEN** 创建或修改请求使用了已被其他菜单占用的路径
- **THEN** 系统拒绝请求

#### Scenario: 父级菜单指向自己
- **WHEN** 修改请求将 `parent_id` 设置为当前菜单自身 ID
- **THEN** 系统拒绝请求

#### Scenario: 修改菜单
- **WHEN** 管理员提交有效的菜单变更字段
- **THEN** 系统只更新提交的字段
- **AND** 如果没有可更新字段，系统拒绝请求

#### Scenario: 删除存在子菜单的菜单
- **WHEN** 管理员删除仍有子菜单的菜单
- **THEN** 系统拒绝删除

#### Scenario: 删除叶子菜单
- **WHEN** 管理员删除没有子菜单的菜单
- **THEN** 系统删除该菜单对应的角色菜单关联
- **AND** 系统删除该菜单对应的菜单 API 关联
- **AND** 系统硬删除菜单记录

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


### Requirement: 用户菜单树
系统 SHALL 从用户拥有的所有角色中推导该用户可见的菜单树。

#### Scenario: 用户拥有角色菜单
- **WHEN** 系统构建用户初始化上下文
- **THEN** 系统收集用户所有角色关联的菜单 ID
- **AND** 系统对菜单 ID 去重
- **AND** 系统返回启用菜单组成的树

#### Scenario: 用户菜单按权限码过滤
- **WHEN** 普通用户的角色绑定了菜单
- **AND** 菜单存在 `permission_code`
- **THEN** 只有当用户拥有该权限码时，系统才在初始化上下文中返回该菜单

#### Scenario: 父级目录保留
- **WHEN** 子菜单或按钮对用户可见
- **AND** 其父级目录本身没有权限码或不直接匹配权限码
- **THEN** 系统保留父级目录以组成完整菜单树

#### Scenario: 超级管理员菜单
- **WHEN** 用户拥有 `admin` 角色
- **THEN** 系统返回全部启用菜单

#### Scenario: 用户没有分配菜单
- **WHEN** 用户的角色没有任何角色菜单关联
- **THEN** 系统返回空菜单列表

### Requirement: 前端路由同步菜单
系统 SHALL 能够根据前端路由元数据创建缺失的菜单记录。

#### Scenario: 同步创建新菜单
- **WHEN** 管理员向同步接口提交路由元数据
- **THEN** 系统为不存在的路径创建菜单记录
- **AND** 如果存在 `parent_path`，系统根据它解析父菜单
- **AND** 系统保存路由元数据中的 `permission_code`
- **AND** 系统返回本次创建的菜单数量

#### Scenario: 同步遇到已存在路径
- **WHEN** 提交的路由路径已存在于菜单表
- **THEN** 系统跳过该条数据

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
```

## organization-management

来源：`openspec/specs/organization-management/spec.md`；SHA-256：`e74357a9fb24eb48b8a552e0f44b87c5c1217e2d7e7aff8de6c1a69873c05abc`。

```markdown
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

### Requirement: 组织树节点可管理标志
`GET /api/admin/organizations/tree` SHALL 在现有按 `parent_id` 组装、同级按 `sort asc, id asc` 排序的组织树中，为每个返回节点提供 `manageable: boolean`；操作者数据范围内的节点为 true，范围外仅为展示树结构保留的祖先为 false。仅用于定位的祖先 SHALL NOT 作为组织详情、移动或成员写入的目标。

#### Scenario: 范围内节点与范围外祖先
- **WHEN** 管理员读取组织树，数据范围内的组织有范围外祖先
- **THEN** 系统返回用于定位的祖先节点并标记 `manageable = false`，同时将范围内节点标记 `manageable = true`
- **AND** 系统不返回与可见节点无关的其他范围外组织

#### Scenario: 拒绝操作定位祖先
- **WHEN** 管理员对 `manageable = false` 的祖先节点请求读取组织成员、修改 `parent_id` 或写入成员归属
- **THEN** 系统拒绝这些操作，而不把树中的定位节点解释为可管理组织

### Requirement: 逐人修改组织归属
系统 SHALL 提供受 `admin.users.id.organizations.put` 权限码保护的 `PUT /api/admin/users/:id/organizations`，请求为 `{organization_ids: number[], expected_access_version: number}`；该请求 SHALL 把提交的组织 ID 集合作为该用户可管理组织关系的完整目标集合，只对该用户的可管理关系做差量增删，保留该用户范围外现有关系及其他用户的全部成员关系。空数组 SHALL 仅清空该用户可管理的组织归属。

#### Scenario: 修改单个普通用户的组织归属
- **WHEN** 有权限的管理员为范围内非本人、且未拥有 `admin` 角色的用户提交完整可管理组织 ID 集合和当前 `expected_access_version`
- **THEN** 系统仅为该用户增加新选择的组织关系并移除未提交的可管理组织关系，保留所有范围外关系及其他用户成员关系
- **AND** 系统提升目标用户的授权版本，返回新的 `access_version`，使其旧 Access Token 和 Refresh Token 在后续请求失效

#### Scenario: 清空可管理组织但保留隐藏归属
- **WHEN** 管理员对有范围外组织归属的可修改用户提交空 `organization_ids` 和当前授权版本
- **THEN** 系统只移除该用户当前可管理的组织关系，保留所有范围外关系及其原有 `joined_at`

#### Scenario: 保留已有关系加入时间
- **WHEN** 请求仍选择目标用户已有的可管理组织，且同时新增另一个组织关系
- **THEN** 系统保留未移除关系原有的 `joined_at`，并仅为新增关系记录新的 UTC `joined_at`
- **AND** 如果后来移除的组织重新加入，新的关系使用新的加入时间

#### Scenario: 拒绝范围外或不存在的组织
- **WHEN** 请求包含不存在的组织、范围外组织或 `manageable = false` 的祖先节点作为新增归属
- **THEN** 系统拒绝整个请求，不修改任何组织关系或授权版本

#### Scenario: 过期组织写入
- **WHEN** 请求的 `expected_access_version` 不是提交时目标用户的当前授权版本
- **THEN** 系统返回 HTTP 409 和 `ORG_CONFLICT`
- **AND** 系统不修改任何成员关系或授权版本，客户端须重新读取详情后才能提交新选择，不得静默覆盖并发变更

#### Scenario: 组织写入受保护或越权
- **WHEN** 请求试图修改操作者本人、已有 `admin` 角色的用户或操作者数据范围外的用户，或操作者缺少 `admin.users.id.organizations.put` 权限
- **THEN** 系统拒绝整个请求，组织关系和授权版本保持不变

#### Scenario: 并发编辑两个归属维度
- **WHEN** 两次角色或组织归属写入基于同一目标用户的 `expected_access_version` 并发提交
- **THEN** 两种写入均在各自数据库事务中锁定该用户的授权版本并先比较再变更，最多一项成功并返回新版本
- **AND** 另一项返回对应模块的 HTTP 409 冲突且不部分提交关系变更

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
```

## access-version-storage

来源：`openspec/specs/access-version-storage/spec.md`；SHA-256：`0e4eae3c0b04fbec97d2ff506cc8da648310cdf7a85aa6da0ea6f1af6b1473cc`。

```markdown
### Requirement: Authorization 独立持有授权版本
系统 SHALL 使用 `user_access_versions` 作为用户授权版本的唯一持久化事实来源。

#### Scenario: 创建授权版本表
- **WHEN** 系统执行数据库 AutoMigrate
- **THEN** 系统 SHALL 创建以 `user_id` 为主键的 `user_access_versions`
- **AND** 每条记录 SHALL 包含默认值为 1 的非空正整数 `version`、`created_at` 和 `updated_at`
- **AND** 版本记录 SHALL NOT 使用软删除字段

#### Scenario: 用户被软删除
- **WHEN** 用户记录被软删除
- **THEN** 系统 SHALL 保留该用户的授权版本记录
- **AND** 数据库外键 SHALL NOT 级联删除授权版本

### Requirement: 新表是唯一读取事实来源
Access Token 和 Refresh Token 的授权版本校验 SHALL 只读取
`user_access_versions`。

#### Scenario: 授权版本存在
- **WHEN** 系统校验 Token 且新表存在用户版本
- **THEN** 系统 SHALL 使用该版本与 Token Claim 比较
- **AND** 系统 SHALL NOT 从用户身份记录读取或推导另一个授权版本

#### Scenario: 认证时授权版本缺失
- **WHEN** 非登录初始化流程校验 Token 时找不到用户版本
- **THEN** 系统 SHALL 拒绝该 Token
- **AND** 系统 SHALL NOT 跳过版本检查或回退其他存储

### Requirement: 新用户版本懒初始化
新用户 SHALL 在首次成功登录时幂等初始化授权版本，而不是由用户创建或 Seed
流程预先写入。

#### Scenario: 新用户首次登录
- **WHEN** Identity 已确认用户存在、启用且密码验证成功
- **AND** `user_access_versions` 不存在该用户记录
- **THEN** Authorization SHALL 通过 `EnsureVersion` 幂等创建版本 1
- **AND** Identity SHALL 使用重新读取的当前版本签发 Access Token 和 Refresh Token

#### Scenario: 并发首次登录
- **WHEN** 同一新用户并发发起多个首次登录请求
- **THEN** 系统 SHALL 最多创建一条授权版本记录
- **AND** 所有成功签发的 Token SHALL 使用已提交的当前版本

#### Scenario: 初始化失败
- **WHEN** 授权版本初始化或重新读取失败
- **THEN** 系统 SHALL 拒绝登录
- **AND** 系统 SHALL NOT 签发缺少可验证版本的 Token

### Requirement: 会话失效原子创建并提升
所有需要使现有会话失效的操作 SHALL 在调用方事务中使用统一的
`EnsureAndIncrement` 语义。

#### Scenario: 已有授权版本时提升
- **WHEN** 系统执行密码变化、用户状态变化、Kick 或授权关系变化
- **THEN** 系统 SHALL 锁定并提升目标用户的授权版本
- **AND** `EnsureAndIncrement` SHALL 返回提交后的最终版本

#### Scenario: 首次登录前发生失效操作
- **WHEN** 用户尚无授权版本记录
- **AND** 系统执行禁用、软删除、Kick、密码变化或授权关系变化
- **THEN** 系统 SHALL 幂等创建版本 1 后再提升
- **AND** 系统 SHALL NOT 将零影响行的普通 UPDATE 视为成功

#### Scenario: 并发初始化和失效
- **WHEN** `EnsureVersion` 与 `EnsureAndIncrement` 并发执行
- **THEN** 系统 SHALL 只保留一条授权版本记录
- **AND** 最终版本 SHALL NOT 倒退

#### Scenario: 授权版本更新失败
- **WHEN** 调用方业务更新或授权版本初始化、锁定、提升任一步失败
- **THEN** 系统 SHALL 回滚同一事务中的全部变化

### Requirement: Seed 不维护用户授权版本
Seed SHALL 只维护基础数据，不得创建或修复用户授权版本。

#### Scenario: Seed 创建超级管理员
- **WHEN** Seed 创建或确认超级管理员用户
- **THEN** Seed SHALL NOT 创建 `user_access_versions`
- **AND** Seed SHALL NOT 扫描其他用户或修复授权版本
- **AND** 超级管理员 SHALL 在首次登录或首次失效操作时初始化授权版本
```

## api-management

来源：`openspec/specs/api-management/spec.md`；SHA-256：`98997e98ab0a850b825179ea5b8d30c1f6a69bb75c4915fd896709291c4643fe`。

```markdown
### Requirement: API 元数据 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除 API 元数据记录。

#### Scenario: 查询 API 列表
- **WHEN** 管理员调用 `GET /api/admin/apis`
- **THEN** 系统分页返回 API 元数据列表
- **AND** 支持按关键词、分组、HTTP 方法、状态、认证标记和审计标记筛选

#### Scenario: 查询 API 详情
- **WHEN** 管理员调用 `GET /api/admin/apis/:id`
- **THEN** 系统返回指定 API 元数据详情
- **AND** API 不存在时系统拒绝请求

#### Scenario: 创建 API 元数据
- **WHEN** 管理员提交名称、HTTP 方法和路径
- **THEN** 系统创建 API 元数据
- **AND** `method + path` SHALL 唯一
- **AND** 系统规范化 HTTP 方法和路径

#### Scenario: 修改 API 元数据
- **WHEN** 管理员修改 API 名称、方法、路径、分组、权限码、状态或开关字段
- **THEN** 系统应用提交的字段
- **AND** 如果方法或路径变更后与其他 API 冲突，系统拒绝请求

#### Scenario: API 元数据路径偏离真实路由
- **WHEN** 管理员把 PermissionControlled API 元数据的 Method 或 Path 修改为不再匹配 Route Catalog 真实路由
- **THEN** 真实路由 SHALL 按 API 未配置的现有安全语义拒绝访问
- **AND** 后续路由同步 SHALL 能够为真实 Method 和 Path 重新创建 API 元数据记录

#### Scenario: 修改已绑定菜单的 API 权限码
- **WHEN** 管理员修改已经通过 `menu_apis` 绑定菜单的 API 权限码
- **THEN** 系统 SHALL 在同一数据库事务中同步更新关联菜单和同一菜单绑定的其他 API 权限码
- **AND** 系统 SHALL 确保权限表存在新权限码
- **AND** 新权限码已存在时系统 SHALL 合并并去重旧权限对应的角色授权
- **AND** 旧权限 SHALL 仅在不再被菜单、API 或角色引用时清理
- **AND** 系统 SHALL 在同一事务中使变更前后受影响用户的旧 token 失效

#### Scenario: 修改已绑定 API 权限码失败
- **WHEN** API、菜单、权限、角色授权或 token 版本任一步写入失败
- **THEN** 系统 SHALL 回滚本次权限码变更的全部数据库写入
- **AND** API、菜单、权限、角色授权和用户 token 版本 SHALL 保持事务开始前状态

#### Scenario: 删除 API 元数据
- **WHEN** 管理员删除 API 元数据
- **THEN** 系统清理该 API 对应的 `menu_apis` 关联
- **AND** 系统硬删除 API 记录

### Requirement: API 辅助选项
系统 SHALL 提供 API 管理页面所需的辅助选项。

#### Scenario: 查询 API 分组列表
- **WHEN** 管理员调用 `GET /api/admin/api-groups`
- **THEN** 系统返回当前 API 元数据中的分组列表
- **AND** 每个分组包含 API 数量

#### Scenario: 查询 HTTP 方法列表
- **WHEN** 管理员调用 `GET /api/admin/api-methods`
- **THEN** 系统返回支持的 HTTP 方法选项

### Requirement: 自动化 API 文档
系统 SHALL 根据 Route Catalog 的已校验路由描述、公开错误定义和必要的 API Metadata Snapshot 生成 OpenAPI 文档，而不是扫描 Gin Engine。

#### Scenario: 访问 Swagger UI 页面
- **WHEN** `api_docs.enabled = true`
- **AND** 用户访问 `GET /docs`
- **THEN** 系统返回 Swagger UI 页面
- **AND** 成功 HTML SHALL NOT 使用业务 JSON 信封包装

#### Scenario: 获取 OpenAPI JSON
- **WHEN** `api_docs.enabled = true`
- **AND** 用户访问 `GET /docs/openapi.json`
- **THEN** 系统返回未包装的 OpenAPI 3.0 JSON
- **AND** 文档包含 `/api/` 前缀下的业务接口
- **AND** 文档结合 Route Catalog 与 `apis` 表元数据生成接口标题、分组、描述和认证要求
- **AND** 对已配置 DTO 映射的 JSON 请求接口生成请求体 Schema
- **AND** 上传接口生成 `multipart/form-data` 文件字段
- **AND** JSON 成功响应 SHALL 描述固定四字段成功信封
- **AND** JSON 错误响应 SHALL 按状态描述固定四字段错误信封
- **AND** 每个错误状态 SHALL 枚举该路由允许的顶层 `error_code`
- **AND** HTTP 422 字段校验响应 SHALL 枚举允许的字段级 `error_code` 并描述 `data.fields`
- **AND** 文档 SHALL 包含 App 根据 Access Level 组合的认证和权限中间件错误
- **AND** 二进制、头像和其他原生成功响应 SHALL 保留对应 Content-Type 和原生 Schema
- **AND** API Doc SHALL NOT 扫描 Gin Engine 或查询全局数据库状态

#### Scenario: OpenAPI 生成失败
- **WHEN** `GET /docs/openapi.json` 在成功文档提交前无法生成文档
- **THEN** 系统返回 HTTP 500 四字段 JSON 错误信封
- **AND** `error_code` SHALL 为稳定技术错误码 `HTTP_INTERNAL_ERROR`
- **AND** 响应 SHALL NOT 包含内部 Metadata、数据库或生成器错误

#### Scenario: 关闭自动化 API 文档
- **WHEN** `api_docs.enabled = false`
- **THEN** 系统不注册 `/docs` 和 `/docs/openapi.json` 路由

### Requirement: API 路由同步
系统 SHALL 能够从 Route Catalog 的已校验路由快照同步 API 元数据。

#### Scenario: 同步已声明 API 路由
- **WHEN** 管理员调用 `POST /api/admin/apis/sync`
- **THEN** 系统读取 Route Catalog 中已校验的 Route Descriptor
- **AND** 只处理 `/api/` 前缀下的路由
- **AND** 对已存在的 `method + path` 跳过创建
- **AND** 为需要权限控制的接口生成默认权限码
- **AND** 对 Public 路由以及当前兼容规则指定的公开字典接口标记为不需要权限码检查
- **AND** 对已存在但错误标记为需要认证的 `POST /api/refresh` 纠正 `need_auth` 并清空 `permission_code`
- **AND** 同步流程 SHALL NOT 扫描 Gin Engine 发现路由

#### Scenario: 同步公开 Refresh 元数据
- **WHEN** Route Catalog 中存在 `POST /api/refresh`
- **AND** API Metadata 中已有该 Method 和 Path 但 `need_auth = 1` 或存在 `permission_code`
- **THEN** 同步 SHALL 将其恢复为公开兼容配置
- **AND** 同步 SHALL 设置 `need_auth = 0` 并清空 `permission_code`
- **AND** 系统 SHALL NOT 因该数据库字段配置把 Refresh 路由注册为 PermissionControlled

#### Scenario: 同步已存在 API 元数据
- **WHEN** Route Catalog 的 Method 和 Path 已存在对应 API 元数据
- **THEN** 系统 SHALL 按既有兼容规则保留管理员维护的展示和运行时策略字段
- **AND** 系统 SHALL NOT 因 Catalog 默认值覆盖现有管理员配置

#### Scenario: 同步软删除 API 元数据
- **WHEN** Route Catalog 的 Method 和 Path 只存在软删除 API 元数据
- **THEN** 系统 SHALL 按现有同步规则恢复该记录
- **AND** 系统 SHALL 保持 `method + path` 唯一

### Requirement: API 权限同步
系统 SHALL 能够将 API 元数据中的权限码同步到权限表。

#### Scenario: 同步 API 权限码
- **WHEN** 管理员调用 `POST /api/admin/apis/sync-permissions`
- **THEN** 系统读取需要认证的 API 记录
- **AND** 为不存在于权限表的 API 权限码创建权限记录
- **AND** 跳过不需要认证的公开 API
- **AND** 已存在权限码不会重复创建

### Requirement: API 动态权限校验
系统 SHALL 基于 API 元数据动态校验管理员接口访问权限。

#### Scenario: API 已启用且用户有权限
- **WHEN** 已认证用户请求 `/api/admin` 下的接口
- **AND** API 元数据中 `status = 1`
- **AND** 用户拥有该 API 绑定的 `permission_code`
- **THEN** 系统允许请求继续

#### Scenario: API 未配置
- **WHEN** 已认证用户请求 `/api/admin` 下的接口
- **AND** 系统找不到匹配 `method + c.FullPath()` 的 API 元数据
- **THEN** 系统拒绝请求

#### Scenario: API 已禁用
- **WHEN** 已认证用户请求的 API 元数据 `status != 1`
- **THEN** 系统拒绝请求

#### Scenario: 普通管理员角色缺少权限码
- **WHEN** 已认证用户不是超级管理员
- **AND** API 需要认证
- **AND** API 未绑定权限码或用户不拥有该权限码
- **THEN** 系统拒绝请求

#### Scenario: 超级管理员兜底
- **WHEN** 已认证用户拥有 `admin` 角色
- **AND** 请求不是被禁用的 API
- **THEN** 系统允许请求继续

#### Scenario: 首次同步例外
- **WHEN** `admin` 角色用户调用 `POST /api/admin/apis/sync` 或 `POST /api/admin/apis/sync-permissions`
- **THEN** 系统允许请求用于初始化 API 元数据

### Requirement: API 生成按钮菜单
系统 SHALL 支持从 API 元数据生成按钮菜单，并建立菜单与 API 的绑定关系。

#### Scenario: 从 API 生成按钮菜单
- **WHEN** 管理员调用 `POST /api/admin/apis/:id/menu-button`
- **AND** 提交父级菜单 ID、按钮名称和排序
- **THEN** 系统校验 API 存在、已启用且需要认证
- **AND** 系统校验父级菜单存在
- **AND** 系统在同一数据库事务中创建 `type = 3` 的按钮菜单
- **AND** 系统将按钮菜单权限码设置为 API 权限码
- **AND** 系统写入 `menu_apis` 关联
- **AND** 系统确保权限表存在对应权限码

#### Scenario: API 缺少权限码时生成按钮
- **WHEN** 管理员从没有权限码的 API 生成按钮菜单
- **THEN** 系统根据 `method + path` 自动生成 API 权限码
- **AND** 系统在同一数据库事务中同步写入 API、菜单、`menu_apis` 和权限表

#### Scenario: 生成按钮事务失败
- **WHEN** API 权限码、按钮菜单、`menu_apis` 或权限记录任一步写入失败
- **THEN** 系统 SHALL 回滚本次生成按钮操作的全部数据库写入
- **AND** 系统 SHALL NOT 留下孤立按钮、部分关联或未完成的权限码

#### Scenario: 公开 API 不能生成按钮权限
- **WHEN** 管理员尝试从 `need_auth = 0` 的 API 生成按钮菜单
- **THEN** 系统拒绝请求

### Requirement: 消息与邮箱接口元数据
系统 SHALL 将消息、WebSocket ticket/upgrade、死信管理和邮箱验证接口纳入 Route Catalog Snapshot、API Metadata 和统一错误契约。

#### Scenario: WebSocket 原生协议文档
- **WHEN** Route Catalog 声明已认证的 WebSocket upgrade 入口
- **THEN** OpenAPI operation SHALL 标记 `x-native-protocol: websocket`
- **AND** 请求 SHALL 声明无 body，101 Switching Protocols SHALL 声明无 JSON body，升级前错误 SHALL 使用统一四字段错误信封

#### Scenario: 受保护消息和邮箱入口同步
- **WHEN** App 同步 Route Catalog
- **THEN** 消息、死信和邮箱验证入口 SHALL 具有稳定默认 Permission Code、认证等级和审计分类
- **AND** API Metadata 或权限同步 SHALL NOT 从 Gin Engine 动态发现路由

#### Scenario: 死信 Outbox 重放权限
- **WHEN** 请求 `GET /api/admin/message-outboxes` 或 `POST /api/admin/message-outboxes/:id/replay`
- **THEN** API Metadata SHALL 将接口限制为超级管理员
- **AND** 重放接口 SHALL 声明默认 Permission Code `admin.messages.outbox.replay`
- **AND** 重放接口 SHALL 仅接受 `dead` 状态的 Outbox

#### Scenario: Consumer 死信管理权限
- **WHEN** 请求 Consumer DLQ 查询、重放或丢弃入口
- **THEN** API Metadata SHALL 将接口限制为超级管理员
- **AND** 接口 SHALL 声明默认 Permission Code `admin.messages.dead-letter.manage`

#### Scenario: 消息接口动态授权
- **WHEN** 非超级管理员请求消息、公告、消息分类或死信管理入口
- **AND** API Metadata 已启用且用户拥有对应消息 Permission Code
- **THEN** 系统 SHALL 允许请求进入 Messaging Handler
- **AND** 用户缺少权限或超出组织数据范围时 SHALL 拒绝请求

#### Scenario: WebSocket 成功帧不创建 API 权限
- **WHEN** 权限同步处理消息 WebSocket 路由
- **THEN** WebSocket 成功帧和 RabbitMQ Exchange/Queue SHALL NOT 被错误创建为 API Permission Code

#### Scenario: 邮箱验证接口契约
- **WHEN** Identity 提供邮箱验证注册、确认、重发接口
- **THEN** Route Catalog SHALL 将接口标记为 Authenticated（注册除外）并纳入 API Metadata
- **AND** 确认接口 SHALL 使用四字段 JSON 错误响应且不得通过 GET 改变状态
- **AND** 用户资料响应 SHALL 只包含 `email`、`pending_email` 和 `email_verified`

#### Scenario: 邮箱更新错误契约
- **WHEN** 用户资料更新提交非空 `email`
- **THEN** OpenAPI Request Schema SHALL 要求 `current_password`
- **AND** 缺失或不匹配时 SHALL 声明 HTTP 422 与 `IDENTITY_CURRENT_PASSWORD_INVALID`
- **WHEN** SMTP 投递不可用、候选邮箱冲突或验证邮件请求被节流
- **THEN** Route Descriptor SHALL 声明稳定 HTTP 503、409 或 429 Identity 错误及 429 的 `Retry-After`
- **AND** 错误 data SHALL NOT 暴露邮箱、token、SMTP 信息或已提交状态
```

## user-management

来源：`openspec/specs/user-management/spec.md`；SHA-256：`0284e88640d9ff6e6a6c10eb212b1d0cd10460a0c55498d52b40fc2bb1a66355`。

```markdown
### Requirement: 用户注册
系统 SHALL 允许用户在通过验证码和密码策略校验后公开注册账号。

#### Scenario: 注册成功
- **WHEN** 请求向 `POST /api/register` 提交唯一用户名、唯一邮箱、有效密码和有效验证码
- **THEN** 系统创建一个角色为 `user`、状态为 `1` 的用户
- **AND** 系统使用 bcrypt 存储密码
- **AND** 响应返回不包含密码的脱敏用户信息

#### Scenario: 用户名或邮箱已存在
- **WHEN** 请求提交的用户名或邮箱已被其他用户使用
- **THEN** 系统拒绝注册

### Requirement: 当前用户资料
系统 SHALL 允许已认证用户读取和修改自己的非敏感资料字段，但头像 SHALL 只能通过专用头像接口修改。

#### Scenario: 查询当前用户资料
- **WHEN** 已认证用户调用 `GET /api/user/info`
- **THEN** 系统 SHALL 返回脱敏后的用户信息
- **AND** 响应 SHALL 永远不包含密码哈希
- **AND** 只有头像对象标识、归属、规范 UUID 扩展名、MIME 和验证状态均可信时，响应头像 SHALL 为 `/api/avatars/<user-id>`
- **AND** 其他情况响应头像 SHALL 为 `/api/avatars/default`

#### Scenario: 修改当前用户资料
- **WHEN** 已认证用户调用 `PUT /api/user/info` 并提交昵称或邮箱
- **THEN** 系统 SHALL 只更新允许修改的字段
- **AND** 如果邮箱已被其他用户使用，系统 SHALL 拒绝请求
- **AND** 如果没有任何可更新字段，系统 SHALL 拒绝请求

#### Scenario: 资料接口提交头像字段
- **WHEN** `PUT /api/user/info` 请求包含 `avatar` 字段
- **THEN** 系统 SHALL 拒绝请求
- **AND** 系统 SHALL NOT 把外部 URL、MinIO 路径或其他客户端值写入头像字段

### Requirement: 头像上传
系统 SHALL 允许已认证用户通过 `POST /api/user/avatar` 上传一个受支持的静态图片，并只保存完整解码和标准化后的头像对象。

#### Scenario: 无透明头像上传成功
- **WHEN** 用户上传未超过头像配置上限、尺寸不超过 8,192×8,192、总像素不超过 40,000,000 且可完整解码的静态 JPEG、PNG 或 WebP
- **AND** 解码后像素不包含透明通道
- **THEN** 系统 SHALL 按比例缩放到最大 1,024×1,024
- **AND** 系统 SHALL 移除非像素元数据并编码为质量 85 的 JPEG
- **AND** 系统 SHALL 使用服务端生成的 `.jpg` object key 写入私有 `image` bucket
- **AND** 系统 SHALL 保存对象标识和 `image/jpeg`，而不是客户端 URL

#### Scenario: 透明头像上传成功
- **WHEN** 用户上传通过验证且解码后包含透明通道的静态 PNG 或 WebP
- **THEN** 系统 SHALL 按比例缩放到最大 1,024×1,024
- **AND** 系统 SHALL 移除非像素元数据并重新编码为 PNG
- **AND** 系统 SHALL 使用服务端生成的 `.png` object key 和 `image/png`

#### Scenario: 头像请求体或输出过大
- **WHEN** 头像请求体、原文件或重新编码后的输出超过 `file_upload.avatar_max_size_mb`
- **THEN** 系统 SHALL 返回 HTTP 413
- **AND** 系统 SHALL NOT 更新用户头像记录

#### Scenario: 头像尺寸或像素超限
- **WHEN** 图片任一边超过 8,192 像素或总像素超过 40,000,000
- **THEN** 系统 SHALL 在完整像素解码前拒绝图片
- **AND** 系统 SHALL 返回 `IMAGE_DIMENSION_LIMIT`

#### Scenario: 头像格式不受支持或损坏
- **WHEN** 头像不是 JPEG、PNG、WebP，声明类型与真实类型不一致，图片不能完整解码，或图片为动画 WebP/APNG
- **THEN** 系统 SHALL 在写入 MinIO 前拒绝上传
- **AND** 系统 SHALL 返回稳定类型或解码错误

#### Scenario: 新头像对象写入失败
- **WHEN** 标准化成功但新头像无法写入 MinIO
- **THEN** 系统 SHALL 保留当前头像记录
- **AND** 系统 SHALL 返回不包含底层存储信息的服务错误

#### Scenario: 新头像写入后数据库确认未提交
- **WHEN** 新头像对象写入成功且用户记录更新明确确认未提交
- **THEN** 系统 SHALL 删除刚写入的新对象
- **AND** 系统 SHALL 保留原头像记录

#### Scenario: 新头像数据库提交结果不确定
- **WHEN** 新头像对象写入成功但用户记录更新返回错误且无法确认是否已经提交
- **THEN** 系统 SHALL 返回不包含底层数据库信息的持久化错误
- **AND** 系统 SHALL NOT 删除可能已经被用户记录引用的新对象
- **AND** 系统 SHALL NOT 尝试删除旧头像对象

#### Scenario: 新头像替换成功
- **WHEN** 新头像对象和数据库记录均更新成功
- **THEN** 系统 SHALL 返回更新后的脱敏用户信息
- **AND** 系统 SHALL 在数据库成功后尝试删除旧的系统生成头像对象
- **AND** 旧对象删除失败 SHALL NOT 回滚已经成功的新头像

### Requirement: 头像内容摘要
系统 SHALL 对用户头像标准化后实际写入 MinIO 的内容计算服务端 SHA-256，并与可信头像元数据一起持久化。

#### Scenario: 上传头像建立摘要
- **WHEN** JPEG、PNG 或 WebP 头像通过验证并完成标准化重新编码
- **THEN** 系统 SHALL 对重新编码后的完整 JPEG 或 PNG 字节计算 SHA-256
- **AND** 系统 SHALL 将 64 个小写十六进制字符的 `avatar_content_sha256` 与头像 object name、规范 MIME、验证状态和验证时间在同一次数据库更新中持久化
- **AND** 系统 SHALL NOT 保存原始上传图片的摘要作为可信头像摘要

#### Scenario: 恢复默认头像清除摘要
- **WHEN** 用户成功通过 `DELETE /api/user/avatar` 恢复系统默认头像
- **THEN** 系统 SHALL 清空 `avatar_content_sha256` 及其他可信自定义头像元数据

#### Scenario: 既有可信头像没有摘要
- **WHEN** 用户具有本策略上线前创建的可信头像且 `avatar_content_sha256` 为空
- **THEN** 系统 SHALL 保留现有可信头像行为
- **AND** 系统 SHALL NOT 在服务启动或头像读取时自动读取 MinIO 补算摘要
- **AND** 用户下次成功上传头像时 SHALL 建立新的摘要基线

#### Scenario: 头像摘要不对外暴露
- **WHEN** 系统返回用户资料、登录信息、初始化上下文或头像内容
- **THEN** 响应 SHALL NOT 包含 `avatar_content_sha256`
- **AND** 系统 SHALL NOT 将头像摘要写入 object key、头像 URL、普通运行日志或审计元数据

#### Scenario: 头像摘要用途受限
- **WHEN** 系统生成或保存头像摘要
- **THEN** 系统 SHALL NOT 信任客户端提供的摘要
- **AND** 系统 SHALL NOT 将摘要用于自动去重、唯一约束或恶意图片判定

### Requirement: 恢复默认头像
系统 SHALL 允许已认证用户通过 `DELETE /api/user/avatar` 恢复系统默认头像。

#### Scenario: 恢复默认头像成功
- **WHEN** 用户调用恢复默认头像接口
- **THEN** 系统 SHALL 清除当前可信头像对象标识并返回默认头像地址
- **AND** 系统 SHALL 在数据库更新成功后尝试删除旧的系统生成头像对象

#### Scenario: 重复恢复默认头像
- **WHEN** 用户已经没有可信头像对象并再次调用恢复默认头像接口
- **THEN** 系统 SHALL 成功返回默认头像结果
- **AND** 系统 SHALL NOT 因数据库字段没有发生变化而返回持久化失败
- **AND** 系统 SHALL NOT 尝试删除不存在或不可信的头像对象

#### Scenario: 删除旧头像对象失败
- **WHEN** 数据库已经恢复默认头像但旧对象删除失败
- **THEN** 系统 SHALL 保持默认头像结果
- **AND** 系统 SHALL 记录受控清理错误但不得向客户端暴露 object key 或 MinIO 原始错误

### Requirement: 历史头像来源过渡
系统 SHALL 把策略上线前的自定义头像视为未验证来源，并 SHALL NOT 自动信任或删除。

#### Scenario: 用户存在历史自定义头像
- **WHEN** 用户头像只存在于旧 `avatar` URL 字段且没有 V1 验证通过的头像对象标识
- **THEN** 用户信息、登录响应和初始化上下文 SHALL 返回系统默认头像
- **AND** 系统 SHALL NOT 请求、代理或内联显示该历史外部 URL

#### Scenario: 历史头像迁移
- **WHEN** 新头像字段首次上线
- **THEN** 数据迁移 SHALL 只写入历史状态
- **AND** 数据迁移 SHALL NOT 读取、解码、重新编码或删除历史头像对象

#### Scenario: 用户重新上传头像
- **WHEN** 具有历史未验证头像的用户成功上传 V1 标准化头像
- **THEN** 系统 SHALL 使用新的系统对象标识
- **AND** 系统 SHALL NOT 自动删除无法确认归属的历史对象

### Requirement: 受控头像读取
系统 SHALL 通过公开只读的应用接口提供可信头像和系统默认头像，同时保持私有 `image` bucket、object key 和历史头像来源不可见。

#### Scenario: 读取可信用户头像
- **WHEN** 客户端调用 `GET /api/avatars/:user_id` 且对应用户具有归属、规范 UUID 扩展名、MIME 和验证状态均可信的头像对象
- **THEN** 系统 SHALL 从私有 `image` bucket 流式返回该标准化 JPEG 或 PNG
- **AND** 响应 SHALL 设置规范 `Content-Type`、`X-Content-Type-Options: nosniff` 和受控公开缓存头
- **AND** 响应 SHALL NOT 暴露 object key、MinIO URL 或存储凭据

#### Scenario: 读取不可信或不可用的用户头像
- **WHEN** 用户不存在、没有可信头像、只有历史头像 URL、可信对象缺失或对象暂时无法读取
- **THEN** `GET /api/avatars/:user_id` SHALL 返回应用内置的系统默认 PNG
- **AND** 系统 SHALL NOT 请求、代理、重定向或内联历史头像 URL
- **AND** 响应差异 SHALL NOT 暴露用户是否存在或对象存储状态

#### Scenario: 读取默认头像
- **WHEN** 客户端调用 `GET /api/avatars/default`
- **THEN** 系统 SHALL 返回应用内置的系统默认 PNG
- **AND** 响应 SHALL 设置 `image/png`、`X-Content-Type-Options: nosniff` 和受控公开缓存头

#### Scenario: 未携带 JWT 读取头像
- **WHEN** 浏览器通过普通 `<img src>` 请求受控头像地址
- **THEN** 系统 SHALL 允许匿名只读访问
- **AND** 接口 SHALL NOT 返回用户资料、验证元数据或任何非图片内容

### Requirement: 修改密码
系统 SHALL 允许已认证用户在验证旧密码后修改密码。

#### Scenario: 修改密码成功
- **WHEN** 旧密码正确
- **AND** 新密码和确认密码一致
- **AND** 新密码与旧密码不同
- **AND** 新密码满足密码复杂度规则
- **THEN** 系统存储新的 bcrypt 密码哈希
- **AND** 系统使该用户旧 token 失效

#### Scenario: 修改密码失败
- **WHEN** 任一密码条件不满足
- **THEN** 系统拒绝修改密码

### Requirement: 前端初始化上下文
系统 SHALL 提供一个已认证接口，一次性返回前端初始化所需数据。

#### Scenario: 客户端请求初始化上下文
- **WHEN** 已认证用户调用 `GET /api/user/context`
- **THEN** 系统返回脱敏用户信息
- **AND** 系统返回 JWT 上下文中的角色码和权限码
- **AND** 系统返回该用户可访问的启用菜单树

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

### Requirement: 管理员修改用户
系统 SHALL 允许管理员修改非保护用户，同时保护自己和管理员用户不被管理端修改。管理员用户更新 SHALL NOT 代改邮箱；邮箱所有权变更由用户本人经密码校验与邮箱验证流程完成。

#### Scenario: 管理员修改普通用户
- **WHEN** 管理员修改另一个非管理员用户的昵称、角色或状态
- **THEN** 系统应用提交的字段
- **AND** 响应返回更新后的脱敏用户信息
- **AND** 当角色或状态变化影响登录态时，系统 SHALL 创建或提升目标用户授权版本
- **AND** 系统使目标用户旧 Access Token 和 Refresh Token 失效

#### Scenario: 管理员尝试修改自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝修改

#### Scenario: 管理员提交邮箱更新
- **WHEN** 管理员更新另一个可管理且非保护的用户，并提交非空 `email`
- **THEN** 系统 SHALL 拒绝整次更新，并返回既有邮箱字段错误 `IDENTITY_ADMIN_EMAIL_NOT_WRITABLE`
- **AND** 系统 SHALL NOT 改变目标用户的邮箱、待验证邮箱或邮箱验证状态
- **AND** 其他同时提交的资料字段 SHALL NOT 部分生效，目标用户授权版本 SHALL NOT 因该被拒绝请求而改变

### Requirement: 管理员删除用户
系统 SHALL 允许管理员软删除非保护用户，并防止删除前 Token 在用户恢复后重新生效。

#### Scenario: 管理员删除普通用户
- **WHEN** 管理员删除另一个非管理员用户
- **THEN** 系统 SHALL 在同一数据库事务中软删除用户并执行 `EnsureAndIncrement`
- **AND** 系统 SHALL 保留该用户的授权版本记录
- **AND** 任一步失败时系统 SHALL 回滚用户删除和授权版本变化

#### Scenario: 软删除用户被恢复
- **WHEN** 系统恢复已软删除用户
- **THEN** 系统 SHALL 沿用删除时已经提升的授权版本
- **AND** 删除前签发的 Access Token 和 Refresh Token SHALL NOT 重新生效

#### Scenario: 管理员尝试删除自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝删除

### Requirement: 管理员切换用户状态
系统 SHALL 允许管理员切换非保护用户的启用状态。

#### Scenario: 管理员切换普通用户状态
- **WHEN** 管理员对非管理员用户调用 `PUT /api/admin/users/:id/status`
- **THEN** 系统将状态从 `1` 改为 `0`，或从 `0` 改为 `1`
- **AND** 系统 SHALL 通过 `EnsureAndIncrement` 创建或提升目标用户授权版本
- **AND** 系统使目标用户旧 Access Token 和 Refresh Token 失效

#### Scenario: 管理员尝试切换自己或其他管理员状态
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝操作

### Requirement: 管理员强制用户下线
系统 SHALL 允许管理员主动使非保护用户的旧 token 失效。

#### Scenario: 管理员强制普通用户下线
- **WHEN** 管理员对非管理员用户调用 `PUT /api/admin/users/:id/kick`
- **THEN** 系统 SHALL 通过 `EnsureAndIncrement` 创建或提升目标用户授权版本
- **AND** 系统不修改目标用户资料和用户状态
- **AND** 目标用户旧 Access Token 和 Refresh Token 在后续请求中失效

#### Scenario: 管理员尝试强制下线自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝操作
```

## auth

来源：`openspec/specs/auth/spec.md`；SHA-256：`4d1f59c84c5ac36eb432d66c44a3f1573b9d31bd704b8ee1498099aaeae48503`。

```markdown
### Requirement: 验证码生成
系统 SHALL 提供公开验证码接口，生成一次性数字图片验证码，并将验证码答案短期存入 Redis。

#### Scenario: 客户端请求验证码
- **WHEN** 客户端调用 `GET /api/captcha`
- **THEN** 系统返回验证码 ID 和 base64 图片内容
- **AND** 系统将验证码答案存入 Redis 的 `captcha:<id>`
- **AND** 验证码在 5 分钟后自动过期

### Requirement: 验证码校验
注册和登录 SHALL 提交有效验证码，并且验证码无论校验成功还是失败都必须被消费。

#### Scenario: 验证码正确
- **WHEN** 用户提交匹配的 `captcha_id` 和 `captcha_code`
- **THEN** 系统继续执行注册或登录流程
- **AND** 系统从 Redis 删除该验证码答案

#### Scenario: 验证码错误或过期
- **WHEN** 用户提交错误、已使用或已过期的验证码
- **THEN** 系统拒绝本次请求
- **AND** 系统返回 HTTP 422、`AUTHN_CAPTCHA_INVALID` 和安全英文消息
- **AND** `data` SHALL 为 `null`
- **AND** 用户必须重新获取验证码后才能重试

### Requirement: 登录锁定
系统 SHALL 基于用户名使用 Redis 记录登录凭据错误累计，并在达到五次阈值后进行渐进式锁定，同时不暴露用户名是否存在。有效验证码之后、未锁定且进入凭据校验时，用户名不存在或密码错误 SHALL 计入累计；锁定期间的重试 SHALL NOT 再次计入凭据错误。

本次失败计入后的累计次数少于 5 次 SHALL 不锁定，5～9 次 SHALL 锁定 1 分钟，10～14 次 SHALL 锁定 5 分钟，15～19 次 SHALL 锁定 15 分钟，20 次及以上 SHALL 锁定 1 小时。第 10 次失败和锁定到期 SHALL NOT 清零累计；每次记录凭据错误 SHALL 将累计记录的有效期续为 24 小时，累计记录到期自动清除，成功登录清除累计与锁定。

#### Scenario: 未达到锁定阈值时登录失败
- **WHEN** 用户提交有效验证码且未处于锁定状态，用户名不存在或启用用户密码错误，且本次错误计入后的累计失败次数少于 5 次
- **THEN** 系统递增 `fail:<username>`
- **AND** 系统返回 HTTP 401、`AUTHN_CREDENTIALS_INVALID` 和相同的模糊英文提示
- **AND** 系统 MAY 在 `data.remaining_attempts` 返回剩余尝试次数
- **AND** 响应 SHALL NOT 通过状态、错误码、消息或详情暴露用户名是否存在

#### Scenario: 登录失败达到锁定阈值
- **WHEN** 本次登录凭据错误使累计失败次数达到 5 次或以上
- **THEN** 系统在 Redis 中设置 `lock:<username>`，采用该累计次数对应的锁定档位
- **AND** 系统返回 HTTP 429 和 `AUTHN_LOGIN_LOCKED`
- **AND** 系统在 `data.retry_after_seconds` 和 `Retry-After` Header 返回锁定时间
- **AND** 锁定存在期间拒绝继续登录

#### Scenario: 账号处于锁定状态
- **WHEN** Redis 中存在 `lock:<username>`
- **THEN** 系统拒绝登录请求
- **AND** 系统返回 HTTP 429、`AUTHN_LOGIN_LOCKED`、`data.retry_after_seconds` 和 `Retry-After` Header

#### Scenario: 登录成功
- **WHEN** 验证码正确、账号启用、密码匹配
- **THEN** 系统清理 `fail:<username>` 和 `lock:<username>`
- **AND** 系统签发 access token 和 refresh token
- **AND** JSON 响应 SHALL 使用 `code=200`、空 `error_code`、`msg=success` 和登录结果 `data`

#### Scenario: 锁定到期保留凭据错误累计
- **WHEN** `lock:<username>` 到期，而 `fail:<username>` 尚未到期
- **THEN** 系统 SHALL 保留已有累计次数
- **AND** 下一次满足计入条件的凭据错误 SHALL 在已有次数上递增，而不是从第一次重新开始

#### Scenario: 第十次失败继续累计
- **WHEN** 已累计 9 次错误、对应锁定已到期，且累计记录仍有效，用户再次发生可计入的凭据错误
- **THEN** 系统 SHALL 将累计次数递增为 10 并锁定 5 分钟
- **AND** 系统 SHALL NOT 在该次锁定后重置累计

#### Scenario: 更高档位继续升级
- **WHEN** 用户未成功登录，累计记录仍有效，后续可计入的凭据错误达到 15 次或 20 次
- **THEN** 系统 SHALL 分别锁定 15 分钟或 1 小时，而不是因第 10 次重置而停留在较低档位

#### Scenario: 累计记录续期与到期
- **WHEN** 系统记录一次新的登录凭据错误
- **THEN** 系统 SHALL 将 `fail:<username>` 的有效期续为该次记录后的 24 小时
- **AND** 在无新计入错误且该记录到期后，系统 SHALL 不保留此前累计，后续凭据错误从新的累计开始

### Requirement: 认证入口使用稳定公开错误
公开登录入口和认证中间件 SHALL 使用稳定认证错误码，且不得把 Token 解析器、密码比较器、Redis 或数据库错误直接返回客户端。

#### Scenario: 缺少认证 Header
- **WHEN** Authenticated 或 PermissionControlled 路由未携带 Authorization Header
- **THEN** 系统 SHALL 返回 HTTP 401 和稳定 `AUTHN_*` 错误码
- **AND** 响应 SHALL 使用四字段错误信封

#### Scenario: Authorization Header 格式错误
- **WHEN** Authorization Header 不是合法 Bearer Token 格式
- **THEN** 系统 SHALL 返回 HTTP 401 和稳定 `AUTHN_*` 错误码
- **AND** 系统 SHALL NOT 回显 Header

#### Scenario: Token 解析失败
- **WHEN** Bearer Token 签名、有效期、用途、黑名单或授权版本校验失败
- **THEN** 系统 SHALL 返回 HTTP 401 和稳定认证错误码
- **AND** 安全英文消息 SHALL NOT 包含解析器或存储内部错误

### Requirement: 账号状态拒绝登录
已知账号未启用时，系统 SHALL 在不泄漏内部状态细节的前提下拒绝签发 Token。

#### Scenario: 禁用账号使用正确凭据登录
- **WHEN** 用户提交有效验证码和正确密码但账号状态不是启用
- **THEN** 系统 SHALL 返回 HTTP 403 和稳定 `IDENTITY_*` 错误码
- **AND** 系统 SHALL NOT 签发 Access Token 或 Refresh Token

### Requirement: JWT 会话
系统 SHALL 在登录成功后签发包含用户 ID、token 版本、令牌用途、角色码、权限码的 JWT。

#### Scenario: 登录成功后返回令牌
- **WHEN** 用户登录成功
- **THEN** 响应包含 access token、refresh token 和脱敏后的用户信息
- **AND** access token 用途为 `access`
- **AND** refresh token 用途为 `refresh`
- **AND** access token 可用于认证受保护的 `/api/user` 和 `/api/admin` 请求

#### Scenario: 兼容无用途标记的存量令牌
- **WHEN** 有效存量 JWT 未携带 `token_type`
- **AND** 其 `iat` 与 `exp` 间隔唯一匹配切换时固定的旧 Access Token 或 Refresh Token 有效期
- **THEN** 系统按匹配到的存量令牌用途继续校验
- **AND** 当前 Token 有效期配置变化不改变存量令牌用途
- **AND** 固定旧有效期缺少、非正数或无法区分时，系统拒绝该 Token

### Requirement: 密码复杂度
系统 SHALL 在注册和修改密码时校验密码复杂度。

#### Scenario: 密码满足规则
- **WHEN** 密码长度不少于 6 位
- **AND** 密码包含大写字母、小写字母、数字、特殊符号中的至少 3 类
- **THEN** 系统允许继续后续流程

#### Scenario: 密码不满足规则
- **WHEN** 密码长度少于 6 位，或字符类型少于 3 类
- **THEN** 系统拒绝请求并返回校验错误

### Requirement: Token 实时失效
系统 SHALL 使用 Authorization 持有的用户授权版本使权限和用户状态变更后的旧 Token 失效，并通过稳定认证错误契约拒绝无效会话。

#### Scenario: Token 版本一致
- **WHEN** 已认证请求携带用途为 Access Token 的有效 JWT
- **AND** JWT 中的 `token_version` 等于 `user_access_versions.version`
- **AND** 用户状态为启用
- **THEN** JWT 中间件允许请求继续

#### Scenario: Token 版本不一致
- **WHEN** Access Token 或 Refresh Token 中的 `token_version` 与 `user_access_versions.version` 不一致，或令牌用途不符合请求场景
- **THEN** 系统拒绝该 Token
- **AND** Access Token 请求 SHALL 返回 HTTP 401 和稳定 `AUTHN_*` 错误码
- **AND** 用户需要重新登录获取新 Token

#### Scenario: 用户被禁用或删除
- **WHEN** Access Token 对应用户不存在或状态不是启用
- **THEN** JWT 中间件拒绝请求
- **AND** 不存在或不可验证身份 SHALL 返回 HTTP 401 和稳定认证错误码
- **AND** 已建立身份但账号被禁用 SHALL 返回 HTTP 403 和稳定 `IDENTITY_*` 错误码

#### Scenario: 登录初始化新用户版本
- **WHEN** 用户通过身份和密码校验但 `user_access_versions` 不存在
- **THEN** 系统幂等创建版本 1
- **AND** 系统重新读取当前版本
- **AND** 系统签发包含该版本的 Access Token 和 Refresh Token

#### Scenario: 登录初始化失败
- **WHEN** 系统无法创建或读取用户授权版本
- **THEN** 系统拒绝登录
- **AND** 系统不签发 Token
- **AND** 响应 SHALL 使用安全的 5xx 错误定义而不包含底层数据库错误

#### Scenario: 使用 Refresh Token 刷新
- **WHEN** 客户端向 `POST /api/refresh` 提交用途、签名、有效期、用户状态和授权版本均有效的 `refresh_token`
- **THEN** 系统返回新的 Access Token 和 Refresh Token
- **AND** 两个新 Token SHALL 使用 `user_access_versions` 中的当前版本并携带对应用途
- **AND** 成功响应 SHALL 使用 `code=200`、空 `error_code`、`msg=success` 和 `data.access_token`、`data.refresh_token`

#### Scenario: Refresh Token 刷新失败
- **WHEN** Refresh Token 无效、过期、用途错误、用户不存在、用户被禁用、新表缺行或授权版本不一致
- **THEN** `POST /api/refresh` SHALL 返回 HTTP 401 和 `AUTHN_REFRESH_TOKEN_INVALID`
- **AND** 安全英文消息 SHALL NOT 区分具体拒绝原因
- **AND** `data` SHALL 为 `null`
- **AND** 系统 SHALL NOT 签发新 Token
- **AND** 系统 SHALL NOT 回退其他授权版本存储

#### Scenario: 认证时授权版本缺失
- **WHEN** 非登录初始化流程校验 Token 时找不到 `user_access_versions`
- **THEN** 系统拒绝该 Token
- **AND** 系统返回 HTTP 401 和稳定认证错误码
- **AND** 系统不跳过授权版本检查

#### Scenario: 权限相关变更提升版本
- **WHEN** 用户密码、用户状态、角色分配、角色权限、角色菜单、菜单记录或组织成员归属发生影响登录态的变更
- **THEN** 系统通过 `EnsureAndIncrement` 提升受影响用户的授权版本
- **AND** 这些用户已持有的旧 Access Token 和 Refresh Token 在下一次使用时失效

### Requirement: 登出黑名单
系统 SHALL 通过 Redis 黑名单使已登出的 access token 失效。

#### Scenario: 用户登出
- **WHEN** 已认证用户调用 `POST /api/user/logout`
- **THEN** 系统将当前 token 写入 Redis 的 `blacklist:<token>`
- **AND** 黑名单 TTL SHALL 取 `jwt.expire` 配置的固定时长，而非该 token 的剩余有效期

#### Scenario: 黑名单 TTL 长于剩余有效期的情形
- **WHEN** 登出的 token 剩余有效期短于 `jwt.expire`
- **THEN** 系统 SHALL 仍按 `jwt.expire` 设置黑名单 TTL
- **AND** 该 token 在自然过期后由签名与过期校验拒绝，黑名单条目 SHALL NOT 影响该结果
- **AND** 黑名单条目 SHALL 在 TTL 到期后由 Redis 自动清除

#### Scenario: 复用已登出的 token
- **WHEN** 请求携带已存在于黑名单中的 token
- **THEN** JWT 中间件拒绝该请求
```

