## MODIFIED Requirements

### Requirement: 历史文件验证状态和重新验证
系统 SHALL 兼容保留历史文件，并允许管理员显式重新验证单个历史对象。普通文件V1类型降级 SHALL 仅作用于managed_file及兼容旧NULL/空用途，不改变消息图片专用用途已验证状态。

#### Scenario: 历史记录迁移
- **WHEN** V1 数据字段首次上线
- **THEN** 系统 SHALL 将既有文件记录回填为 `legacy_unverified`
- **AND** 迁移 SHALL NOT 读取、解压、删除或重写 MinIO 对象

#### Scenario: 上传类型策略收窄
- **WHEN** 本策略上线时存在状态为 `validated` 且规范 MIME 不属于 PDF、TXT、CSV 的管理员文件
- **THEN** 系统 SHALL 将该记录更新为 `legacy_unverified`
- **AND** 系统 SHALL 保留对象和原有元数据
- **AND** 迁移 SHALL NOT 读取、删除或重写 MinIO 对象
- **AND** 该普通文件降级 SHALL NOT 作用于message_image专用用途

#### Scenario: 重新验证成功
- **WHEN** 管理员通过 `POST /api/admin/files/:id/revalidate` 重新验证 `legacy_unverified` 或 `validation_error` 文件且对象通过当前策略
- **THEN** 系统 SHALL 更新规范 MIME、检测 MIME、`content_sha256`、状态 `validated`、策略版本和验证时间
- **AND** 系统 SHALL 只提供附件下载能力

#### Scenario: 重新验证确认不合规
- **WHEN** 重新验证发现文件是图片、Office、其他不支持类型、类型不一致、内容损坏或危险内容
- **THEN** 系统 SHALL 将文件状态更新为 `blocked`
- **AND** 系统 SHALL 禁止该文件下载和预览
- **AND** 系统 SHALL NOT 在 V1 自动删除对象

#### Scenario: 重新验证遇到临时错误
- **WHEN** MinIO、网络或其他临时错误导致无法完成检查
- **THEN** 系统 SHALL 将状态更新为 `validation_error`
- **AND** 系统 SHALL 保留对象并允许以后重试
- **AND** 系统 SHALL NOT 把临时错误标记为危险内容

#### Scenario: 对已验证或封锁文件重复重新验证
- **WHEN** 管理员对 `validated` 或 `blocked` 文件调用 V1 重新验证接口
- **THEN** 系统 SHALL 返回 HTTP 409
- **AND** 系统 SHALL 保持文件状态不变

#### Scenario: 系统启动时存在历史文件
- **WHEN** 服务启动或执行数据库自动迁移
- **THEN** 系统 SHALL NOT 为V1重新验证而自动批量读取或扫描历史存储对象
- **AND** 本限制 SHALL NOT 禁止迁移完成后独立后台任务按消息图片清理契约处理过期未绑定图片

#### Scenario: 新队列迁移不降低消息图片验证状态
- **WHEN** 启动迁移创建消息图片清理队列且已有validated消息图片
- **THEN** 系统 SHALL 保持这些消息图片的验证状态
- **AND** 重复迁移 SHALL NOT 预登记清理任务、删除对象或自动升级未知历史验证状态

### Requirement: 文件轮转
系统 SHALL 支持按配置将旧文件从热 bucket 轮转到冷 bucket。所有未绑定message_image和存在pending/dead清理队列的对象 SHALL 排除轮转；普通文件与已绑定且无清理队列的图片继续遵守轮转规则。

#### Scenario: 文件轮转未启用
- **WHEN** 配置未启用文件轮转
- **THEN** 系统跳过轮转

#### Scenario: 轮转符合条件的文件
- **WHEN** 文件轮转已启用
- **AND** 热 bucket 中存在超过配置天数阈值且未被清理保护排除的文件
- **THEN** 系统最多处理配置的 batch size 数量
- **AND** 系统将文件从热 bucket 移动到冷 bucket
- **AND** 移动成功后更新数据库元数据中的 bucket 字段

#### Scenario: 轮转时数据库更新失败
- **WHEN** 对象移动成功，但数据库 bucket 字段更新失败
- **THEN** 系统尝试将对象移回热 bucket
- **AND** 系统继续处理后续文件

#### Scenario: 未绑定消息图片不参与轮转
- **WHEN** message_image尚未绑定，无论绑定期限是否已过以及是否已登记清理
- **THEN** 系统 SHALL NOT 将该图片选作轮转候选
- **AND** 清理候选与可轮转图片 SHALL 不重叠

#### Scenario: 切换前排空旧轮转
- **WHEN** 部署首次启用C5清理
- **THEN** 部署过程 SHALL 先停止旧实例及其在途Copy和反向Move，再启动新清理
- **AND** 系统 SHALL NOT 将“检查热冷桶不存在”当作旧复制已经结束的证据

