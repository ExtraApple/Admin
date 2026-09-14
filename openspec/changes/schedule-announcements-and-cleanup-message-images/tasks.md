## 1. 实施边界与基线

- [ ] 1.1 阅读proposal、design D1–D12及三份delta，确认采用本次六项集中裁决；只在本change内记录后续偏差，不恢复讨论中已替代方案。
- [ ] 1.2 定位三个旧服务、所有调用方、消息图片绑定/读取、轮转、迁移和日志适配；通过LSP references（可用时）确认导出契约修改范围，保留现有HTTP路由与响应基线。
- [ ] 1.3 为真实验证准备独立MySQL／MinIO探针，记录配置和清理范围但不记录凭据；Go命令设置工作区GOCACHE，不对陈旧admin库或非探针bucket执行破坏性测试。

## 2. Files 模型与迁移

- [ ] 2.1 新增Files所有的message_image_cleanup_jobs模型：稳定ID、唯一file_id、完整冻结路径、SHA-256路径唯一键、pending/dead、retry_count、next_retry_at、last_error_code、UTC时间；无软删除、processing、completed或租约字段。
- [ ] 2.2 将新模型加入files.Models及现有启动迁移，建立固定长度hash唯一索引和到期查询索引；路径字段不少于现有100/500长度，无长路径前缀唯一索引和级联丢队列风险。
- [ ] 2.3 将downgradeValidatedManagedFilesOutsideV1Policy限定为managed_file及旧NULL/空用途，不再降级message_image；不自动升级既有未知验证状态，不在迁移中读取/清理对象。
- [ ] 2.4 增加独立MySQL迁移回归：重复Migrate不重复建任务、不改变合法消息图片状态，唯一file_id/hash约束有效，普通文件V1降级仍生效。

## 3. Files 数据库生命周期

- [ ] 3.1 在现有Files-owned MessageImageRepository增加窄候选/登记/重试预留/条件终结能力及有界结果类型，所有新增Dependencies字段依照接线护栏注明required/optional与来源。
- [ ] 3.2 实现未绑定到期图片候选SQL：数据库内用途、时间、无pending/dead过滤，ID升序、after/upper边界和LIMIT；不全表加载或应用层无界去重。
- [ ] 3.3 实现逐对象短事务登记：锁File Record后复核资格，保存完整路径与hash；相同对象去重，不同路径hash冲突或不同File Record共用路径拒绝，提交失败不执行存储删除。
- [ ] 3.4 改造绑定为与登记相同的升序File Record短事务锁协议，锁后按当前时间和队列状态检查全部图片，保留操作者/未绑定/用途及消息外层事务原子性。
- [ ] 3.5 为新的图片读取资格查询排除pending/dead，复用既有受控HTTP拒绝；普通文件入口隔离不变，不宣称撤销已交付reader。
- [ ] 3.6 实现重试CAS预留：比对id/status/retry_count/next_retry_at，成功后计数加1且下一时间为当前时刻后一小时；失败/取消后不得无条件UPSERT或覆盖较新结果。
- [ ] 3.7 实现初次不计重试、后续24次尝试、最后一次崩溃到期转dead以及成功条件终结；成功后同事务物理删除File Record与job，数据库终结失败保留可恢复记录。
- [ ] 3.8 增加数据库行为回归：绑定/登记两种提交顺序、hash冲突、重复登记、多实例争同次重试、成功与迟到失败交错、dead不重入、已删对象数据库终结失败后恢复；并发锁与唯一约束用真实MySQL验证。

## 4. 对象存储与轮转

- [ ] 4.1 轮转查询排除所有未绑定message_image以及任何pending/dead队列对象，保留普通文件和已绑定图片规则；确认不存在解绑路径破坏资格不相交。
- [ ] 4.2 在Files存储边界复用现有Stat/Delete，按冻结原始bucket及当前启用的非空热冷bucket去重处理；校验生成对象键与数据库归属冲突，不扫描任意bucket。
- [ ] 4.3 扩展供应商无关错误分类以区分NoSuchKey、AccessDenied与其他不可用/取消；缺桶不能当作对象不存在，普通HTTP映射保持既有受控响应，不新增公开错误码。
- [ ] 4.4 实现提交登记后初次删除和重试调用；不在数据库事务或可重试回调内访问对象存储；一个位置失败保留记录并继续其他对象，完整清理成功后才数据库终结。
- [ ] 4.5 删除旧DeleteExpiredMessageImages先删数据库路径并迁移全部调用与测试；不保留新旧清理别名、可达旧接口或仅声明未接线的依赖。
- [ ] 4.6 用隔离MinIO验证已存在/已不存在对象、缺桶、权限拒绝、超时、部分位置成功、原始bucket已离开当前配置；确认只触碰受信生成键和允许位置，存储失败不丢数据库定位。
- [ ] 4.7 验证轮转候选与未绑定清理资格不交叠，且普通文件/已绑定图片仍可轮转；记录上线需排空旧Copy及禁止混跑的证据边界，不用双桶扫描冒充跨版本并发保证。

## 5. 公告到期处理与事件

