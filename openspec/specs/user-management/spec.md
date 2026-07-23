# user-management Specification

## Purpose

用户管理覆盖账号注册、登录返回的用户信息、个人资料维护、头像上传、修改密码、前端初始化上下文，以及管理员对用户的管理操作。

## Requirements

### Requirement: 用户注册
系统 SHALL 允许用户在通过验证码和密码策略校验后公开注册账号。

#### Scenario: 注册成功
- **WHEN** 请求向 `POST /api/register` 提交唯一用户名、唯一邮箱、有效密码和有效验证码
- **THEN** 系统创建一个角色为 `user`、状态为 `1` 的用户
- **AND** 系统使用 bcrypt 存储密码
- **AND** 响应返回不包含密码的脱敏用户信息

#### Scenario: 用户名或邮箱已存在
- **WHEN** 请求提交的用户名或邮箱已被其他用户使用
- **THEN** 系统拒绝注册

### Requirement: 当前用户资料
系统 SHALL 允许已认证用户读取和修改自己的非敏感资料字段，但头像 SHALL 只能通过专用头像接口修改。

#### Scenario: 查询当前用户资料
- **WHEN** 已认证用户调用 `GET /api/user/info`
- **THEN** 系统 SHALL 返回脱敏后的用户信息
- **AND** 响应 SHALL 永远不包含密码哈希
- **AND** 只有头像对象标识、归属、规范 UUID 扩展名、MIME 和验证状态均可信时，响应头像 SHALL 为 `/api/avatars/<user-id>`
- **AND** 其他情况响应头像 SHALL 为 `/api/avatars/default`

#### Scenario: 修改当前用户资料
- **WHEN** 已认证用户调用 `PUT /api/user/info` 并提交昵称或邮箱
- **THEN** 系统 SHALL 只更新允许修改的字段
- **AND** 如果邮箱已被其他用户使用，系统 SHALL 拒绝请求
- **AND** 如果没有任何可更新字段，系统 SHALL 拒绝请求

#### Scenario: 资料接口提交头像字段
- **WHEN** `PUT /api/user/info` 请求包含 `avatar` 字段
- **THEN** 系统 SHALL 拒绝请求
- **AND** 系统 SHALL NOT 把外部 URL、MinIO 路径或其他客户端值写入头像字段

### Requirement: 头像上传
系统 SHALL 允许已认证用户通过 `POST /api/user/avatar` 上传一个受支持的静态图片，并只保存完整解码和标准化后的头像对象。

#### Scenario: 无透明头像上传成功
- **WHEN** 用户上传未超过头像配置上限、尺寸不超过 8,192×8,192、总像素不超过 40,000,000 且可完整解码的静态 JPEG、PNG 或 WebP
- **AND** 解码后像素不包含透明通道
- **THEN** 系统 SHALL 按比例缩放到最大 1,024×1,024
- **AND** 系统 SHALL 移除非像素元数据并编码为质量 85 的 JPEG
- **AND** 系统 SHALL 使用服务端生成的 `.jpg` object key 写入私有 `image` bucket
- **AND** 系统 SHALL 保存对象标识和 `image/jpeg`，而不是客户端 URL

#### Scenario: 透明头像上传成功
- **WHEN** 用户上传通过验证且解码后包含透明通道的静态 PNG 或 WebP
- **THEN** 系统 SHALL 按比例缩放到最大 1,024×1,024
- **AND** 系统 SHALL 移除非像素元数据并重新编码为 PNG
- **AND** 系统 SHALL 使用服务端生成的 `.png` object key 和 `image/png`

#### Scenario: 头像请求体或输出过大
- **WHEN** 头像请求体、原文件或重新编码后的输出超过 `file_upload.avatar_max_size_mb`
- **THEN** 系统 SHALL 返回 HTTP 413
- **AND** 系统 SHALL NOT 更新用户头像记录

#### Scenario: 头像尺寸或像素超限
- **WHEN** 图片任一边超过 8,192 像素或总像素超过 40,000,000
- **THEN** 系统 SHALL 在完整像素解码前拒绝图片
- **AND** 系统 SHALL 返回 `IMAGE_DIMENSION_LIMIT`

#### Scenario: 头像格式不受支持或损坏
- **WHEN** 头像不是 JPEG、PNG、WebP，声明类型与真实类型不一致，图片不能完整解码，或图片为动画 WebP/APNG
- **THEN** 系统 SHALL 在写入 MinIO 前拒绝上传
- **AND** 系统 SHALL 返回稳定类型或解码错误

#### Scenario: 新头像对象写入失败
- **WHEN** 标准化成功但新头像无法写入 MinIO
- **THEN** 系统 SHALL 保留当前头像记录
- **AND** 系统 SHALL 返回不包含底层存储信息的服务错误

#### Scenario: 新头像写入后数据库确认未提交
- **WHEN** 新头像对象写入成功且用户记录更新明确确认未提交
- **THEN** 系统 SHALL 删除刚写入的新对象
- **AND** 系统 SHALL 保留原头像记录

