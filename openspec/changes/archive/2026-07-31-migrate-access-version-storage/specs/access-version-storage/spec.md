## ADDED Requirements

### Requirement: 授权版本独立存储
系统 SHALL 使用 Authorization 拥有的 `user_access_versions` 表持久化用户授权版本。

#### Scenario: 创建授权版本表
- **WHEN** 系统执行数据库 AutoMigrate
- **THEN** 系统 SHALL 创建以 `user_id` 为主键的 `user_access_versions`
- **AND** 每条记录 SHALL 包含默认值为 1 的非空正整数 `version`、`created_at` 和 `updated_at`
- **AND** 版本记录 SHALL NOT 使用软删除字段

#### Scenario: 用户被软删除
- **WHEN** 用户记录被软删除
- **THEN** 系统 SHALL 保留该用户的授权版本记录
- **AND** 数据库外键 SHALL NOT 级联删除授权版本

### Requirement: 存量授权版本幂等迁移
系统 SHALL 在恢复写流量前，将固定存量用户集合的 `users.token_version` 幂等迁移到新表。

#### Scenario: 回填正版本
- **WHEN** 存量用户的 `users.token_version` 大于 0
- **THEN** 系统 SHALL 将原版本值写入 `user_access_versions`
- **AND** 系统 SHALL NOT 将该用户版本重置为 1

#### Scenario: 回填未初始化版本
- **WHEN** 存量用户的 `users.token_version` 为 NULL 或小于等于 0
- **THEN** 系统 SHALL 将新表版本写为 1
- **AND** 系统 SHALL 将 cutoff 范围内旧字段镜像为新表的最终版本 1

#### Scenario: 重试回填
- **WHEN** 迁移在部分记录写入后失败并重新运行
- **THEN** 系统 SHALL 沿用首次记录的 `cutoff_user_id`
- **AND** cutoff 范围 SHALL 包含迁移开始前的软删除用户
- **AND** 系统 SHALL 补齐缺失记录
- **AND** 系统 SHALL NOT 覆盖新表中更高的版本
- **AND** 当新表最终版本不低于归一化旧版本时，系统 SHALL 将该最终版本镜像回 cutoff 范围内的旧字段
- **AND** 当新表版本更低时，系统 SHALL NOT 降低旧字段且 SHALL 使迁移校验失败

#### Scenario: 回填校验失败
- **WHEN** cutoff 范围内存在缺失记录、数量不一致或版本倒退
- **THEN** 系统 SHALL 将迁移视为失败
- **AND** 系统 SHALL NOT 对外启动服务

### Requirement: 迁移锁和状态
系统 SHALL 使用数据库级迁移锁和持久化迁移状态保证只有一个迁移执行者推进授权版本迁移。

#### Scenario: 首次启动迁移
- **WHEN** 一个实例取得迁移锁且迁移状态不存在
- **THEN** 系统 SHALL 记录迁移名称、running 状态、固定 `cutoff_user_id` 和开始时间
- **AND** 系统 SHALL 在回填及校验完成后记录 completed 状态和完成时间

#### Scenario: 其他实例同时启动
- **WHEN** 一个实例正在持有授权版本迁移锁
- **THEN** 其他实例 SHALL NOT 同时修改迁移 cutoff 或完成状态

#### Scenario: 已完成迁移再次启动
- **WHEN** 迁移状态已经为 completed
- **THEN** 系统 SHALL 验证已完成状态
- **AND** 系统 SHALL NOT 将迁移后的新用户扩大到原存量集合

### Requirement: 切换部署禁止混合版本
授权版本读取切换 SHALL 在停止旧实例和写流量后进行，新旧应用版本 SHALL NOT 混合处理请求。

#### Scenario: 执行切换部署
- **WHEN** 运维人员部署读取新表的版本
- **THEN** 运维人员 SHALL 先停止全部旧实例和写流量
- **AND** 系统 SHALL 完成建表、回填和一致性校验
- **AND** 仅在新版本全部具备懒初始化、缺行提升、只读新表和镜像写能力后恢复流量

#### Scenario: 尝试混合运行
- **WHEN** 旧版本仍可能写入 `users.token_version`
- **THEN** 读取 `user_access_versions` 的新版本 SHALL NOT 与旧版本同时提供请求服务

### Requirement: 新表是唯一读取事实来源
切换后 Access Token 和 Refresh Token 的授权版本校验 SHALL 只读取 `user_access_versions`。

#### Scenario: 新表版本存在
- **WHEN** 系统校验 Token 且新表存在用户版本
- **THEN** 系统 SHALL 使用该版本与 Token Claim 比较
- **AND** 系统 SHALL NOT 同时读取旧字段并取最大值

#### Scenario: 认证时新表版本缺失
- **WHEN** 非登录初始化流程校验 Token 时找不到用户版本
- **THEN** 系统 SHALL 拒绝该 Token
- **AND** 系统 SHALL NOT 回退读取 `users.token_version`

### Requirement: 新用户版本懒初始化
迁移完成后的新用户 SHALL 在首次成功登录时幂等初始化授权版本，而不是由用户创建或 Seed 流程预先写入。

#### Scenario: 新用户首次登录
- **WHEN** Identity 已确认用户存在、启用且密码验证成功
- **AND** `user_access_versions` 不存在该用户记录
- **THEN** Authorization SHALL 幂等创建版本 1
- **AND** 观察期内系统 SHALL 在同一数据库事务中把版本 1 镜像写入 `users.token_version`
- **AND** Identity SHALL 使用重新读取的当前版本签发 Access Token 和 Refresh Token

