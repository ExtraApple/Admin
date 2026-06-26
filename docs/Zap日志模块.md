# Zap日志模块设计文档

> **文档定位**: 待实现设计草案。当前已确认的日志行为以 `openspec/specs/logging/spec.md` 为准；实现 Zap 日志模块前应先创建 OpenSpec change。

## 当前状态

> **状态**: 待实现 — 先作为日志基础设施设计文档

Zap 日志模块用于统一项目中的运行日志输出，负责记录程序启动、初始化、错误、告警、调试信息等内容。

它和 `操作日志` 不是同一个东西：

| 模块 | 主要用途 | 写入位置 |
|---|---|---|
| Zap日志模块 | 服务运行日志、错误日志、初始化日志、调试日志 | 控制台 / 本地日志文件 |
| 操作日志模块 | 用户做了什么操作、调用了哪个 API、审计追踪 | MySQL 审计表 |

## 模块目标

统一替换项目中零散的 `fmt.Println`、`log.Println`、`log.Fatalf`，让后端日志更适合生产环境排查问题。

最终效果：

```text
main.go
  → initialize.InitLogger()
  → global.Logger 可全局调用
  → 各模块使用 zap 输出结构化日志
```

示例：

```go
global.Logger.Info("mysql connected")
global.Logger.Error("redis connect failed", zap.Error(err))
global.Logger.Warn("login failed", zap.String("username", username))
```

## 推荐目录结构

```text
global/
  logger.go              // 全局 Logger 变量

initialize/
  logger.go              // 初始化 zap

config.yaml              // 日志配置

logs/
  app.log                // 普通运行日志
  error.log              // 错误日志，可选
```

## 配置设计

建议在 `config.yaml` 中增加：

```yaml
logger:
  level: "debug"         # debug / info / warn / error
  format: "console"      # console / json
  output: "logs/app.log"
  max_size: 100          # 单个日志文件最大 MB
  max_backups: 7         # 最多保留几个旧文件
  max_age: 30            # 最多保留多少天
  compress: true         # 是否压缩旧日志
```

说明：

| 配置 | 作用 |
|---|---|
| `level` | 控制日志最低输出级别 |
| `format` | 开发环境用 console，生产环境可用 json |
| `output` | 日志文件路径 |
| `max_size` | 单个日志文件大小上限 |
| `max_backups` | 旧日志文件保留数量 |
| `max_age` | 旧日志文件保留天数 |
| `compress` | 是否压缩归档日志 |

日志轮转建议使用：

```text
gopkg.in/natefinch/lumberjack.v2
```

## 依赖选择

```bash
go get go.uber.org/zap
go get gopkg.in/natefinch/lumberjack.v2
```

| 依赖 | 用途 |
|---|---|
| `go.uber.org/zap` | 高性能结构化日志 |
| `lumberjack` | 本地日志文件切割、保留、压缩 |

## 全局变量设计

### global/logger.go

```go
package global

import "go.uber.org/zap"

var Logger *zap.Logger
```

## 初始化设计

### initialize/logger.go

核心职责：

- 读取日志配置
- 设置日志级别
- 设置输出格式
- 设置文件轮转
- 初始化 `global.Logger`

伪代码：

```go
func InitLogger(conf LoggerConfig) {
    encoder := getEncoder(conf.Format)
    writer := getWriter(conf)
    level := getLevel(conf.Level)

    core := zapcore.NewCore(encoder, writer, level)
    global.Logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}
```

## 日志级别规范

| 级别 | 使用场景 |
|---|---|
| `Debug` | 调试细节，本地开发使用 |
| `Info` | 正常启动、初始化成功、关键流程完成 |
| `Warn` | 非致命异常，例如配置缺省、登录失败、第三方服务短暂失败 |
| `Error` | 业务或系统错误，需要排查 |
| `Fatal` | 程序无法继续运行，例如配置读取失败、数据库初始化失败 |

## 使用规范

### 初始化成功

```go
global.Logger.Info("mysql connected")
global.Logger.Info("redis connected")
global.Logger.Info("minio connected")
```

### 初始化失败

```go
global.Logger.Fatal("mysql connect failed", zap.Error(err))
```

### 业务错误

```go
global.Logger.Warn("login failed",
    zap.String("username", req.Username),
    zap.String("reason", "invalid password"),
)
```

### 系统错误

```go
global.Logger.Error("upload file failed",
    zap.Uint("user_id", userID),
    zap.String("bucket", bucket),
    zap.Error(err),
)
```

## Gin 请求日志

后续可以写一个 Gin 中间件，把每次请求输出到 Zap：

```text
middleware/zap_logger.go
```

记录字段：

| 字段 | 说明 |
|---|---|
| `method` | 请求方法 |
| `path` | 请求路径 |
| `status` | HTTP 状态码 |
| `latency` | 请求耗时 |
| `client_ip` | 客户端 IP |
| `user_agent` | User-Agent |
| `user_id` | 当前登录用户 ID，可选 |

示例日志：

```json
{
  "level": "info",
  "msg": "http request",
  "method": "POST",
  "path": "/api/login",
  "status": 200,
  "latency": "25ms",
  "client_ip": "127.0.0.1"
}
```

## 与操作日志的边界

Zap 日志不适合直接当作后台操作日志查询数据源。

原因：

- Zap 日志主要用于开发和运维排错
- 操作日志需要分页、筛选、按用户查询，应该入库
- Zap 文件日志不适合复杂查询

推荐边界：

| 场景 | 使用 |
|---|---|
| 数据库连接失败 | Zap |
| Redis 初始化失败 | Zap |
| MinIO 上传异常 | Zap |
| 管理员删除用户 | 操作日志入库 + Zap 可选输出 |
| 登录成功/失败审计 | 操作日志入库 |
| API 请求耗时统计 | Zap 请求日志 |

## 实现步骤

| 步骤 | 文件 | 内容 |
|---|---|---|
| 1 | `global/logger.go` | 创建全局 Logger |
| 2 | `initialize/logger.go` | 初始化 zap |
| 3 | `initialize/server.go` | 增加 LoggerConfig 配置结构 |
| 4 | `config.yaml` | 增加 logger 配置 |
| 5 | `main.go` | 在数据库、Redis、MinIO 初始化前调用 InitLogger |
| 6 | `middleware/zap_logger.go` | 可选：接管 Gin 请求日志 |
| 7 | 全项目 | 逐步替换 `log.Println` / `fmt.Println` |

## 推荐实现顺序

1. 先实现 `global.Logger`
2. 再实现 `initialize.InitLogger`
3. 在 `main.go` 中启动时初始化
4. 替换初始化阶段的错误日志
5. 再做 Gin 请求日志中间件
6. 最后再和操作日志模块联动

不要一开始全项目大规模替换日志，容易把业务改乱。先从初始化链路开始接入最稳。

## 注意事项

- `global.Logger` 初始化失败时可以降级为 `zap.NewNop()` 或直接 `panic`
- `Fatal` 会输出日志后退出程序，不适合普通业务错误
- 日志里不要输出密码、token、验证码等敏感信息
- 生产环境建议用 JSON 格式，方便后续接入 ELK、Loki、Grafana
- 开发环境建议用 console 格式，可读性更好
- Windows 下日志目录不存在时，需要初始化时自动创建 `logs/`

## 下一步建议

下一步可以先写最小可用版本：

```text
config.yaml
global/logger.go
initialize/logger.go
main.go 调用 InitLogger
```

确认服务启动时能输出：

```text
logger initialized
config loaded
mysql connected
redis connected
minio connected
server started
```
