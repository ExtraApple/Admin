## Why

已有启用超级管理员时，单条存在性查询失败会被当成“没有管理员”，原程序随后覆盖既有密码并启动；ID-SEED-COUNT已经以原程序和真实MySQL语句中断核实。需要恢复已发布的既有管理员密码保留保证，而不是重新设计初始化系统。

## What Changes

- 让active-admin与用户角色绑定两处Count明确返回查询错误，禁止用false代替读取失败。
- 错误沿现有Finalize／App启动链返回，失败阶段不提交且不进入HTTP服务；已成功阶段仍保留。
- 正常识别已有管理员时不重置密码；普通用户名不误提升；Seed仍不创建或修复授权版本。
- 补充单条语句故障、正常重启与原程序启动拒绝的针对性回归。

**性质与来源**：现行承诺下已核实缺陷；ID-SEED-COUNT。依据 `docs/reviews/contract-readiness-id-seed-count.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不新增整轮Seed事务、自动重试、授权版本修复、管理员密码运维重置接口或旧Token可用性结论。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `rbac`: 明确兜底初始化读取失败不得触发密码恢复，并保留已有正常初始化及管理员身份规则。

## Impact

- 代码：`internal/identity/adapters/gorm/seed.go`、`internal/app/seed.go`、`internal/app/seeddata/seed.go`。
- 入口：无新增路由；影响main.go启动与Seed结果，HTTP仅用于证明失败时不提供服务。
- 数据：`users`、`roles`、`user_roles`、`user_access_versions（只验证不被Seed改变）`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：无外部change前置；与其他独立修复可分别实施。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
