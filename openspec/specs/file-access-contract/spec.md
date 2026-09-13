# file-access-contract Specification

## Purpose
TBD - created by archiving change remove-unwired-ports. Update Purpose after archive.
## Requirements
### Requirement: 文件访问授权来源

**普通文件记录**（`purpose = managed_file`）的访问授权 SHALL 完全由路由级权限码与认证中间件决定，
SHALL NOT 存在对象级所有权校验或数据范围过滤。

具体地：

- 普通文件的上传、列表、详情、更新、删除、浏览、下载、预览与重新验证
  SHALL 只要求调用方通过对应路由的认证与权限码校验。
- 系统 SHALL NOT 依据 `uploader_id` 限制普通文件记录的可见范围。
- 系统 SHALL NOT 依据调用方所属组织或角色数据范围过滤普通文件记录。
- 普通文件记录上的 `uploader_id` SHALL 只作为审计与归属元数据保存与返回，
  SHALL NOT 作为这些操作的访问判据。

**范围排除**：消息图片的临时 File Record（`purpose = message_image`）
在绑定到消息时 SHALL 按上传者归属校验（只有上传者本人可绑定自己创建的临时记录），
该约束属消息能力的最小 Contract，不在本要求的范围内；
其可见性仍由消息当前可见性决定，不由 `uploader_id` 决定。

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

