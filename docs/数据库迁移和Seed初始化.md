# 数据库迁移和 Seed 初始化

## 模块定位

数据库迁移和 Seed 初始化用于解决新环境部署、版本升级、基础数据补齐时的可重复执行问题。

当前项目已经有 GORM `AutoMigrate`，它适合开发阶段快速创建表结构，但不适合长期承担完整数据库版本管理。

后续推荐把数据库初始化拆成两层：

```text
Migration：管理表结构变化
Seed：管理基础业务数据
```

## 当前实现状态

当前已经完成第一阶段：Go 代码版 Seed 基础化。

已实现入口：

```text
seed.Run(conf, routeList)
```

启动时机：

```text
InitMysql
InitRedis
InitMinio
InitRouter
seed.Run
RunServer
```

当前 Seed 会自动补齐：

- 默认角色：`admin`、`user`。
- 默认权限分组：`auth`、`user`、`role`、`permission`、`menu`、`api`、`organization`、`dict`、`file`、`audit`、`system`。
- 默认字典类型和字典项：用户状态、角色状态、菜单类型、HTTP 方法、数据范围。
- 当前 Gin 路由对应的 API 元数据。
- API 对应权限码。
- 默认后台菜单。
- admin 角色拥有全部权限。
- admin 角色拥有全部菜单。
- 配置中的超级管理员账号。

当前仍保留 GORM `AutoMigrate` 负责表结构自动迁移，暂未引入正式 SQL migration 工具。

## 授权版本存储（永久状态）

`migrate-access-version-storage` 已完成退出里程碑。Authorization 拥有的
`user_access_versions` 是 Access Token 和 Refresh Token 授权版本的唯一持久化事实来源。

当前启动流程只通过 GORM AutoMigrate 声明长期模型：

- 创建或校正 `user_access_versions`；
- 不声明、读取或重建已经删除的 `users.token_version`；
- 不创建已经退役的 `access_version_migration_states`；
- 不再运行旧字段回填、数据库级迁移锁、一致性观察或旧版本回滚预检。

`user_access_versions` 的长期行为：

- 新用户首次登录通过 `EnsureVersion` 幂等初始化为版本 1；
- 用户禁用、软删除、Kick、密码或授权关系变化通过
  `EnsureAndIncrement` 创建或提升版本；
- 用户软删除后版本记录保留，恢复用户不会使删除前 Token 重新生效；
- 登录、Refresh Token 和 JWT 校验均只读取该表，缺行时拒绝 Token，
  不回退到 Identity 的用户记录。

Seed 不创建、扫描或修复 `user_access_versions`。历史停机切换、旧字段镜像、
三小时资格验收、删列 DDL 和迁移状态清理证据保留在
`openspec/changes/migrate-access-version-storage/evidence/`；它们不再是当前启动能力。

## 当前问题

当前项目已有这些基础能力：

- GORM 自动迁移模型表。
- 启动时通过 Seed 兜底创建超级管理员。
- 启动时通过 Seed 同步 API 路由。
- 启动时通过 Seed 同步 API 权限码。
- 菜单、角色、权限等模块已经具备管理接口。

但新环境部署时仍然存在几个问题：

- 表结构变化没有版本号记录。
- 某次字段修改是否已经执行过，不容易判断。
- 默认角色、权限、菜单、API 已有基础 Seed，但还没有版本化 Seed 记录。
- 多人开发时，不同数据库状态可能不一致。
- 生产环境不适合盲目依赖 `AutoMigrate` 自动改表。

## 目标

最终目标是做到：

```text
新环境启动
  -> 执行数据库迁移
  -> seed.Run 初始化基础数据
  -> seed.Run 创建超级管理员
  -> seed.Run 同步 API / 权限 / 菜单
  -> 服务可直接使用
```

并且要求：

- 可重复执行。
- 可追踪版本。
- 执行失败能定位。
- 不重复插入基础数据。
- 不误删用户已有业务数据。

## 推荐目录结构

建议后续新增：

