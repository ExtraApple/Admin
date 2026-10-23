## ADDED Requirements

### Requirement: 装配与响应审计证据
本 change SHALL 通过真实 App 组合根、代表性 HTTP／JWT 和规范化快照比较，核对 Route Catalog、Gin 实际业务路由、API Metadata／Permission 同步、OpenAPI paths、依赖接线和响应契约，并 SHALL 将结果按四个 A3 章节记录。

#### Scenario: 快照与实际集合证据
- **WHEN** 审计比较 Route Catalog、Gin、同步结果或 OpenAPI
- **THEN** 审计 SHALL 记录规范化的 Method／Path 集合、技术／原生协议标签、差异项和代码基线
- **AND** 数据库中未声明的 API 记录 SHALL NOT 被视为实际路由

#### Scenario: 访问与响应矩阵证据
- **WHEN** 审计执行代表性 Public、Authenticated、PermissionControlled、技术路由或原生响应 Case
- **THEN** 审计 SHALL 记录认证／权限前置、HTTP 状态、统一 JSON envelope 或原生 Content-Type、稳定错误码、`data` 形状和最终审计结果
- **AND** 未匹配路径、错误 Method、OpenAPI 生成失败和流提交后错误 SHALL 按各自协议边界分类

#### Scenario: 组合根接线证据
- **WHEN** 审计检查 required／optional 依赖、后台任务或技术入口
- **THEN** 审计 SHALL 记录实际 App 注入、启动结果、fallback／禁用语义和代表性运行请求
- **AND** 仅存在接口、字段或测试 fake SHALL NOT 被标记为生产接线通过

### Requirement: 审计型规格不改变装配与响应契约
本 change 的 `assembly-response-audit` delta SHALL 只约束审计工件和证据，不得改变 `route-catalog`、`modular-layered-architecture`、`api-management` 或 `api-response-contract` 的运行时 Requirement、路由、权限码、错误码或响应结构。

#### Scenario: 发现装配或响应偏差
- **WHEN** A3 Case 证明实际路由、权限、错误 owner、OpenAPI 或响应行为偏离当前产品规格
- **THEN** 本 change SHALL 保存去敏偏差证据并标记适用边界
- **AND** 后续产品修复、规格裁决或文档修复 SHALL 通过独立 change 处理

#### Scenario: A3 change 归档
- **WHEN** A3 审计任务完成并归档
- **THEN** 归档 SHALL 使用跳过产品规格同步的流程
- **AND** `assembly-response-audit` SHALL NOT 被合并进产品主规格作为运行时能力