#### Scenario: 并发首次登录
- **WHEN** 同一新用户并发发起多个首次登录请求
- **THEN** 系统 SHALL 最多创建一条授权版本记录
- **AND** 所有成功签发的 Token SHALL 使用已提交的当前版本

#### Scenario: 初始化失败
- **WHEN** 授权版本初始化或重新读取失败
- **THEN** 系统 SHALL 拒绝登录
- **AND** 系统 SHALL NOT 签发缺少可验证版本的 Token

### Requirement: 缺行失效操作原子创建并提升
所有需要使现有会话失效的操作 SHALL 使用统一的 `EnsureAndIncrement` 语义。

#### Scenario: 首次登录前发生失效操作
- **WHEN** 新用户尚无授权版本记录
- **AND** 系统执行禁用、软删除、Kick、密码变化或授权关系变化
- **THEN** 系统 SHALL 在调用方事务中幂等创建版本 1 并提升为更高版本
- **AND** 系统 SHALL NOT 将零影响行的普通 UPDATE 视为成功

#### Scenario: 并发初始化和失效
- **WHEN** `EnsureVersion` 与 `EnsureAndIncrement` 并发执行
- **THEN** 系统 SHALL 只保留一条版本记录
- **AND** 最终版本 SHALL NOT 倒退

### Requirement: 观察期原子镜像旧字段
观察期内系统 SHALL 主写新表，并在同一数据库事务中把初始化或提升后的最终版本镜像到 `users.token_version`。

#### Scenario: 版本初始化或提升成功
- **WHEN** 系统成功初始化或提升 `user_access_versions.version`
- **THEN** 系统 SHALL 在同一事务中把最终版本值写入 `users.token_version`
- **AND** 两个存储在提交后 SHALL 相等

#### Scenario: 任一镜像写失败
- **WHEN** 新表初始化、提升或旧字段镜像任一步失败
- **THEN** 系统 SHALL 回滚两个存储的本次变化
- **AND** 系统 SHALL 记录镜像写失败指标

### Requirement: 一致性观察区分合法缺行
系统 SHALL 按迁移 cutoff 和用户初始化状态扫描新旧版本差异。

#### Scenario: 扫描存量用户
- **WHEN** 用户 ID 小于等于 `cutoff_user_id`
- **THEN** 新表记录 SHALL 存在
- **AND** 新旧版本 SHALL 相等

#### Scenario: 扫描 cutoff 后已有新表记录
- **WHEN** cutoff 后用户已经初始化或发生过失效操作
- **THEN** 新表记录 SHALL 存在
- **AND** 新旧版本 SHALL 相等

#### Scenario: 扫描 cutoff 后合法缺行
- **WHEN** cutoff 后用户尚未登录且从未发生失效操作
- **AND** 旧字段为 NULL、0 或 1
- **THEN** 新表缺行 SHALL 被视为合法懒初始化状态
- **AND** 该用户 SHALL NOT 计入一致性差异

#### Scenario: 扫描异常缺行
- **WHEN** cutoff 后用户旧字段大于 1 但新表缺行
- **THEN** 系统 SHALL 将该用户计入一致性差异

### Requirement: 指标达标后退出旧字段
系统 SHALL 仅在观察指标和认证授权回归全部达标后停止镜像写并删除旧字段。

#### Scenario: 执行加速耐久性资格验收
- **WHEN** 系统准备退出旧字段
- **THEN** 外部运行器 SHALL 在真实 MySQL 和 Docker Redis 上连续至少 3 小时运行
- **AND** 系统 SHALL 完成至少 100,000 次混合认证授权操作
- **AND** 系统 SHALL 维持至少 8 个并发工作单元
- **AND** 系统 SHALL 完成至少 3 次受控应用进程重启
- **AND** 系统 SHALL 至少每 15 分钟、每次重启后和结束时执行一致性扫描
- **AND** 一致性扫描差异数始终为 0
- **AND** 非预期镜像写失败数为 0
- **AND** 成功签发 Token 后缺行数和 `EnsureAndIncrement` 后缺行数均为 0
- **AND** 系统 SHALL 通过原子回滚故障注入证明任一步失败时不会部分提交

#### Scenario: 退出门槛未满足
- **WHEN** 三小时资格窗口、工作量、并发、重启、零失败、零缺行、零一致性差异、
  原子回滚故障注入或关键回归测试任一门槛未满足
- **THEN** 系统 SHALL 保持旧字段和镜像写
- **AND** 系统 SHALL NOT 执行删除列 DDL

#### Scenario: 发布退出版本
- **WHEN** 所有退出门槛满足
- **THEN** 系统 SHALL 先发布停止镜像写且移除全部运行时读写、User GORM Model 字段和 AutoMigrate 旧列声明的版本
- **AND** 系统 SHALL 验证启动、登录、刷新和授权失效链路不再访问旧字段

#### Scenario: 物理删除旧字段
- **WHEN** 退出版本验证完成
- **THEN** 运维人员 SHALL 通过独立数据库变更删除 `users.token_version`
- **AND** 系统 SHALL 在删除列后清理仅用于迁移观察的代码、状态和过渡测试

### Requirement: Seed 不维护用户授权版本
Seed SHALL 只维护基础数据，不得创建或修复用户授权版本。

#### Scenario: Seed 创建超级管理员
- **WHEN** Seed 创建或确认超级管理员用户
- **THEN** Seed SHALL NOT 创建 `user_access_versions`
- **AND** Seed SHALL NOT 扫描其他用户或修复历史授权版本
- **AND** 超级管理员 SHALL 在首次登录或首次失效操作时初始化授权版本
