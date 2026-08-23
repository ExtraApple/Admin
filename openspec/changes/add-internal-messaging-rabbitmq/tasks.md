## 1. 基础设施与模块骨架

- [ ] 1.1 使用 Goldmark 编译 Markdown、Bluemonday 按白名单清洗 HTML，并确认图片处理依赖，避免重复引入
- [ ] 1.2 增加 RabbitMQ 连接、vhost、TLS、重试、死信和消息内容上限配置
- [ ] 1.3 创建 `internal/messaging` 模块及 Domain、Application、Adapter 分层骨架
- [ ] 1.4 在 App 组合根装配 Messaging、RabbitMQ Publisher、Redis、Outbox Worker 和 WebSocket Consumer
- [ ] 1.5 扩展架构边界测试，允许 `messaging` 一级模块并禁止跨模块 Adapter 依赖

## 2. 消息领域模型与持久化

- [ ] 2.1 定义私信、管理员群发、通知公告、草稿、定时、发布、过期和撤销领域状态
- [ ] 2.2 定义消息主体、组织副本、分类、动态受众和私信收件关系模型
- [ ] 2.3 定义用户消息状态、未读计算和私信删除墓碑模型
- [ ] 2.4 定义消息 Outbox、事件消费幂等和并发抢占模型
- [ ] 2.5 增加 MySQL 索引、唯一约束和 AutoMigrate/迁移注册
- [ ] 2.6 验证跨组织副本和分类编码缺失时的事务全回滚

## 3. 跨模块 Contract 与 Repository

- [ ] 3.1 定义 Messaging 调用 Identity 的用户身份和启用状态 Contract
- [ ] 3.2 定义 Messaging 调用 Organization 的组织成员、子组织和角色受众 Contract
- [ ] 3.3 定义 Messaging 调用 Authorization 的权限与消息组织范围 Contract
- [ ] 3.4 定义 Messaging 调用 Files 的消息图片用途和受可见性控制读取 Contract
- [ ] 3.5 定义 Messaging 调用 Audit 的受控元数据写入 Contract
- [ ] 3.6 实现消息、分类、受众、收件关系、用户状态和 Outbox Repository
- [ ] 3.7 增加 Repository 分页、筛选、唯一约束和动态受众查询验证

## 4. 内容安全与消息图片

- [ ] 4.1 实现标题和 Markdown 正文长度校验
- [ ] 4.2 实现 Markdown 到受限 HTML 的转换和 XSS 清洗
- [ ] 4.3 实现危险协议、脚本、事件属性和不允许标签的拒绝测试
- [ ] 4.4 增加消息图片专用用途并复用 Files 安全验证链路
- [ ] 4.5 实现 JPEG、PNG、WebP、5 MiB 和 4,096 像素边界验证
- [ ] 4.6 实现消息图片读取时的当前消息可见性校验
- [ ] 4.7 实现 HTTPS 公网外链代理、SSRF 防护、重定向限制、超时和 MIME 限制
- [ ] 4.8 验证普通文件接口仍拒绝图片且不暴露消息图片对象

## 5. 用户私信与收件箱

- [ ] 5.1 实现同组织启用用户校验和私信发送用例
- [ ] 5.2 实现收件箱分页、详情、关键词、分类、类型和已读筛选
- [ ] 5.3 实现私信已读、未读总数和按消息类型统计
- [ ] 5.4 实现私信收件关系删除墓碑并保持消息主体不变
- [ ] 5.5 实现普通用户撤销自己发送的私信
- [ ] 5.6 实现动态群发/公告可见性和懒创建用户消息状态
- [ ] 5.7 验证受众变化、撤销、过期和删除后的可见性边界

## 6. 管理员群发、公告与分类

- [ ] 6.1 实现组织、角色和全体用户动态受众解析
- [ ] 6.2 实现普通管理员所属组织及子组织范围校验
- [ ] 6.3 实现超级管理员跨组织复制和分类编码匹配
- [ ] 6.4 实现管理员群发创建、管理查询和发送后不可编辑
- [ ] 6.5 实现公告草稿、定时发布、立即发布和有效期任务
- [ ] 6.6 实现公告发布后编辑、已读保留和撤销终态
- [ ] 6.7 实现组织级分类 CRUD、停用和已引用分类删除保护
- [ ] 6.8 实现组织管理员普通用户消息撤销和超级管理员全局撤销
- [ ] 6.9 验证权限码、消息类型、发送者身份和组织范围组合矩阵

