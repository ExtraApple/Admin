# Issue Tracker: GitHub

本项目的外部需求和缺陷入口位于 GitHub Issues：

- Repository: `https://github.com/ExtraApple/Admin`
- 使用 `gh` CLI 读取、创建、评论和关闭 Issue。
- Pull Request 默认不作为需求入口。

## 与 OpenSpec 的分工

GitHub Issues 用于记录外部需求、Bug 和讨论入口，不直接取代项目规格。

需求进入开发流程后：

1. 使用 `grill-with-docs` 或 `openspec-explore` 澄清需求。
2. 使用 `openspec-propose` 创建变更。
3. 将 `openspec/changes/<change>/` 作为该变更的设计和任务来源。
4. 使用 `openspec-apply-change` 实现变更。
5. 完成后同步或归档规格。

## 信息来源优先级

发生描述冲突时，按以下顺序处理：

1. 当前变更的 `openspec/changes/<change>/`
2. 当前行为规格 `openspec/specs/`
3. `docs/adr/`
4. `CONTEXT.md`
5. `docs/` 中的实现、运维和背景文档
6. GitHub Issue 中尚未沉淀到 OpenSpec 的讨论

发现冲突时应明确指出，不得静默选择其中一种描述。

## 常用命令

- 创建：`gh issue create`
- 查看：`gh issue view <number> --comments`
- 列表：`gh issue list`
- 评论：`gh issue comment <number> --body "..."`
- 关闭：`gh issue close <number> --comment "..."`

## Pull Requests as a triage surface

**PRs as a request surface: no.**

当 Skill 要求“发布到 issue tracker”时，创建 GitHub Issue；当 Skill 要求“读取相关 ticket”时，使用 `gh issue view <number> --comments`。
