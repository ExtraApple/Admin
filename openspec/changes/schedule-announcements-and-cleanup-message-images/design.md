## Context

本设计承接 `docs/reviews/c5-design-discussion.md`，采用本次提案生成期间用户明确确认的边界修订。它是目标设计，不是实施完成报告。

当前证据：

- `internal/messaging/application/announcement_service.go:99-155`：到期查询无上限，整批事务；`adapters/gorm/repository.go:231-295` 已保证单次 `ChangeMessage` 的 aggregate_version CAS 与 Outbox 原子写入。
- `internal/messaging/domain/lifecycle.go:47-54`：scheduled 目前只可转 published，需增加仅用于到期终结的 scheduled→expired；不增加状态。
- `internal/app/messaging.go:88-121,216-240`：cleanup 当前整体受 RabbitMQ 配置门控，启动执行后等待一小时，含两个既有数据库保留期任务。
- `internal/files/application/service.go:185-204` 与 `adapters/gorm/repository.go:112-132`：旧图片清理先删数据库再删对象，不能保留为新队列旁路。
- `internal/files/application/rotation.go:27-47`、`internal/platform/objectstorage/store.go:72-79`：轮转先 Copy、删源，再更新数据库，失败时反向 Move。仅查询排除队列不能阻止已开始的复制。
- `internal/files/adapters/gorm/repository.go:81-109`：绑定已有条件更新，但请求时间先于事务取得，不能据时间条件假定绑定与清理互斥。
- `internal/app/migrate.go:19-52,83-99`：Files.Models 参与启动迁移；普通文件 MIME 降级未限定 purpose，需在本次迁移兼容保护中限定用途。

分层以 ADR 0006 和当前 `internal/app` 唯一组合根为准。OpenSpec 配置中旧顶层 handler/service/model 路径描述不作为恢复旧目录的理由。Domain 不读配置、数据库、存储或日志全局状态。

## Goals / Non-Goals

**Goals:**

- 到期公告真正生效，已错过有效期的公告不再补发布；独立组织副本可分别推进。
- 有限工作量、跨轮推进、失败隔离，数据库提交与对象存储操作可恢复。
- 清理队列由 Files 所有；数据库事务短、不跨对象存储请求；不因轮转交错删除错位置。
- 配置、迁移、日志和运维边界可验证，保留既有消息投递和 cleanup 职责。

**Non-Goals:**

不新增 C5 开关、HTTP/API 权限、人工触发命令、dry-run、消息状态、事件种类、公告失败表、任务历史表、分布式锁、清理租约或 processing 状态。不重构既有 Outbox/Consumer、不扩大到头像或普通文件自动删除、不保证恢复任意外部写入或版本化 bucket 的所有历史对象版本。

## Decisions

### D1. 本次集中裁决覆盖讨论记录旧口径

| 问题 | 用户确认的最终选择 |
| --- | --- |
| 轮转与清理并发 | 所有未绑定消息图片排除轮转，不仅排除已有队列；普通文件和已绑定图片仍轮转。切换前排空旧版本轮转 |
| 跨桶归属 | 以本应用独占 bucket 内 `message-images/UUID` 命名空间为部署信任前提，不新增归属标记；不满足前提的历史对象不得自动跨桶清理 |
| 坏记录占满批次 | 每进程跨轮保留内存 ID 游标，到扫描上界才回绕，失败也推进；重启从头 |
| 已错过公告有效期 | scheduled→expired，仅 MessageExpired；不补 MessagePublished |
| 数据库互斥 | 允许仅覆盖资格检查和数据库提交的短事务行锁；不跨网络持锁，不新增分布式锁或租约 |
| RabbitMQ 未配置 | 跳过自动发布；图片及过期仍运行。已配置但断连仍提交 Outbox |

技术口径统一：初次删除不计入 24 次重试；第 24 次重试失败即 dead；对象不存在并成功终结记录计 succeeded。其余细节采用下述有界实现，不再保留相互矛盾的选项。

### D2. 单一任务、固定延迟、分项超时

将 `messaging-cleanup` 注册移到 `if !configured { return composition }` 之前；Outbox、Consumer、DLQ worker 的配置门控不动。复用现有 workerID。启动迁移完成后执行一轮；整轮结束再等待一小时，沿用 fixed-delay，不能宣传整点或最多延迟一小时。

每轮 UTC `now` 用于资格查询；建立 15 分钟父 context，顺序为：

