# logging Specification

## Purpose

日志模块定义应用运行日志和计划中的审计日志边界。运行日志用于帮助定位服务行为，审计日志用于记录安全相关的业务操作。

## Requirements

### Requirement: 运行日志
系统 SHALL 为服务启动、定时任务和运行错误提供运行日志。

#### Scenario: 文件轮转任务运行
- **WHEN** 文件轮转任务启动、跳过、移动文件或遇到错误
- **THEN** 系统写入描述执行结果的运行日志

### Requirement: Zap 日志模块文档边界
项目 SHALL 在实现被确认前将 Zap 日志模块的设计说明保留在普通文档中，而不是把它当作已实现行为写入规格。

#### Scenario: Zap 模块仅存在设计文档
- **WHEN** Zap 日志仅作为实现说明或设计文档存在
- **THEN** 相关细节保留在 `docs/`
- **AND** OpenSpec 只记录已经确认的运行日志行为

### Requirement: 审计日志待办边界
系统 SHALL 将 API 调用日志、登录日志、操作日志、权限变更日志和数据访问日志视为待实现能力，直到通过 OpenSpec change 完成设计和实现。

#### Scenario: 开始实现审计日志
- **WHEN** 开始处理审计日志需求
- **THEN** 必须先在 `openspec/changes/` 下创建 change
- **AND** change 中必须定义记录动作、字段、保留策略、查询接口、权限和敏感数据处理方式


