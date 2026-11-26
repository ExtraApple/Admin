# access-version-storage Specification Delta

## MODIFIED Requirements

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

#### Scenario: 用户被受控匿名化
- **WHEN** 已软删除的用户记录执行受控匿名化转为墓碑
- **THEN** 系统 SHALL 在同一数据库事务中对该用户执行 `EnsureAndIncrement` 并永久保留其 `user_access_versions` 记录
- **AND** 系统 SHALL NOT 删除该用户的授权版本记录