### Requirement: 消息图片专用用途与可见性
系统 SHALL 支持内部消息调用的专用 JPEG、PNG、WebP 临时图片用途，并按消息当前可见性控制读取；该能力 SHALL NOT 放宽普通管理员文件入口的拒绝策略。已登记pending/dead清理的图片 SHALL 拒绝新的绑定与读取资格查询。

#### Scenario: 临时消息图片绑定
- **WHEN** 图片通过完整格式、大小和尺寸验证且在 15 分钟绑定期内被消息创建、编辑或发布引用
- **THEN** Files SHALL 在同一事务中绑定当前操作者持有的临时记录到逻辑消息
- **AND** 过期、已绑定或不属于操作者的记录 SHALL 被拒绝
- **AND** Files SHALL 在与清理登记一致的短事务行锁协议内检查无pending/dead清理记录，不能使用过时请求时间绕过绑定期

#### Scenario: 消息图片读取和审计
- **WHEN** 用户读取消息图片
- **THEN** Files SHALL 先验证消息当前可见性并只返回规范 MIME 的图片
- **AND** 响应和 Audit metadata SHALL NOT 包含图片内容、MinIO bucket、object key、签名 URL、存储凭据或解析器原始错误

#### Scenario: 通过消息图片入口上传
- **WHEN** 已认证用户或具备消息管理权限的管理员通过消息图片专用入口上传 JPEG、PNG 或 WebP
- **AND** 图片通过完整解码、大小、格式和尺寸验证
- **THEN** Files SHALL 创建带消息图片用途、上传者引用和 15 分钟绑定时限的临时 File Record
- **AND** 临时记录 SHALL NOT 具有消息所有者引用，普通管理员文件上传接口 SHALL NOT 接受该图片

#### Scenario: 普通文件入口上传图片保持拒绝
- **WHEN** 客户端通过 `POST /api/admin/files` 上传 JPEG、PNG、WebP、SVG 或其他图片
- **THEN** 系统 SHALL 保持 HTTP 415 拒绝行为
- **AND** 系统 SHALL NOT 因消息图片用途写入普通文件记录

#### Scenario: 临时图片过期清理
- **WHEN** 临时消息图片超过 15 分钟绑定时限且尚未绑定
- **THEN** Files SHALL 在有界后台任务中先持久化清理队列，再删除受信范围内对象
- **AND** 对象删除全部成功或确认不存在后 SHALL 同事务物理删除File Record和队列
- **AND** 对象删除失败 SHALL 保留可定位记录，按有限重试与dead规则处理，不保证到期瞬间完成存储删除

#### Scenario: 消息图片不可见时拒绝读取
- **WHEN** 关联消息已撤销、私信关系被删除、消息已过期或用户不再符合动态受众
- **THEN** Files SHALL 拒绝图片读取
- **AND** 系统 SHALL NOT 返回图片内容或存储状态

#### Scenario: 消息图片安全审计
- **WHEN** 消息图片验证成功或因大小、格式、解码、尺寸策略被拒绝
- **THEN** Audit metadata SHALL 记录用途、清洗文件名、大小、声明 MIME、检测 MIME、`accepted`/`rejected` 结果和策略版本或稳定原因码
- **AND** metadata SHALL NOT 记录图片内容、object key、签名 URL、MinIO 凭据或解析器原始错误

#### Scenario: 清理登记先于绑定提交
- **WHEN** 清理登记已提交且后续绑定或读取资格查询开始
- **THEN** Files SHALL 拒绝该图片的新绑定与读取
- **AND** 系统 SHALL NOT 宣称能撤回此前已交付的reader

#### Scenario: 绑定先于清理登记提交
- **WHEN** 合法绑定先提交，清理随后重新检查同一File Record
- **THEN** Files SHALL NOT 登记或删除已绑定图片
- **AND** 多图片绑定 SHALL 保持消息外层事务原子性和稳定ID锁顺序

## ADDED Requirements

### Requirement: 消息图片持久化清理队列

Files SHALL 拥有 `message_image_cleanup_jobs` 队列，包含稳定ID、唯一file_id、冻结的完整bucket/object_name、唯一SHA-256路径键、pending/dead状态、retry_count、next_retry_at、last_error_code及创建更新时间。路径键 SHALL 按bucket、NUL分隔符和object_name原始字节生成，不使用文件内容摘要；原始错误 SHALL NOT 持久化。

每个对象 SHALL 逐条在短数据库事务中锁定并重查用途、未绑定和到期资格，提交队列后才调用存储。系统 SHALL NOT 在事务重试回调内执行存储操作，不增加File Record清理状态字段、processing状态或租约。

#### Scenario: 登记失败不丢失对象定位
- **WHEN** 队列写入或事务提交失败
- **THEN** File Record SHALL 保留且本次 SHALL NOT 执行对象删除
- **AND** 系统 SHALL 记录受控失败并继续其他候选

