# Domain Docs

本项目采用 single-context 领域文档布局。

## 开始工作前

根据任务范围阅读：

- `CONTEXT.md`：统一领域术语，只记录业务概念，不记录实现细节。
- `docs/adr/`：难以撤销且存在明确取舍的架构决策。
- `openspec/specs/`：系统当前行为的事实来源。
- `openspec/changes/<change>/`：正在实施变更的目标、设计和任务。
- `docs/`：实现说明、运维说明、测试说明和背景资料。

不存在相关文件时可以继续工作，不需要为了形式创建空文档。

## 文件结构

```text
/
├── CONTEXT.md
├── openspec/
│   ├── specs/
│   └── changes/
└── docs/
    ├── agents/
    └── adr/
```

## 文档职责

### CONTEXT.md

只维护领域术语、定义、边界和应避免使用的同义词。

不得把以下内容写入 `CONTEXT.md`：

- API 详细行为
- 数据库结构
- 实现任务
- 临时设计草稿
- 架构决策过程

### OpenSpec

- `openspec/specs/` 描述系统当前行为。
- `openspec/changes/` 描述计划中的行为变化、设计和实施任务。
- 不在 `CONTEXT.md` 中复制 Requirement 和 Scenario。

### ADR

仅当决策同时满足以下条件时创建 ADR：

1. 后续改变成本较高。
2. 缺少背景时会令人困惑。
3. 存在真实的方案取舍。

## 使用统一术语

代码、测试、Issue、OpenSpec 和审查意见应优先使用 `CONTEXT.md` 中定义的术语。

如果实现或新设计与现有 ADR、OpenSpec 或术语定义冲突，必须明确指出冲突。