1. 既有 `CleanupAudienceDeliveries(now,500)`；
2. 既有 `CleanupFinalConsumerDeadLetters(now-30天,500)`；
3. 图片清理；
4. 公告发布（按启动配置门控）；
5. 公告过期。

既有两项分别最多 30 秒，新 C5 三项分别最多 5 分钟，全部受父 context 限制。前置项失败或子超时后继续下一项；父取消后后续项记 skipped。各时长是上限，不承诺后项一定获得完整 5 分钟。旧两项放前面避免对象存储长期故障使现有保留期任务永远得不到机会。未配置 RabbitMQ 时旧两项也会首次生效，需在 runbook 明示。

一个进程只运行一个轮次，不另开未受控 goroutine 强行中断不响应 context 的调用。任务停止等待调用返回，不能以超时为理由启动重叠工作。轮次选择时间固定，但请求 deadline、重试资格预留及耗时使用当前时钟，不拿过时的轮次 now 安排“下一小时”。

### D3. 配置、工作量与公平性

唯一配置 `messaging.cleanup_batch_size` 默认 100；0 先填默认；有效范围 1–1000，负数和超界 Load 失败。更新真实 `config.yaml`、字段注释和校验，应用层接收值而非读取配置。

三类任务各有 N 个对象／副本尝试额度，数据库内先过滤资格，查询 LIMIT 不超过剩余额度。不按成功数补满，不做无界扫描或 COUNT。一个图片工作单位是一个对象的整组受信位置清理，可能涉及多个存储请求。原有两项仍各 500，不挪用 C5 额度。

公告发布、公告过期、图片新登记、图片重试各保存进程内 `{after_id, upper_id}` 扫描游标。开始一轮扫描周期取得相应记录空间最大 ID 为固定 upper_id；后续轮次只读 `(after_id, upper_id]`，按 ID 升序，成功／失败／冲突都推进 after_id。选出的条目尚未尝试就因取消退出，不推进该条。无更多合格候选或到上界时结束该扫描周期，下次重新取上界。这样新插入记录不会让回绕无限推迟；资格后来变化的较小 ID 在下一扫描周期重访。游标不持久化，不承诺频繁重启下严格公平性。

图片 N 额度由到期重试与新登记交替使用；先给重试 `ceil(N/2)`、新登记 `floor(N/2)`，任一队列无候选时另一队列使用剩余额度。N=1 时跨轮轮换优先队列。两类查询有独立游标，首次登记后当轮直接尝试一次，不再次进入本轮重试候选。总尝试不超过 N。

新增窄用途候选查询，避免修改现有管理列表的 `publish_at DESC,id DESC`、Offset、total 契约。缺依赖／查询错误为任务级失败，不用空结果伪装无积压。

### D4. 副本级公告转换

发布资格：announcement、scheduled、publish_at 非空且 <= now，expires_at 为空或 > now。

过期资格：announcement、expires_at 非空且 <= now，状态 published，或状态 scheduled 且 publish_at 非空且 <= now。后者直接 scheduled→expired。每项短事务读取当前副本并复核资格，生成稳定本次事件 ID，沿用 `ChangeMessage` CAS 与同事务 Outbox；外层不得把一页套进一个事务。增加 scheduled→expired 状态机转换，并在应用层限定到期资格，不能让任意人工动作绕过既有规则。

每个成功副本 aggregate_version 加一，事件与该版本一致。CAS 冲突记 skipped，表示本次未提交，不推断已发布；其他错误记 failed，继续后项。事务 runner 内部死锁重试不得重复累加统计、写逐条日志或调用外部存储。事件插入失败回滚该副本状态。

已错过窗口不发 MessagePublished；人工发布仍保持既有过期拒绝规则。过期不撤回已送达刷新，也不改变动态收件箱的现有时间过滤。

### D5. Outbox 边界不重建

未配置 RabbitMQ 时发布任务不查待发数量，记录 `rabbitmq_not_configured`；过期事件仍可进入 pending Outbox，不能声称完全没有积压。启用配置需重启；已配置后的网络重连由既有设施负责。

`outbox_worker.go:110-116` 与 `repository.go:683-705`：失败次数到 5 即 dead，只在前四次安排下一次 Outbox 等待（默认 1/2/4/8 秒）；不修改现有配置中完整五段序列或 worker 行为。dead 不会因 broker 恢复自动复活，且可阻挡同副本更高版本事件发布。

