## MODIFIED Requirements

### Requirement: 用户注册
系统 SHALL 允许用户在通过验证码和既有密码策略校验后公开注册账号。所有新设密码入口 SHALL 复用共享密码策略，在执行 bcrypt Hash 前限制密码为最多 72 个 UTF-8 字节；系统 SHALL NOT 截断密码或更换密码算法。注册 DTO、接口说明和客户端提示 SHALL 与该边界一致，SHALL NOT 将 255 个字符描述为均可接受的新密码。

#### Scenario: 注册成功
- **WHEN** 请求向 `POST /api/register` 提交唯一用户名、唯一邮箱、有效密码和有效验证码
- **THEN** 系统 SHALL 创建一个角色为 `user`、状态为 `1` 的用户
- **AND** 系统 SHALL 使用 bcrypt 存储密码
- **AND** 响应 SHALL 返回不包含密码的脱敏用户信息

#### Scenario: 用户名或邮箱已存在
- **WHEN** 请求提交的用户名或邮箱已被其他用户使用
- **THEN** 系统 SHALL 拒绝注册

#### Scenario: 注册密码超过字节上限
- **WHEN** 注册请求通过既有前置校验，而新密码超过 72 个 UTF-8 字节，包括字符数未超过旧 DTO 上限的多字节密码
- **THEN** 共享密码策略 SHALL 返回 HTTP 422 和 `IDENTITY_VALIDATION_INVALID`
- **AND** 响应 SHALL 保留既有 `password / IDENTITY_PASSWORD_INVALID` 字段错误结构
- **AND** 系统 SHALL NOT 调用 bcrypt Hash、创建用户或将该输入错误返回为 HTTP 500
- **AND** DTO SHALL NOT 以旧最大字符数校验提前将此超限业务错误改为 HTTP 400

#### Scenario: 保留注册校验分层
- **WHEN** 注册请求缺少必填字段、未达到既有最小字符长度，或不满足既有密码复杂度规则
- **THEN** 系统 SHALL 保留结构性缺字段及最小字符长度校验的既有 HTTP 400 分层
- **AND** 共享业务密码复杂度失败 SHALL 保留既有 HTTP 422 字段错误
- **AND** 系统 SHALL NOT 改变既有最小长度、复杂度、验证码或唯一性要求

### Requirement: 当前用户资料
系统 SHALL 允许已认证用户读取和修改自己的非敏感资料字段，但头像 SHALL 只能通过专用头像接口修改。本人邮箱修改的既有请求字段、密码校验和邮箱验证协议 SHALL 保留；本轮个人资料界面 SHALL 只开放昵称编辑，SHALL NOT 增加邮箱换绑功能。

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

#### Scenario: 个人资料界面提交昵称
- **WHEN** 用户在个人资料界面保存昵称
- **THEN** 客户端 SHALL 只发送昵称字段，并要求其非空、非全空白且最多 100 个 Unicode 字符
- **AND** 客户端 SHALL NOT 自动 Trim、改变大小写或提供清空昵称操作
- **AND** 用户 ID、用户名、邮箱、角色和启用状态 SHALL 保持只读
- **AND** 成功响应 SHALL 沿用既有脱敏用户信息结构，SHALL NOT 添加资料版本或暴露头像摘要及存储标识

#### Scenario: 保留本人邮箱修改协议
- **WHEN** 既有客户端通过本人资料接口提交非空 `email`
- **THEN** 系统 SHALL 保留 `current_password` 校验、邮箱唯一性检查及邮箱验证流程
- **AND** 已验证邮箱的变更 SHALL 保留待验证邮箱语义，未验证邮箱 SHALL 保留既有直接更新及验证语义
- **AND** 本轮昵称界面接入 SHALL NOT 删除邮箱字段、绕过密码校验或改变邮箱验证响应语义

#### Scenario: 纯资料修改不提升授权版本
- **WHEN** 用户仅修改自己的昵称、上传头像或恢复默认头像，且未发生其他身份或授权状态变化
- **THEN** 系统 SHALL NOT 创建或提升该用户的授权版本
- **AND** 仍有效的 Access Token 和 Refresh Token SHALL 按既有会话规则继续有效

