## ADDED Requirements

### Requirement: 文件访问授权来源

文件模块的访问授权 SHALL 完全由路由级权限码与认证中间件决定，
SHALL NOT 存在对象级所有权校验或数据范围过滤。

具体地：

- 文件的上传、列表、详情、更新、删除、浏览、下载、预览与重新验证
  SHALL 只要求调用方通过对应路由的认证与权限码校验。
- 系统 SHALL NOT 依据 `uploader_id` 限制文件记录的可见范围。
- 系统 SHALL NOT 依据调用方所属组织或角色数据范围过滤文件记录。
- `uploader_id` SHALL 只作为审计与归属元数据保存，SHALL NOT 作为访问判据。

#### Scenario: 管理员凭权限码读取任意文件记录

- **WHEN** 调用方通过认证并持有 `admin.files.get` 与 `admin.files.id.get`
- **THEN** 系统 SHALL 返回任意文件记录的详情
- **AND** 系统 SHALL NOT 因调用方不是该文件的上传者而拒绝

#### Scenario: 管理员凭权限码下载任意文件

- **WHEN** 调用方通过认证并持有 `admin.files.id.download.get`
- **THEN** 系统 SHALL 为该文件签发下载链接并允许读取内容
- **AND** 系统 SHALL NOT 依据 `uploader_id` 或组织归属拒绝

#### Scenario: 缺少权限码的调用方被拒绝

- **WHEN** 调用方通过认证但不持有该文件路由的权限码
- **THEN** 系统 SHALL 由权限中间件拒绝请求
- **AND** 系统 SHALL NOT 进入文件 Handler

#### Scenario: 上传者归属不构成访问判据

- **WHEN** 调用方 A 上传文件，调用方 B 持有对应权限码并请求该文件
- **THEN** 系统 SHALL 允许调用方 B 的请求
- **AND** 响应 SHALL NOT 因归属不同而改变

#### Scenario: 文件记录不参与组织数据范围过滤

- **WHEN** 调用方持有限定数据范围的角色，且持有文件路由权限码
- **THEN** 文件列表与详情 SHALL 保持与全量范围一致的结果
- **AND** 系统 SHALL NOT 按组织或数据范围裁剪文件记录