- [ ] 5.1 在Messaging仓储增加专用ID有界到期候选查询，不修改管理列表的publish_at倒序、Offset、total；发布过滤已过期scheduled，过期包含已到期scheduled和published。
- [ ] 5.2 增加announcement scheduled→expired域转换并由应用服务限定到期资格；错过窗口仅生成MessageExpired，不补MessagePublished，不新增状态。
- [ ] 5.3 将公告处理改为单组织副本事务，事务内复核资格并复用ChangeMessage的aggregate_version CAS和Outbox原子写入；取消整页外事务，事务重试不重复计数或日志。
- [ ] 5.4 失败按副本隔离：CAS为skipped、其余为failed，不推断冲突即别人已发布，不新增公告失败表；返回有界处理结果和已尝试游标。
- [ ] 5.5 增加服务回归：publish_at/ expires_at精确边界、错过窗口直接过期、多个组织部分失败、Outbox写入失败回滚单副本、并发编辑/CAS及现有人工发布到期拒绝保持。
- [ ] 5.6 核对并保留既有Outbox有限失败/dead与原事件重放：测试C5不因投递失败回滚或生成第二个发布事件；记录现有GET/POST message-outboxes入口及超级管理员限制，不新增恢复控制面。

## 6. 配置与 App 调度接线

- [ ] 6.1 增加Messaging.CleanupBatchSize及真实config.yaml默认100；Load先将0填默认，再拒绝负数/大于1000；覆盖缺省、0、两端边界和非法值的配置行为。
- [ ] 6.2 在App为发布/过期/图片新登记/图片重试保存跨轮内存after_id与upper_id，固定扫描上界后递增并回绕，尝试失败也前移，尚未尝试即取消不跳过记录。
- [ ] 6.3 实现图片新登记与重试共用N预算、两类分额/空队列让额、N=1交替优先；任何冲突/失败均计尝试，禁止当轮重新重试刚登记对象。
- [ ] 6.4 将唯一messaging-cleanup移到RabbitMQ未配置提前返回之前；启动立即执行、轮末等一小时、无重叠，复用workerID，每轮生成run_id和资格now。
- [ ] 6.5 保留原有快照/终态Consumer死信清理各500条且先执行，各30秒；其后固定图片/发布/过期各至多5分钟，整轮15分钟，父取消跳过后续项。
- [ ] 6.6 发布仅受启动RabbitMQ配置门控，未配置只写跳过汇总不查数量；图片和过期不门控；已配置断连不暂停发布，配置变更需重启。
- [ ] 6.7 增加调度行为回归：真实任务入口首次执行、单进程无重叠、子失败隔离、父取消、旧任务保留、Broker两类状态、批量上限、坏前页跨轮推进、固定上界回绕及N=1队列公平性；使用受控时钟/等待边界，不等待真实一小时。

## 7. 运行日志与安全映射

- [ ] 7.1 由App将Files/Messaging有界结果映射到现有RuntimeLogger，不让Files导入Messaging业务日志类型；接入七个固定事件名、run_id和复用worker_id。
- [ ] 7.2 同步App窄字段白名单与安全编码：run_at字符串、duration_ms整数、各类稳定记录ID/计数；不输出对象路径、bucket或原始内部错误。
- [ ] 7.3 落实稳定运行码实际分类路径（含scope_untrusted），NoSuchKey幂等成功而非失败；hash冲突、登记失败与dead为Warn重点日志，不新增HTTP错误码。
- [ ] 7.4 覆盖真实日志sink输出：新字段未被丢弃、processed分解不重复、配置跳过计数0、查询失败0处理量、超时保留已完成计数、对象不存在成功终结计succeeded、敏感错误不泄漏。

## 8. 验证、文档与交付

- [ ] 8.1 运行一次完整格式/静态与编译验证：gofmt受影响Go文件、go vet受影响包、go build ./...；不得以禁用检查或无效fallback使验证通过。
- [ ] 8.2 运行受影响Files/Messaging/App/Config行为测试、架构护栏及go test ./... -count=1，确认导出契约调用方、事务边界和路由响应兼容。
- [ ] 8.3 运行仓库既有真实MySQL与MinIO集成门禁（独立探针范围），记录并发、错误分类和迁移证据；区分实际执行与未执行，不把SQLite通过当作MySQL锁语义证明。
- [ ] 8.4 启动实际后台任务路径并在隔离环境放入到期公告/图片，观察状态和Outbox、队列重试及日志；配置未启用Broker与已配置断连分别验证，证明能力真正接线。
- [ ] 8.5 更新docs/runbooks/messaging.md：首次存量、fixed-delay、限额、超时、专属命名空间/非版本化/旧轮转排空前提、dead人工原子终结、历史bucket与Outbox原事件重放；不新增专用命令或假定外部告警已配置。
- [ ] 8.6 更新docs/runbooks/runtime-logging.md与审计索引，记录本change实施结果、用户六项集中裁决和实际验证偏差；按ADR三条件记录难以撤回的对象清理所有权/轮转取舍，不改写讨论历史。
- [ ] 8.7 运行openspec validate本change及主规格校验，对照所有Scenario确认实现证据；未实施项不得勾选，不提前同步或归档主规格。
- [ ] 8.8 移除本次临时探针、临时缓存及测试数据（仅本次创建范围），给出受影响文件、验证结果和部署/回滚限制；未经用户授权不执行Git提交或推送。
