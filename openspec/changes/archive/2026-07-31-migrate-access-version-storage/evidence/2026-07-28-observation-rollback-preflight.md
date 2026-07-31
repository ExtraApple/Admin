# 2026-07-28 授权版本观察期停机回滚预检

状态：通过

执行时间：2026-07-28 23:00:23 +08:00

## 验证环境

- 数据库：Windows 本地隔离真实 MySQL，`127.0.0.1:23306`，
  数据库 `admin_rehearsal`。
- 既有 Windows `MySQL80`（`127.0.0.1:3306`）未用于测试、未重启、
  未修改。
- Docker 未运行 MySQL。
- 数据库凭据未输出、未写入仓库；密码只从
  `%TEMP%\admin-access-version-rehearsal-20260728\.env.shared` 注入当前
  PowerShell 测试进程。
- 原始测试输出保存在同一 `%TEMP%` 隔离目录，未提交到仓库。

## 执行方式

预检模式：只读

可执行脚本：`run-access-version-rollback-preflight.ps1`

在停止写流量并摘除全部新实例后，Runbook 要求执行：

```powershell
.\initialize\testutil\run-access-version-rollback-preflight.ps1
```

脚本运行带有 `mysql_integration,rollback_preflight` 构建标签的真实 MySQL
门禁。门禁只读取持久化迁移状态、新旧授权版本和用户记录，不修改数据库。

## 结果

- 迁移状态：completed
- `cutoff_user_id`：`1`
- 一致性扫描差异数：0
- cutoff 后合法懒初始化缺行数：`0`
- 回滚门禁：通过

门禁输出：

```text
rollback preflight passed: cutoff_user_id=1 consistency_differences=0 legal_post_cutoff_missing=0
```

## 回滚边界

本次验证证明，在演练数据库当前状态下，只有当迁移状态完整且一致性扫描差异数
为 0 时，预检门禁才允许进入启动 `legacy-users-token-version` 回滚制品的审批
步骤。预检本身不执行流量切换、不启动旧版本，也不代表已经批准或实施生产回滚。
