# Zap Runtime Logging Design

## Boundary

Zap 运行日志用于服务运行排错，不作为后台审计数据源。

审计日志仍按 `openspec/specs/logging/spec.md` 的要求，后续通过独立 OpenSpec change 实现。

## Components

| 文件 | 作用 |
|---|---|
| `global/logger.go` | 保存全局 `*zap.Logger` |
| `initialize/logger.go` | 根据配置初始化 Zap |
| `middleware/zap_logger.go` | 记录 Gin 请求元信息 |
| `config.yaml` | 提供 logger 配置 |

## Log Outputs

默认同时输出到：

- 控制台
- `logs/app.log`

本地日志文件使用 lumberjack 做按大小切割和保留。

## Sensitive Data

请求日志只记录 method、path、status、latency、client_ip、user_agent、user_id，不记录 body、Authorization header、password、captcha、token。