#### Scenario: 并发资料写入
- **WHEN** 多个合法昵称或头像写请求先后实际提交到数据库
- **THEN** 系统 SHALL 以最后实际数据库写入的值为准
- **AND** 系统 SHALL NOT 为本轮自助维护引入 CAS、编辑锁或资料版本字段

### Requirement: 头像上传
系统 SHALL 允许已认证用户通过 `POST /api/user/avatar` 上传一个受支持的静态图片，并只保存完整解码和标准化后的头像对象。原文件和标准化输出的独立字节上限 SHALL 均为当前 `file_upload.avatar_max_size_mb × 1024 × 1024`；multipart 总请求体上限 SHALL 为该字节上限额外加 256 KiB，额外开销 SHALL NOT 扩大原文件或输出上限。接口说明 SHALL 完整列出静态 JPEG、PNG 和 WebP，SHALL NOT 因本次规则读取而改变既有后端图片处理行为。

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
- **WHEN** 头像 multipart 总请求体超过配置字节上限加 256 KiB，或原文件、重新编码后的输出任一超过配置字节上限
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

### Requirement: 修改密码
系统 SHALL 允许已认证用户在验证旧密码后通过 `PUT /api/user/password` 修改密码。新密码 SHALL 通过与注册共用的密码策略并最多为 72 个 UTF-8 字节；新密码与确认密码 DTO、接口说明和客户端提示 SHALL 与该边界一致，SHALL NOT 继续宣称 255 个字符均可新设。该限制 SHALL NOT 收紧登录密码或旧密码的验证范围。

#### Scenario: 修改密码成功
- **WHEN** 旧密码正确
- **AND** 新密码和确认密码一致
- **AND** 新密码与旧密码不同
- **AND** 新密码满足密码复杂度规则和 72 个 UTF-8 字节上限
- **THEN** 系统 SHALL 存储新的 bcrypt 密码哈希
- **AND** 系统 SHALL 使该用户旧 token 失效
- **AND** 系统 SHALL 沿用既有成功信封返回 HTTP 200

#### Scenario: 修改密码失败
- **WHEN** 任一密码条件不满足
- **THEN** 系统 SHALL 拒绝修改密码

#### Scenario: 新密码超过字节上限
- **WHEN** 请求满足既有结构性校验，且新密码与确认密码一致并超过 72 个 UTF-8 字节
- **THEN** 共享密码策略 SHALL 在 bcrypt Hash 前返回 HTTP 422 和 `IDENTITY_VALIDATION_INVALID`
- **AND** 响应 SHALL 包含既有 `password / IDENTITY_PASSWORD_INVALID` 字段错误，供客户端定位密码输入
- **AND** 系统 SHALL NOT 截断密码、执行密码写入或授权版本提升，也 SHALL NOT 返回 Hash 导致的 HTTP 500
- **AND** 新密码及确认密码 DTO SHALL NOT 以旧最大字符数规则将此业务错误提前转为 HTTP 400

#### Scenario: 保留改密校验与旧密码兼容
- **WHEN** 请求缺少必填密码字段、未达到既有新密码或确认密码最小字符长度、复杂度不足、确认不一致、新旧相同或旧密码不正确
- **THEN** 系统 SHALL 保留既有最小长度、复杂度、新旧不同、确认一致及旧密码验证要求
- **AND** 结构性缺字段及最小字符长度失败 SHALL 保留既有 HTTP 400 分层，业务失败 SHALL 保留既有 HTTP 422 字段错误
- **AND** 登录和旧密码验证 SHALL NOT 因新设密码上限而增加 72 字节拒绝条件

## ADDED Requirements

### Requirement: 头像上传规则读取
系统 SHALL 提供 `GET /api/user/avatar-upload-policy`，静态访问等级为 Authenticated，允许已认证用户只读获取实际上传大小规则；系统 SHALL NOT 为该端点增加 Permission Code、配置项、Redis key、数据库或存储模型。

#### Scenario: 读取当前头像文件上限
- **WHEN** 已认证用户请求头像上传规则且当前实际配置有效
- **THEN** 系统 SHALL 以 HTTP 200 返回既有 `code`、`error_code`、`msg`、`data` 四字段信封
- **AND** `data` SHALL 仅为 `{max_bytes: 正整数}`，其值来自上传处理实际使用的 `file_upload.avatar_max_size_mb × 1024 × 1024`
- **AND** 响应 SHALL 设置 `Cache-Control: no-store`
- **AND** 响应 SHALL NOT 包含 multipart 额外开销、对象键、摘要、bucket 或存储配置

