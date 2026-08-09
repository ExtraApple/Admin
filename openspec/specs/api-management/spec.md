# api-management Specification

## Purpose

API 管理维护后台接口元数据，用于接口分组、启停、权限码绑定、路由同步、动态接口权限校验、菜单按钮联动和后续审计策略扩展。它不动态生成 Gin 路由，真实接口仍由代码注册。

## Requirements

### Requirement: API 元数据 CRUD
系统 SHALL 允许管理员创建、查询、修改和删除 API 元数据记录。

#### Scenario: 查询 API 列表
- **WHEN** 管理员调用 `GET /api/admin/apis`
- **THEN** 系统分页返回 API 元数据列表
- **AND** 支持按关键词、分组、HTTP 方法、状态、认证标记和审计标记筛选

#### Scenario: 查询 API 详情
- **WHEN** 管理员调用 `GET /api/admin/apis/:id`
- **THEN** 系统返回指定 API 元数据详情
- **AND** API 不存在时系统拒绝请求

#### Scenario: 创建 API 元数据
- **WHEN** 管理员提交名称、HTTP 方法和路径
- **THEN** 系统创建 API 元数据
- **AND** `method + path` SHALL 唯一
- **AND** 系统规范化 HTTP 方法和路径

#### Scenario: 修改 API 元数据
- **WHEN** 管理员修改 API 名称、方法、路径、分组、权限码、状态或开关字段
- **THEN** 系统应用提交的字段
- **AND** 如果方法或路径变更后与其他 API 冲突，系统拒绝请求

#### Scenario: API 元数据路径偏离真实路由
- **WHEN** 管理员把 PermissionControlled API 元数据的 Method 或 Path 修改为不再匹配 Route Catalog 真实路由
- **THEN** 真实路由 SHALL 按 API 未配置的现有安全语义拒绝访问
- **AND** 后续路由同步 SHALL 能够为真实 Method 和 Path 重新创建 API 元数据记录

#### Scenario: 修改已绑定菜单的 API 权限码
- **WHEN** 管理员修改已经通过 `menu_apis` 绑定菜单的 API 权限码
- **THEN** 系统 SHALL 在同一数据库事务中同步更新关联菜单和同一菜单绑定的其他 API 权限码
- **AND** 系统 SHALL 确保权限表存在新权限码
- **AND** 新权限码已存在时系统 SHALL 合并并去重旧权限对应的角色授权
- **AND** 旧权限 SHALL 仅在不再被菜单、API 或角色引用时清理
- **AND** 系统 SHALL 在同一事务中使变更前后受影响用户的旧 token 失效

#### Scenario: 修改已绑定 API 权限码失败
- **WHEN** API、菜单、权限、角色授权或 token 版本任一步写入失败
- **THEN** 系统 SHALL 回滚本次权限码变更的全部数据库写入
- **AND** API、菜单、权限、角色授权和用户 token 版本 SHALL 保持事务开始前状态

#### Scenario: 删除 API 元数据
- **WHEN** 管理员删除 API 元数据
- **THEN** 系统清理该 API 对应的 `menu_apis` 关联
- **AND** 系统硬删除 API 记录

### Requirement: API 辅助选项
系统 SHALL 提供 API 管理页面所需的辅助选项。

#### Scenario: 查询 API 分组列表
- **WHEN** 管理员调用 `GET /api/admin/api-groups`
- **THEN** 系统返回当前 API 元数据中的分组列表
- **AND** 每个分组包含 API 数量

#### Scenario: 查询 HTTP 方法列表
- **WHEN** 管理员调用 `GET /api/admin/api-methods`
- **THEN** 系统返回支持的 HTTP 方法选项

### Requirement: 自动化 API 文档
系统 SHALL 根据 Route Catalog 的已校验路由描述和必要的 API Metadata Snapshot 生成 OpenAPI 文档，而不是扫描 Gin Engine。

#### Scenario: 访问 Swagger UI 页面
- **WHEN** `api_docs.enabled = true`
- **AND** 用户访问 `GET /docs`
- **THEN** 系统返回 Swagger UI 页面

#### Scenario: 获取 OpenAPI JSON
- **WHEN** `api_docs.enabled = true`
- **AND** 用户访问 `GET /docs/openapi.json`
- **THEN** 系统返回 OpenAPI 3.0 JSON
- **AND** 文档包含 `/api/` 前缀下的业务接口
- **AND** 文档结合 Route Catalog 与 `apis` 表元数据生成接口标题、分组、描述和认证要求
- **AND** 对已配置 DTO 映射的 JSON 请求接口生成请求体 Schema
- **AND** 上传接口生成 `multipart/form-data` 文件字段
- **AND** API Doc SHALL NOT 扫描 Gin Engine 或查询全局数据库状态

#### Scenario: 关闭自动化 API 文档
- **WHEN** `api_docs.enabled = false`
- **THEN** 系统不注册 `/docs` 和 `/docs/openapi.json` 路由

### Requirement: API 路由同步
系统 SHALL 能够从 Route Catalog 的已校验路由快照同步 API 元数据。