#### Scenario: 新头像数据库提交结果不确定
- **WHEN** 新头像对象写入成功但用户记录更新返回错误且无法确认是否已经提交
- **THEN** 系统 SHALL 返回不包含底层数据库信息的持久化错误
- **AND** 系统 SHALL NOT 删除可能已经被用户记录引用的新对象
- **AND** 系统 SHALL NOT 尝试删除旧头像对象

#### Scenario: 新头像替换成功
- **WHEN** 新头像对象和数据库记录均更新成功
- **THEN** 系统 SHALL 返回更新后的脱敏用户信息
- **AND** 系统 SHALL 在数据库成功后尝试删除旧的系统生成头像对象
- **AND** 旧对象删除失败 SHALL NOT 回滚已经成功的新头像

### Requirement: 头像内容摘要
系统 SHALL 对用户头像标准化后实际写入 MinIO 的内容计算服务端 SHA-256，并与可信头像元数据一起持久化。

#### Scenario: 上传头像建立摘要
- **WHEN** JPEG、PNG 或 WebP 头像通过验证并完成标准化重新编码
- **THEN** 系统 SHALL 对重新编码后的完整 JPEG 或 PNG 字节计算 SHA-256
- **AND** 系统 SHALL 将 64 个小写十六进制字符的 `avatar_content_sha256` 与头像 object name、规范 MIME、验证状态和验证时间在同一次数据库更新中持久化
- **AND** 系统 SHALL NOT 保存原始上传图片的摘要作为可信头像摘要

#### Scenario: 恢复默认头像清除摘要
- **WHEN** 用户成功通过 `DELETE /api/user/avatar` 恢复系统默认头像
- **THEN** 系统 SHALL 清空 `avatar_content_sha256` 及其他可信自定义头像元数据

#### Scenario: 既有可信头像没有摘要
- **WHEN** 用户具有本策略上线前创建的可信头像且 `avatar_content_sha256` 为空
- **THEN** 系统 SHALL 保留现有可信头像行为
- **AND** 系统 SHALL NOT 在服务启动或头像读取时自动读取 MinIO 补算摘要
- **AND** 用户下次成功上传头像时 SHALL 建立新的摘要基线

#### Scenario: 头像摘要不对外暴露
- **WHEN** 系统返回用户资料、登录信息、初始化上下文或头像内容
- **THEN** 响应 SHALL NOT 包含 `avatar_content_sha256`
- **AND** 系统 SHALL NOT 将头像摘要写入 object key、头像 URL、普通运行日志或审计元数据

#### Scenario: 头像摘要用途受限
- **WHEN** 系统生成或保存头像摘要
- **THEN** 系统 SHALL NOT 信任客户端提供的摘要
- **AND** 系统 SHALL NOT 将摘要用于自动去重、唯一约束或恶意图片判定

### Requirement: 恢复默认头像
系统 SHALL 允许已认证用户通过 `DELETE /api/user/avatar` 恢复系统默认头像。

#### Scenario: 恢复默认头像成功
- **WHEN** 用户调用恢复默认头像接口
- **THEN** 系统 SHALL 清除当前可信头像对象标识并返回默认头像地址
- **AND** 系统 SHALL 在数据库更新成功后尝试删除旧的系统生成头像对象

#### Scenario: 重复恢复默认头像
- **WHEN** 用户已经没有可信头像对象并再次调用恢复默认头像接口
- **THEN** 系统 SHALL 成功返回默认头像结果
- **AND** 系统 SHALL NOT 因数据库字段没有发生变化而返回持久化失败
- **AND** 系统 SHALL NOT 尝试删除不存在或不可信的头像对象

#### Scenario: 删除旧头像对象失败
- **WHEN** 数据库已经恢复默认头像但旧对象删除失败
- **THEN** 系统 SHALL 保持默认头像结果
- **AND** 系统 SHALL 记录受控清理错误但不得向客户端暴露 object key 或 MinIO 原始错误

### Requirement: 历史头像来源过渡
系统 SHALL 把策略上线前的自定义头像视为未验证来源，并 SHALL NOT 自动信任或删除。

#### Scenario: 用户存在历史自定义头像
- **WHEN** 用户头像只存在于旧 `avatar` URL 字段且没有 V1 验证通过的头像对象标识
- **THEN** 用户信息、登录响应和初始化上下文 SHALL 返回系统默认头像
- **AND** 系统 SHALL NOT 请求、代理或内联显示该历史外部 URL

#### Scenario: 历史头像迁移
- **WHEN** 新头像字段首次上线
- **THEN** 数据迁移 SHALL 只写入历史状态
- **AND** 数据迁移 SHALL NOT 读取、解码、重新编码或删除历史头像对象

#### Scenario: 用户重新上传头像
- **WHEN** 具有历史未验证头像的用户成功上传 V1 标准化头像
- **THEN** 系统 SHALL 使用新的系统对象标识
- **AND** 系统 SHALL NOT 自动删除无法确认归属的历史对象

