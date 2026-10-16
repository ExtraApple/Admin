## MODIFIED Requirements

### Requirement: 超级管理员兜底初始化
系统 SHALL 在启动时保证新 RBAC 体系中存在启用的超级管理员。

初始化中的管理员存在性和角色绑定读取 SHALL 区分业务不存在与查询失败；读取失败 SHALL 返回错误，不得当成无管理员或无绑定继续恢复、创建或误提升用户。失败的初始化阶段 SHALL 回滚并阻止本次应用提供HTTP服务；此前成功阶段不因此撤销，Seed授权版本规则保持不变。

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

#### Scenario: 启用管理员存在性读取失败
- **WHEN** 已有启用用户绑定启用admin角色，仅用于判断该事实的查询报错，后续存储仍可用
- **THEN** 系统 SHALL 返回启动初始化错误，不重写既有密码、不新增管理员或改变本阶段用户身份/绑定
- **AND** 本次应用不进入HTTP服务，Seed不创建或修复user_access_versions

#### Scenario: 配置用户名角色绑定读取失败
- **WHEN** 不存在已识别启用管理员，配置用户名存在，但其绑定读取报错
- **THEN** 系统 SHALL 传播读取错误，不将其伪装成普通用户名冲突或已确认无绑定
- **AND** 不得恢复密码、提升身份或提交本阶段部分写入

#### Scenario: 错误消除后重新启动
- **WHEN** 相同配置在存储恢复后重跑初始化
- **THEN** 系统 SHALL 按真实管理员事实执行既有幂等兜底规则
- **AND** 保留已成功阶段，不重复创建关系或重置已启用管理员密码
