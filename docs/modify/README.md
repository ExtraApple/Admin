# modify 架构与修改记录说明

`docs/modify/` 保存跨模块、架构级、权限链路级的设计和历史记录。当前行为不以本目录为事实来源；请优先查看 `openspec/specs/`、`docs/adr/` 和 `docs/runbooks/`。

## 文件索引

### 当前设计

| 文件 | 说明 |
|---|---|
| [`layered-monolith-restructuring.md`](layered-monolith-restructuring.md) | 当前目录分层、模块归属、路由和事务边界 |
| [`project-improvement-roadmap.md`](project-improvement-roadmap.md) | 全项目长期优先级和执行顺序 |

### 背景与历史

| 文件 | 说明 |
|---|---|
| [`authorization-flow-change-log.md`](authorization-flow-change-log.md) | 认证、RBAC、菜单/API 和 Token 失效演进历史 |
| [`mature-admin-system-comparison-recommendations.md`](mature-admin-system-comparison-recommendations.md) | 成熟后台项目对比导航和阶段结论 |
| [`mature-admin-system-comparison-overview.md`](mature-admin-system-comparison-overview.md) | 已有能力、工程化差距和候选模块 |
| [`file-upload-security-recommendations.md`](file-upload-security-recommendations.md) | 文件安全 V1 背景和 V2 候选方向 |

## 维护规则

- 跨多个模块的结构性修改优先写入本目录。
- 当前行为先更新 OpenSpec；本目录只保留设计过程、背景和历史结论。
- 接口路径、请求字段、响应 Schema 和在线调试以 Swagger UI / OpenAPI JSON 为准。
- 已接受且改变成本较高的决策写入 `docs/adr/`。
- 可执行的部署、切换和排障步骤写入 `docs/runbooks/`。
