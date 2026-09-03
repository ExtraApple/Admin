# RabbitMQ 与 MySQL 集成门禁

## 前置条件

- RabbitMQ：本机 Docker `rabbitmq` 容器中的独立 vhost `admin_messaging_integration`；专用测试账号只获该 vhost 的 configure/write/read 权限。
- MySQL：本机独立数据库 `admin_messaging_integration`；通过进程环境变量 `ADMIN_TEST_MYSQL_DSN` 连接。
- 凭据仅注入测试进程，未写入此证据文件或版本库。

## 执行命令

```powershell
go test -tags rabbitmq_integration ./internal/messaging/adapters/rabbitmq -run TestRabbitMQMessagingIntegrationDeclaresQuorumTopologyAndConfirmsPublish -count=1

go test -tags mysql_integration ./internal/messaging/adapters/gorm ./internal/messaging/application -run 'TestMySQL(ClaimOutboxSkipsLockedEarlierRow|EventConsumerRollsBackEverySnapshotBatchBeforeRefresh|EventConsumerCompetingClaimsProduceOneCompleteSnapshot)' -count=1
```

## 结果

2026-09-03：两项门禁通过。

- RabbitMQ：真实 Quorum 拓扑声明与 Publisher Confirm 通过。
- MySQL：`FOR UPDATE SKIP LOCKED`、快照批次回滚、竞争 Consumer 快照通过。

首次 MySQL 门禁暴露测试清理顺序错误和固定 Outbox 副本 ID 的残留冲突；已改为在删除测试行后关闭连接，并为每次运行生成唯一副本 ID，再次执行全部 MySQL 门禁通过。

## 清理

- 已删除本次创建的 RabbitMQ vhost 与测试账号。
- 已删除本次创建的 MySQL 测试数据库。
- RabbitMQ Docker 容器已恢复为测试前的停止状态。
