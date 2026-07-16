# Triage Labels

Matt Pocock Skills 使用五种标准 triage 角色。项目当前直接采用默认标签名称：

| Skill 中的角色 | GitHub 标签 | 含义 |
| --- | --- | --- |
| `needs-triage` | `needs-triage` | 等待维护者评估 |
| `needs-info` | `needs-info` | 等待报告者补充信息 |
| `ready-for-agent` | `ready-for-agent` | 规格完整，可交给 Agent 实现 |
| `ready-for-human` | `ready-for-human` | 需要人工判断或实现 |
| `wontfix` | `wontfix` | 已决定不处理 |

当 Skill 提到标准 triage 角色时，使用表中对应的 GitHub 标签。

如果仓库以后采用不同标签名称，只修改右侧“GitHub 标签”列，不要改变左侧标准角色。

## Category labels

每个完成 triage 的 Issue 还应包含一个分类标签：

| 分类角色 | GitHub 标签 | 含义 |
| --- | --- | --- |
| `bug` | `bug` | 已有行为发生故障 |
| `enhancement` | `enhancement` | 新功能或改进 |

一个完成 triage 的 Issue 应恰好包含一个分类标签和一个状态标签。