#### Scenario: 同步已声明 API 路由
- **WHEN** 管理员调用 `POST /api/admin/apis/sync`
- **THEN** 系统读取 Route Catalog 中已校验的 Route Descriptor
- **AND** 只处理 `/api/` 前缀下的路由
- **AND** 对已存在的 `method + path` 跳过创建
- **AND** 为需要权限控制的接口生成默认权限码
- **AND** 对 Public 路由以及当前兼容规则指定的公开字典接口标记为不需要权限码检查
- **AND** 对已存在但错误标记为需要认证的 `POST /api/refresh` 纠正 `need_auth` 并清空 `permission_code`
- **AND** 同步流程 SHALL NOT 扫描 Gin Engine 发现路由

#### Scenario: 同步公开 Refresh 元数据
- **WHEN** Route Catalog 中存在 `POST /api/refresh`
- **AND** API Metadata 中已有该 Method 和 Path 但 `need_auth = 1` 或存在 `permission_code`
- **THEN** 同步 SHALL 将其恢复为公开兼容配置
- **AND** 同步 SHALL 设置 `need_auth = 0` 并清空 `permission_code`
- **AND** 系统 SHALL NOT 因该数据库字段配置把 Refresh 路由注册为 PermissionControlled

#### Scenario: 同步已存在 API 元数据
- **WHEN** Route Catalog 的 Method 和 Path 已存在对应 API 元数据
- **THEN** 系统 SHALL 按既有兼容规则保留管理员维护的展示和运行时策略字段
- **AND** 系统 SHALL NOT 因 Catalog 默认值覆盖现有管理员配置

#### Scenario: 同步软删除 API 元数据
- **WHEN** Route Catalog 的 Method 和 Path 只存在软删除 API 元数据
- **THEN** 系统 SHALL 按现有同步规则恢复该记录
- **AND** 系统 SHALL 保持 `method + path` 唯一

### Requirement: API 权限同步
系统 SHALL 能够将 API 元数据中的权限码同步到权限表。

#### Scenario: 同步 API 权限码
- **WHEN** 管理员调用 `POST /api/admin/apis/sync-permissions`
- **THEN** 系统读取需要认证的 API 记录
- **AND** 为不存在于权限表的 API 权限码创建权限记录
- **AND** 跳过不需要认证的公开 API
- **AND** 已存在权限码不会重复创建

### Requirement: API 动态权限校验
系统 SHALL 基于 API 元数据动态校验管理员接口访问权限。

#### Scenario: API 已启用且用户有权限
- **WHEN** 已认证用户请求 `/api/admin` 下的接口
- **AND** API 元数据中 `status = 1`
- **AND** 用户拥有该 API 绑定的 `permission_code`
- **THEN** 系统允许请求继续

#### Scenario: API 未配置
- **WHEN** 已认证用户请求 `/api/admin` 下的接口
- **AND** 系统找不到匹配 `method + c.FullPath()` 的 API 元数据
- **THEN** 系统拒绝请求

#### Scenario: API 已禁用
- **WHEN** 已认证用户请求的 API 元数据 `status != 1`
- **THEN** 系统拒绝请求

#### Scenario: 普通管理员角色缺少权限码
- **WHEN** 已认证用户不是超级管理员
- **AND** API 需要认证
- **AND** API 未绑定权限码或用户不拥有该权限码
- **THEN** 系统拒绝请求

#### Scenario: 超级管理员兜底
- **WHEN** 已认证用户拥有 `admin` 角色
- **AND** 请求不是被禁用的 API
- **THEN** 系统允许请求继续

#### Scenario: 首次同步例外
- **WHEN** `admin` 角色用户调用 `POST /api/admin/apis/sync` 或 `POST /api/admin/apis/sync-permissions`
- **THEN** 系统允许请求用于初始化 API 元数据

### Requirement: API 生成按钮菜单
系统 SHALL 支持从 API 元数据生成按钮菜单，并建立菜单与 API 的绑定关系。

#### Scenario: 从 API 生成按钮菜单
- **WHEN** 管理员调用 `POST /api/admin/apis/:id/menu-button`
- **AND** 提交父级菜单 ID、按钮名称和排序
- **THEN** 系统校验 API 存在、已启用且需要认证
- **AND** 系统校验父级菜单存在
- **AND** 系统在同一数据库事务中创建 `type = 3` 的按钮菜单
- **AND** 系统将按钮菜单权限码设置为 API 权限码
- **AND** 系统写入 `menu_apis` 关联
- **AND** 系统确保权限表存在对应权限码

#### Scenario: API 缺少权限码时生成按钮
- **WHEN** 管理员从没有权限码的 API 生成按钮菜单
- **THEN** 系统根据 `method + path` 自动生成 API 权限码
- **AND** 系统在同一数据库事务中同步写入 API、菜单、`menu_apis` 和权限表

#### Scenario: 生成按钮事务失败
- **WHEN** API 权限码、按钮菜单、`menu_apis` 或权限记录任一步写入失败
- **THEN** 系统 SHALL 回滚本次生成按钮操作的全部数据库写入
- **AND** 系统 SHALL NOT 留下孤立按钮、部分关联或未完成的权限码

#### Scenario: 公开 API 不能生成按钮权限
- **WHEN** 管理员尝试从 `need_auth = 0` 的 API 生成按钮菜单
- **THEN** 系统拒绝请求
