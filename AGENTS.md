# AGENTS.md

## General instructions

After completing each task, provide a final summary.

Summary rules:

- If the final summary is already in Chinese, keep it unchanged.
- If the final summary is in another language, append a complete Chinese summary.

Command execution rules:

- Prefer PowerShell for commands on Windows.
- If a command cannot run correctly in PowerShell because of shell compatibility, quoting, or environment differences, retry it with `cmd.exe`.

## Agent skills

### Issue tracker

需求和缺陷入口使用 GitHub Issues；进入开发后，以 OpenSpec 变更文件作为实现依据。详见 `docs/agents/issue-tracker.md`。

### Triage labels

使用 Matt Pocock Skills 默认的五类 triage 标签。详见 `docs/agents/triage-labels.md`。

### Domain docs

本项目采用 single-context：领域术语位于根目录 `CONTEXT.md`，架构决策位于 `docs/adr/`。详见 `docs/agents/domain.md`。
