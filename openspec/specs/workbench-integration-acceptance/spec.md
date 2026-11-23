# workbench-integration-acceptance Specification

## Purpose

定义工作台前后端集成验收的隔离环境、页面与 API 矩阵、角色权限与数据范围、失败恢复、WebSocket 与资源、可访问性和证据要求，确保验收可重复、可复核，并且不把未执行项写成通过。

## Requirements

### Requirement: Isolated workbench integration acceptance environment

前后端集成验收 SHALL 使用独立的 MySQL、Redis、MinIO、Mailpit、RabbitMQ、Go 后端和 Vite，并使用临时配置与独立数据。验收 SHALL NOT 连接生产数据库、生产对象存储或修改仓库默认 `config.yaml`／`.env`。

#### Scenario: Isolated stack starts

- **WHEN** 验收启动
- **THEN** 后端、前端和依赖使用专用端口与独立卷
- **AND** 真实浏览器可完成登录、刷新和受保护页面访问
- **AND** 默认配置与业务代码保持不变

### Requirement: Layered workbench acceptance matrix

验收 SHALL 覆盖所有可见工作台路由的加载与读取、每个模块的代表写链路、前端引用 API 的成功契约、Route Descriptor 的静态元数据、代表性错误契约，以及原生 blob／raw／docs 响应。验收 SHALL NOT 将页面、接口和权限做成无差别全笛卡尔积。

#### Scenario: Page and API coverage

- **WHEN** 验收执行页面矩阵
- **THEN** 每个可见路由至少验证加载、权限渲染、读取结果和空／错误状态
- **AND** 每个模块至少验证一条真实写链路
- **AND** 高影响操作（权限、数据范围、菜单／API 同步、用户角色／组织、邮箱、头像）全部覆盖

#### Scenario: API contract coverage

- **WHEN** 验收执行 API 矩阵
- **THEN** 每个前端引用的 API 至少有一次成功调用并校验统一信封与字段
- **AND** Route Descriptor 的 Method／Path／Access／Permission／Audit／OpenAPI 被静态抽查
- **AND** 原生成功响应不按 JSON 信封解析

### Requirement: Role, permission, and data-scope coverage

验收 SHALL 使用超级管理员、受限管理员、无管理权限用户、停用用户，以及组织范围内／范围外两类数据。

#### Scenario: Permission mismatch detection

- **WHEN** 受限管理员访问未授权页面或 API
- **THEN** 导航隐藏未授权入口，直接访问被页面守卫和 API 403 同时拒绝
- **AND** 范围内数据可见，范围外数据不可见或被拒绝
- **AND** 停用用户的登录和旧会话在后续请求失效

### Requirement: Failure and weak-network recovery

验收 SHALL 覆盖离线、慢网、访问凭据／Refresh 失败、403、422、409、后端断连和 Redis 不可用。预期失败 SHALL 与真实缺陷区分。

#### Scenario: Failure does not report success

- **WHEN** 网络、认证或依赖失败
- **THEN** 页面不显示成功状态或伪造已提交结果
- **AND** 用户输入和必要的重试入口保留
- **AND** 恢复网络／依赖后可重试，旧响应不覆盖新状态

### Requirement: WebSocket and resource integration

消息模块没有前端页面，验收 SHALL 通过浏览器脚本验证 WebSocket ticket、升级、未授权、重连和消息刷新，不新增前端页面。头像与下载资源 SHALL 验证真实 MinIO 读取和错误处理。

#### Scenario: WebSocket lifecycle

- **WHEN** 已认证用户申请 ticket 并升级 WebSocket
- **THEN** ticket 一次性、绑定用户，非法 ticket 被拒绝
- **AND** 断线后可按游标重连，刷新事件可到达
- **AND** 未授权连接不建立协议升级

### Requirement: Accessibility, performance, and evidence

验收 SHALL 对登录、首页、用户列表、用户详情／编辑、个人资料执行 Lighthouse 可访问性检查和性能 trace；所有页面 SHALL 无未解释控制台错误。证据 SHALL 记录环境、矩阵结果、状态码、网络失败、控制台、数据库状态、截图、基线和清理结果。

#### Scenario: Acceptance evidence

- **WHEN** 验收结束
- **THEN** 严重／关键可访问性问题为零
- **AND** 性能基线已记录，不以未验证的数字阈值替代
- **AND** 未执行项明确标记，不写成通过
