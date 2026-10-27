# api-management Specification Delta

## MODIFIED Requirements

### Requirement: API 元数据 CRUD

系统 SHALL 允许管理员查询和修改 API 元数据，并 SHALL 在返回列表、详情和同步预览时提供非持久化 Route Catalog 匹配状态。

- API Metadata 不创建、替换或注册 Gin 路由。
- `method + path` 继续保持唯一并按现有规则规范化。
- 展示名称、分组、权限码、备注、排序、启用状态、认证标记和审计标记继续遵循现有 CRUD 契约；权限码变化仍由现有 Navigation Coordinator 原子联动。
- `route_state` SHALL 由当前 Route Catalog Snapshot 计算，不写入数据库；允许值为 `matched`、`metadata_only`、`catalog_only` 和 `access_mismatch`。`catalog_only` 只出现在同步预览或同步结果中。
- Route Catalog 的静态 Access Level 仍是最低认证要求，Metadata 不能把 Authenticated 或 PermissionControlled 路由降级为匿名。
- API 权限、状态、删除、同步、权限同步和按钮生成等高影响写入 SHALL 遵循 `menu-api-impact-preview` 的 fingerprint 约束；普通展示字段更新可以保持现有直接写入语义。

#### Scenario: 查询匹配 API

- **WHEN** API Metadata 的规范化 Method/Path 与当前 Route Catalog 路由一致，且静态访问策略兼容
- **THEN** 列表和详情返回 `route_state=matched`
- **AND** 不修改 API Metadata 存储

#### Scenario: 查询漂移 API

- **WHEN** API Metadata 存在但 Route Catalog 没有对应 Method/Path
- **THEN** 列表和详情返回 `route_state=metadata_only`
- **AND** 系统不因该记录注册新 Gin 路由

#### Scenario: 静态访问等级不兼容

- **WHEN** Metadata 的策略字段与 Route Catalog 静态 Access Level 组合不兼容
- **THEN** 返回 `route_state=access_mismatch`
- **AND** 运行时继续执行静态最低认证要求
- **AND** 系统不把数据库配置解释为匿名路由授权

#### Scenario: 删除 API Metadata

- **WHEN** 管理员删除没有菜单绑定的 API Metadata，并提交当前 fingerprint
- **THEN** 系统硬删除该记录并清理不再引用的派生权限
- **AND** 不删除或改变 Route Catalog/Gin 路由

- **WHEN** API 仍有菜单绑定或 fingerprint 已过期
- **THEN** 系统返回 API Metadata 冲突
- **AND** API、菜单、权限、角色授权和用户版本保持不变

### Requirement: API 路由同步

系统 SHALL 从当前 Route Catalog Snapshot 同步 API Metadata，并 SHALL 支持先预览后提交。

- 同步 SHALL NOT 扫描 Gin Engine。
- 已存在的 Method/Path SHALL 保留管理员维护的展示和运行时策略字段，除既有兼容修复外不被默认值覆盖。
- Catalog 中存在而 Metadata 缺失的路由在预览中标记为 `catalog_only`；Metadata 存在而 Catalog 缺失的记录标记为 `metadata_only`。
- 正式同步 SHALL 使用同步预览返回的 fingerprint；事实变化时返回 `API_META_CONFLICT`，不部分提交。
- 同步的全部 API Metadata、权限码、菜单传播、角色授权和适用用户版本仍按既有最外层事务提交或回滚。

#### Scenario: 预览 API 路由同步

- **WHEN** 管理员请求 API 路由同步预览
- **THEN** 系统返回待创建、跳过、恢复和漂移项摘要及 fingerprint
- **AND** 不创建 API Metadata、权限或授权版本

#### Scenario: 提交 API 路由同步

- **WHEN** 管理员提交未过期的同步 fingerprint
- **THEN** 系统创建或恢复应同步的 Metadata
- **AND** 保留已有管理员策略字段
- **AND** 返回最终提交的新增记录和数量

#### Scenario: 提交陈旧同步 fingerprint

- **WHEN** Route Catalog 或 API Metadata 在预览后发生变化
- **THEN** 系统返回 `API_META_CONFLICT`
- **AND** 不返回部分成功统计
- **AND** 不留下前项新增记录或版本提升

### Requirement: API 生成按钮菜单

系统 SHALL 支持从启用且需要认证的 API Metadata 生成按钮菜单，并保持菜单/API/权限事务一致。

- 父级菜单必须存在、未删除，且类型为目录或页面。
- 生成的菜单必须为启用的按钮类型、无 path/component，并与 API 建立 menu/API 关系。
- 公开、停用或不存在的 API SHALL 被拒绝。
- 生成操作 SHALL 遵循 fingerprint 约束；失败时回滚按钮、关系、权限和版本事实。

#### Scenario: 从 API 生成按钮

- **WHEN** 管理员为启用且需要认证的 API 提交目录或页面父级、按钮名称、排序及当前 fingerprint
- **THEN** 系统在同一事务中创建按钮菜单、绑定 API、确保权限记录并返回按钮事实
- **AND** 按现有规则处理受影响用户授权版本

#### Scenario: 父级不是目录或页面

- **WHEN** 管理员选择按钮作为生成按钮的父级
- **THEN** 系统返回 API Metadata 验证或冲突错误
- **AND** 不创建菜单、不写入 menu/API、不修改权限或版本

#### Scenario: 公开或停用 API 生成按钮

- **WHEN** API 的 `need_auth=0` 或 `status!=1`
- **THEN** 系统拒绝生成按钮
- **AND** 不留下孤立按钮或部分权限事实
