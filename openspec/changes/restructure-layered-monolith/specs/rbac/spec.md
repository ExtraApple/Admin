## MODIFIED Requirements

### Requirement: 路由权限同步
系统 SHALL 能够从 Route Catalog 的已校验路由快照同步缺失的权限记录，而不是扫描已注册的 Gin 路由。

#### Scenario: 同步创建缺失权限
- **WHEN** 管理员调用 `POST /api/admin/permissions/sync`
- **THEN** 系统读取 Route Catalog Snapshot
- **AND** 系统跳过 `Public` 路由
- **AND** 系统根据受保护 Route Descriptor 的默认 Permission Code 创建缺失权限码；没有默认值时按既有 `method + path` 规则生成
- **AND** 系统只为不存在的权限码创建记录
- **AND** 系统 SHALL NOT 通过扫描 Gin Engine 发现路由

#### Scenario: 启动 Seed 同步权限
- **WHEN** App 启动执行权限 Seed
- **THEN** Seed SHALL 消费与 API Metadata 和 API Doc 相同的 Route Catalog Snapshot
- **AND** Seed SHALL 保持幂等，不重复创建已有权限码
- **AND** Route Catalog 收集和 Gin 路由注册 SHALL NOT 隐式创建权限记录