```text
migrations/
  000001_init_schema.up.sql
  000001_init_schema.down.sql
  000002_create_user_access_versions.up.sql
  000002_create_user_access_versions.down.sql

seed/
  seed.go          # seed.Run 总入口和执行流程
  defaults.go      # 默认角色、菜单、字典、权限分组等基础数据
  types.go         # Seed 内部 DTO / Summary 结构

initialize/
  migration.go     # 后续引入 migration 工具时再添加
```

如果暂时不想引入 SQL 文件，也可以先使用 Go 代码实现 Seed，迁移仍然保留 GORM `AutoMigrate`，后续再切换到正式 migration 工具。

## Migration 迁移

### 作用

Migration 负责表结构变化：

- 创建表。
- 新增字段。
- 修改索引。
- 新增唯一约束。
- 新增关联表。
- 修改字段类型。
- 必要时迁移旧数据。

### 不建议放入 Migration 的内容

- 默认管理员账号。
- 默认角色。
- 默认菜单。
- 默认权限。
- 业务测试数据。

这些应该放到 Seed。

### 推荐工具

Go 项目常见选择：

| 工具 | 说明 |
|---|---|
| `golang-migrate/migrate` | SQL 文件迁移工具，简单直接，生产常用 |
| `pressly/goose` | SQL 和 Go 迁移都支持，使用方便 |
| `Atlas` | 能做 schema diff，能力强，但学习成本更高 |
| GORM `AutoMigrate` | 开发方便，但版本控制能力弱 |

当前项目建议优先考虑：

```text
goose 或 golang-migrate
```

原因是项目还在快速变化阶段，SQL 迁移文件更直观，也更容易在生产环境审查。

### 当前判断

当前阶段不建议马上把 `AutoMigrate` 替换成正式 migration 工具。

原因是项目仍在快速补模块，表结构还会继续调整。如果现在就把每次字段变化都固化成 SQL migration，短期会增加很多维护成本，反而容易打断功能建设节奏。

更合适的做法是分两步：

```text
开发期：继续使用 AutoMigrate 快速迭代表结构
稳定期：引入 goose 或 golang-migrate 管理结构版本
```

也就是说，当前真正优先要补的不是 migration 工具，而是 Seed 初始化。因为项目现在最明显的问题不是“表创建不出来”，而是新环境启动后仍然需要手动补：

- 默认角色。
- 默认权限。
- 默认菜单。
- 默认 API 元数据。
- 默认角色权限关系。
- 默认角色菜单关系。
- 超级管理员账号。

这些数据如果没有 Seed，每次换环境都会依赖手动同步接口或手动配置，容易漏步骤。

### 我的落地建议

短期保持：

```text
AutoMigrate + Go 代码 Seed
```

这个组合适合当前项目阶段：

- `AutoMigrate` 负责开发期快速补表和字段。
- `Seed` 负责把基础业务数据补齐。
- 每个 Seed 函数都写成幂等逻辑，重复执行不会产生脏数据。
- 等核心表结构稳定后，再把表结构变更沉淀为 SQL migration。

中期再切换为：

```text
goose/golang-migrate + Seed
```

到这个阶段，表结构变更要从“启动时自动处理”变成“部署前显式执行”：

```text
提交 SQL migration
代码评审时一起审 SQL
部署时执行 migration up
成功后启动服务
```

长期生产环境不建议服务启动时自动改表。生产数据库结构变更应该是一件明确、可审查、可回滚的部署动作。

### 不建议现在做的事

当前不建议一次性做这些：

- 立刻移除 `AutoMigrate`。
- 手写完整首版全量建表 SQL。
- 给每一次开发期字段调整都补 migration。
- 在 Seed 中强制覆盖用户已经修改过的菜单、角色、权限名称。
- 在服务启动时自动删除生产库里“代码里已经没有”的基础数据。

这些事情更适合等项目核心模块稳定以后再做。

当前更稳的路线是先让新环境做到：

