## Why

有效Access JWT对应的独立授权版本缺行时当前返回内部错误，C6已证明请求被拒绝但状态码违反现行认证契约。需要准确区分不可验证会话与真实存储故障。

## What Changes

- 通过现有错误cause链保留缺版本的类型分类，在App桥接为Identity可消费的不可验证会话原因。
- Authenticate只将该原因映射为HTTP 401及AUTHN_TOKEN_INVALID；真实数据库故障仍安全5xx。
- 不初始化缺行、不回退用户旧字段、不签发Token；保持Refresh既有稳定拒绝行为。

**性质与来源**：现行承诺下已核实缺陷；C6。依据 `docs/reviews/contract-readiness-c6b-authorization-writes.md` 与 `docs/reviews/contract-readiness.md` 对应裁决；历史证据仅按已记录局部复用，未执行本change的产品任务。

**非目标**：不将所有AUTHZ内部错误转成401，不吞掉DB故障，不新增版本懒修复、其他存储fallback或认证绕过。

## Capabilities

### New Capabilities

无；使用既有业务能力，不新增一级模块。

### Modified Capabilities

- `auth`: 精确化Access缺版本与基础设施错误的不同结果及不补行保证。

## Impact

- 代码：`internal/authorization/adapters/gorm/access_version.go`、`internal/authorization/application/service.go`、`internal/app/identity.go`、`internal/identity/application/contracts.go`、`internal/identity/application/service.go`。
- 入口：已认证Access请求代表GET /api/user/info；POST /api/refresh作为既有拒绝对照。
- 数据：`user_access_versions（只读）`。
- 不新增依赖包、HTTP路径、Redis key、MinIO bucket或权限码；当前主规格在apply/归档流程前保持不变。
- 实施/发布前置：无外部change前置；与其他独立修复可分别实施。
- 验收以design/tasks的真实服务行为为准；工件完成不代表产品已修复或已验证。
