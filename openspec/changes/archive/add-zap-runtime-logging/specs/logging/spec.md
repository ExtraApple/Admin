## ADDED Requirements

### Requirement: Zap 运行日志
系统 SHALL 使用 Zap 作为运行日志核心。

#### Scenario: 服务启动时初始化日志
- **WHEN** 应用启动并读取配置成功
- **THEN** 系统初始化全局 Zap logger
- **AND** 后续初始化流程可以通过全局 logger 写入运行日志

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
- **THEN** 系统通过 Zap 写入描述执行结果的运行日志
