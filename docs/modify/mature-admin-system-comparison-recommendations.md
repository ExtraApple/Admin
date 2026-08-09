# 成熟后台项目对比与改进建议

## 修改时间

2026-07-24

## 文档说明

原文档同时包含项目现状、能力差距、长期路线图和文件上传安全设计，内容增长后不利于快速定位。当前结构性结论已经进一步整合到 [layered-monolith-restructuring](layered-monolith-restructuring.md)，本文件只保留历史导航和阶段结论。

## 建议阅读顺序

1. [layered-monolith-restructuring](layered-monolith-restructuring.md)
   - 当前目录问题。
   - 模块化分层目标。
   - 依赖方向、迁移阶段和验收标准。
2. [mature-admin-system-comparison-overview](mature-admin-system-comparison-overview.md)
   - 当前已有基础。
   - 与成熟后台项目的主要差距。
   - 适合后续补充的业务模块。
3. [project-improvement-roadmap](project-improvement-roadmap.md)
   - 阶段划分。
   - 前置依赖和完成标准。
   - 精确执行顺序。
4. [file-upload-security-recommendations](file-upload-security-recommendations.md)
   - 已实现并归档的文件上传安全 V1。
   - 普通文件和头像安全策略。
   - 历史文件、审计、配置及 V1/V2 边界。
   - ClamAV、隔离区、限流、配额等后续优化。

## 当前结论

- 文件上传安全 V1 已完成实现、验收、主规格同步和 change 归档。
- 当前应进入核心回归测试保护阶段，再补可观测能力，并按模块化分层方案调整项目结构。
- 项目继续采用模块化单体，不进行微服务拆分。
- 目录结构采用“业务模块优先、复杂模块内部再分层”。
- 文件上传安全 V2 保留外部恶意文件扫描、隔离区、限流、配额和批量上传等独立 change。
- 首页、数据导入导出、岗位、内部消息等业务模块放在底盘稳定之后。
- migrations、代码生成器和 PostgreSQL 属于更后期的工程化工作。

## 文档维护边界

- 项目能力发生变化时，更新“对比概览”。
- 优先级、前置依赖或执行顺序变化时，更新“项目优化路线图”。
- 目录、模块边界、依赖方向和迁移阶段变化时，更新“项目分层重构方案”。
- 文件上传安全访谈、设计边界或验收标准变化时，更新“文件上传安全优化建议”。
- 已稳定落地的行为以 OpenSpec 为准，`docs/modify` 不替代行为规格。
