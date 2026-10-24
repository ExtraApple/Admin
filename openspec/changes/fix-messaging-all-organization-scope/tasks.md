## 1. 组合根能力闭合

- [x] 1.1 在 `messagingOrganizationReader` 实现 `AllOrganizationIDs(context.Context) ([]uint, error)`，直接转发既有 `organization.ScopeReader.AllOrganizationIDs`。
- [x] 1.2 保持 `DescendantOrganizationIDs`、受限管理员范围和既有组织筛选语义不变，并保留编译期 `OrganizationAudienceReader` 合约检查。

## 2. 回归验证

- [x] 2.1 增加组合根/适配器测试，确认 All-scope 返回 ScopeReader 组织集合且不复制组织查询逻辑。
- [x] 2.2 增加真实 App HTTP 测试：超级管理员读取消息分类、创建群发、创建并发布公告；确认成功或稳定业务错误，不返回内部适配器错误。
- [x] 2.3 增加受限管理员回归测试，确认其组织及子组织范围行为未扩大。
- [x] 2.4 运行 Messaging、App 和相关 HTTP 定向测试，确认未改变路由、权限码、事件和数据模型。

## 3. 交付边界

- [x] 3.1 复核变更仅触及组合根和测试；不修改组织接口、路由、权限码、Redis key、MinIO bucket 或历史数据。
- [x] 3.2 保留现有 `internal-messaging` change-local delta，归档时使用 `--skip-specs`，不回写主规格。
