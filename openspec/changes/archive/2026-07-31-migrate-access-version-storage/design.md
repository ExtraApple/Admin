## Context

系统当前把授权版本保存在 `users.token_version`。JWT 中携带该版本，认证中间件通过版本比较使权限、角色、菜单、数据范围或账号状态变化后的旧 Token 失效。登录会签发 Refresh Token，但仓库当前没有 Refresh Token 换取新 Token 的 HTTP 入口。

该字段位于 Identity 所有的用户记录中，但版本变化主要由 Authorization 规则驱动。继续使用该字段会使用户生命周期、授权关系和 Token 失效逻辑共同依赖 `users` 表，也会阻塞后续 `restructure-layered-monolith` 对 Identity 与 Authorization 所有权的拆分。

本 Change 需要在不使存量有效 Token 集体失效的前提下切换读取事实来源，并保留一个有退出门槛的短期回滚窗口。项目当前依据 ADR 继续使用 GORM AutoMigrate 和幂等 Go 回填，因此不在本次引入新的 SQL migration 工具。

约束：

- 数据库为 MySQL。
- Access Token 和 Refresh Token 的 `token_version` Claim 格式不变。
- Redis 黑名单、既有 HTTP API、Permission Code 和用户状态规则不变；本 Change 补齐 `POST /api/refresh`。
- 不支持旧代码只写旧字段、新代码只读新表的混合实例部署。
- `restructure-layered-monolith` 只需等待本 Change 达到切换里程碑，不等待旧字段退出。

## Goals / Non-Goals

**Goals:**

- 将授权版本的唯一读取事实来源迁移到 Authorization 拥有的 `user_access_versions`。
- 保持迁移前合法 Token 的版本值不变，避免全员强制重新登录。
- 为存量回填提供迁移锁、固定 cutoff、幂等重试和启动失败保护。
- 为新用户提供并发安全的懒初始化。
- 为首次登录前的禁用、删除、Kick 和授权变化提供统一的原子创建并提升语义。
- 在观察期内原子镜像旧字段，使停机回滚可验证。
- 使用指标和一致性扫描控制旧字段退出。
- 确保用户软删除不会使删除前 Token 在恢复后重新生效。
- 提供 `POST /api/refresh`，让合法 Refresh Token 使用统一授权版本来源换取新的 Token 对。

**Non-Goals:**

- 不实现零停机滚动切换。
- 不允许长期双读、取最大值或缺行时回退旧字段。
- 不把镜像写发展为长期双写架构。
- 不修改 JWT Claim、Token 有效期、Redis 黑名单 key 或既有 HTTP 契约；仅新增缺失的 Refresh Token 入口。
- 不让 Seed 扫描用户或承担历史回填。
- 不在本 Change 中执行项目目录重构。
- 不引入 goose、golang-migrate 或其他正式 migration 工具。

## Decisions

### 1. Authorization 拥有独立授权版本表

新增：

```text
user_access_versions
├── user_id        primary key
├── version        int not null default 1
├── created_at
└── updated_at
```

表不使用 `deleted_at`。用户软删除后保留版本记录；若建立外键，不使用 `ON DELETE CASCADE`。物理删除用户时由显式清理流程决定是否清除版本记录。

Identity 继续拥有用户存在性、启用状态、JWT、Refresh Token 和 Redis 黑名单。Authorization 拥有版本读取、比较、初始化和提升。

选择独立表而不是继续使用 `users.token_version`，是为了让授权事实由 Authorization 持有，并避免所有授权变化都写入用户聚合。备选的 Redis 版本会引入持久化与缓存恢复问题；独立 Token 表按 Token 记录会增加写放大和清理成本。

### 2. Token 校验保持身份和授权顺序分离

认证顺序为：

```text
Identity 解析 Token
→ Identity 校验 Redis 黑名单
→ Identity 校验用户存在且启用
→ Authorization 读取 user_access_versions
→ 比较 Token 版本
```

Access Token 和 Refresh Token 都使用同一授权版本来源。Authorization 不重复拥有用户状态规则。