#### Scenario: 未认证读取规则
- **WHEN** 未认证客户端请求头像上传规则
- **THEN** 系统 SHALL 按既有认证协议拒绝请求
- **AND** 系统 SHALL NOT 将该路由变为公开端点或要求新增业务权限码

#### Scenario: 规则配置无效或读取失败
- **WHEN** 当前实际规则无法得到正整数上限或规则读取发生服务故障
- **THEN** 系统 SHALL 返回安全的失败响应，SHALL NOT 返回伪造默认值或成功的无效规则
- **AND** 客户端 SHALL 仅禁用选图和上传并允许重试，SHALL NOT 因该局部失败阻止头像展示、恢复默认或其他编辑区
- **AND** 真实认证终态 SHALL 继续触发现有全局会话处理，不得被当作局部规则错误吞掉

### Requirement: 共享用户读取错误与提交结果分类
系统 SHALL 在共享用户读取及写后回读中仅将 `errors.Is(err, ErrUserNotFound)` 为真的既有错误判定为身份不存在；其他读取故障 SHALL 保留为内部服务故障。分类 SHALL 覆盖 UserService.Get、本人资料更新、改密及其共享 Get 的受保护 JSON 消费者，包括管理端更新后回读；Application SHALL 复用现有错误体系，SHALL NOT 引入 ORM 错误依赖。其他已有上下文／头像用例 SHALL 保留其接口语义，公开头像读取的默认图片回退 SHALL NOT 被此 JSON 错误分类替换。

#### Scenario: 已确认当前身份不存在
- **WHEN** 当前身份的用户读取返回与既有 `ErrUserNotFound` 匹配的错误，包括被包装的错误
- **THEN** 系统 SHALL 沿用该调用既有身份不存在及认证失效映射
- **AND** 管理端目标不存在等非本人身份语义 SHALL 保留既有未找到映射，SHALL NOT 被改为操作者认证失效

#### Scenario: 真实用户读取故障
- **WHEN** 用户读取或共享回读发生非 `ErrUserNotFound` 的数据库或 Repository 故障
- **THEN** HTTP 边界 SHALL 返回 HTTP 500、`IDENTITY_INTERNAL_ERROR` 和 `data: null` 的既有四字段信封
- **AND** 响应 SHALL NOT 暴露数据库、ORM、连接、查询或底层错误原因
- **AND** 系统 SHALL NOT 将该错误伪装成用户不存在或触发身份不存在所对应的登出

#### Scenario: 写入前用户读取失败
- **WHEN** 本人资料、密码或其他受影响写用例在提交前读取用户发生真实故障
- **THEN** 系统 SHALL 按内部故障拒绝本次操作，SHALL NOT 宣称目标身份不存在
- **AND** 系统 SHALL NOT 继续基于空用户执行写入

#### Scenario: 写入已提交但回读失败
- **WHEN** 资料或管理端更新已确认提交，而为了生成响应进行的用户回读发生真实故障
- **THEN** 系统 SHALL 返回安全的 HTTP 500 和 `IDENTITY_INTERNAL_ERROR`，`data` 为 null
- **AND** 系统 SHALL NOT 声称写入已经回滚、伪造成功用户信息或把回读失败当作写入未提交
- **AND** 客户端 SHALL 说明未取得最终资料并允许显式重新读取，SHALL NOT 自动重发写请求
- **AND** 管理保护、邮箱禁写及既有授权版本更新事实 SHALL 保持，不得因响应回读失败绕过保护或重复失效

#### Scenario: 写入结果未知或响应丢失
- **WHEN** 写入无法确认是否提交，或客户端未收到可确认结果的响应
- **THEN** 系统和客户端 SHALL 将结果视为未知，SHALL NOT 假报成功或回滚
- **AND** 客户端 SHALL NOT 自动重发写请求或通过试登录推断改密结果
- **AND** 已有头像补偿 SHALL 继续仅按明确提交分类执行，SHALL NOT 删除可能已被提交记录引用的新对象