## 7. RabbitMQ Outbox 与消费者

- [ ] 7.1 实现持久化 Topic Exchange、WebSocket Durable Queue、死信 Exchange 和绑定声明
- [ ] 7.2 实现 Outbox Worker 的批量抢占、Publisher Confirm 和发布状态更新
- [ ] 7.3 实现 RabbitMQ 连接失败、Confirm 超时和有界退避重试
- [ ] 7.4 实现消费者手动 ACK、幂等记录和重复事件安全处理
- [ ] 7.5 实现超过重试上限进入 Dead Letter Exchange 和受控运行日志
- [ ] 7.6 验证 RabbitMQ 不可用时消息事务成功、Outbox 保留并恢复后补发
- [ ] 7.7 预留邮件/短信事件发布 Contract，不创建空操作消费者或第三方适配器

## 8. WebSocket Gateway 与恢复

- [ ] 8.1 实现一次性 60 秒 WebSocket ticket 签发和 Redis 摘要存储
- [ ] 8.2 实现 ticket 重放、过期、用户绑定和错误脱敏拒绝
- [ ] 8.3 实现 WebSocket 升级入口和连接生命周期管理
- [ ] 8.4 实现 RabbitMQ 事件写入 24 小时 Redis Stream 恢复缓存
- [ ] 8.5 实现 Redis Pub/Sub 到在线 Gateway 节点的刷新提示分发
- [ ] 8.6 实现游标补发、幂等和超出窗口全量刷新控制事件
- [ ] 8.7 验证 WebSocket 事件不携带正文、Token、图片或外链 URL
- [ ] 8.8 验证断线、重连、撤销、编辑、过期和动态受众变化行为

## 9. HTTP Adapter、路由与 API 元数据

- [ ] 9.1 定义 Messaging DTO、稳定 `MSG_*` 错误和四字段 HTTP 映射
- [ ] 9.2 实现用户私信、收件箱、已读、删除、撤销和图片 HTTP Handler
- [ ] 9.3 实现 WebSocket ticket 和升级 Handler
- [ ] 9.4 实现管理员群发、公告、撤销和分类 HTTP Handler
- [ ] 9.5 为全部 HTTP 和 WebSocket 入口创建完整 Route Descriptor
- [ ] 9.6 增加消息路由 API Metadata 默认 Permission Code 和审计分类
- [ ] 9.7 更新 API 权限同步、OpenAPI Schema 和原生 WebSocket 描述
- [ ] 9.8 验证 Route Catalog、Gin 路由、API Metadata 和权限表集合一致

## 10. 审计、运行日志与安全脱敏

- [ ] 10.1 记录消息发送、编辑、发布、撤销、分类变更和权限拒绝审计元数据
- [ ] 10.2 记录 Outbox、RabbitMQ 发布、消费、重试和死信运行日志
- [ ] 10.3 脱敏 WebSocket ticket、Authorization、消息正文、图片内容和外链 URL
- [ ] 10.4 验证审计和运行日志不泄露 Markdown、HTML、对象存储路径或基础设施错误

## 11. 契约与集成验证

- [ ] 11.1 增加消息 Domain/Application 状态机和权限矩阵测试
- [ ] 11.2 增加消息 HTTP 四字段响应、错误码和 OpenAPI 契约测试
- [ ] 11.3 增加真实 RabbitMQ Publisher Confirm、ACK、重试和死信集成测试
- [ ] 11.4 增加 Outbox 并发抢占、重启补发和事件幂等测试
- [ ] 11.5 增加 WebSocket ticket、游标恢复和超窗全量刷新测试
- [ ] 11.6 增加 Markdown、图片、SSRF、重定向、MIME 和日志脱敏安全测试
- [ ] 11.7 增加跨组织事务、动态受众、未读语义和撤销边界测试
- [ ] 11.8 运行受影响模块测试和完整 Go 测试套件
- [ ] 11.9 更新 CONTEXT、模块导航、OpenSpec 主规格和部署运行说明