### Requirement: 受控头像读取
系统 SHALL 通过公开只读的应用接口提供可信头像和系统默认头像，同时保持私有 `image` bucket、object key 和历史头像来源不可见。

#### Scenario: 读取可信用户头像
- **WHEN** 客户端调用 `GET /api/avatars/:user_id` 且对应用户具有归属、规范 UUID 扩展名、MIME 和验证状态均可信的头像对象
- **THEN** 系统 SHALL 从私有 `image` bucket 流式返回该标准化 JPEG 或 PNG
- **AND** 响应 SHALL 设置规范 `Content-Type`、`X-Content-Type-Options: nosniff` 和受控公开缓存头
- **AND** 响应 SHALL NOT 暴露 object key、MinIO URL 或存储凭据

#### Scenario: 读取不可信或不可用的用户头像
- **WHEN** 用户不存在、没有可信头像、只有历史头像 URL、可信对象缺失或对象暂时无法读取
- **THEN** `GET /api/avatars/:user_id` SHALL 返回应用内置的系统默认 PNG
- **AND** 系统 SHALL NOT 请求、代理、重定向或内联历史头像 URL
- **AND** 响应差异 SHALL NOT 暴露用户是否存在或对象存储状态

#### Scenario: 读取默认头像
- **WHEN** 客户端调用 `GET /api/avatars/default`
- **THEN** 系统 SHALL 返回应用内置的系统默认 PNG
- **AND** 响应 SHALL 设置 `image/png`、`X-Content-Type-Options: nosniff` 和受控公开缓存头

#### Scenario: 未携带 JWT 读取头像
- **WHEN** 浏览器通过普通 `<img src>` 请求受控头像地址
- **THEN** 系统 SHALL 允许匿名只读访问
- **AND** 接口 SHALL NOT 返回用户资料、验证元数据或任何非图片内容

### Requirement: 修改密码
系统 SHALL 允许已认证用户在验证旧密码后修改密码。

#### Scenario: 修改密码成功
- **WHEN** 旧密码正确
- **AND** 新密码和确认密码一致
- **AND** 新密码与旧密码不同
- **AND** 新密码满足密码复杂度规则
- **THEN** 系统存储新的 bcrypt 密码哈希
- **AND** 系统使该用户旧 token 失效

#### Scenario: 修改密码失败
- **WHEN** 任一密码条件不满足
- **THEN** 系统拒绝修改密码

### Requirement: 前端初始化上下文
系统 SHALL 提供一个已认证接口，一次性返回前端初始化所需数据。

#### Scenario: 客户端请求初始化上下文
- **WHEN** 已认证用户调用 `GET /api/user/context`
- **THEN** 系统返回脱敏用户信息
- **AND** 系统返回 JWT 上下文中的角色码和权限码
- **AND** 系统返回该用户可访问的启用菜单树

### Requirement: 管理员用户列表
系统 SHALL 允许管理员分页查询用户列表。

#### Scenario: 管理员查询用户列表
- **WHEN** 管理员调用 `GET /api/admin/users`
- **THEN** 系统返回分页后的脱敏用户列表
- **AND** 响应包含总数、页码和每页数量

### Requirement: 管理员修改用户
系统 SHALL 允许管理员修改非保护用户，同时保护自己和管理员账号不被管理端修改。

#### Scenario: 管理员修改普通用户
- **WHEN** 管理员修改另一个非管理员用户的昵称、邮箱、角色或状态
- **THEN** 系统应用提交的字段
- **AND** 响应返回更新后的脱敏用户信息
- **AND** 系统使目标用户旧 token 失效

#### Scenario: 管理员尝试修改自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝修改

### Requirement: 管理员删除用户
系统 SHALL 允许管理员软删除非保护用户。

#### Scenario: 管理员删除普通用户
- **WHEN** 管理员删除另一个非管理员用户
- **THEN** 系统软删除该用户记录

#### Scenario: 管理员尝试删除自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝删除

### Requirement: 管理员切换用户状态
系统 SHALL 允许管理员切换非保护用户的启用状态。

#### Scenario: 管理员切换普通用户状态
- **WHEN** 管理员对非管理员用户调用 `PUT /api/admin/users/:id/status`
- **THEN** 系统将状态从 `1` 改为 `0`，或从 `0` 改为 `1`
- **AND** 系统使目标用户旧 token 失效

#### Scenario: 管理员尝试切换自己或其他管理员状态
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝操作

### Requirement: 管理员强制用户下线
系统 SHALL 允许管理员主动使非保护用户的旧 token 失效。

#### Scenario: 管理员强制普通用户下线
- **WHEN** 管理员对非管理员用户调用 `PUT /api/admin/users/:id/kick`
- **THEN** 系统提升目标用户的 `token_version`
- **AND** 系统不修改目标用户资料和账号状态
- **AND** 目标用户旧 access token 和 refresh token 在后续请求中失效

#### Scenario: 管理员尝试强制下线自己或其他管理员
- **WHEN** 目标用户是操作者本人，或目标用户角色为 `admin`
- **THEN** 系统拒绝操作


