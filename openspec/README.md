# OpenSpec 使用说明

这个目录是当前系统行为和未来变更计划的规格源头。

## 目录职责

- `specs/`：当前系统已经具备的真实行为。内容要简洁，重点描述行为和约束。
- `changes/`：未来要做的变更。每个功能或重要行为调整都应该有一个独立 change。
- `../docs/`：实现说明、运维说明、测试步骤、背景材料。
- `../TODO.md`：只保留路线图清单，不再写详细设计。

## 当前规格

- `auth`：验证码、密码策略、登录锁定、JWT、登出黑名单。
- `user-management`：注册、个人资料、密码、头像、初始化上下文、管理员用户操作。
- `rbac`：角色、权限、权限分组、关联分配、权限同步。
- `menu-management`：菜单树、菜单 CRUD、角色/用户菜单可见性、路由同步。
- `file-management`：文件上传、列表、详情、更新、删除、浏览、轮转。
- `logging`：已确认的运行日志行为，以及审计日志的待办边界。
- `dict-management`：字典类型、字典条目和前端字典读取。
- `organization-management`：组织单位 CRUD 和组织树。

## 工作流

已经完成并稳定的行为，维护到 `specs/`。

新功能先创建 change：

```text
/opsx:propose add-operation-audit-logs
```

先审查生成的 proposal、design、tasks 和 delta specs，再进入实现。实现和验证完成后，再 archive 这个 change，把变更合并回主规格。

## 写作规则

- 规格写“系统可观察行为”，不要写成长篇实现教程。
- 保留 `Requirement` 和 `Scenario` 结构，方便 OpenSpec 和 AI 识别。
- 代码片段、Apifox 测试步骤、长篇实现记录放在 `docs/`。
- 如果路由、模型、Redis key、MinIO bucket、权限码会影响行为，要在规格里明确写出来。
