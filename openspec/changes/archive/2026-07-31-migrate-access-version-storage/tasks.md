## 1. 迁移基线与安全护栏

- [x] 1.1 固化当前 Access Token、Refresh Token、登录、禁用、Kick 和权限变更后的 Token 失效回归测试
- [x] 1.2 创建包含 `users.token_version` 为 NULL、0、1 和大于 1 的 MySQL 迁移测试数据
- [x] 1.3 固化 `/api/login` 返回 `refresh_token` 和管理员用户操作路由的现有 HTTP 请求/响应兼容测试，并记录当前尚无 Refresh Token HTTP 入口
- [x] 1.4 增加部署检查，禁止读取新表的版本与只写旧字段的服务版本混合提供流量
- [x] 1.5 定义切换里程碑和退出里程碑的可验证验收清单
- [x] 1.6 创建隔离的 SQLite 测试 Helper，使用独立临时库或唯一 shared-memory DSN，并建立 SQLite 快速门禁与 MySQL 强制门禁

## 2. Model 与 AutoMigrate

- [x] 2.1 新增 `UserAccessVersion` GORM Model，使用 `user_id` 主键、正整数 `version` 和时间戳
- [x] 2.2 确认授权版本 Model 不包含 `deleted_at`，且外键不使用 `ON DELETE CASCADE`
- [x] 2.3 新增持久化迁移状态 Model，包含名称、状态、`cutoff_user_id`、开始时间和完成时间
- [x] 2.4 将授权版本表和迁移状态表加入 AutoMigrate 收集顺序
- [x] 2.5 使用 SQLite 快速覆盖 Model、默认版本、基础唯一约束和普通事务行为，并在 MySQL 覆盖真实 AutoMigrate、索引、外键和软删除交互

## 3. 幂等迁移与启动阻断

- [x] 3.1 实现授权版本迁移的 MySQL 数据库级锁，并保证锁释放覆盖成功和失败路径
- [x] 3.2 实现首次迁移时记录固定 `cutoff_user_id` 和 running 状态
- [x] 3.3 实现正版本原值复制和 NULL/非正版本归一化为 1 的幂等回填
- [x] 3.4 保证重试沿用同一 cutoff，补齐缺行且不覆盖新表更高版本
- [x] 3.5 实现 cutoff 范围的用户数量、缺行、新旧版本和版本倒退校验
- [x] 3.6 在迁移成功后持久化 completed 状态和完成时间
- [x] 3.7 将迁移接入启动流程，并在锁、回填、校验或状态持久化失败时阻止服务启动
- [x] 3.8 增加多实例争抢迁移锁、失败重启、固定 cutoff 和幂等回填的真实 MySQL 集成测试

## 4. 授权版本存储 Service

- [x] 4.1 实现只从 `user_access_versions` 读取当前授权版本的 Repository/Service
- [x] 4.2 实现并发安全的 `EnsureVersion`，在事务中缺行时幂等创建版本 1、锁定并重新读取
- [x] 4.3 实现加入调用方事务的 `EnsureAndIncrement`，缺行时创建后再锁定提升
- [x] 4.4 使 `EnsureAndIncrement` 返回新表计算出的最终版本值
- [x] 4.5 在观察期内将 `EnsureVersion` 初始化或 `EnsureAndIncrement` 提升得到的最终版本值镜像写入 `users.token_version`
- [x] 4.6 保证新表初始化/提升和旧字段镜像属于同一事务，任一步失败时全部回滚
- [x] 4.7 禁止双读取最大值、认证缺行回退旧字段和两个存储分别提交
- [x] 4.8 增加初始化数、异常缺行数、提升数和镜像写失败数指标
- [x] 4.9 增加 `EnsureVersion` 与 `EnsureAndIncrement` 并发、唯一键冲突、行锁和版本不倒退测试

## 5. 登录、JWT 与 Refresh Token 切换

- [x] 5.1 调整登录 Service，在用户身份和密码校验成功后调用 `EnsureVersion`
- [x] 5.2 保证授权版本初始化或重新读取失败时登录失败且不签发 Token
- [x] 5.3 调整 Access Token 签发逻辑，使 Claim 使用新表返回的当前版本
- [x] 5.4 调整 Refresh Token 签发和刷新校验逻辑，使其使用同一授权版本来源
- [x] 5.5 新增 `POST /api/refresh`，接收 `refresh_token`，成功时返回使用当前授权版本的新 Access Token 和 Refresh Token，失败时返回稳定 401 契约
- [x] 5.6 调整 JWT 中间件，在用户存在和启用校验后只读取 `user_access_versions`
- [x] 5.7 在非登录认证流程缺少新表记录时拒绝 Token，不读取 `users.token_version`
- [x] 5.8 增加迁移前有效 Access Token 和 Refresh Token 在切换后继续有效的兼容测试
- [x] 5.9 增加版本不一致、账号禁用、用户删除和新表缺行时拒绝 Token 的认证测试

## 6. 用户生命周期与授权失效入口

