## MODIFIED Requirements

### Requirement: 用户角色分配
系统 SHALL 允许管理员替换某个角色下的用户列表。

管理端角色下用户写入 SHALL 完整替换该角色关系；系统 SHALL 在任何写入前验证原始目标及本次实际新增/移除的旧关系用户，任一本人、受保护身份、范围外、不存在或非法目标均整次拒绝，空目标集合不豁免实际移除对象校验。停用普通用户本身不是拒绝配置的理由。

#### Scenario: 分配用户到角色
- **WHEN** 管理员提交角色 ID 和用户 ID 列表
- **THEN** 系统清除该角色已有的用户角色记录
- **AND** 系统插入本次提交的关联记录
- **AND** 系统使新旧关联用户的旧 token 失效

#### Scenario: 查询角色用户
- **WHEN** 管理员查询某个角色下的用户
- **THEN** 系统返回分配到该角色的脱敏用户列表
- **AND** 如果没有用户，系统返回空列表

#### Scenario: 批量分配包含禁止目标
- **WHEN** 有写入权限的请求在目标或实际被移除集合中触及本人、受保护用户、范围外或非法用户
- **THEN** 系统 SHALL 拒绝整个请求，不部分替换角色关系或提升版本

#### Scenario: 直接编辑admin角色成员
- **WHEN** 管理员直接修改admin角色的用户集合
- **THEN** 系统 SHALL 拒绝，即使目标集合与当前相同或为空

#### Scenario: 停用普通用户可预配置
- **WHEN** 目标普通用户仍存在但已停用，且满足范围、保护和全部引用前提
- **THEN** 系统 SHALL 允许正常配置关系，不因此授予其当前登录权限

### Requirement: 角色权限分配
系统 SHALL 允许管理员替换某个角色拥有的权限列表。

管理端直接编辑admin角色权限 SHALL 拒绝。普通角色的原始权限ID SHALL 全部合法且存在，再去重；Permission没有启停字段，系统 SHALL NOT 为统一校验新增权限状态前提。

#### Scenario: 分配权限到角色
- **WHEN** 管理员提交角色 ID 和权限 ID 列表
- **THEN** 系统清除该角色已有的角色权限记录
- **AND** 系统插入本次提交的关联记录
- **AND** 系统使该角色下用户的旧 token 失效

#### Scenario: 查询角色权限
- **WHEN** 管理员查询某个角色拥有的权限
- **THEN** 系统返回该角色的权限列表，并按排序排列
- **AND** 如果没有权限，系统返回空列表

#### Scenario: 非法权限ID整次拒绝
- **WHEN** 普通角色权限替换包含0或不存在权限
- **THEN** 系统 SHALL 拒绝全部写入，旧权限关联与版本保持不变

#### Scenario: admin权限关联受保护
- **WHEN** 管理端尝试直接替换admin角色权限
- **THEN** 系统 SHALL 拒绝；合法初始化及普通对象生命周期的受控引用清理按其明确契约处理

### Requirement: 角色数据权限
系统 SHALL 支持按角色配置数据范围，用于限制管理员可见的业务数据。

custom SHALL 至少包含一个合法存在组织；原始非法ID不得被过滤。非custom模式的organization_ids SHALL 省略或为空，非空列表必须整次拒绝；全局配置管理权限不被臆造的另一套组织数据范围政策替代。

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

## ADDED Requirements

### Requirement: 管理端受保护身份与角色关联
管理端保护身份 SHALL 依据用户与现存roles.code=admin的实际关联，不受该角色启停状态影响，也不等同于当前有效管理员授权。直接修改受保护身份或admin关联 SHALL 拒绝；合法普通共享配置对操作者的间接影响不得被误判为直接本人写入。

#### Scenario: 停用admin角色不解除身份保护
- **WHEN** 目标用户仍关联现存admin角色但该角色已停用
- **THEN** Identity、Authorization与Organization管理写入 SHALL 继续应用受保护身份规则
- **AND** 运行时有效权限仍按启停规则判断，不因保护身份授予权限

#### Scenario: 允许非保护共享配置间接影响操作者
- **WHEN** 有权限者修改普通共享角色的权限或菜单，且本人属于该配置依赖用户
- **THEN** 系统 SHALL 允许满足其他前提的操作，操作者版本按正常依赖规则处理
- **AND** 不得豁免本人失效，也不得仅因间接影响本人拒绝整个配置写入
