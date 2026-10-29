# menu-management Specification Delta

## ADDED Requirements

### Requirement: 菜单管理页面
正式工作台 SHALL 提供基于真实菜单 API 的菜单树管理页面，并 SHALL 让 UI 字段约束与菜单类型、父级、路径和绑定规则保持一致。

#### Scenario: 查看菜单树和详情
- **WHEN** 具有菜单查询权限的管理员打开 `/menus`
- **THEN** 页面显示真实菜单树、节点类型、状态和排序
- **AND** 选择节点后显示真实详情
- **AND** 刷新或直接打开带 `menu_id` 的地址时重新读取该对象

#### Scenario: 按菜单类型编辑字段
- **WHEN** 管理员创建或修改目录、页面或按钮菜单
- **THEN** 页面分别遵守目录无 path/component、页面要求非空 path、按钮无 path/component且父级为目录或页面的字段约束
- **AND** 页面 SHALL 不把不合法组合提交给用户作为成功候选
- **AND** 最终约束仍由后端服务端校验

#### Scenario: 菜单高影响保存
- **WHEN** 管理员创建、修改或删除菜单
- **THEN** 页面先获取影响预览并展示受影响菜单、API、角色、用户和版本摘要
- **AND** 确认后提交当前 fingerprint
- **AND** 陈旧 fingerprint 返回 `NAV_CONFLICT` 时保留草稿并要求重新预览

### Requirement: 菜单管理页面的绑定与同步工作流
菜单管理页面 SHALL 将 API 绑定、显式清空和路由同步作为可预览、可恢复的独立操作。

#### Scenario: 按钮绑定 API
- **WHEN** 管理员在按钮详情中选择启用且需要认证的 API
- **THEN** 页面展示去重后的目标集合和授权影响
- **AND** 页面提示所选接口的 permission_code 会被统一改为按钮权限码
- **AND** 确认后提交非空 API ID 集合及 fingerprint
- **AND** 目录和页面不提供可执行绑定控件

#### Scenario: 显式清空 API
- **WHEN** 管理员确认清空按钮的 API 绑定
- **THEN** 页面提交 `clear=true` 和当前 fingerprint
- **AND** 空数组不被用于表达清空
- **AND** 菜单自身的 permission_code 不被误清除

#### Scenario: 同步前端路由
- **WHEN** 管理员在菜单页面发起 Route Catalog/前端路由同步
- **THEN** 页面使用的目录来自前端既有路由表，而不是管理员手工输入
- **AND** 页面先展示新增、跳过、未知父级、重复路径、循环或类型冲突
- **AND** 存在冲突时禁止提交
- **AND** 预览无副作用，确认后使用 fingerprint 原子提交
