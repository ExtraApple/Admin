## Context

A2 真实 HTTP 审计证明，超级管理员进入 Messaging 的 All-scope 分支时，`messagingOrganizationReader` 未实现 `AllOrganizationIDs`，导致组织分类查询、群发创建和公告创建返回 `MSG_INTERNAL_ERROR`。`organization.ScopeReader` 已经拥有该能力，组织模块的 `ListAllUnits` 语义是当前系统范围的既有来源。

该修复只闭合组合根漏接，不重新设计组织范围、权限码或消息事务。

## Goals / Non-Goals

**Goals:**

- 在 Messaging 组织适配器中实现 `AllOrganizationIDs` 转发。
- 让现有 All-scope 业务使用同一组织范围来源。
- 保持受限管理员 `DescendantOrganizationIDs` 路径不变。
- 通过单元、组合根和真实 HTTP 回归验证分类、群发和公告路径。

**Non-Goals:**

- 不修改 `organization.ScopeReader` 接口或其组织筛选语义。
- 不新增路由、权限码、Redis key、MinIO bucket、消息模型或事件字段。
- 不改变禁用/软删除组织的既有查询行为。
- 不回写或修复 A2 归档工件。

## Decisions

### 1. 由组合根转发既有 ScopeReader 能力

在 `messagingOrganizationReader` 增加 `AllOrganizationIDs(context.Context) ([]uint, error)`，直接调用 `reader.scope.AllOrganizationIDs`。这样 Messaging 继续依赖自己的 `OrganizationAudienceReader` 最小 Contract，App 负责把组织模块能力装配进去。

备选方案是让 Messaging application 直接依赖完整组织 ScopeReader，或复制组织查询逻辑；前者扩大模块边界，后者形成第二套组织语义，均不采用。

### 2. 不新增组织过滤

All-scope 继续使用 `ScopeReader.AllOrganizationIDs` 的当前结果。修复只解决能力缺失，不额外排除 disabled organization，也不改变软删除过滤。

### 3. 以真实路由验证闭合

回归验证覆盖超级管理员读取分类、创建群发、创建公告并发布；受限管理员仍验证只能使用其既有组织范围。失败请求必须保留稳定业务错误，不泄露底层适配器错误。

## Risks / Trade-offs

- 如果组织模块后续改变 `AllOrganizationIDs` 语义，Messaging All-scope 会随之改变；这是复用单一事实来源的预期代价。
- 真实 HTTP 测试需要隔离 MySQL、JWT 和消息相关依赖；不使用 fake 代替组合根验证。
- 本 change 不补充历史数据，也不修复其他 A2 发现。
