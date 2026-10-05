# authorization-overview Specification

## Purpose

授权总览提供按操作者数据范围裁剪的只读授权摘要、配置风险识别和分页风险核对能力，遵守统一认证、权限、响应和访问边界约束。

## Requirements

### Requirement: 授权总览读取
系统 SHALL 提供受 `admin.authorization.overview.get` 保护的 `GET /api/admin/authorization-overview`，按操作者数据范围返回授权总览。

#### Scenario: 超级管理员读取总览
- **WHEN** 已认证用户拥有 `admin` 角色且请求授权总览
- **THEN** 系统返回其全部可见角色、用户和组织的范围摘要
- **AND** 系统返回按影响范围排序的前 N 个授权配置风险对象
- **AND** 响应使用统一四字段成功信封

#### Scenario: 范围受限管理员读取总览
- **WHEN** 已认证用户拥有总览权限但数据范围不是全部组织
- **THEN** 系统只使用操作者可见的角色、用户和组织关系计算摘要与风险
- **AND** 系统不得通过计数、对象名称、编码或风险详情泄露范围外数据

#### Scenario: 总览无风险
- **WHEN** 操作者可见范围内不存在授权配置风险
- **THEN** 系统返回空的 `risks.items` 数组
- **AND** `risks.total` SHALL 为 `0`
- **AND** 系统 SHALL NOT 伪造风险提示

### Requirement: 授权总览数据契约
授权总览成功数据 SHALL 包含 `scope`、`summary` 和 `risks`，并保持稳定的 JSON 字段语义。

#### Scenario: 返回范围摘要
- **WHEN** 授权总览读取成功
- **THEN** `scope` SHALL 返回稳定的 `data_scope` 和可见组织数量
- **AND** `summary.roles` SHALL 返回角色总数、启用数和已被用户使用数
- **AND** `summary.users` SHALL 返回用户总数、启用数和无角色用户数
- **AND** `summary.organizations` SHALL 返回组织总数和可管理组织数

#### Scenario: 返回风险分页摘要
- **WHEN** 风险对象数量超过总览上限
- **THEN** `risks.items` SHALL 只包含按影响范围排序的前 10 个风险对象
- **AND** `risks.total` SHALL 表示全部风险对象数量
- **AND** `risks.limit` SHALL 表示总览返回上限
- **AND** `risks.has_more` SHALL 表示是否存在未在总览中返回的风险对象

#### Scenario: 返回风险对象
- **WHEN** 一个角色或用户存在一个或多个授权配置风险
- **THEN** 系统 SHALL 将同一目标对象聚合为一个风险对象
- **AND** 风险对象 SHALL 返回目标资源类型、ID、显示名称、问题数量和稳定风险类型集合
- **AND** 风险对象 SHALL 返回潜在受影响用户数、潜在受影响组织数和 `revalidate_on_next_request` 会话规则
- **AND** 风险对象 SHALL NOT 声称每个用户的最终有效权限或在线会话数量

### Requirement: 授权配置风险规则
系统 SHALL 使用统一风险分类规则识别授权配置风险。

#### Scenario: 已分配菜单已停用
- **WHEN** 角色显式分配了状态不是启用的菜单
- **AND** 该角色处于操作者可见范围
- **THEN** 系统将该角色标记为 `disabled_assigned_menu` 风险对象

#### Scenario: 菜单权限码缺失
- **WHEN** 角色显式分配的菜单要求权限码
- **AND** 该角色没有对应权限码
- **THEN** 系统将该角色标记为 `missing_menu_permission` 风险对象

#### Scenario: 已使用启用角色无权限
- **WHEN** 启用角色已经绑定至少一个可见用户
- **AND** 该角色没有任何权限
- **THEN** 系统将该角色标记为 `used_role_without_permissions` 风险对象

#### Scenario: 可管理用户无角色
- **WHEN** 用户处于操作者可管理范围内
- **AND** 用户没有任何角色关系
- **THEN** 系统将该用户标记为 `managed_user_without_role` 风险对象

#### Scenario: 合法空状态不报警
- **WHEN** 启用角色尚未绑定用户，或组织单位没有成员
- **THEN** 系统 SHALL NOT 仅因该空状态创建授权配置风险

### Requirement: 风险排序与分页清单
系统 SHALL 提供受 `admin.authorization.risks.get` 保护的 `GET /api/admin/authorization-risks`，用于分页读取总览之外的完整风险对象。

#### Scenario: 读取风险清单
- **WHEN** 具有风险清单权限的管理员请求合法的 `page` 和 `size`
- **THEN** 系统按相同风险规则和操作者数据范围返回分页风险对象
- **AND** 返回 `list`、`total`、`page` 和 `size`
- **AND** 空结果的 `list` SHALL 序列化为 `[]`

#### Scenario: 筛选风险清单
- **WHEN** 请求提供 `kind`、`resource` 或 `keyword`
- **THEN** 系统只返回符合风险类型、目标资源类型或目标显示字段的风险对象
- **AND** 筛选后的 `total` SHALL 与筛选结果一致
- **AND** 系统不得通过筛选结果泄露范围外对象

#### Scenario: 稳定排序
- **WHEN** 多个风险对象同时满足查询条件
- **THEN** 系统按潜在受影响用户数降序、潜在受影响组织数降序、资源类型和资源 ID 的稳定顺序返回
- **AND** 前端 SHALL NOT 重新计算或覆盖服务端风险排序

### Requirement: 总览和风险清单访问边界
总览和风险清单 SHALL 遵守现有认证、API 元数据、权限码和统一错误响应约束。

#### Scenario: 缺少认证
- **WHEN** 匿名请求任一授权总览接口
- **THEN** 系统返回 HTTP 401 和稳定认证错误信封
- **AND** 系统不返回任何范围、计数或风险数据

#### Scenario: 缺少权限
- **WHEN** 已认证用户不拥有对应总览或风险清单权限
- **THEN** 系统返回 HTTP 403 和稳定授权错误信封
- **AND** 系统不返回部分总览数据

#### Scenario: 读取依赖失败
- **WHEN** 聚合读取在响应提交前遇到未分类内部失败
- **THEN** 系统返回现有统一 5xx 错误信封
- **AND** 响应不得包含数据库、Redis、内部查询或基础设施错误详情

#### Scenario: 读取接口只读
- **WHEN** 客户端调用任一授权总览接口
- **THEN** 系统不得修改角色、权限、菜单、用户、组织、授权版本或审计历史
- **AND** 接口不得提供写入动作或批量修复语义
