## ADDED Requirements

### Requirement: C5 维护运行日志

系统 SHALL 沿用App适配的RuntimeLogger为维护轮次、子任务和单条失败输出受控结构化日志。每轮 SHALL 生成run_id并复用现有messaging worker_id，所有该轮日志可据此关联；系统 SHALL NOT 新增任务历史表或为成功对象逐条刷日志。

固定事件名 SHALL 为 messaging_cleanup_started、messaging_cleanup_finished、messaging_cleanup_task_started、messaging_cleanup_task_finished、messaging_cleanup_item_failed、messaging_cleanup_task_skipped、messaging_cleanup_dead_lettered。失败和dead转移 SHALL 输出Warn重点日志；该日志 SHALL NOT 被描述为已经完成外部告警投递。

#### Scenario: 轮次与子任务汇总
- **WHEN** 一轮维护或其中一个子任务开始和结束
- **THEN** 系统 SHALL 输出关联run_id、worker_id及任务名的开始/结果日志
- **AND** 汇总 SHALL 包含status、processed、succeeded、skipped、failed、duration_ms和timed_out
- **AND** 新字段 SHALL 通过实际App白名单编码输出，而不是仅在调用方构造后被过滤

#### Scenario: 区分任务结果与单条结果
- **WHEN** 子任务处理完成、发生错误、超时或执行前被跳过
- **THEN** status SHALL 分别表达succeeded、failed、timed_out或skipped
- **AND** 已尝试项 SHALL 满足processed=succeeded+skipped+failed，不重复计数
- **AND** 对象不存在且数据库成功终结 SHALL 计succeeded；CAS冲突计skipped
- **AND** 执行前配置跳过 SHALL 计数全0；查询失败允许status=failed且processed=0

#### Scenario: 单条失败和人工处理信号
- **WHEN** 对象/副本处理失败、登记失败、hash冲突或队列转dead
- **THEN** 系统 SHALL 记录任务阶段、可用的File Record/队列/消息副本ID、稳定failure_code和受控重试次数
- **AND** 未创建队列时 SHALL 使用File Record ID而不伪造队列ID
- **AND** 后台运行码 SHALL NOT 加入对外HTTP错误码目录

#### Scenario: 日志脱敏与安全编码
- **WHEN** C5记录正常、失败或取消日志
- **THEN** 日志 SHALL NOT 包含bucket、object_name、主机名、IP、正文、凭据或数据库/Broker/SDK原始错误
- **AND** 完整对象定位 SHALL 仅保存在受控数据库记录中
- **AND** run_at SHALL 编码为UTC时间字符串，duration_ms SHALL 为毫秒整数

#### Scenario: 未配置 Broker 的发布跳过
- **WHEN** 自动发布因RabbitMQ启动时未配置而跳过
- **THEN** 系统 SHALL 记录单条任务级跳过日志和rabbitmq_not_configured原因
- **AND** 系统 SHALL NOT 为日志查询积压数量或逐条记录scheduled公告
