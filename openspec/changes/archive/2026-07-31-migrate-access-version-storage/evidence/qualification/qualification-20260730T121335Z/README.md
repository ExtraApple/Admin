# 三小时授权版本资格验收接续说明

## 本次运行

- Run ID：`qualification-20260730T121335Z`
- 启动时间：2026-07-30 20:13:36 +08:00
- 运行方式：独立 Windows PowerShell 后台进程
- 隔离 MySQL：`127.0.0.1:23306/admin_rehearsal`
- Redis：Docker 容器 `redis`
- 正式门槛：连续至少 3 小时、至少 100,000 次混合操作、至少 8 并发、
  至少 3 次进程重启、最长 15 分钟扫描间隔

凭据没有写入本目录、命令行参数或日志。

## 后续状态查询

在仓库根目录执行：

```powershell
& ".\cmd\access-version-qualification\get-background-status.ps1" `
  -EvidenceDirectory `
  ".\openspec\changes\migrate-access-version-storage\evidence\qualification\qualification-20260730T121335Z"
```

状态含义：

- `running`：资格窗口仍在执行
- `completed`：进程退出码为 0 且存在 `summary.json`
- `failed`：进程退出码非 0，或退出码为 0 但缺少 `summary.json`
- `lost`：包装进程已消失但没有退出码证据

## 最终核验文件

- `policy.json`
- `events.jsonl`
- `phase-01.json` 至 `phase-04.json`
- `phase-01.log` 至 `phase-04.log`
- `launcher.json`
- `launcher-runtime.json`
- `launcher-completion.json`
- `exit-code.txt`
- `summary.json`

只有 `summary.json` 中 `qualified=true`，并且时长、操作数、并发、重启、
周期扫描、故障注入、非预期失败和缺行指标全部达标，才可以将本次运行作为
任务 8.6/9.1 的资格证据。
