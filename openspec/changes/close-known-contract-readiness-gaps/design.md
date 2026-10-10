## Context

本变更来自 `docs/reviews/contract-readiness.md` 的第一批收尾访谈，两个业务裁决已经确认：管理员不能代改邮箱；登录凭据错误继续累计并升级锁定，第 10 次不清零。登出 TTL 与 OpenAPI 错误信封采用已经存在的运行行为，不引入新策略。

当前生产代码按 ADR 0006 的业务模块与 App 唯一组合根组织。实现保留 HTTP Adapter → Application → Domain 的方向，仅触及 Identity 的不可达分支、API Doc 的行为测试及相关文档，不恢复顶层 `handler`、`dto` 或 `router` 目录。

事实依据：

- `internal/identity/application/user_service.go`：`UpdateByAdmin` 先拒绝非空邮箱，后续同条件邮箱更新分支不可达；共享 `UserChanges.Email` 仍服务于自助邮箱流程。
- `internal/identity/application/service.go`、`internal/identity/adapters/redis/store.go`：按累计次数选择锁定档位；每次记录失败续期 24 小时，成功登录清除累计。
- `internal/app/identity.go`：以 `durationMinutes(config.Jwt.Expire)` 装配登出 TTL；`auth` 主规格已明确固定时长。
- `internal/apidoc/service.go`、`internal/platform/httpresponse/response.go`：生成失败交由共享错误 writer 返回 HTTP 500 和 `HTTP_INTERNAL_ERROR`；原始 cause 不进入响应。
- README、模块导航及 ADR 0011 仍有指向归档前路径的可点击链接。

## Goals / Non-Goals

**Goals:**

- 对齐用户管理、认证和 API 文档的既有运行契约，保留安全边界。
- 删除管理员邮箱的不可达更新逻辑，避免未来误恢复绕过所有权验证的路径。
- 通过真实 HTTP handler 回归保护 OpenAPI 失败信封和泄漏防护。
- 修正当前运维说明与归档链接，为本批记录可复核的实施验收证据。

**Non-Goals:**

- 不开展剩余 C6b 全量审计，不新增工作台或后端能力。
- 不改变邮箱验证、自助资料、角色或状态更新、登录档位、Redis key、登出 TTL 或响应 writer 的运行语义。
- 不修改模型、配置、数据库、迁移、依赖、权限码、MinIO 或生产路由注册。
- 不删除共享邮箱字段，不新增字段存在性校验，不改变空邮箱字符串的现有处理，不重写历史验收工件。

## Decisions

### D1. 以已确认邮箱所有权边界修正旧承诺

修改 `user-management` 的完整「管理员修改用户」Requirement：普通用户资料更新场景不再列出邮箱，另补管理员携带非空邮箱时整次更新被拒绝、邮箱状态和其他提交字段不变的场景。已有自己／管理员保护和其他可达字段的语义全部保留；不重复修改 `identity` 已有的禁止代改要求。

在 `UpdateByAdmin` 中保留前置拒绝，删除后续邮箱查重与赋值分支，并去掉本方法空更新判断中只为该不可达分支保留的邮箱条件。HTTP DTO 的邮箱输入仍保留，以沿用现有拒绝与字段错误；共享 `UserChanges.Email` 不删除。

不采用恢复管理员代写的方案：它与已确认的邮箱所有权边界冲突。也不通过静默忽略非空邮箱来保存其他字段。复用并增强 `TestUpdateByAdminRejectsEmailField` 的行为断言，检查混合提交不会改变用户资料和邮箱状态；不新增检查源代码文本或调用次数的测试。

### D2. 明确累计升级规则，不修改运行策略

`auth` 的完整「登录锁定」delta 保留既有公开错误和成功登录场景，补齐 5／10／15／20 次档位、锁定到期后保留累计、每次记录失败续期 24 小时及成功清除。删除「达到配置阈值」这类容易误认为新增可配置能力的表述，明确现有五次阈值。

修正 `docs/runbooks/login-security.md` 的第 10 次重置说法，并区分锁定到期与错误累计到期。复用已有认证行为验证，不为文档措辞或内部常量新增重复测试。

