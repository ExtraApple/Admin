## MODIFIED Requirements

### Requirement: 管理员群发与公告
具备对应权限和组织数据范围的管理员 SHALL 能按组织、角色或全体用户创建群发，并能管理公告的 draft、scheduled、published、expired 和 revoked 生命周期。超级管理员的系统范围 SHALL 解析为现有 Organization Scope Reader 返回的全部组织集合；普通管理员 SHALL 继续使用自身组织及子组织范围。每个目标组织 SHALL 创建独立消息副本、分类和动态受众规则；跨组织重叠用户的副本、已读状态和 Outbox 事件不得去重。任一分类缺失或副本写入失败 SHALL 回滚整批事务。

#### Scenario: 群发副本原子创建
- **WHEN** 管理员向多个组织群发消息
- **THEN** 系统 SHALL 为每个目标组织创建独立副本，并在任一副本失败时回滚全部事实

#### Scenario: 超级管理员系统范围群发
- **WHEN** 具备群发权限的超级管理员按系统范围读取分类或创建群发/公告
- **THEN** Messaging SHALL 使用既有组织 Scope Reader 的全部组织集合
- **AND** 系统 SHALL 不因组合根遗漏范围能力返回内部错误
- **AND** 受限管理员的组织范围规则 SHALL 保持不变

### Requirement: 组织分类
系统 SHALL 提供组织级消息分类的创建、修改、停用和删除。编码只在同一组织内唯一；已引用分类可停用但不可删除；普通管理员受组织及子组织范围限制，超级管理员可跨组织管理；系统不得预置默认分类。超级管理员的跨组织范围 SHALL 使用既有 Organization Scope Reader 的全部组织集合，不新增组织过滤语义。

#### Scenario: 消息分类约束
- **WHEN** 管理员创建、修改或删除消息分类
- **THEN** 系统 SHALL 执行组织范围、编码唯一和已引用分类保护规则

#### Scenario: 超级管理员读取分类
- **WHEN** 超级管理员查询消息分类
- **THEN** 系统 SHALL 返回当前 Scope Reader 可见组织内的分类
- **AND** SHALL NOT 因缺少 All-scope 适配器返回内部错误
