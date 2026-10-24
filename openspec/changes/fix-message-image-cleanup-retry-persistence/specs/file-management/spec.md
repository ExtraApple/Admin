## MODIFIED Requirements

### Requirement: 消息图片持久化清理队列

Files SHALL 拥有 `message_image_cleanup_jobs` 队列，包含稳定ID、唯一file_id、冻结的完整bucket/object_name、唯一SHA-256路径键、pending/dead状态、retry_count、next_retry_at、last_error_code及创建更新时间。清理 Job 的时间值 SHALL 以 UTC 毫秒精度持久化并用于条件更新；`retry_count` SHALL 由数据库 CHECK 约束限制在0–24；路径键 SHALL 按bucket、NUL分隔符和object_name原始字节生成，不使用文件内容摘要；原始错误 SHALL NOT 持久化。

每个对象 SHALL 逐条在短数据库事务中锁定并重查用途、未绑定和到期资格，提交队列后才调用存储。系统 SHALL NOT 在事务重试回调内执行存储操作，不增加File Record清理状态字段、processing状态或租约。

#### Scenario: 登记失败不丢失对象定位
- **WHEN** 队列写入或事务提交失败
- **THEN** File Record SHALL 保留且本次 SHALL NOT 执行对象删除
- **AND** 系统 SHALL 记录受控失败并继续其他候选

#### Scenario: 同对象重复登记
- **WHEN** 多实例登记同一路径和File Record
- **THEN** 数据库 SHALL 仅保留一个队列记录
- **AND** 已有pending或dead的重试次数与状态 SHALL NOT 被重新初始化

#### Scenario: 存储失败状态持久化
- **WHEN** 初次对象删除失败
- **THEN** 队列 SHALL 保持 `pending`，`retry_count` SHALL 保持0，`next_retry_at` SHALL 保留
- **AND** `last_error_code` SHALL 持久化稳定失败分类
- **AND** 条件更新 SHALL 使用与数据库相同的 UTC 毫秒时间精度

#### Scenario: 存储成功而数据库终结失败
- **WHEN** 对象已删除但File Record和队列的终结事务失败
- **THEN** 队列 SHALL 保留以便后续幂等恢复
- **AND** 系统 SHALL NOT 把该次数据库失败统计为完整清理成功

### Requirement: 消息图片清理有限重试

初次删除 SHALL 不计入24次重试；后续pending到期后，系统 SHALL 以条件更新预留一次重试并把next_retry_at设为当前时间后一小时，成功预留后才执行存储。预留和失败更新 SHALL 使用 UTC 毫秒精度的 `next_retry_at` 及 Job ID、状态、retry_count 条件，避免纳秒时间截断导致失败结果丢失。预留后崩溃 SHALL 消耗一次不确定调度尝试，不额外无限补次数。

第24次重试失败 SHALL 转dead；第24次预留后崩溃的pending SHALL 在下次资格时间转dead而不执行第25次存储尝试。成功或确定对象不存在后，系统 SHALL 条件物理删除File Record与队列。dead SHALL 保留人工处理，不再自动重试或重新登记。

#### Scenario: 多实例竞争同次重试
- **WHEN** 多实例读取相同pending记录和重试计数
- **THEN** 仅条件预留成功者 SHALL 执行本次存储尝试
- **AND** 其他实例 SHALL 跳过且不能重复递增计数

#### Scenario: 达到重试终态
- **WHEN** 第24次重试失败或第24次预留崩溃后到达下次资格时间
- **THEN** 系统 SHALL 标记dead、清空下一重试时间并输出Warn重点事件
- **AND** 系统 SHALL NOT 执行第25次自动存储尝试

#### Scenario: 成功与迟到失败交错
- **WHEN** 成功实例已条件终结队列，另一个实例随后提交失败结果
- **THEN** 迟到失败 SHALL NOT UPSERT重建队列、复活File Record或覆盖较新尝试状态