新表记录缺失不能被解释为“跳过版本检查”。除登录初始化流程外，认证时缺行属于错误并拒绝 Token。

新增公开路由：

```text
POST /api/refresh
request:  {"refresh_token":"<jwt>"}
success:  {"code":200,"msg":"刷新成功","data":{"access_token":"<jwt>","refresh_token":"<jwt>"}}
failure:  HTTP 401，稳定 code=401
```

Refresh Token 流程不重新校验密码或验证码。Identity 解析并校验 Refresh Token 的签名、有效期和用户存在/启用状态，Authorization 只读取 `user_access_versions` 比较版本；校验成功后签发使用当前版本的新 Access Token 和 Refresh Token。旧 Refresh Token 的一次性消费或重放检测不在本 Change 范围内。

### 3. 存量用户按原版本幂等回填

迁移锁取得后，在恢复写流量前记录：

```text
cutoff_user_id = 当前 users 最大 ID
```

存量集合定义为 `users.id <= cutoff_user_id` 的全部用户，包括软删除用户。

回填规则：

```text
users.token_version > 0
  → 复制原值

users.token_version IS NULL 或 <= 0
  → 写入 1
```

回填不得覆盖新表中更高版本。每次回填后，对于不存在版本倒退的 cutoff 记录，迁移 SHALL 将
新表的最终版本值镜像回 `users.token_version`：这会把 NULL 或非正旧值归一化为 1，并使重试时
保留的新表更高版本也与旧字段一致。若新表版本低于归一化后的旧版本，迁移不得降低旧字段，且
后续校验必须阻止启动。迁移完成前，每次启动都使用同一 cutoff 补齐并重新校验；完成后不得扩大
存量集合。

相比统一写 1，该规则能保持迁移前签发 Token 的合法版本。相比双读取最大值，它不会产生两个长期事实来源。

### 4. 迁移状态持久化并在失败时阻止启动

通过 AutoMigrate 创建 `user_access_versions` 和迁移状态存储。迁移状态至少包含：

```text
name
status              # running/completed
cutoff_user_id
started_at
completed_at
```

执行顺序：

```text
取得数据库级迁移锁
→ AutoMigrate 创建表
→ 创建或读取迁移状态
→ 固定 cutoff
→ 幂等回填
→ 校验数量、缺行和版本倒退
→ 标记 completed
→ 释放迁移锁
```

任何步骤失败都阻止服务对外启动。迁移状态不由 Seed 管理。

数据库级锁用于阻止多个迁移进程同时推进状态；固定 cutoff 用于避免失败重试时将后续新用户误当成存量数据。

### 5. 新用户首次登录时懒初始化

正常用户创建和 Seed 不创建授权版本。Identity 在确认用户存在且启用后调用 Authorization 的 `EnsureVersion`：

```text
开启或加入数据库事务
→ 查询版本
→ 缺行时 INSERT version=1 ON DUPLICATE KEY DO NOTHING
→ 锁定并重新读取当前版本
→ 若本次完成初始化，观察期内镜像写 users.token_version = 当前版本
→ 提交事务
→ 签发 Token
```

初始化失败时登录失败。成功签发 Token 时新表必须已有记录。

选择懒初始化是为了避免 Identity 用户创建事务必须直接拥有 Authorization Repository，也避免 Seed 承担业务数据修复。唯一键保证并发首次登录只产生一行。

### 6. 所有失效操作使用 EnsureAndIncrement

用户禁用、软删除、Kick、密码变化和授权关系变化统一调用 `EnsureAndIncrement`：

```text
加入调用方最外层事务
→ 幂等确保版本记录至少为 1
→ 锁定版本行
→ version + 1
→ 取得最终新版本
→ 镜像写 users.token_version = 最终新版本
```

缺行时不能执行普通 UPDATE 后将零影响行当作成功。镜像字段不能独立 `+1`，必须写入新表计算出的最终值，避免旧字段为 NULL、0 或并发变化时产生分歧。

