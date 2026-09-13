## ADDED Requirements

### Requirement: 声明依赖必须由组合根显式装配

`internal/app` SHALL 显式装配每个依赖结构体的 `required` 字段；
依赖结构体 SHALL 为每个字段标注 `required` 或 `optional`，
SHALL NOT 存在未标注的字段。

具体地：

- 依赖结构体 SHALL 与 `Dependencies` 同构，且每个字段 SHALL 带有
  `// wiring: required` 或 `// wiring: optional` 注解。
- 注解 SHALL 说明该字段缺失时的后果与兜底来源。
- `required` 字段 SHALL 在 `internal/app` 的对应字段字面量中显式赋值。
- `optional` 字段仅当构造器存在显式兜底分支且兜底值为安全默认时方可标注。
- 系统 SHALL 在自动化架构测试中校验上述约束，
  SHALL NOT 依赖人工检查或代码评审发现遗漏。

#### Scenario: required 字段已在组合根装配

- **WHEN** 依赖结构体的某字段标注为 `required`
- **AND** 该字段在 `internal/app` 的字段字面量中被显式赋值
- **THEN** 架构测试 SHALL 通过

#### Scenario: required 字段漏装配

- **WHEN** 依赖结构体的某字段标注为 `required`
- **AND** 该字段未出现在 `internal/app` 的任何字段字面量中
- **THEN** 架构测试 SHALL 失败
- **AND** 失败信息 SHALL 指明模块与字段名

#### Scenario: 字段缺少接线注解

- **WHEN** 依赖结构体存在未标注 `required` 或 `optional` 的字段
- **THEN** 架构测试 SHALL 失败
- **AND** 系统 SHALL NOT 为该字段假定默认语义

#### Scenario: 注解书写不符合约定

- **WHEN** 字段注解不是精确的 `// wiring: required` 或 `// wiring: optional`
- **THEN** 架构测试 SHALL 失败
- **AND** 系统 SHALL NOT 把拼写偏差解释为其他语义

#### Scenario: optional 字段省略装配

- **WHEN** 依赖结构体的某字段标注为 `optional`
- **AND** 构造器对该字段存在显式兜底分支
- **AND** 组合根未显式赋值该字段
- **THEN** 架构测试 SHALL 通过
- **AND** 构造器 SHALL 使用兜底值继续工作

#### Scenario: 别名或位置参数形式绕过校验

- **WHEN** 存在依赖结构体的类型别名声明
- **OR** 组合根使用位置参数形式的依赖字面量
- **THEN** 架构测试 SHALL 失败
- **AND** 系统 SHALL NOT 静默跳过该结构体的校验
