# 开发阶段使用 AutoMigrate 与幂等 Seed

项目仍在快速调整模型，因此开发阶段继续使用 GORM `AutoMigrate` 演进表结构，并使用幂等 Go Seed 补齐角色、权限、菜单、API、字典和超级管理员等基础数据。相比立即维护完整 SQL migration，这能降低当前迭代成本；待核心表结构稳定后，应改为由部署流程显式执行可审查、可追踪的正式 migration。
