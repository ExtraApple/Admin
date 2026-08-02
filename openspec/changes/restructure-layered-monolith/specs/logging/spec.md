## MODIFIED Requirements

### Requirement: Zap 运行日志
系统 SHALL 使用 Zap 作为运行日志核心，为服务启动、定时任务、HTTP 请求和运行错误提供运行日志；App/Platform SHALL 负责创建和装配 Logger，Domain 和 Application SHALL 通过显式依赖使用日志能力，不依赖全局运行时状态。

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
- **WHEN** HTTP 请求经过 Gin 路由
- **THEN** 系统写入请求运行日志
- **AND** 日志 SHALL 包含 method、path、status、latency、client_ip、user_agent
- **AND** 日志 SHALL NOT 记录请求体、Authorization header、密码、验证码或 token

#### Scenario: 文件轮转任务运行
- **WHEN** 文件轮转任务启动、跳过、移动文件或遇到错误
- **THEN** 系统通过注入的 Zap Logger 写入描述执行结果的运行日志
