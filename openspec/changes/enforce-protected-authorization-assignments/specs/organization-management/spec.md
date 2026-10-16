## MODIFIED Requirements

### Requirement: 组织成员绑定
系统 SHALL 支持管理员将用户绑定到组织。

组织整组覆盖 SHALL 对原始目标及实际移除用户完整校验存在和适用范围；本人或受保护关系的真实新增/移除 SHALL 整次拒绝。满足其他前提的原有本人/受保护关系可以原样保留，不能仅因出现在目标集合而拒绝维护其他合法成员。

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

#### Scenario: 保留受保护关系维护普通成员
- **WHEN** 目标集合原样保留已有本人或受保护成员，只有其他合法普通成员关系变化
- **THEN** 系统 SHALL 允许满足全部存在与范围前提的请求
- **AND** 不得改写保留关系的joined_at

#### Scenario: 拒绝实际改变受保护成员
- **WHEN** 整组覆盖或清空会实际新增/移除本人或受保护身份归属
- **THEN** 系统 SHALL 拒绝整次请求，关系与版本无本请求变化

#### Scenario: 整组覆盖校验实际删除对象
- **WHEN** 完整目标省略一个范围外或其他不允许移除的旧成员
- **THEN** 系统 SHALL 整次拒绝，不静默改成仅可管理部分替换

### Requirement: 组织单位 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除组织单位。

组织删除若将连带移除操作者本人或受保护身份的组织归属 SHALL 整次拒绝；普通权限/菜单生命周期的受控引用清理例外不得扩大为组织成员删除豁免。

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

#### Scenario: 删除组织保护其成员归属
- **WHEN** 合法删除请求的目标组织含本人或受保护身份成员
- **THEN** 系统 SHALL 拒绝删除，组织、成员与授权版本保持不变
