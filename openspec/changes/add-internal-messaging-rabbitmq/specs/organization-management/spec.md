# organization-management Delta Specification

## ADDED Requirements

### Requirement: 组织成员加入时间

系统 SHALL 持久化每个当前组织成员关系的加入时间，供内部消息首次可见公告时判定历史已读状态。

#### Scenario: 覆盖成员时保留既有加入时间
- **WHEN** 管理员用 `POST /api/admin/organizations/:id/users` 覆盖组织成员
- **THEN** 系统 SHALL 保留仍属于该组织用户的原始成员加入时间
- **AND** 系统 SHALL 仅为新增成员写入新的加入时间

#### Scenario: 迁移既有成员关系
- **WHEN** 系统升级包含既有 `user_organizations` 记录的数据库
- **THEN** 系统 SHALL 为缺失加入时间的当前成员补写迁移执行时间
- **AND** 后续成员覆盖 SHALL 保留该补写时间
