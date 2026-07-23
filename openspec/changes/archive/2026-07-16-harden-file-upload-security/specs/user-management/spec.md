## MODIFIED Requirements

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

#### Scenario: 头像请求体、原文件或输出过大
- **WHEN** 整个 multipart 请求体超过 `file_upload.avatar_max_size_mb + 256 KiB`，或原文件 part、重新编码后的输出超过 `file_upload.avatar_max_size_mb`
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

## ADDED Requirements

### Requirement: 恢复默认头像
系统 SHALL 允许已认证用户通过 `DELETE /api/user/avatar` 恢复系统默认头像。

#### Scenario: 恢复默认头像成功
- **WHEN** 用户调用恢复默认头像接口
- **THEN** 系统 SHALL 清除当前可信头像对象标识并返回默认头像地址
- **AND** 系统 SHALL 在数据库更新成功后尝试删除旧的系统生成头像对象

#### Scenario: 恢复默认头像时数据库更新失败
- **WHEN** 用户具有可信头像且清除数据库中的可信头像标识失败
- **THEN** 系统 SHALL 返回不包含底层数据库信息的持久化错误
- **AND** 系统 SHALL 保留原可信头像记录
- **AND** 系统 SHALL NOT 删除原头像对象

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