`EnsureVersion` 与 `EnsureAndIncrement` 并发时依靠主键唯一约束和行锁，使版本不倒退且只保留一行。

### 7. 用户软删除与版本提升原子提交

管理员软删除用户时，用户记录更新和 `EnsureAndIncrement` 在同一个 MySQL 事务中执行。任一步失败全部回滚。

版本记录在软删除后保留。恢复用户不重置版本，因此删除前 Access Token 和 Refresh Token 不能重新生效。

用户禁用和 Kick 在首次登录前也必须使用 `EnsureAndIncrement`，不能假设版本行已存在。

### 8. 切换期间只读新表并原子镜像旧字段

新版本发布后，版本初始化和版本提升都遵循：

```text
读取：只读 user_access_versions
主写：user_access_versions
镜像写：users.token_version
```

新表初始化/提升与旧字段镜像必须属于同一数据库事务，任一写入失败全部回滚并记录指标。

禁止：

- 双读并取最大值。
- 新表缺行时长期回退旧字段。
- 两个字段分别提交。
- 在新旧服务实例之间滚动混跑。

只读新表能够尽早验证新事实来源；短期镜像旧字段只为停机回滚服务。

### 9. 使用停机切换而不是混合版本滚动发布

切换顺序：

```text
停止全部旧实例和写流量
→ 启动单个迁移进程或由一个实例取得迁移锁
→ 完成建表、回填和一致性校验
→ 启动同时具备 EnsureVersion、EnsureAndIncrement、只读新表和镜像写的新版本
→ 恢复流量
```

懒初始化、缺行提升、只读新表和镜像写必须在同一发布制品中交付。

观察期内回滚：

```text
停止全部新实例和写流量
→ 完成新旧字段一致性扫描
→ 回滚到读取 users.token_version 的旧版本
→ 恢复流量
```

不选择兼容滚动发布，是因为旧实例不会维护新表，混跑会使新版本读取到过期授权版本。

### 10. 一致性扫描区分存量、已初始化和合法缺行

扫描规则：

| 用户类别 | 新表要求 | 判定 |
| --- | --- | --- |
| `user_id <= cutoff_user_id` | 必须存在 | 新旧版本相等 |
| cutoff 后已初始化或已发生失效操作 | 必须存在 | 新旧版本相等 |
| cutoff 后尚未登录且从未发生失效操作 | 允许缺行 | 旧字段为 NULL、0 或 1 时不计差异 |

cutoff 后旧字段大于 1 但新表缺行，或任意已有新表记录与旧字段不一致，均计为差异。

同时记录：

- 成功签发 Token 时新表缺行数。
- `EnsureAndIncrement` 完成后新表缺行数。
- 镜像写失败数。
- 一致性扫描差异数。

### 11. 使用三小时加速耐久性资格验收控制旧字段退出

用户不希望为本次类生产验证持续运行应用一个完整发布周期或连续 7 个自然日，因此
退出门槛改为固定、可重复且具有明确负载下限的加速耐久性资格验收。该资格验收
替代原完整发布周期和连续 7 个自然日观察门槛，不把零散测试时间或历史进程内指标
拼接为有效观察。

资格验收必须同时满足：

1. 在 Windows 本地隔离真实 MySQL 与 Docker Redis 上连续至少 3 小时运行。
2. 完成至少 100,000 次混合认证授权操作，并维持至少 8 个并发工作单元。
3. 混合操作覆盖登录、Access Token 校验、Refresh Token、黑名单以及会话失效后的
   Token 拒绝。
4. 在资格窗口内完成至少 3 次受控应用进程重启，外部运行器持续保存累计统计，
   不因进程内指标清零而丢失历史。
5. 至少每 15 分钟执行一次新旧授权版本一致性扫描，并在开始、每次重启后和结束时
   额外扫描；所有扫描差异均为 0。
6. 非预期镜像写失败数为 0，成功签发 Token 后缺行数和
   `EnsureAndIncrement` 后缺行数均为 0。
