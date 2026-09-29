## ADDED Requirements

### Requirement: 消息周期维护调度

App SHALL 在迁移成功后注册唯一 `messaging-cleanup` 后台任务，启动立即执行一轮，整轮完成后等待一小时再运行下一轮；同一进程 SHALL NOT 重叠执行。每轮 SHALL 使用统一 UTC 资格判断时间，在15分钟父超时内先运行既有受众快照清理和终态 Consumer 死信清理，再顺序运行图片清理、公告发布、公告过期。既有两项各最多30秒，C5三项各最多5分钟，均受父取消约束，不保证后项获得完整时间预算。

系统 SHALL 保留既有快照到期及终态死信30天清理规则和各500条上限；子项失败不阻止后项，父取消后 SHALL 停止启动新子项。迁移本身 SHALL NOT 执行存储清理。

#### Scenario: 启动处理到期存量
- **WHEN** 迁移成功且应用后台任务开始运行
- **THEN** 系统 SHALL 立即执行一轮维护并允许处理历史到期数据
- **AND** 系统 SHALL NOT 以启动时间水位排除历史数据
- **AND** 一轮结束后 SHALL 等待一小时，不承诺整点执行

#### Scenario: 子任务失败隔离
- **WHEN** 图片清理失败或其子 context 超时但父 context 仍有效
- **THEN** 系统 SHALL 记录图片任务结果并继续公告任务
- **AND** 已提交的数据库变更 SHALL 保留

#### Scenario: 整轮取消
- **WHEN** 父 context 到期或应用关闭
- **THEN** 系统 SHALL 取消当前工作并将尚未启动的任务记录为 skipped
- **AND** 系统 SHALL NOT 启动重叠轮次或无取消的后台补偿

#### Scenario: 原有清理继续执行
- **WHEN** C5 接入维护任务，包括启动时未配置 RabbitMQ 的部署
- **THEN** 系统 SHALL 保留并执行既有两项数据库保留期清理
- **AND** 系统 SHALL NOT 将 pending Outbox 或 pending Consumer 死信当作终态历史数据删除

### Requirement: 消息维护批量配置与推进

系统 SHALL 在配置加载阶段读取 `messaging.cleanup_batch_size`，缺省或0取100，1–1000有效，负数或超界启动失败。每次查询及每类C5任务单轮尝试数量 SHALL 不超过该值；图片新登记与到期重试 SHALL 共享图片额度，发布与过期额度相互独立。冲突和失败同样消耗额度，不以成功数量决定是否继续扫描。

候选 SHALL 在数据库内过滤资格、按ID升序有界查询，不改变管理列表排序、分页或total。系统 SHALL 为各候选类别跨轮保存进程内游标与固定扫描上界，已尝试的成功/失败/冲突均推进游标，到扫描尾部再回绕。重启 SHALL 允许重置游标，不承诺跨任意重启的持久公平性。

#### Scenario: 批量默认与非法配置
- **WHEN** 配置缺失或为0
- **THEN** 每类C5任务 SHALL 使用100条额度
- **AND** 负数或大于1000 SHALL 在Load时失败，不以默认值掩盖非法输入

#### Scenario: 有界且独立的任务额度
- **WHEN** 三类任务均有超过N条到期记录且cleanup_batch_size为N
- **THEN** 每类任务 SHALL 最多尝试N个对象或组织副本
- **AND** 图片失败 SHALL NOT 挤占公告发布或公告过期额度
- **AND** 图片首次登记与重试总尝试 SHALL 不超过N

#### Scenario: 坏记录不持续占满前页
- **WHEN** 低ID候选持续失败且进程持续运行
- **THEN** 系统 SHALL 在后续轮次从已尝试ID之后继续至固定扫描上界，再回绕
- **AND** 后续合格记录 SHALL 获得尝试机会，不因每轮从头只取失败前页而永久饥饿