不恢复十次清零：这会改变已确认规则，并使正常累计无法触发更高档位。

### D3. 登出 TTL 只纠正文档

保留已经发布的 `auth`「登出黑名单」Requirement，不提交无变化的重复 delta。运维文档改为从登出时起按 `jwt.expire` 配置固定时长保留黑名单，说明这不等于该 Token 的剩余有效期。

不调整 TTL 计算，不扩大为所有会话撤销或 Refresh Token 策略变更；既有生产装配与登出验证作为实施检查依据。

### D4. 在 HTTP 边界验证 OpenAPI 生成失败

`api-management` delta 复制完整「自动化 API 文档」Requirement，只把失败场景的稳定技术错误码明确为现有 `HTTP_INTERNAL_ERROR`，不丢失成功文档、Swagger UI 或关闭文档的场景。

在 `internal/apidoc/service_test.go` 中复用 Metadata 测试替身，增加可控读取失败，挂载真实 `Service.OpenAPI` handler 并发出 HTTP 请求。断言 HTTP 500、四个信封字段、`code=500`、`error_code=HTTP_INTERNAL_ERROR`、`data=null`，并确认响应不含注入的内部错误或敏感标记。注入值只使用测试数据，不读取或记录真实凭据。

永久回归覆盖实际序列化和共享错误处理，不只检查 `Document()` 返回错误，不测试转发调用或复制 writer 的实现，不重钉错误提示文案。已有成功文档行为验证继续保留。

另外用临时入口启动本地 HTTP 服务，真实请求 `/docs/openapi.json` 的受控失败路径并记录响应；不为了造失败破坏数据库或其他本地服务。该路径不依赖真实 MySQL、Redis 或 MinIO。冒烟完成后移除临时入口，不能以一次冒烟代替永久回归。

### D5. 仅修正活文档中的归档链接

修正 README、`docs/module-navigation.md` 和 ADR 0011 的相对链接，分别指向：

- `openspec/changes/archive/2026-10-01-standardize-api-response-contract/`
- `openspec/changes/archive/2026-10-01-build-admin-workbench/`

逐项确认目标文件或目录实际存在，包括联合验收记录、响应基线、错误矩阵、窄屏截图和任务记录。保持正文中的历史日期、结果和职责边界，不把验收记录复制回活动 change，不创建别名或重定向。

更新审计索引时只关闭本批对应遗留，保留剩余 C6b 未核状态；不得用 artifact 完成或静态源码核对冒充实施验收。

## Risks / Trade-offs

- [误删共享邮箱流程] → 只清理 `UpdateByAdmin` 的不可达分支和失效条件，复用自助邮箱及管理员拒绝行为测试。
- [MODIFIED 丢失其他场景] → 三份 delta 均复制完整 Requirement；对照主规格验证已有场景集合和非目标正文未被删改。
- [使用旧报告错误码] → 断言当前 `HTTP_INTERNAL_ERROR`，不恢复旧记录中的 `INTERNAL_ERROR`，不新增兼容别名。
- [测试只有内部失败、未保护 HTTP] → 发出真实 handler 请求，并另做一次本地 HTTP 冒烟，验证外部状态、信封和泄漏边界。
- [文档修正被误读为行为改造] → 保留现有策略和运行接口，明确本批不含管理员协助变更、锁定重设或会话撤销扩展。
- [把已知收尾当作全部审计结束] → 实施记录逐项附证据，C6b 后续范围另行讨论，不自动纳入新规格或宣称全量完成。

## Migration Plan

无数据库迁移、回填、Redis 状态清理或基础设施变更。实施时完成最小代码删除、永久回归和文档修正，执行相关行为验证、HTTP 冒烟与 OpenSpec 校验；通过后再同步 delta 到主规格并归档。生成工件阶段不修改主规格、不发布代码。

如需回滚，只回退本批代码／测试／文档提交，不恢复管理员代写能力、不清理 Redis 状态，也不覆写原历史验收工件。

## Open Questions

本批没有阻塞性的业务未决项。剩余 C6b 的审计范围、批次顺序及旧证据可复用条件由后续讨论决定，不阻塞本变更创建，也不进入本批实施任务。
