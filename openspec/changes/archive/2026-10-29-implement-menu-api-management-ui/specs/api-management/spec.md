# api-management Specification Delta

## ADDED Requirements

### Requirement: 接口管理页面
正式工作台 SHALL 提供基于真实 API Metadata 的分页接口管理页面，并 SHALL 将 Route Catalog 作为路由事实来源。

- 页面 SHALL 支持关键词、分组、HTTP Method、status、NeedAuth、NeedAudit、RouteState 和分页筛选；`route_state` SHALL 由当前 Route Catalog Snapshot 投影后在服务端过滤。
- Method、Path、NeedAuth、API ID 和 RouteState SHALL 只读。
- 页面 SHALL 不提供手工创建 API；新增 Metadata 通过 Route Catalog 同步产生。
- 页面可编辑名称、分组、permission_code、备注、排序、status 和 need_audit。

#### Scenario: 分页查询接口
- **WHEN** 具有 API 查询权限的管理员打开 `/apis`、设置筛选条件或翻页
- **THEN** 页面请求真实分页接口并显示服务端总数、页码和结果
- **AND** 筛选条件可以从 URL query 恢复
- **AND** 当请求包含 `route_state` 筛选时，返回结果和总数 SHALL 只包含该状态的 API Metadata

#### Scenario: 服务端筛选 RouteState
- **WHEN** 管理员按 `matched`、`metadata_only`、`catalog_only` 或 `access_mismatch` 筛选接口列表
- **THEN** 现有 API 列表接口按当前 Route Catalog Snapshot 服务端过滤
- **AND** 分页总数与结果只反映筛选后的记录

#### Scenario: 查看 RouteState
- **WHEN** API Metadata 返回 `matched`、`metadata_only`、`catalog_only` 或 `access_mismatch`
- **THEN** 页面显示对应文字标签和诊断说明
- **AND** 页面不把 Metadata 当作动态路由来源

#### Scenario: 编辑允许字段
- **WHEN** 管理员编辑名称、分组、权限码、备注、排序、status 或 need_audit
- **THEN** 页面先请求影响预览并展示授权影响
- **AND** 确认后提交 fingerprint
- **AND** 不提供 Method、Path 或 NeedAuth 的编辑控件

### Requirement: 接口管理页面的同步和按钮生成
接口管理页面 SHALL 提供 Route Catalog 同步、API 删除和生成按钮菜单的真实预览提交流程。

#### Scenario: 同步 Route Catalog
- **WHEN** 管理员发起 API 路由同步
- **THEN** 页面展示待新增、跳过、metadata_only、catalog_only、access_mismatch 和冲突摘要
- **AND** 预览不修改 Metadata、权限或授权版本
- **AND** 只有无冲突且用户确认后才能提交 fingerprint

#### Scenario: 从 API 生成按钮菜单
- **WHEN** 管理员为启用且需要认证的 API 选择目录或页面父级、输入按钮名称和排序
- **THEN** 页面展示按钮、API 关系、权限码和用户影响预览
- **AND** 确认后提交 fingerprint 创建按钮
- **AND** 按钮父级非法时不创建任何菜单、关系或权限事实

#### Scenario: 删除接口 Metadata
- **WHEN** 管理员请求删除没有菜单绑定的 API 并确认当前影响预览
- **THEN** 页面提交 fingerprint 并在成功后移除列表记录
- **AND** API 仍有菜单绑定或 fingerprint 过期时显示冲突且不显示成功