- [x] 6.1 将修改密码后的会话失效切换为 `EnsureAndIncrement`
- [x] 6.2 将管理员修改用户角色或状态后的会话失效切换为 `EnsureAndIncrement`
- [x] 6.3 将 `PUT /api/admin/users/:id/status` 的会话失效切换为 `EnsureAndIncrement`
- [x] 6.4 将 `PUT /api/admin/users/:id/kick` 的会话失效切换为 `EnsureAndIncrement`
- [x] 6.5 将角色、角色权限、用户角色、角色菜单、菜单和组织成员变化的版本提升切换为新 Service
- [x] 6.6 将用户软删除和授权版本提升放入同一个 MySQL 事务
- [x] 6.7 保证用户软删除后版本记录保留，并为用户恢复后旧 Token 仍失效增加测试
- [x] 6.8 覆盖用户首次登录前被禁用、删除、Kick 或调整授权时的缺行创建并提升测试
- [x] 6.9 验证任一授权版本主写或镜像写失败时，对应用户操作按规格回滚

## 7. Seed 与切换发布

- [x] 7.1 确认普通用户创建流程不预先创建 `user_access_versions`
- [x] 7.2 确认 Seed 创建或确认超级管理员时不写入授权版本表，也不扫描用户或修复历史版本
- [x] 7.3 构建同时包含迁移、`EnsureVersion`、`EnsureAndIncrement`、只读新表和镜像写的单一发布制品
- [x] 7.4 编写停机切换操作步骤：停止旧实例和写流量、运行迁移、启动新版本、恢复流量
- [x] 7.5 在类生产环境执行停机切换演练，并验证不出现新旧实例混跑
- [x] 7.6 运行登录、刷新、禁用、软删除、恢复、Kick 和授权变更端到端测试
- [x] 7.7 验证所有 Token 版本读取来自新表且镜像写原子后，记录切换里程碑完成
- [x] 7.8 在切换里程碑完成后允许 `restructure-layered-monolith` 开始实施

## 8. 一致性扫描与观察期

- [x] 8.1 实现存量 `user_id <= cutoff_user_id` 必须有新表记录且版本相等的扫描
- [x] 8.2 实现 cutoff 后已有新表记录必须与旧字段相等的扫描
- [x] 8.3 将 cutoff 后旧字段为 NULL、0 或 1 的未初始化缺行识别为合法状态
- [x] 8.4 将 cutoff 后旧字段大于 1 但新表缺行识别为差异
- [x] 8.5 记录成功签发 Token 后缺行、`EnsureAndIncrement` 后缺行和新旧版本差异指标
- [x] 8.6 连续执行至少 3 小时加速耐久性资格验收：真实 MySQL + Docker Redis、
  至少 100,000 次混合认证授权操作、至少 8 个并发工作单元、至少 3 次受控应用
  进程重启、每 15 分钟及重启后的一致性扫描、零非预期镜像失败、零缺行和原子
  回滚故障注入；该门槛替代原完整发布周期和连续 7 个自然日观察门槛
- [x] 8.7 在观察期内不设置固定执行时间；每次用户要求开始本 Change 的任一剩余任务前，先运行关键认证授权、真实 MySQL 并发与事务门禁和一致性扫描，并按实际执行日期归档证据
  - 前置门禁未通过时不得开始对应任务。
  - 该任务触发门禁不能替代 8.6 的三小时加速耐久性资格验收。
- [x] 8.8 文档化观察期停机回滚步骤，并验证回滚前一致性扫描

## 9. 旧字段退出里程碑

- [x] 9.1 确认 3 小时加速耐久性资格验收通过：工作量、并发、重启、周期性及最终
  扫描、零非预期镜像失败、零缺行、原子回滚故障注入和全部关键回归测试均达标
- [x] 9.2 确认不再需要回滚到读取 `users.token_version` 的旧代码版本
- [x] 9.3 完成退出前最后一次新旧版本一致性扫描
- [x] 9.4 发布停止镜像写并移除全部 `users.token_version` 运行时读写的退出版本
- [x] 9.5 从 User GORM Model 和 AutoMigrate 声明中移除 `token_version`
- [x] 9.6 验证退出版本启动、登录、Refresh Token、JWT 和全部授权失效入口不再访问旧字段
- [x] 9.7 通过独立数据库变更物理删除 `users.token_version`
- [x] 9.8 在删列后重新运行启动、认证授权和全量测试
- [x] 9.9 删除仅用于旧字段镜像、迁移观察和旧版本回滚的代码、状态和过渡测试
- [x] 9.10 记录退出里程碑完成，并关闭本 Change

## 10. 文档与最终验证

- [x] 10.1 更新认证、用户管理、权限链路和数据库迁移文档
- [x] 10.2 更新 ADR，记录授权版本所有权、停机切换和旧字段退出顺序
- [x] 10.3 运行 SQLite Repository、Service 数据库 Adapter、顺序初始化和失败注入快速测试
- [x] 10.4 验证 OpenSpec delta specs 与实际登录、Token 校验、用户生命周期和迁移行为一致
- [x] 10.5 运行真实 MySQL 下的迁移锁、回填、并发初始化、行锁、死锁重试、事务镜像和软删除回滚测试
- [x] 10.6 运行真实 Redis 下的 Access Token、Refresh Token 和黑名单组合测试
- [x] 10.7 运行 `go test ./... -count=1`，确认无数据竞争、不稳定测试或旧字段读取引用
