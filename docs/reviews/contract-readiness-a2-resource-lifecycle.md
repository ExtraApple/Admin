# A2 Resource Lifecycle Contract Readiness

## 结论

本报告记录当前基线下的资源与生命周期审计结果，不是产品修复报告。

- 审计使用真实隔离 App、HTTP/JWT、MySQL、Redis、MinIO 和 RabbitMQ 组合。
- 正常消息可见性、文件补偿、对象位置回滚、消息图片 NoSuchKey 终结、重试竞争和 RuntimeLogger 白名单路径获得了可复核证据。
- 发现明确实现偏差：超级管理员 All-scope 消息管理、消息图片清理失败状态落库、消息图片清理后的图片读取错误、审计日志正文/签名查询脱敏。
- 本 change 未修改产品代码、主规格、永久测试、路由、权限码、模型、Redis key、对象存储配置或第三方依赖。
- 所有原始证据均在写入机器 JSON 前去除凭据、JWT、Authorization header、DSN、对象路径、原始基础设施错误、消息正文和图片内容。

机器证据：[`docs/reviews/evidence/a2-resource-lifecycle.json`](evidence/a2-resource-lifecycle.json)

## 基线与适用前提

- Commit：`6d07b452516f0ab0bcb25e4f2eaf09cc00b7ac3a`
- Go：`go1.25.0 windows/amd64`
- 规格判据：`file-management`、`internal-messaging`、`logging`
- 运维判据：messaging、file-management、runtime-logging、audit-logging、configuration runbook
- App 使用专属本地端口和隔离容器；共享开发库与生产数据未连接。
- 测试专用 MySQL trigger、隔离 MinIO 停止、不可达对象端点和受控时间/关系更新均在证据中标注为注入，不冒充真实物理故障。

## 契约一致

### 消息当前可见性

通过真实 HTTP/JWT 创建并读取：

- 私信、群发、已发布公告的可见用户能够读取列表、详情和对应消息图片。
- 删除私信关系后，原收件人的列表、详情和图片读取均拒绝。
- 公告过期后，列表、详情和图片读取均拒绝。
- 动态受众移除后，受众用户列表为空，详情和图片读取均拒绝。
- 撤销群发详情返回受控 `revoked` 状态且正文为空；图片读取拒绝。这与当前消息规格的 revoked/expired 受控详情语义一致。
- 组织外用户的列表为空，代表性详情和图片读取返回 `MSG_NOT_FOUND`。

### 文件、对象和清理

- 普通文件记录写入失败时，HTTP 返回 `PERSISTENCE_FAILED`；数据库记录未创建，上传对象被补偿删除。
- 普通文件删除遇到 MinIO 不可用时，返回 `STORAGE_UNAVAILABLE`；File Record 和对象均保留。
- 轮转对象移动失败时数据库位置不变；数据库位置更新失败时对象移动回原位置，原位置下载成功。
- 消息图片对象不存在时按 NoSuchKey 处理，清理成功终结 File Record 和队列。
- 对象已删除而数据库终结失败时，队列和 File Record 保留；迟到重试随后成功，未复活记录。
- MySQL 并发注册/重试竞争只产生一个条件预留赢家；第 24 次、`dead` 不重入和迟到失败边界的定向测试通过。
- MinIO 真实集成覆盖缺失位置、权限拒绝、NoSuchKey、重试和超时恢复；受信位置之外的对象未被清理。

### 运行日志

真实 App 最终 sink 中观察到：

- Outbox 发布失败、第五次重试和 dead 转移。
- RabbitMQ Consumer、DLQ Recorder 通道不可用及受控 deferred 事件。
- 消息周期维护、单条清理失败和稳定错误码。
- Messaging 日志字段均落在现行白名单内；本次 sink 摘要未发现凭据、Token、消息正文、对象路径或原始基础设施错误。

## 实现偏差

### A2-MSG-004：超级管理员 All-scope 消息管理返回内部错误

超级管理员真实 JWT 访问消息分类查询、群发创建和公告创建均返回 `500 / MSG_INTERNAL_ERROR`。当前 `messagingOrganizationReader` 没有提供 All-scope 所需的 `AllOrganizationIDs` 能力，导致超级管理员分支无法完成组织集合解析。

