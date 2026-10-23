## MODIFIED Requirements

### Requirement: API 路由同步
系统 SHALL 能够从 Route Catalog 的已校验路由快照同步 API 元数据。

#### Scenario: 同步已声明 API 路由
- **WHEN** 管理员调用 `POST /api/admin/apis/sync`
- **THEN** 系统读取 Route Catalog 中已校验的 Route Descriptor
- **AND** 只处理 `/api/` 前缀下的路由
- **AND** 对已存在的 `method + path` 跳过创建
- **AND** 为需要权限控制的接口生成默认权限码
- **AND** 对 Public 路由以及当前兼容规则指定的公开字典接口标记为不需要权限码检查
- **AND** 对从 Public Route Descriptor 新建的 API Metadata SHALL 持久化 `need_auth = 0`
- **AND** 对从 Authenticated 或 PermissionControlled Route Descriptor 新建的 API Metadata SHALL 持久化 `need_auth = 1`
- **AND** Public 新记录 SHALL NOT 生成 Permission Code
- **AND** 对已存在但错误标记为需要认证的 `POST /api/refresh` 纠正 `need_auth` 并清空 `permission_code`
- **AND** 同步流程 SHALL NOT 扫描 Gin Engine 发现路由

#### Scenario: Public 新 Metadata 写入失败
- **WHEN** 系统创建 Public Route Descriptor 对应的 API Metadata
- **AND** 显式持久化 `need_auth = 0` 失败
- **THEN** 系统 SHALL 回滚该条 API Metadata 的创建
- **AND** 同步 SHALL 返回既有安全同步错误
- **AND** 系统 SHALL NOT 留下 `need_auth = 1` 的半成品 Public Metadata

#### Scenario: 同步公开 Refresh 元数据
- **WHEN** Route Catalog 中存在 `POST /api/refresh`
- **AND** API Metadata 中已有该 Method 和 Path 但 `need_auth = 1` 或存在 `permission_code`
- **THEN** 同步 SHALL 将其恢复为公开兼容配置
- **AND** 同步 SHALL 设置 `need_auth = 0` 并清空 `permission_code`
- **AND** 系统 SHALL NOT 因该数据库字段配置把 Refresh 路由注册为 PermissionControlled

#### Scenario: 同步已存在 API 元数据
- **WHEN** Route Catalog 的 Method 和 Path 已存在对应 API 元数据
- **THEN** 系统 SHALL 按既有兼容规则保留管理员维护的展示和运行时策略字段
- **AND** 系统 SHALL NOT 因 Catalog 默认值覆盖现有管理员配置

#### Scenario: 同步软删除 API 元数据
- **WHEN** Route Catalog 的 Method 和 Path 只存在软删除 API 元数据
- **THEN** 系统 SHALL 按现有同步规则恢复该记录
- **AND** 系统 SHALL 保持 `method + path` 唯一
- **AND** 系统 SHALL 按既有 Metadata 兼容规则保留该记录的运行时策略字段