已核实恢复入口：`GET /api/admin/message-outboxes` 和 `POST /api/admin/message-outboxes/:id/replay`，权限 `admin.messages.outbox.replay` 加超级管理员范围；原 dead 记录重置 pending，保留原事件身份。不同于 Consumer DLQ replay。C5 不新建恢复入口、不回滚 published、不生成第二次发布事件。

### D6. Files 清理表与接口

采用职责明确名称 `message_image_cleanup_jobs`，替代讨论暂用名 `message_image_cleanup_failures`；该表在首次删除前登记，不能称作仅失败流水。Files 模型与 `Models()` 统一登记，App 聚合迁移，无新模块。

| 字段 | 设计 |
| --- | --- |
| id | 与 Files 一致的无符号稳定主键 |
| file_id | File Record ID，唯一索引；禁止级联删除队列失去对象定位 |
| bucket / object_name | varchar(100) / varchar(500)，不小于现有文件字段；冻结、不可截断 |
| object_key_hash | SHA-256 十六进制64字符，唯一索引；按 `bucket + NUL + object_name` 字节计算，不是内容摘要 |
| status | pending / dead |
| retry_count | 默认0，后续被预留的重试次数，范围0–24 |
| next_retry_at | 非空时调度资格；dead为空 |
| last_error_code | 稳定运行码，不保存原始错误 |
| created_at / updated_at | UTC 时间；不嵌入软删除字段 |

增加 `(status,next_retry_at,id)` 查询索引，完整路径以字节比较验证 hash 冲突。同 hash 同路径且同 file_id 复用原记录；hash 路径不一致或同路径映射不同 File Record 均拒绝登记，不覆盖、不重置 dead，留 File Record 并告警。

Files application 定义清理输入、游标、计数和有限逐条结果（至多 N），由 App 转成既有 RuntimeLogger；不把 Messaging 日志接口导入 Files。扩展现有 Files-owned MessageImageRepository 契约以包含候选、登记、预留重试、条件终结，复用事务 runner，不额外造通用任务框架。

旧 `DeleteExpiredMessageImages` 接口和先删 File Record 的实现及测试同步迁移删除，不保留两个可达清理路径。现有 HTTP 响应不新增清理字段。

### D7. 登记、绑定与读取一致性

每个新对象先进入 Files 短事务，按 File Record ID 锁当前行并重新核对 purpose=message_image、未绑定、binding_expires_at<=本轮now、没有队列。登记 pending、retry_count=0，next_retry_at=登记时当前时间+1小时。提交后才尝试存储删除；若进程在提交后崩溃，该记录最迟在后续符合时间的轮次恢复。

绑定也在消息已有外层事务中按升序锁相关 File Record，锁后重新检查操作者、用途、当前时间仍在绑定期、未绑定且不存在 pending/dead 队列，再原子更新全部指定图片。不使用事务前取得的旧时间绕过到期规则。所有多行入口采用同一 ID 顺序，嵌套 runner 加入外层事务；锁仅涵盖数据库操作，禁止跨存储调用。

登记先提交则后续绑定拒绝；绑定先提交则清理资格消失，不登记不删除。通过图片读取仓储排除 pending/dead，保证登记提交后开始的新资格查询拒绝；不宣称能撤销已经交付给调用方的 reader。普通文件入口维持用途隔离。

### D8. 首次删除、24次重试与终结

首次登记后立即尝试一次，不增加 retry_count。对象不存在是幂等成功，不记失败。所有目标位置处理完成后，短事务按 File Record→job 顺序锁行，复核 file_id、路径和未绑定状态，物理删除两者。对象删完而终结事务失败，队列仍可恢复；不会把数据库失败误记成功。

重试资格为 pending 且 next_retry_at<=轮次now。存储调用前以当前读取的 id/status/retry_count/next_retry_at 做 CAS，把 retry_count 加1、next_retry_at 设为当前时间+1小时；受影响0行则本条 skipped，不调用存储。该步骤是一次调度尝试预留，不设置 processing、worker占有权或租约。预留后崩溃消耗一次不确定尝试，下一资格时间继续；不通过失败后再加计数造成多实例重复消耗。