#### Scenario: 新登记与重试均获机会
- **WHEN** 图片新候选与到期重试同时积压
- **THEN** 系统 SHALL 在同一N额度内给两者处理机会；N为1时跨轮交替优先
- **AND** 一侧为空时另一侧 SHALL 能使用剩余额度
- **AND** 同一图片首次尝试后 SHALL NOT 当轮再次作为重试处理

### Requirement: 到期公告副本原子转换

系统 SHALL 按组织副本独立事务推进公告，保持状态、aggregate_version和既有Outbox事件的原子性。发布只选择scheduled且publish_at<=now、expires_at为空或>now的公告；过期选择expires_at<=now的published公告，以及publish_at<=now且已到期的scheduled公告。已错过有效期的scheduled SHALL 直接变为expired，仅产生MessageExpired，不补MessagePublished。

每个成功副本 SHALL 递增一次聚合版本并生成对应最小事件；系统 SHALL NOT 聚合多个组织副本回滚、合并跨组织事件或只写状态不写事件。CAS冲突 SHALL 作为本次未提交跳过，其他单条错误记录并继续，不建立公告失败表。

#### Scenario: 到期公告正常发布
- **WHEN** scheduled公告到达publish_at且尚未达到expires_at
- **THEN** 系统 SHALL 在单副本事务中写入published、新聚合版本和MessagePublished Outbox
- **AND** 系统 SHALL 沿用既有投递链路，不等待Broker确认

#### Scenario: 停机后已错过有效期
- **WHEN** scheduled公告的publish_at和expires_at均不晚于本轮now
- **THEN** 过期任务 SHALL 将该副本直接转为expired并写MessageExpired
- **AND** 发布任务 SHALL NOT 为其产生MessagePublished

#### Scenario: 已发布公告到期
- **WHEN** published公告的expires_at<=now
- **THEN** 过期任务 SHALL 在同一副本事务中写expired状态、版本及MessageExpired

#### Scenario: 并发修改或事件插入失败
- **WHEN** 当前副本CAS冲突或Outbox插入失败
- **THEN** 本次尝试 SHALL NOT 留下无对应事件的新状态
- **AND** CAS冲突 SHALL NOT 被统计为成功发布
- **AND** 其他组织副本 SHALL 继续处理，已成功副本不回滚

### Requirement: 自动公告发布的 Broker 配置边界

自动发布 SHALL 仅受启动时RabbitMQ是否配置的门控，不受瞬时连接健康门控。未配置时 SHALL 跳过自动发布并记录rabbitmq_not_configured，不额外查询待发布数量；图片和公告过期 SHALL 仍运行。配置变更 SHALL 经重启生效，不热创建publisher。

已配置但连接失败时，C5 SHALL 继续提交公告与Outbox，沿用既有有限重试和dead规则；Outbox dead SHALL NOT 回滚公告或自动重新发布。系统 SHALL 复用现有受保护Outbox重放入口和原事件身份，不新增恢复控制面。

#### Scenario: 启动未配置 RabbitMQ
- **WHEN** 应用启动未配置RabbitMQ
- **THEN** 自动发布 SHALL 每轮跳过并记录一次配置原因，计数为0
- **AND** 图片清理与公告过期 SHALL 继续运行，过期事件允许保留在pending Outbox

#### Scenario: 已配置但临时断连
- **WHEN** RabbitMQ已配置但连接不可用
- **THEN** 公告状态转换 SHALL 不等待连接恢复
- **AND** 事件 SHALL 交给既有Outbox有限重试，dead不会因网络恢复自动重放

#### Scenario: 人工重放原 Outbox
- **WHEN** 事件已dead且具备既有超级管理员范围和重放权限的操作者请求恢复
- **THEN** 系统 SHALL 通过既有 `/api/admin/message-outboxes/:id/replay` 重置原Outbox
- **AND** C5 SHALL NOT 回滚公告、创建第二个发布事件或新增API