7. 执行原子回滚故障注入，证明新表主写、旧字段镜像和用户生命周期业务更新在失败
   时不会部分提交。
8. 关键认证授权快速回归、真实 MySQL 并发与事务门禁和真实 Redis 组合门禁全部通过。

三小时资格验收提高短时间内发现并发、重启、事务和外部依赖问题的概率，但不能被
描述为等价于真实生产 7 日运行。该取舍通过固定工作量、周期扫描、重启和故障注入
取得可重复的发布资格证据。

### 12. 旧字段退出分为代码里程碑和独立 DDL

退出门槛必须同时满足：

1. 连续至少 3 小时加速耐久性资格验收通过。
2. 至少 100,000 次混合认证授权操作和至少 8 个并发工作单元门槛达成。
3. 至少 3 次受控应用进程重启完成。
4. 周期性及最终一致性扫描差异为 0。
5. 非预期镜像写失败、Token 签发后缺行和版本提升后缺行均为 0。
6. 原子回滚故障注入和全部关键认证授权回归通过。
7. 不再需要回滚到读取旧字段的代码版本。

退出顺序：

```text
完成最后一次一致性扫描
→ 发布退出版本：停止镜像写
→ 移除全部运行时读写、User GORM Model 字段和 AutoMigrate 列声明
→ 验证启动、登录、刷新和授权失效链路不再访问旧字段
→ 独立 DDL 删除 users.token_version
→ 删除仅用于迁移观测的代码、状态和过渡测试
```

先删除 DDL 再移除 GORM Model 会导致运行时 SQL 报错或 AutoMigrate 重新创建列，因此不采用。

### 13. 两个 OpenSpec Change 使用里程碑依赖

本 Change 包含：

- **切换里程碑**：全部读取来自新表、存量 Token 兼容、镜像写原子、关键认证测试通过。
- **退出里程碑**：观察指标达标、退出版本部署验证、旧字段 DDL 删除、过渡代码清理。

`restructure-layered-monolith` 在切换里程碑后即可开始。本 Change 在观察期继续保持打开状态，并在目录重构进行期间完成退出里程碑。

### 14. 数据库测试采用 SQLite 快速反馈和 MySQL 强制验收

项目已经包含 SQLite 测试驱动。以下与数据库方言无关的行为优先使用隔离的 SQLite 测试，以缩短反馈时间：

- Model 映射、默认值和基础唯一约束。
- Repository CRUD 和普通事务回滚。
- 回填值转换函数及幂等业务判断。
- `EnsureVersion`、`EnsureAndIncrement` 的顺序执行和失败注入。
- 登录、用户生命周期 Service 使用数据库 Adapter 时的快速集成测试。

SQLite 测试必须使用每个测试独立的临时库，或唯一命名的 shared-memory DSN 并控制连接生命周期，不能直接假设所有 `:memory:` 连接共享同一数据库。

以下场景必须在真实 MySQL 中验收，SQLite 结果不能替代：

- 数据库级迁移锁和多实例争抢。
- 固定 cutoff、迁移状态及启动阻断的完整流程。
- `INSERT ... ON DUPLICATE KEY` 或对应 GORM 方言行为。
- `SELECT ... FOR UPDATE`、并发首次登录和并发失效操作。
- 事务隔离、死锁识别与有界重试。
- 新表主写和旧字段镜像的原子提交。
- AutoMigrate、索引、外键、软删除交互和独立删列 DDL。

可复用的 Repository 测试应形成同一套测试用例：SQLite 作为开发和普通 CI 的快速门禁，MySQL 套件作为合并和发布前的强制门禁。

## Risks / Trade-offs

