# ID-SEED-COUNT 定向核实

## 结论与边界

2026-10-15；代码基线 `c4b96ce1d64daede23a8d867d0a990e1c5887c04`。**ID-SEED-COUNT 从源码候选提升为已核实的现行契约实现不符，Owner 为 Identity，尚未修复。**

已有启用管理员时，只中断 active-admin Count，后续用户名和角色关联读取仍完成；原程序随后重写该管理员密码、提交 Seed 事务并正常提供 HTTP。正常重启对照不重置密码，因此不是单纯更改管理员配置就会产生的结果。

本次仅核实这一条条件路径；没有扩展 A1 其他风险，没有改产品、测试、主规格、CONTEXT 或 ADR，没有创建 change／proposal／design／tasks／delta spec。[当前有限审计计划](contract-readiness.md#c6-当前执行计划)继续有效。

**后续实施状态（2026-10-16）**：用户另行授权的[修复 change](../../openspec/changes/archive/2026-10-20-fix-admin-seed-query-error-handling/proposal.md)已实现并完成定向验收，当时尚未同步主规格或归档。两处 Count 均保留原始错误并停止本阶段；三个相关包回归通过。原 `main.go` 的真实 MySQL 单条 active-admin Count 中断返回1317，程序退出2且未监听HTTP，密码／身份／绑定／版本不变，Finalize回滚而前序API同步提交保留；撤除故障后同配置正常启动，原密码仍有效。第二处绑定读取故障由独立SQLite存储边界回归覆盖，不冒充第二次原生MySQL故障。新增[去敏验收记录](evidence/id-seed-count-fix-2026-10-16.json)与历史JSON分开保存；本文其余内容保留2026-10-15审计事实与停止点。其他风险及全批进度见[C6当前计划](contract-readiness.md#c6-当前执行计划)。

**后续规格／归档状态（2026-10-20）**：该修复已随本批完成主规格智能同步并归档；上方change链接已迁移到归档位置。此记录不修改原审计事实、历史JSON或其他候选风险的分类。

## 现行依据

[`rbac`「已存在启用超级管理员」](../../openspec/specs/rbac/spec.md#scenario-已存在启用超级管理员)，主规格第 90–93 行：

> - **WHEN** 服务启动时已经存在启用用户绑定到 `roles.code = admin`
> - **THEN** 系统不创建新的超级管理员
> - **AND** 系统不重置现有超级管理员密码

这里不存在需要先裁决的新业务语义：测试前后的用户、启用 admin 角色和绑定均真实存在。查询失败不能证明这些事实不存在。本次判错依据是**既有管理员密码被重置**；“应用继续启动”是实际结果，不单独推导出所有读取错误均必须拒绝启动的新要求。

## 入口与具体根因

实际组合根 `main.go` → App Seed → `seeddata.Finalize` → `identity/adapters/gorm.SeedAdminUser`，不是直接调用 helper 的替代程序。

1. `internal/identity/adapters/gorm/seed.go:73-76`：`hasActiveSuperAdmin` 丢弃 Count 返回的 `.Error`，只返回 `count > 0`。本次错误后 `count=0`，原事务 `tx.Error=nil`，读取失败被当成没有启用管理员。
2. 同文件 `:43-49`：配置用户名读取成功，角色关联判断为真，进入 `restoreSeedAdminUser`。
3. 同文件 `:65-70`：对既有用户写入配置密码的 bcrypt 哈希，并写入启用／恢复字段。
4. `internal/app/seed.go:56`：到达 Finalize 事务成功提交后的日志语句；外层未收到该 Count 错误。随后实际 HTTP 登录证明密码已经替换。

`hasUserRoleInTx:79-82` 也有丢弃 Count.Error 的源码线索，但本次没有向其注入错误，不将它的失败后果另算已核实问题，也不重复登记 Identity Owner。

## 环境、对照与故障方法

使用未为本次验证改写的原 `main.go`，仅以 `-gcflags=all=-N -l` 编译便于断点观察。真实 MySQL 8.0.19／InnoDB／REPEATABLE-READ、Redis 8.0.5 和独立 MinIO 临时数据；没有新建 Go 探针。调试设备的启动／连接尝试未提供可用执行证据，成功取证使用原生 Delve 1.26.3 CLI 和显式隔离工作目录。

隔离 schema 为 `admin_seed_count_3dc1545b3295`。先用密码 A 初始化一个启用管理员；将其授权版本设置为独立哨兵值 7，然后仅把隔离配置密码改为 B。A／B、密码哈希、数据库口令、JWT 和验证码答案均不写入证据。

- **正常对照**：同一原程序在配置 B 下重启，不注入故障，观察密码与版本并实际登录。
- **故障路径**：在迁移和此前 Seed 阶段完成后，断于 Identity `seed.go:75`；外部会话仅对隔离 schema 的 `users` 取得 WRITE 锁。继续执行，确认 connection 22 的 `Execute` 正在等待该条 Count 的表元数据锁，再执行一次 `KILL QUERY 22`，不是断开连接或令整个数据库不可用。
- **保持后续读取可用**：断于 `:76` 读取 `count` 和原事务错误；释放外部锁后继续。实际进入 `:65` 恢复分支及 App `seed.go:56` 提交后位置，再继续到 HTTP。没有改调试变量、强制返回或跳过程序语句。

目标 SQL：

```sql
SELECT count(*) FROM `users`
JOIN user_roles ON user_roles.user_id = users.id
WHERE user_roles.role_id = 1
  AND users.status = 1
  AND users.deleted_at IS NULL
```

原程序实际报告 `Error 1317 (70100): Query execution was interrupted`，位置为 Identity `seed.go:75`。

## 观察结果

两组开始时均已有启用管理员，配置密码均为 B，原存储密码均为 A。

| 实际断言 | 正常重启对照 | 仅 active-admin Count 被中断 |
| --- | --- | --- |
| Count 错误 | 无注入 | MySQL 1317；随后 count=0、原 tx.Error=nil |
| Seed／HTTP 启动 | 成功 | 成功，且到达事务提交后位置 |
| 既有密码哈希 | 保持不变 | 已改变 |
| 原密码 A 登录 | HTTP 200 | HTTP 401，AUTHN_CREDENTIALS_INVALID |
| 配置密码 B 登录 | HTTP 401，AUTHN_CREDENTIALS_INVALID | HTTP 200 |
| 启用管理员数 | 1 | 1，提交前后不变 |
| 用户／角色／绑定 | 原用户1、admin角色1、绑定1→1 | 同一用户、角色、绑定，启用且未删除 |
| 授权版本 | 7 | 7，故障前后不变 |

因此确认的反例是：**数据库单条存在性读取失败，被误当成“没有启用管理员”，最终覆盖了当前规格明确要求保留的密码。** 未发生新建管理员；不把本次密码替换夸大成已证明远程提权或认证绕过。

## 分类、历史证据与剩余边界

- **现行契约问题**：ID-SEED-COUNT 已补足此前只有源码线索的定向运行证据，确认为现行承诺下的实现不符。不是依据新目标倒推旧实现有错。
- **规范变更／语义待澄清**：本次没有新增目标，也没有阻塞这一判定的业务歧义；读取失败后的完整可用性策略未在此设计或裁决。
- **历史证据**：复用[RBAC 矩阵](contract-readiness-rbac.md)及[首轮报告](contract-readiness-c6b-authorization-writes.md)的条款、入口和已有正常路径边界。旧 SQLite Seed 成功、普通登录与历史真实初始化只证明其适用正常路径；不替代本次单条错误证据。原 2026-10-14 JSON 与条款快照保留不变。
- **证据缺口**：仅覆盖 MySQL 1317 这一真实语句中断路径，不代表所有数据库错误、第二个 Count 的故障或完整 Identity／Seed 均已审计。旧 Access Token 的额外请求返回 401，但观察时该 Token 已过期；不能据此判断密码覆盖后的旧会话是否仍可用。版本未变是独立的真实存储观察，不冒充旧 Token 仍被接受的证明。
- **验证方式**：调试编译、实际启动、真实 SQL 故障、事务提交后状态与真实验证码／登录 HTTP。未新增或运行测试、lint、formatter；未修复或规划具体实现方案。

## 证据与停止点

去敏结构化记录：[id-seed-count-2026-10-15.json](evidence/id-seed-count-2026-10-15.json)。记录是原程序、数据库、HTTP 和调试观察的人工整理，不是完整原始调试转录。

隔离schema、应用／调试器／Redis／MinIO进程及临时目录均已清理，自用端口均已关闭，结果见JSON的 `teardown`。共享 `admin` schema 未作迁移、Seed或写入。此次交付停在**创建 change 之前**，不启动A1其他风险、不实施修复。
