## MODIFIED Requirements

### Requirement: Zap 运行日志
系统 SHALL 使用 Zap 作为运行日志核心，为服务启动、定时任务、HTTP 请求、分类错误和恢复失败提供运行日志；App/Platform SHALL 负责创建和装配 Logger，Domain 和 Application SHALL 通过显式依赖使用日志能力，不依赖全局运行时状态。

#### Scenario: 服务启动时初始化日志
- **WHEN** App 启动并读取配置成功
- **THEN** `internal/platform/logging` SHALL 创建 Zap Logger
- **AND** App SHALL 将 Logger 注入需要日志能力的模块和后台任务
- **AND** 日志级别、输出配置和现有启动日志行为 SHALL 保持兼容

#### Scenario: 初始化失败
- **WHEN** MySQL、Redis 或 MinIO 初始化失败
- **THEN** 系统写入 error 或 fatal 级别运行日志
- **AND** 失败日志 SHALL 包含错误对象

#### Scenario: Gin 请求日志
- **WHEN** HTTP 请求经过 Gin Engine
- **THEN** 系统写入请求运行日志
- **AND** 日志 SHALL 包含 method、匹配后的 path、status、latency、client_ip、user_agent
- **AND** 日志 SHALL NOT 记录请求体、Authorization header、密码、验证码或 token

#### Scenario: 记录分类服务端错误
- **WHEN** HTTP 请求返回 5xx 且 Gin Context 中存在分类错误的内部 cause
- **THEN** App HTTP 错误日志中间件 SHALL 以 error 级别记录 cause、公开 `error_code`、method、path 和 status
- **AND** 公开 JSON 响应 SHALL NOT 包含 cause

#### Scenario: 记录未分类服务端错误
- **WHEN** HTTP 请求返回 5xx 但没有可用内部 cause
- **THEN** App SHALL 记录公开错误码、method、path 和 status
- **AND** App SHALL NOT 伪造数据库、Redis、MinIO 或其他内部原因

#### Scenario: 处理预期客户端失败
- **WHEN** HTTP 请求返回预期 4xx
- **THEN** 系统 SHALL 保留普通请求运行日志
- **AND** 系统 SHALL 默认不再写一条重复 error 级别日志
- **AND** 401/403 请求 SHALL 继续由现有 API 审计链路记录，不得把内部 cause 写入审计数据

#### Scenario: 恢复未提交响应的 panic
- **WHEN** Handler 或中间件 panic 且响应尚未提交
- **THEN** App 恢复中间件 SHALL 记录 panic 和请求上下文
- **AND** 系统 SHALL 返回 HTTP 500、`HTTP_INTERNAL_ERROR` 和安全四字段错误信封

#### Scenario: 原生响应提交后失败
- **WHEN** 二进制或其他原生响应已经提交后发生 panic 或流错误
- **THEN** 系统 SHALL 记录服务端错误并终止响应
- **AND** 系统 SHALL NOT 向已提交内容追加 JSON 错误信封

#### Scenario: 文件轮转任务运行
- **WHEN** 文件轮转任务启动、跳过、移动文件或遇到错误
- **THEN** 系统通过注入的 Zap Logger 写入描述执行结果的运行日志