为完成组织内的真实审计，夹具创建改用具有明确组织范围的受限管理员；超级管理员偏差保留为独立后续修复归属。本 change 未修复。

### A2-FILE-006：对象删除失败未完整写入清理失败状态

真实隔离 MinIO 停止后，清理 Runner 返回 `message_image_cleanup_storage_unavailable`，File Record 和对象保留，但队列观察值仍为 `pending`、`retry_count=0`，`last_error_code` 为空。当前结果不足以满足失败原因/重试状态持久化契约。

本 change 只保留证据，不修改清理实现。

### A2-FILE-011：已登记清理图片读取返回内部错误

已登记 `message_image_cleanup_jobs` 的图片通过真实消息图片读取入口返回 `500 / MSG_INTERNAL_ERROR`，而不是稳定的资源拒绝错误。数据库绑定未创建，图片未被重新绑定；问题在 Files 错误向 Messaging HTTP 错误分类传播的边界。

本 change 只保留证据，不修改错误映射。

### A2-LOG-002：审计 Body 与下载签名查询未完全脱敏

真实审计查询发现：

- 私信、群发、公告请求审计记录的 Body 仍包含请求正文的夹具标记，说明消息标题/Markdown 未被完全移除。
- 文件下载审计 Query 仍包含 `expires` 与 `signature` 查询键。
- RuntimeLogger 白名单检查未发现同类字段；偏差位于 HTTP Audit Log 数据路径。

本 change 只保留字段级证据，不修改审计中间件。

## 语义待裁决

### 审计查询的数据范围

真实查询入口验证结果：

- 未授权用户访问审计查询返回 `403 / API_META_PERMISSION_DENIED`。
- 固定路径分页返回稳定 `total` 和不重叠的 page 结果。
- 受限管理员与超级管理员对固定路径看到相同数量的审计记录。
- 当前 Audit Log 模型含 `user_id`，但没有组织维度；因此无法从当前查询契约证明组织范围过滤。该点标为语义待裁决，不升级为实现偏差。

### 实时分页写入

不带固定路径的连续分页查询期间，审计查询自身会产生新的审计记录，导致 total 变化和页边界移动。固定路径查询可以复核分页；实时全量分页不提供快照语义。

## 证据不足与未覆盖边界

以下路径没有被写成通过：

- 完整 RabbitMQ Consumer 业务消息注入、Consumer DLQ 投影落库、受保护重放/丢弃；本轮只执行了 RabbitMQ 通道不可用、Consumer deferred 和 Outbox dead。
- 历史文件批量重新验证、头像生命周期、全消息功能、生产部署、随机压力和跨实例容量。
- A3 装配/响应契约范围。

## 实际验证

- `go test ./internal/files/application -run` 定向清理、受信位置、NoSuchKey 和故障恢复测试。
- `go test ./internal/files/adapters/gorm -run` 定向绑定、清理注册、重试、终态和迟到结果测试。
- `go test -tags=mysql_integration ./internal/files/adapters/gorm -run TestMySQLCleanupConcurrentRegistrationAndRetry -count=1`：通过。
- `go test -tags=minio_integration ./internal/files/application -run TestMinIOMessageImageCleanupRecovery -count=1`：通过。
- 真实 HTTP/JWT 入口：消息创建、列表、详情、图片读取、文件上传/下载/删除、审计查询。
- 真实后台 sink：Outbox、Consumer、DLQ Recorder 通道不可用、维护清理和失败日志。

## 停止边界

发现明确产品偏差后，本 change 没有修改产品代码、规格或测试。后续修复或语义裁决必须建立独立 change；本报告与 JSON 作为去敏输入保留。

## 清理状态

已完成清理：App 进程已停止，A2 MySQL/Redis/MinIO/RabbitMQ 临时容器已删除，隔离 schema、Redis 数据和对象存储数据随无持久卷容器移除，临时配置、凭据文件、Runner、验证码、日志目录均已删除。共享业务数据库未写入。
