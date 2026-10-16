## MODIFIED Requirements

### Requirement: 角色菜单分配
系统 SHALL 允许管理员替换某个角色拥有的菜单列表，并 SHALL 在管理员查询分配结果时返回按 `sort asc, id asc` 排序的平铺已分配菜单项（含 `parent_id` 和 `status`），不以运行时用户菜单可见性或未分配父级的存在与否裁剪配置。

管理端直接替换admin角色菜单 SHALL 拒绝。普通角色菜单替换 SHALL 完整验证全部原始ID、存在性及适用管理前提，再去重；存在的普通停用角色与菜单允许预配置，不因此授予用户当前可见性或访问权限。

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

#### Scenario: admin角色菜单直接编辑拒绝
- **WHEN** 管理员直接修改admin角色菜单集合
- **THEN** 系统 SHALL 拒绝，不改变关联或版本

#### Scenario: 菜单非法目标与停用目标分开
- **WHEN** 普通角色请求包含0/不存在菜单，或仅包含合法存在但停用菜单
- **THEN** 系统 SHALL 对非法目标整次拒绝，对满足其他前提的停用普通菜单允许配置
- **AND** 管理员配置读取与用户运行时可见性仍分离

### Requirement: 菜单 API 联动
系统 SHALL 允许管理员把菜单按钮和 API 元数据绑定，并保证菜单权限码、API 权限码、角色授权和相关用户会话状态一致。

菜单绑定API SHALL 先校验原始api_ids中每个ID，再仅对合法ID去重；零ID SHALL 整次拒绝，不得在归一化时静默丢弃。此要求同样适用于直接调用Application的现有生产路径。

普通菜单修改中的权限码变更 SHALL 消费与菜单绑定相同的既有直接联动规则，而不得只更新menus。菜单字段、直接绑定API、对应权限和授权关联及本次会话失效 SHALL 在同一最外层事务完成；响应 SHALL 基于实际提交结果。

菜单API绑定列表 SHALL 保持非空；不能借完整替换语义新增空数组解绑。API启用且需要认证的明确前提不因其他停用普通配置目标可预配置而放宽。

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

#### Scenario: 混合有效API与零ID
- **WHEN** 合法菜单绑定请求同时包含存在且可绑定的API ID与0
- **THEN** 系统 SHALL 以既有Navigation校验错误拒绝整次请求
- **AND** 菜单、API、menu_apis、权限、角色授权和用户版本均不留下该请求写入

#### Scenario: 合法重复API ID
- **WHEN** 其余前提合法的请求包含重复有效API ID
- **THEN** 系统 SHALL 去重后按既有绑定规则处理
- **AND** 不得创建重复关联，也不得将合法去重与丢弃非法ID混同

#### Scenario: 内部调用不能绕过原始校验
- **WHEN** 内部生产调用向同一菜单绑定用例传入混合有效与零ID
- **THEN** 系统 SHALL 同样拒绝且不写入关系或版本

#### Scenario: 菜单侧修改已绑定权限码
- **WHEN** 菜单与直接绑定API原本一致，管理员通过PUT /api/admin/menus/:id提交合法的新权限码
- **THEN** 系统 SHALL 同步菜单与这些直接绑定API的权限码，并按既有绑定码规则确保权限、合并去重角色授权
- **AND** menu_apis目标集合不因仅改码被改变，相关版本与事实共同提交

#### Scenario: 菜单侧联动中途失败
- **WHEN** 菜单改码过程中API、权限、角色授权或任一用户版本写入失败
- **THEN** 系统 SHALL 回滚该请求的全部菜单/API/权限/关系/版本写入
- **AND** 不得已提交菜单新码却保留直接绑定API旧码

#### Scenario: 未绑定菜单改码
- **WHEN** 合法菜单没有绑定API且提交权限码变更
- **THEN** 系统 SHALL 保留现有未绑定菜单的合法更新语义，不创建虚假API绑定

#### Scenario: 绑定菜单显式空码沿用既有解析
- **WHEN** 菜单侧显式空码进入已有绑定码解析，直接绑定API仍满足现有绑定前提
- **THEN** 系统 SHALL 使用与既有菜单绑定相同的权限码解析与失败语义
- **AND** 不新增解绑、授权撤销或多菜单传播优先级规则

#### Scenario: 空API集合不是解绑
- **WHEN** 管理员提交空api_ids请求
- **THEN** 系统 SHALL 拒绝，不清空既有绑定或改变任何版本