- 重试1–23失败：仅匹配本次已预留计数和时间的 pending 行更新 last_error_code；不重新插入、不修改新尝试的计划。
- 第24次失败：同样条件更新为 dead，next_retry_at清空并输出重点 Warn。
- 第24次预留后崩溃：下一资格时间只将该pending终结为dead，不执行第25次删除。
- 任一有效删除尝试成功：按稳定队列ID与完整文件身份条件终结；即使另一个实例刚记录失败，也不重建任务。
- 失败更新若发现行已成功删除，按并发结束 skipped，不能 UPSERT 恢复旧任务。

成功/失败结果只在数据库结果明确后计数；父context取消时不为记录错误额外开启无取消后台事务。已预留记录在下轮按上述协议恢复。dead 不自动重入、不过期；人工在受控数据库与存储权限下确认对象清除后，同事务删除 File Record 和队列，不只删队列。无新运维命令。

### D9. 轮转资格与受信删除范围

轮转候选永久排除所有未绑定 message_image（含未到期、到期及pending/dead），并排除任何存在队列的对象。已绑定图片没有自动解绑路径，因此不会成为清理候选；未来新增解绑必须重新审查这个不相交前提。普通文件和已绑定图片保留原轮转规则。上线必须停止旧版本服务、排空旧 Copy/反向Move 后才启动新清理；不支持旧轮转与新清理混跑。

对象删除目标为冻结原始 bucket，加当前 `FileRotation.Enabled` 时非空的 `HotBucket/ColdBucket`，去重；上传实际仍用 Files 固定 `files` bucket，不把 FileRotation.HotBucket 误写成上传配置。未启用轮转则不自动加入冷桶；历史队列原始bucket仍尝试，不猜测其他历史桶。

跨桶删除只适用于经部署确认的本应用独占命名空间：服务端生成 `message-images/<UUID>.<规范扩展名>`、无外部覆盖／复用、没有其他活跃 File Record 与对象键冲突。实现检查生成键格式及数据库冲突；专属桶历史与访问控制是上线前提，不是这些检查能证明的事实。前提不满足时不启动本变更的自动清理部署，先人工隔离／处置相应历史数据；运行中发现不可信键或冲突则保留记录并受控失败，不能扩大删除范围。

所有允许位置成功或确认 NoSuchKey 后才终结；NoSuchBucket、AccessDenied、超时均不能作为“不存在”吞掉。部署使用非版本化、无 Object Lock 的受信桶；本变更不实现全版本擦除、外部写入协调或 bucket 改名迁移。关闭轮转或更换配置前须清理／登记历史位置，不能以当前配置缺失证明旧副本不存在。

### D10. 存储分类与普通 HTTP 兼容

复用 platform Store 已有 Stat/Delete，不新增 SDK。`normalizePlatformError` 当前只识别 NoSuchKey，AccessDenied 被折叠为不可用；在 Platform 增加供应商无关的拒绝分类，Files 将其转为稳定后台码，普通 HTTP 仍沿既有受控 storage-unavailable 映射，不新增公开错误码。保留 errors.Is 的 context 取消识别，不靠字符串匹配原始错误。

Stat 在删除前确认精确目标可访问；只将确定的 NoSuchKey 视为不存在。Delete 返回nil或确定NoSuchKey为成功；缺桶和权限拒绝保留任务。不得把bucket路径和SDK错误透传给日志。精确 SDK 行为由实施阶段 MinIO 门禁验证。

参考官方文档：
- https://github.com/minio/minio-go/blob/master/docs/API.md （StatObject、RemoveObject）
- https://github.com/minio/minio-go/blob/master/_autodocs/12-errors-and-exceptions.md （NoSuchKey、NoSuchBucket、AccessDenied）

### D11. 日志与统计

App 复用 `RuntimeLogger` 的 Info/Warn，扩充有限白名单，不引入 Error/Alert 接口或暗示已接通外部告警系统。Files 返回有限结果，由App统一输出；每轮UUID run_id和复用worker_id，不写数据库。

固定事件：`messaging_cleanup_started`、`messaging_cleanup_finished`、`messaging_cleanup_task_started`、`messaging_cleanup_task_finished`、`messaging_cleanup_item_failed`、`messaging_cleanup_task_skipped`、`messaging_cleanup_dead_lettered`。

任务名：三个C5名称加原有快照／死信任务名称。字段白名单包括 run_id、worker_id、task、stage、status、failure_code、processed、succeeded、skipped、failed、duration_ms、timed_out、file_id、cleanup_job_id、message_copy_id、retry_attempt、batch_size、run_at。run_at编码RFC3339Nano字符串，duration_ms整数，不将time.Time直接传入当前不支持它的适配器。

