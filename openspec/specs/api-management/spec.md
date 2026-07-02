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

#### Scenario: 创建 API 元数据
- **WHEN** 管理员提交名称、HTTP 方法和路径
- **THEN** 系统创建 API 元数据
- **AND** `method + path` SHALL 唯一
- **AND** 系统规范化 HTTP 方法和路径

#### Scenario: 修改 API 元数据
- **WHEN** 管理员修改 API 名称、方法、路径、分组、权限码、状态或开关字段
- **THEN** 系统应用提交的字段
- **AND** 如果方法或路径变更后与其他 API 冲突，系统拒绝请求

#### Scenario: 修改已绑定菜单的 API 权限码
- **WHEN** 管理员修改已经通过 `menu_apis` 绑定菜单的 API 权限码
- **THEN** 系统同步更新关联菜单的 `permission_code`
- **AND** 系统同步更新同一菜单绑定的其他 API 权限码
- **AND** 系统确保权限表存在对应权限码

#### Scenario: 删除 API 元数据
- **WHEN** 管理员删除 API 元数据
- **THEN** 系统清理该 API 对应的 `menu_apis` 关联
- **AND** 系统硬删除 API 记录

### Requirement: API 路由同步
系统 SHALL 能够从 Gin 已注册路由同步 API 元数据。

#### Scenario: 同步已注册 API 路由
- **WHEN** 管理员调用 `POST /api/admin/apis/sync`
- **THEN** 系统扫描已注册路由
- **AND** 只处理 `/api/` 前缀下的路由
- **AND** 对已存在的 `method + path` 跳过创建
- **AND** 为需要认证的接口生成默认权限码
- **AND** 对登录、注册、验证码和公开字典接口标记为不需要认证

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
- **AND** 系统创建 `type = 3` 的按钮菜单
- **AND** 系统将按钮菜单权限码设置为 API 权限码
- **AND** 系统写入 `menu_apis` 关联
- **AND** 系统确保权限表存在对应权限码

#### Scenario: API 缺少权限码时生成按钮
- **WHEN** 管理员从没有权限码的 API 生成按钮菜单
- **THEN** 系统根据 `method + path` 自动生成 API 权限码
- **AND** 系统同步写入 API、菜单和权限表

#### Scenario: 公开 API 不能生成按钮权限
- **WHEN** 管理员尝试从 `need_auth = 0` 的 API 生成按钮菜单
- **THEN** 系统拒绝请求