```text
拉代码
配置环境变量
启动服务
自动补齐基础数据
超级管理员可登录
API / 权限 / 菜单可用
```

只要这条链路稳定，后面接 migration 工具会更自然。

## Seed 初始化

### 作用

Seed 负责基础数据：

- 默认超级管理员角色。
- 默认普通用户角色。
- 默认权限分组。
- 默认权限码。
- 默认菜单树。
- 默认 API 元数据。
- 默认字典类型和字典项。
- 必要的系统配置项。

### 基本原则

Seed 必须是幂等的。

也就是说，同一个 Seed 执行多次，结果应该稳定：

```text
第一次执行：创建缺失数据
第二次执行：发现已存在，跳过或更新必要字段
第三次执行：仍然不会重复插入
```

### 推荐写法

每类基础数据单独一个函数：

```text
SeedRoles()
SeedPermissions()
SeedMenus()
SeedAPIs()
SeedAdmin()
SeedDicts()
```

启动入口统一调用：

```text
seed.Run()
```

推荐执行顺序：

```text
1. SeedRoles
2. SeedPermissionGroups
3. SeedDicts
4. SeedAPIsFromRoutes
5. SeedPermissionsFromAPIs
6. SeedMenus
7. SeedRolePermissions
8. SeedRoleMenus
9. SeedAdmin
```

## 和当前模块的关系

### 超级管理员

当前超级管理员兜底初始化已经纳入 Seed 体系：

```text
SeedAdmin
  -> 检查 admin 角色
  -> 检查是否存在启用的 admin 用户
  -> 不存在则用配置创建
```

这样管理员初始化逻辑的位置更清晰。

### API 管理

当前 API 管理已经在启动 Seed 中自动执行：

```text
SeedAPIsFromRoutes()
SeedPermissionsFromAPIs()
```

手动接口仍然保留：

```http
POST /api/admin/apis/sync
POST /api/admin/apis/sync-permissions
```

它们主要用于调试、数据修复或不重启服务时主动补齐 API 元数据。

### 菜单管理

菜单数据建议由 Seed 初始化一套基础后台菜单：

```text
系统管理
  - 用户管理
  - 角色管理
  - 权限管理
  - 菜单管理
  - API 管理
  - 字典管理
  - 组织管理
  - 操作日志
```

按钮级菜单可以结合 API 元数据生成。

### 权限管理

权限码来源建议统一：

```text
API 权限码
菜单按钮权限码
手动业务权限码
```

Seed 时要避免重复创建相同 `code`。

### 字典管理

字典适合 Seed 默认类型：

```text
user_status
role_status
menu_type
api_method
data_scope
```

## 启动流程建议

当前方案：

```text
InitConfig
InitLogger
InitMysql
AutoMigrate
InitRedis
InitMinio
InitRouter
seed.Run
```

中期方案：

```text
InitConfig
InitLogger
InitMysql
RunMigrations
seed.Run
InitRedis
InitMinio
InitRouter
```

长期方案：

```text
部署阶段执行 migration
服务启动阶段只执行必要 Seed 校验
服务运行阶段不自动改表
```

生产环境更推荐长期方案：表结构变更应该由部署流程显式执行，而不是服务启动时自动执行。

## 数据表建议

如果使用迁移工具，它通常会自动维护版本表，例如：

```text
schema_migrations
```

如果自己实现，需要一张表记录 Seed 执行状态：

```text
seed_records
```

字段建议：

| 字段 | 说明 |
|---|---|
| `id` | 主键 |
| `name` | Seed 名称 |
| `version` | Seed 版本 |
| `checksum` | 内容校验值 |
| `executed_at` | 执行时间 |
| `status` | 执行状态 |

不过早期不一定要做这么复杂。可以先保证每个 Seed 函数本身幂等。

## 风险点

### 1. 不要覆盖用户修改过的数据

Seed 更新基础数据时要谨慎。

例如菜单名称、排序、角色描述，用户可能已经在后台改过。Seed 不应该每次启动都强制覆盖。

建议规则：