`processed=succeeded+skipped+failed`，只计算已进入处理的单个对象／副本；不存在且数据库成功终结为succeeded，CAS冲突为skipped。配置／父取消导致未执行计数全0。查询失败可status=failed而processed=0；执行中超时保留先前结果，当前已尝试但未完成项计failed并status=timed_out。status优先超时，再失败，再成功；预执行跳过独立表示。成功不逐条刷日志。

使用讨论中的公告、图片、队列稳定运行码及 rabbitmq_not_configured；补充 `message_image_cleanup_scope_untrusted` 表示拒绝跨桶归属边界。不保存原始错误；object_not_found是成功结果而非失败码。hash冲突、登记失败、dead转移为Warn重点事件。日志不包含完整对象路径、bucket、正文、凭据、数据库/Broker/SDK原始错误。

### D12. 模型迁移不改变消息图片验证结果

新表通过 `internal/files.Models()` 加入 `internal/app/migrate.go`，成功后才启动任务；迁移不扫存储、不删数据、不预登记全量历史图片。hash使用固定长度索引，file_id唯一，显式物理删除避免软删除占住唯一键。

现有 `downgradeValidatedManagedFilesOutsideV1Policy` SQL 没有purpose过滤，会匹配message_image的JPEG/PNG/WebP。将其限于managed_file和兼容旧NULL/空用途，作为本次启动兼容必要修正；合法消息图片迁移后保持原validated。已有legacy_unverified不自动升级，不能推测历史状态来源。

## Risks / Trade-offs

- [每类100条、fixed-delay和超时导致积压] → 跨轮游标提供持续运行期间推进机会，日志区分失败与无数据，不承诺一小时内完成；最多可配置1000。
- [多次重启丢失内存游标] → 明示非持久公平性，启动仍处理存量；不增加水位表。
- [短事务行锁有等待/死锁] → 所有相关入口稳定顺序、context超时、复用既有TransactionRunner；禁止在可重试回调做存储或日志副作用。
- [预留后崩溃可能耗用一次重试而未实际删对象] → 24为调度尝试上限，不承诺24次成功发出的网络请求；dead保留定位可人工处理。
- [删除不可逆、跨桶信任非代码可自动证明] → 切换前专属命名空间、非版本化、历史桶及旧进程排空是部署门禁；不满足不发布，不以双桶扫描当保障。
- [Outbox dead阻挡同副本后续事件] → 保留既有重放入口，运行手册明确published与投递状态分离，不在C5重建投递控制面。
- [15分钟父上限截短后项] → 所有子超时均为最大值，旧两项优先且各30秒；父已取消则跳过，不宣称完整五分钟公平分时。
- [dead长期保留增长] → 每条记录对应尚未处置的对象，不能为容量自动丢弃；人工确认后删除，不增加自动保留期。

## Migration Plan

1. 审核专属命名空间、历史热冷bucket、版本化/Object Lock、到期公告及图片数量。归属不清或历史位置遗漏先人工处理，不能直接启用。
2. 在独立MySQL和MinIO探针验证迁移、唯一约束、事务互斥、取消和存储幂等；不使用陈旧admin开发库跑结构门禁。
3. 停止旧版本实例及其轮转任务，确认无在途Copy/反向Move；不做旧轮转与新清理的滚动混跑。
4. 部署新版本，AutoMigrate建表及用途限定迁移成功后启动任务。默认即处理存量，无新增开关。核对Info/Warn汇总及既有Outbox管理入口。
5. 更新消息与日志runbook，记录配置重启、fixed-delay、dead人工终结、Outbox重放及各信任前提。
6. 回滚时先停止所有新实例，保留队列与File Record，不删表；已删对象不可恢复。旧版本不认识队列读/绑/轮转保护，不能带pending/dead直接回滚运行。先人工完成队列处置或维持停机后部署前向修复；不得把DDL回滚称为业务回滚。

## Open Questions

本次影响实现方向的业务选择已集中确认，无需继续逐条访谈。真实数据库/存储验证尚未执行，属于tasks中的强制验收，不是允许省略的设计空白。若部署无法满足D9专属命名空间或非版本化前提，或发现存在解绑/外部同名写入，必须暂停启用并另行裁决，不降低“禁止误删”要求。