- [回填版本错误导致全员下线] → 保留正版本原值，只归一化 NULL 和非正数，并使用迁移前 Token 回归测试。
- [失败重试扩大存量集合] → 首次运行固定 cutoff，后续重试沿用同一值。
- [多个实例同时迁移] → 使用数据库级迁移锁和持久化状态，失败时阻止启动。
- [新旧实例混跑造成新表过期] → 使用停机切换，部署流程显式禁止滚动混跑。
- [新用户缺行导致失效操作未生效] → 所有失效入口使用 `EnsureAndIncrement`，零影响 UPDATE 不得视为成功。
- [首次登录和失效操作并发导致版本倒退] → 使用主键唯一约束、幂等插入和行锁，并覆盖 MySQL 并发测试。
- [镜像字段和新表不一致] → 初始化和提升都镜像最终版本值，并与新表主写处于同一事务，记录失败指标。
- [双写长期化] → 设定唯一退出阶段和三小时加速耐久性资格门槛，不允许其他功能
  读取旧字段。
- [三小时窗口遗漏低频生产问题] → 用至少 100,000 次操作、8 个并发工作单元、
  3 次重启、每 15 分钟扫描和原子回滚故障注入提高缺陷暴露率，并明确该资格验收
  不是对真实生产 7 日运行的统计等价替代。
- [先删列导致 GORM 启动失败] → 先发布移除运行时、Model 和 AutoMigrate 引用的退出版本，再独立执行 DDL。
- [Seed 职责扩张] → Seed 不创建任何用户授权版本；存量由迁移、新用户由登录或失效用例处理。
- [SQLite 测试通过但 MySQL 语义失败] → SQLite 只覆盖方言无关行为，迁移锁、行锁、死锁、并发和 DDL 必须由真实 MySQL 门禁。

## Migration Plan

### 切换里程碑

1. 新增 Model、Repository、迁移状态和数据库级锁。
2. 实现 `EnsureVersion`、`EnsureAndIncrement`、新表读取和镜像写。
3. 修改登录、JWT、Refresh Token、用户状态、删除、Kick 和授权失效调用点。
4. 补齐迁移、并发、事务、认证和回滚测试。
5. 构建包含全部切换能力的单一发布制品。
6. 停止旧实例和写流量。
7. 运行 AutoMigrate、固定 cutoff、回填并校验。
8. 启动新版本并恢复流量。
9. 验证所有读取来自新表、旧 Token 保持有效、镜像写一致。
10. 标记切换里程碑完成，允许 `restructure-layered-monolith` 开始。

### 观察期

1. 启动外部加速耐久性运行器，使用真实 MySQL 和 Docker Redis 连续至少 3 小时
   运行。
2. 维持至少 8 个并发工作单元，完成至少 100,000 次混合认证授权操作。
3. 每 15 分钟、每次重启后和结束时执行按 cutoff 分类的一致性扫描。
4. 完成至少 3 次受控应用进程重启，由外部运行器保存累计指标。
5. 执行原子回滚故障注入，并持续验证登录、刷新、Token 校验、黑名单和失效后拒绝。
6. 保持旧字段只作为回滚镜像，不允许业务读取。

### 退出里程碑

1. 满足连续至少 3 小时、至少 100,000 次操作、至少 8 个并发工作单元、
   至少 3 次重启、零非预期镜像失败、零缺行和零扫描差异的加速耐久性资格门槛。
2. 完成最后一次一致性扫描。
3. 发布不读取、不写入、不映射且不通过 AutoMigrate 声明旧列的退出版本。
4. 验证退出版本全部认证授权链路。
5. 独立执行 DDL 删除 `users.token_version`。
6. 删除迁移观察代码、迁移状态和过渡测试。

### 回滚

- 迁移或校验失败：保持写流量关闭，不启动新版本；修复后使用同一 cutoff 幂等重试。
- 新版本观察期故障：停止全部新实例，确认镜像一致后整体回滚到读取旧字段的旧版本。
- 退出版本部署后：只允许回滚到仍读取新表但保留旧列无依赖的版本，不再回滚到旧字段读取版本。
- 物理删列后：旧字段恢复需要独立数据库变更，不属于常规应用回滚。

## Open Questions

无阻断实施的开放问题。表所有权、回填规则、部署方式、读写策略、懒初始化、软删除语义、观察指标和旧字段退出顺序均已确认。