```text
不存在则创建
存在则只补关键字段
用户可编辑字段不强制覆盖
```

这里的“用户可编辑字段不强制覆盖”指的是：Seed 只负责补齐系统运行必须存在的数据，不应该每次启动都把用户在后台已经调整过的内容改回默认值。

例如 Seed 默认创建一个菜单：

```text
name: API 管理
path: /system/apis
permission_code: admin.apis.get
sort: 10
```

如果后续管理员在后台把菜单改成：

```text
name: 接口管理
sort: 3
```

Seed 再次执行时不应该强制把 `name` 改回 `API 管理`，也不应该强制把 `sort` 改回 `10`。否则 Seed 会变成“重置用户配置”，而不是“补齐基础数据”。

通常可以按下面的边界处理：

| 字段类型 | 示例 | Seed 行为 |
|---|---|---|
| 系统识别字段 | `code`、`path`、`method`、`permission_code` | 可以创建时写入，必要时补齐 |
| 用户展示字段 | `name`、`nickname`、`description`、`remark`、`icon` | 已存在时不强制覆盖 |
| 用户排序/状态字段 | `sort`、`status` | 默认不强制覆盖，除非是安全兜底场景 |
| 关联关系 | 角色-权限、角色-菜单、菜单-API | 可以补缺失关系，但不要随意删除已有关系 |

代码逻辑上建议这样写：

```text
如果不存在：
  创建默认数据

如果已存在：
  只补系统必须字段或缺失关联
  不覆盖用户可编辑字段
```

例如菜单 Seed 可以补齐缺失的 `permission_code`，但不主动覆盖用户调整过的 `name`、`sort`、`remark`。

### 2. 不要删除生产数据

Seed 不应该做危险删除。

比如某个默认菜单从代码里移除，不代表生产数据库也要自动删除。

### 3. 权限码不能随意改名

权限码一旦分配给角色，就会影响访问控制。

如果要改权限码，需要同步处理：

```text
apis.permission_code
permissions.code
menus.permission_code
role_permissions
调用 Authorization 的 EnsureAndIncrement 使受影响用户旧 Token 失效
```

### 4. 迁移失败要停止启动

Migration 如果失败，服务不应该继续启动，否则可能出现代码和数据库结构不匹配。

## 推荐实施顺序

### 第一阶段：Seed 基础化

已完成 Go 代码 Seed，不急着引入迁移工具：

```text
SeedRoles
SeedPermissionGroups
SeedDicts
SeedMenus
SeedAPIs
SeedPermissions
SeedRolePermissions
SeedRoleMenus
SeedAdmin
```

目标是减少手动 SQL 和手动同步接口。

### 第二阶段：Migration 工具化

引入 `goose` 或 `golang-migrate`：

```text
migrations/*.sql
schema_migrations
```

目标是记录表结构版本。

### 第三阶段：部署流程标准化

把迁移纳入部署流程：

```text
部署前备份数据库
执行 migration up
启动服务
执行健康检查
```

### 第四阶段：测试保障

已使用 SQLite 内存库补充 Seed 基础测试：

- 空库启动测试。
- 重复执行 Seed 幂等测试。
- 权限和菜单缺失补齐测试。
- 软删除菜单恢复测试。
- 用户可编辑菜单字段不覆盖测试。
- API 路由同步测试。

测试使用 `github.com/glebarez/sqlite`，这是纯 Go SQLite 驱动，不依赖 CGO，比 `gorm.io/driver/sqlite` 在本项目当前测试环境里更轻便。

## 建议下一步

当前第一阶段已经落地，下一步建议做：

```text
Migration 工具选型
Seed 测试继续扩展
```

先不急着替换现有 `AutoMigrate`，因为项目仍在快速变化。等核心表结构稳定后，再引入正式 migration 工具会更稳。

推荐后续补充：

```text
1. 超级管理员异常场景测试
2. API 权限码软删除恢复测试
3. 字典类型和字典项软删除恢复测试
4. goose / golang-migrate 选型
5. migrations 目录规划
```
