# menu-management Specification Delta

## MODIFIED Requirements

### Requirement: 菜单 CRUD

系统 SHALL 允许管理员创建、修改和删除菜单记录，并 SHALL 在创建和修改时统一校验菜单类型、状态和类型相关字段。

- `type=1` 表示目录，`type=2` 表示页面菜单，`type=3` 表示按钮菜单。
- `status` 只能为 `0` 或 `1`；显式非法值 SHALL 被拒绝，不再按默认启用静默归一化。
- 目录的 `path`、`component` SHALL 为空且不能拥有菜单 API 绑定。
- 页面 SHALL 具有唯一的非空 `path`，`component` 可以为空且不能拥有菜单 API 绑定。
- 按钮的 `path`、`component` SHALL 为空；按钮的父级 SHALL 是目录或页面。
- `name` 不能显式为空或全为空白；路径冲突、父级不存在、自引用、循环父级和类型交叉约束 SHALL 拒绝整个请求。
- 既有无效历史记录可以被读取，但系统 SHALL NOT 在启动或普通读取时静默修复、删除或改变它们。
- 真实菜单字段、关系或派生权限变化仍 SHALL 按现有规则在同一事务内处理授权版本；合法同值操作不提升版本。

#### Scenario: 创建目录

- **WHEN** 管理员提交合法目录名称、父级、图标、排序和状态
- **THEN** 系统创建 `type=1` 的目录
- **AND** 系统拒绝非空 path、component 或 API 绑定字段

#### Scenario: 创建页面

- **WHEN** 管理员提交合法页面名称和唯一非空 path
- **THEN** 系统创建 `type=2` 的页面菜单
- **AND** component 可以为空

#### Scenario: 创建按钮

- **WHEN** 管理员提交 `type=3` 的按钮菜单
- **THEN** 系统允许 path 和 component 为空
- **AND** 系统只允许目录或页面作为父级

#### Scenario: 非法类型或状态

- **WHEN** 创建或修改请求提交不在允许范围内的 type 或 status
- **THEN** 系统返回菜单验证错误
- **AND** 菜单、关系和授权版本保持不变

#### Scenario: 目录或页面绑定 API

- **WHEN** 管理员尝试为目录或页面创建或替换 API 绑定
- **THEN** 系统返回菜单验证错误
- **AND** 不创建或修改任何 menu/API、权限、角色授权或版本事实

#### Scenario: 删除菜单

- **WHEN** 管理员删除没有子菜单的合法菜单
- **THEN** 系统清理该菜单的角色菜单和 menu/API 关系并硬删除菜单
- **AND** 所有现有授权版本处理仍在同一事务中完成

- **WHEN** 管理员删除仍有子菜单的菜单
- **THEN** 系统返回 `NAV_CONFLICT`
- **AND** 不改变任何配置事实

### Requirement: 菜单 API 联动

系统 SHALL 允许按钮菜单绑定 API 元数据，并 SHALL 保持菜单权限码、API 权限码、角色授权和受影响用户授权版本的一致性。

- 新绑定目标必须存在、未删除、已启用且 `need_auth=1`。
- 原始 API ID 列表中的 `0`、不存在 ID 或不满足绑定条件的目标 SHALL 使整个请求失败；合法重复 ID 可以去重。
- 非清空绑定请求的 API 集合必须非空；空数组仍不是解绑语义。
- 现有 `POST /api/admin/menus/:id/apis` SHALL 支持显式 `clear=true` 模式。该模式要求 API ID 和手工权限码为空，并清理全部 menu/API 关系；菜单自身独立的 permission_code 不因解绑被静默清空。
- 绑定、清空、权限码联动、权限引用清理、角色授权合并和适用用户版本 SHALL 处于同一事务；无变化的清空 SHALL 成功且不提升版本。
- 绑定、清空和权限码变化等高影响写入 SHALL 遵循 `menu-api-impact-preview` 的 fingerprint 约束。

#### Scenario: 绑定 API

- **WHEN** 管理员为按钮提交一个或多个有效且启用的认证 API
- **THEN** 系统覆盖写入目标 menu/API 关系
- **AND** 系统同步解析最终权限码、权限记录、角色授权和适用用户版本
- **AND** 失败时回滚全部关系、权限和版本写入

#### Scenario: 显式解除全部绑定

- **WHEN** 管理员提交 `clear=true` 且当前按钮存在 API 绑定
- **THEN** 系统删除全部 menu/API 关系
- **AND** 只清理不再被引用的派生权限
- **AND** 按现有受影响角色规则处理授权版本
- **AND** 返回成功且不暴露底层数据库错误

#### Scenario: 清空已经没有绑定的按钮

- **WHEN** 管理员提交 `clear=true` 且当前菜单没有 API 绑定
- **THEN** 系统返回成功
- **AND** 不重建关系、不清理无关权限、不提升授权版本

#### Scenario: 绑定停用或公开 API

- **WHEN** 绑定目标不存在、已删除、已停用或 `need_auth=0`
- **THEN** 系统拒绝整个请求
- **AND** 保持菜单、API、权限、角色授权和版本事实不变

### Requirement: 前端路由同步菜单

系统 SHALL 根据提交的路由元数据创建缺失菜单，并 SHALL 在写入前完成整个批次的规范化和父级解析。

- 已存在路径 SHALL 分类为 skip，不覆盖管理员维护字段。
- 同一批次重复路径、未知 `parent_path`、父级循环、非法 type 或违反菜单类型字段规则 SHALL 返回稳定冲突/验证错误。
- 同一批次中新项可以引用另一个新项；系统 SHALL 按稳定输入顺序拓扑排序后创建。
- 任何一项不可解析或后续事务失败时，整个批次 SHALL 回滚，不保留部分创建或版本提升。
- 合法全 skip 且无派生写入 SHALL 返回创建数 0 且不提升版本。
- 同步预览和正式同步 SHALL 使用同一计划构建器，并遵循 fingerprint 约束。

#### Scenario: 同步遇到未知父级

- **WHEN** 路由项的 `parent_path` 既不匹配现有菜单，也不匹配本批次可解析的新菜单
- **THEN** 系统返回 `NAV_CONFLICT`
- **AND** 系统 SHALL NOT 将该项挂到根节点
- **AND** 同批次其他项也 SHALL NOT 留下部分写入

#### Scenario: 同步成功创建缺失菜单

- **WHEN** 所有路由项均可解析且目标路径不存在
- **THEN** 系统按拓扑顺序创建缺失菜单
- **AND** 返回实际创建数量
- **AND** 相关授权版本更新与菜单写入保持事务一致