#### Scenario: 同对象重复登记
- **WHEN** 多实例登记同一路径和File Record
- **THEN** 数据库 SHALL 仅保留一个队列记录
- **AND** 已有pending或dead的重试次数与状态 SHALL NOT 被重新初始化

#### Scenario: 路径键冲突
- **WHEN** hash相同但完整路径不同，或同路径对应不同File Record
- **THEN** 系统 SHALL 拒绝新登记、保留原文件、输出受控冲突告警
- **AND** 系统 SHALL NOT 覆盖、截断路径或追加冲突序号绕过去重约束

#### Scenario: 存储成功而数据库终结失败
- **WHEN** 对象已删除但File Record和队列的终结事务失败
- **THEN** 队列 SHALL 保留以便后续幂等恢复
- **AND** 系统 SHALL NOT 把该次数据库失败统计为完整清理成功

### Requirement: 消息图片清理有限重试

初次删除 SHALL 不计入24次重试；后续pending到期后，系统 SHALL 以条件更新预留一次重试并把next_retry_at设为当前时间后一小时，成功预留后才执行存储。预留后崩溃 SHALL 消耗一次不确定调度尝试，不额外无限补次数。

第24次重试失败 SHALL 转dead；第24次预留后崩溃的pending SHALL 在下次资格时间转dead而不执行第25次存储尝试。成功或确定对象不存在后，系统 SHALL 条件物理删除File Record与队列。dead SHALL 保留人工处理，不再自动重试或重新登记。

#### Scenario: 多实例竞争同次重试
- **WHEN** 多实例读取相同pending记录和重试计数
- **THEN** 仅条件预留成功者 SHALL 执行本次存储尝试
- **AND** 其他实例 SHALL 跳过且不能重复递增计数

#### Scenario: 删除对象不存在
- **WHEN** 所有受信目标均已删除或确定为NoSuchKey
- **THEN** 系统 SHALL 将其作为幂等成功并尝试数据库终结
- **AND** NoSuchBucket、AccessDenied和超时 SHALL NOT 被当成NoSuchKey

#### Scenario: 达到重试终态
- **WHEN** 第24次重试失败或第24次预留崩溃后到达下次资格时间
- **THEN** 系统 SHALL 标记dead、清空下一重试时间并输出Warn重点事件
- **AND** 系统 SHALL NOT 执行第25次自动存储尝试

#### Scenario: 成功与迟到失败交错
- **WHEN** 成功实例已条件终结队列，另一个实例随后提交失败结果
- **THEN** 迟到失败 SHALL NOT UPSERT重建队列、复活File Record或覆盖较新尝试状态

#### Scenario: 人工完成 dead 处置
- **WHEN** 运维确认目标对象均已清除
- **THEN** 人工数据库处置 SHALL 同事务删除匹配的File Record和dead队列，避免只删队列后重新入队
- **AND** 系统 SHALL NOT 新增人工HTTP接口、专用命令或completed状态

### Requirement: 消息图片受信存储清理范围

清理 SHALL 始终保留并使用队列冻结的原始位置；额外位置只来自当前启用且非空的文件轮转热冷bucket，去重后使用，不猜测任意历史bucket。跨桶清理 SHALL 以本应用专属、无外部覆盖/复用的 `message-images/UUID` 命名空间以及非版本化、无Object Lock存储为部署前提。系统 SHALL NOT 仅凭任意同名对象推断业务归属。

#### Scenario: 同对象多个受支持副本
- **WHEN** 已满足专属命名空间前提且同一生成对象键在多个受支持位置存在
- **THEN** 系统 SHALL 删除所有这些对应副本后才终结数据库记录
- **AND** 任一位置错误 SHALL 保留队列，不宣称全副本已清理

#### Scenario: 历史原始 bucket 已不在当前配置
- **WHEN** 已登记对象的原始bucket不再出现在当前配置中
- **THEN** 系统 SHALL 继续按冻结原始位置重试
- **AND** 系统 SHALL NOT 改写队列路径或猜测其他历史bucket

#### Scenario: 归属或配置范围无法保证
- **WHEN** 部署无法确认专属命名空间、发现外部同名复用或历史位置遗漏
- **THEN** 部署 SHALL 在启用自动清理前完成人工核实和处置
- **AND** 运行中发现不可信生成键或数据库对象归属冲突时 SHALL 保留记录并受控失败，不自动跨桶删除

#### Scenario: 关闭轮转不证明历史冷桶为空
- **WHEN** 当前轮转未启用或冷桶配置为空
- **THEN** 新清理范围 SHALL 不自动扩展到默认冷桶
- **AND** 配置切换前 SHALL 处置或明确登记旧位置，不以当前配置缺失认定历史副本不存在
