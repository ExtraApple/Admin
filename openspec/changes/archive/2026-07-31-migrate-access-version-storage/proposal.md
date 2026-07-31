## Why

授权版本当前存储在 Identity 所有的 `users.token_version` 字段中，使身份数据与授权会话失效规则耦合，也阻碍后续模块化分层重构。需要先把授权版本迁移到 Authorization 拥有的独立表，同时保持迁移前有效 Token 可用，并为安全回滚和旧字段退出建立可验证流程。

## What Changes

- **BREAKING（内部数据模型）**：新增 Authorization 拥有的 `user_access_versions` 表，以 `user_id` 为主键保存授权版本。
- 使用 GORM AutoMigrate 创建新表和迁移状态表，使用幂等 Go 迁移复制存量 `users.token_version`。
- 存量正版本保持原值；NULL 或小于等于 0 的版本归一化为 1，避免迁移导致全员下线。
- 迁移使用数据库级迁移锁、固定 `cutoff_user_id`、持久化状态和数量/版本一致性校验；失败时阻止服务启动。
- 切换期间停止全部旧实例和写流量，不允许新旧版本服务实例混合处理请求。
- 切换后所有 Access Token 和 Refresh Token 版本读取只使用 `user_access_versions`，不进行长期双读或旧字段回退。
- 补齐当前缺失的 `POST /api/refresh`，使用 Refresh Token 换取包含当前授权版本的新 Access Token 和 Refresh Token。
- 观察期内主写新表，并在同一数据库事务中将最终版本值镜像写入 `users.token_version`，支持受控回滚。
- 新用户不由 Seed 创建版本记录；首次登录通过幂等 `EnsureVersion` 初始化为 1，并在观察期内原子镜像该初始版本。
- 用户在首次登录前发生禁用、软删除、Kick 或授权变化时，通过原子 `EnsureAndIncrement` 创建并提升版本。
- 用户软删除与授权版本提升属于同一数据库事务；版本记录保留，恢复用户时沿用提升后的版本。
- 通过连续至少 3 小时的加速耐久性资格验收决定退出旧字段；该资格验收以真实
  MySQL、Docker Redis、固定工作量与并发、受控应用进程重启、周期性一致性扫描
  和原子回滚故障注入，替代原完整发布周期和连续 7 个自然日观察门槛。
- 退出时先发布完全移除运行时读写、GORM Model 字段和 AutoMigrate 列声明的版本，验证后再通过独立 DDL 删除 `users.token_version`。
- `restructure-layered-monolith` 可在本 Change 达到切换里程碑后开始，不需要等待旧字段观察期结束。
- 除新增 `POST /api/refresh` 外，保持现有 JWT Claim 和 `token_version` 语义、既有业务 HTTP 路由与请求/响应、Redis 黑名单 key、Permission Code、MinIO bucket 和现有认证授权语义不变；新增 `token_type` 用于区分 Access Token 与 Refresh Token，并兼容无该标记的存量 Token；路由同步会纠正该公开接口可能遗留的错误 `need_auth` 和 `permission_code` 元数据。
- 非目标：零停机滚动切换、长期双读/双写、引入正式 SQL migration 工具、让 Seed 承担历史回填或修改 `token_version` 语义，或重构全项目目录。

## Capabilities

### New Capabilities

- `access-version-storage`: 定义授权版本独立表、迁移状态、停机切换、懒初始化、原子版本提升、镜像观察和旧字段退出要求。

### Modified Capabilities

- `auth`: Token 实时失效改为读取 Authorization 拥有的授权版本；登录初始化、版本比较和权限变更失效不再以 `users.token_version` 为读取事实来源。
- `user-management`: 用户禁用、软删除、恢复和强制下线必须正确创建或提升授权版本，软删除与版本提升必须原子完成。

## Impact

- 数据库新增 `user_access_versions` 和迁移状态记录；观察期结束后独立删除 `users.token_version`。
- 影响登录、JWT 中间件、Refresh Token、管理员用户状态、软删除/恢复、Kick 和所有授权关系变化后的会话失效。
- 影响 AutoMigrate 启动顺序、迁移失败策略、部署/回滚流程、指标和一致性扫描。
- 迁移期间需要短暂停止旧实例和写流量；不支持新旧服务实例混合运行。
- 新增公开的 `POST /api/refresh`；除此之外不新增或修改 HTTP 路由，不修改 Redis key、MinIO 或其他现有 API 元数据和权限码；路由同步仅纠正该公开接口可能遗留的错误认证元数据。
- 本 Change 的切换里程碑是 `restructure-layered-monolith` 的实施前置条件；旧字段退出里程碑可以与目录重构并行推进。
