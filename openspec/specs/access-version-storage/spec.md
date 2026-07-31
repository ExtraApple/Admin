# access-version-storage Specification

## Purpose

授权版本存储由 Authorization 负责，使用独立持久化记录为 Access Token 和
Refresh Token 提供统一的实时失效版本，并与用户身份资料解耦。

## Requirements

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
