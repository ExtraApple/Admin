## 1. 共享契约与预览基础

- [x] 1.1 在 Navigation 与 API Metadata Application 层定义菜单/API 影响摘要、预览操作和无状态 SHA-256 fingerprint 的最小类型，不引入新的持久化状态。
- [x] 1.2 扩展菜单/API HTTP DTO，加入显式 `clear`、`expected_fingerprint`、预览操作和安全影响摘要字段，并补齐长度、格式和互斥校验。
- [x] 1.3 统一预览与正式写入的规范化目标、依赖发现、受影响角色/用户计算和冲突分类，确保两条路径使用同一业务事实构建器。

## 2. Navigation 菜单与绑定契约

- [x] 2.1 在菜单创建和修改服务中落实目录/页面/按钮的 type、status、path、component、parent 交叉校验，并保持历史非法记录可读取且不静默迁移。
- [x] 2.2 在菜单 API 绑定和按钮生成调用链中强制仅按钮可绑定 API、按钮父级只能为目录或页面，同时保留启用且需要认证的 API 前提。
- [x] 2.3 为现有 `POST /api/admin/menus/:id/apis` 实现显式 `clear=true` 模式，事务内清理 menu/API 关系和无引用权限；覆盖已清空 no-op 与失败回滚。
- [x] 2.4 重构菜单同步的规范化计划，检测重复路径、未知 parent_path、父级循环和非法类型，支持批次内拓扑排序并保证整批原子提交。
- [x] 2.5 将菜单创建、修改、删除、绑定/清空和同步接入 fingerprint 预览与提交前复核，过期事实返回 `NAV_CONFLICT` 且不产生部分写入。
- [x] 2.6 保持菜单权限码、角色授权、适用用户 access version 和提交后缓存失效的既有事务边界，补充清空与真实无变化的版本断言。

## 3. API Metadata 与 Route Catalog 诊断

- [x] 3.1 使用当前 Route Catalog Snapshot 为 API Metadata 列表、详情和同步预览投影 `matched`、`metadata_only`、`catalog_only`、`access_mismatch` 状态，不持久化诊断结果。
- [x] 3.2 实现 API Metadata 影响预览，覆盖元数据高影响修改、删除、路由同步、权限同步和按钮生成，并返回安全影响摘要与 fingerprint。
- [x] 3.3 将 API 权限码、状态、删除、同步、权限同步和按钮生成接入 fingerprint 复核，保持 API Metadata、菜单、权限、角色授权和用户版本的原子回滚。
- [x] 3.4 为 API 生成按钮补充父级必须为目录或页面的校验，并保持公开/停用 API 拒绝、权限记录确保和事务失败回滚。
- [x] 3.5 保持 API 路由同步的 Route Catalog 唯一来源、已有管理员策略字段保留和最终提交统计准确，增加预览后的陈旧冲突处理。

## 4. HTTP、组合根与调用方迁移

- [x] 4.1 在菜单和 API HTTP Routes 中注册影响预览入口，复用现有 `admin.menus.get`/`admin.apis.get` 权限并完整声明请求、响应、错误和 OpenAPI Schema。
- [x] 4.2 将新增应用 Contract、RouteSource 投影和预览服务在唯一 App 组合根中接线，保持模块不直接依赖 Gin、ORM、Redis 或其他 Adapter。
- [x] 4.3 迁移仓库内所有菜单/API 写入调用方、测试夹具和集成客户端以提交 `expected_fingerprint` 或显式 `clear`，不保留旧协议兼容别名或静默重放。

## 5. 回归与真实契约验证

- [x] 5.1 增加菜单类型矩阵、父级/路径、显式清空、无变化 no-op 和同步图解析的 Application 回归测试。
- [x] 5.2 增加菜单 HTTP/Route Catalog 测试，覆盖预览无副作用、稳定错误码、OpenAPI Schema、目录/页面绑定拒绝和未知 parent_path 冲突。
- [x] 5.3 增加 API Metadata Route Catalog 状态、预览 fingerprint、陈旧冲突、按钮父级和同步统计的 Application/HTTP 回归测试。
- [x] 5.4 增加跨模块事务测试，覆盖权限码联动、角色授权合并、版本提升、清空清理和任一步失败的完整回滚。
- [x] 5.5 在真实 MySQL/HTTP 隔离环境验证预览→提交、并发陈旧 fingerprint、菜单/API 绑定边界、Route Catalog 漂移和旧凭据失效语义。

## 6. 文档与变更验收

- [x] 6.1 更新菜单管理、API Metadata 和菜单/API 联动模块文档，记录 clear、预览 fingerprint、类型边界、Route Catalog 状态和无迁移约束。
- [x] 6.2 运行 OpenSpec change 严格校验、主规格严格校验和受影响 Go 测试门禁，确认任务、规格、OpenAPI 和实现保持一致。