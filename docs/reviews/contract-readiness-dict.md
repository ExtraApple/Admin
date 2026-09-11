# 契约可信度调研 · 字典模块（格式样板）

> 本文件是"契约可信度调研"的**第一份、也是格式样板**，只覆盖 `dict-management` 一个模块。
> 目的：验证调研方法与产出格式，之后按模块扩展，并据此产出 OpenSpec change。

## 调研背景与方法

本调研的直接起因：项目当前处于「后端已验收、等待前端」阶段
（`openspec/changes/standardize-api-response-contract/` 状态为 `backend-accepted / release-blocked`），
需要回答的问题是 **"后端契约是否可信到可以作为前端的开发依据"**，
而不是"是否可以上线"。

一次先前调研（2026-09-22 会话）使用的口径是「构建/vet/测试门禁是否通过」。
该口径只能证明**代码自洽**，不能证明**代码与它声称的行为一致**，已废弃。

### 本调研使用的口径

对每条路由与每条行为契约，核对**三个契约存储位置**：

| 位置 | 文件 | 承载什么 |
| --- | --- | --- |
| 规格 | `openspec/specs/<capability>/spec.md` | 对外可观察行为、业务约束 |
| 测试 | `internal/<module>/**/*_test.go` | 路由存在性、access level、权限码、错误状态 |
| 描述符 | `internal/<module>/http.go` 或 `adapters/http/routes.go` | access level、权限码、OpenAPI、公开错误码 |

### 判定分类（互斥）

| 判定 | 含义 | 后续动作 |
| --- | --- | --- |
| ✅ 一致 | 至少两处声明且互相一致 | 无 |
| 📄 未记录 | 代码与测试有，规格无 | 补规格 |
| 🧪 未验证 | 只有规格声明，代码与测试无对应断言 | 补测试或补规格 |
| ⚠️ 冲突 | 两处声明不一致 | 定一个真相后统一 |
| ❌ 未实现 | 规格要求，代码无 | 补代码 |

### 调研纪律

1. 任何"规格要求 X"的主张必须附**逐字引用**，不接受只给行号。
   （原因：先前调研中曾出现"精确行号 + 编造结论"的引用，两次主要主张均为误报。）
2. 路由清单必须取自**运行时代码**，不接受任何转录稿。
   （原因：先前调研中曾依据自制的路由转录稿误判 `DELETE /api/admin/users/:id` 不存在。）
3. 任何"代码有缺陷"的结论必须**同时核对删除/写入路径的实际实现**，不接受从声明推断。
   （原因：本次调研中曾推断"字典软删除与唯一索引冲突"，核对后发现字典全部为硬删除，该结论不成立。）

---

## 一、路由层核对

规格文件：`openspec/specs/dict-management/spec.md`（45 行，3 个 Requirement，5 个 Scenario）
代码：`internal/dictionary/http.go:47-57`（9 条路由）
测试：`internal/dictionary/http_test.go:39-54`（`TestRoutesExposeDictionaryHTTPContract`，断言全部 9 条）

| # | 路由 | 规格出处 | 测试出处 | 描述符声明 | 判定 |
| --- | --- | --- | --- | --- | --- |
| 1 | `GET /api/dicts/:type_code/items` | `spec:42-45` | `http_test.go:46,56-60` | Public / 无权限码 / 200,400,404,409,422,500 | ✅ 一致 |
| 2 | `GET /api/admin/dict-types` | `spec:13-15` | `http_test.go:47` | PermissionControlled / `admin.dict-types.get` | ✅ 一致 |
| 3 | `POST /api/admin/dict-types` | `spec:17-20` | `http_test.go:48` | PermissionControlled / `admin.dict-types.post` | ✅ 一致 |
| 4 | `PUT /api/admin/dict-types/:id` | **无** | `http_test.go:49` | PermissionControlled / `admin.dict-types.put` | 📄 未记录 |
| 5 | `DELETE /api/admin/dict-types/:id` | `spec:23` 有行为句"管理员删除字典类型"，**但无路径、无方法** | `http_test.go:50,145-153` | PermissionControlled / `admin.dict-types.delete` | 📄 未记录 |
| 6 | `GET /api/admin/dict-items` | `spec:30-32` | `http_test.go:51` | PermissionControlled / `admin.dict-items.get` | ✅ 一致 |
| 7 | `POST /api/admin/dict-items` | `spec:34-37` | `http_test.go:52` | PermissionControlled / `admin.dict-items.post` | ✅ 一致 |
| 8 | `PUT /api/admin/dict-items/:id` | **无** | `http_test.go:53` | PermissionControlled / `admin.dict-items.put` | 📄 未记录 |
| 9 | `DELETE /api/admin/dict-items/:id` | **无** | `http_test.go:54,206-214` | PermissionControlled / `admin.dict-items.delete` | 📄 未记录 |

**规格逐字引用**（用于核对上表"规格出处"列）：

- `spec:14` — "**WHEN** 管理员请求 `GET /api/admin/dict-types`"
- `spec:18` — "**WHEN** 管理员请求 `POST /api/admin/dict-types`"
- `spec:23` — "**WHEN** 管理员删除字典类型"（未指明 Method 与 Path）
- `spec:31` — "**WHEN** 管理员请求 `GET /api/admin/dict-items`"
- `spec:35` — "**WHEN** 管理员请求 `POST /api/admin/dict-items`"
- `spec:43` — "**WHEN** 请求 `GET /api/dicts/:type_code/items`"

**结论**：字典模块 9 条路由中 5 条有规格出处、4 条为 📄 未记录。
注意这不是"未实现"—— 4 条未记录路由均有 handler、描述符与测试覆盖，**前端可以使用**，
只是读规格的人会误判它们不存在。

**规格只声明了 3 个 GET 与 2 个 POST，零个 PUT、零个 DELETE**（逐字提取结果，
见上文"规格逐字引用"）。而 Requirement 正文承诺了完整 CRUD：

- `spec:10-11` — "系统 SHALL 支持管理员**创建、查询、修改和删除**字典类型。"
  对应 Scenario 只有查询（`:13-15`）、创建（`:17-20`）、删除（`:22-25`）。
- `spec:27-28` — "系统 SHALL 支持管理员**创建、查询、修改和删除**字典条目。"
  对应 Scenario 只有查询（`:30-32`）、创建（`:34-37`）。

**Requirement 正文与它自己的 Scenario 集合不一致**：正文承诺 4 个动作，场景只落实 2-3 个。
经项目所有者确认（2026-09-22 会话），此形态为**漏写**，不是有意省略 —— 规格应覆盖完整 CRUD。
据此，本节的路由层核对对全部模块均为必要工作。

### 反向：规格提到但代码没有的端点

无。

---

## 二、行为契约层核对

规格中有三句不在任何路由场景内、但定义了具体行为：

### 契约 A｜分页语义

- **规格**：`spec:15` — "**THEN** 系统分页返回字典类型列表"；`spec:32` 对条目列表有同义要求。
- **代码**：`service.go:41-52`、`service.go:231-242` 调用 `normalizePage`（`service.go:262-270`）：`page<=0 → 1`，`pageSize<=0 → 10`。
- **HTTP 参数名**：`http.go:80-81` 与 `:133-134` 读取 `page` 与 `size`。
- **响应字段**：`http.go:87` 返回 `TypeListResponse{List, Total, Page, Size}`。
- **判定**：✅ 一致（规格已满足，但规格**未声明参数名与响应字段名**）

> 前端影响：参数名 `size`（非 `page_size`/`limit`）与响应字段 `Size` 只存在于代码与描述符里。
> 规格无法告诉前端这件事。属于规格**描述不足**，不属缺陷。

### 契约 B｜唯一性

- **规格**：`spec:20` — "**AND** 字典类型编码 SHALL 在**未删除数据**中唯一"；`spec:37` — "**AND** 同一字典类型下条目值 SHALL 唯一"。
- **代码**：
  - 应用层存在性检查：`service.go:24-30`（`TypeCodeExists`）、`service.go:135-141`（`ItemValueExists`）
  - 仓储层查询：`repository.go:43-51`、`repository.go:110-118` —— 经 GORM 默认作用域，隐含 `deleted_at IS NULL`
  - 数据库约束：`model.go:8` `Code ... uniqueIndex`；`model.go:18-20` `uniqueIndex:idx_dict_item_type_value`
- **删除路径（决定"未删除"语义是否可达）**：`repository.go:97,101,135` 全部使用 `Unscoped().Delete()` —— **硬删除**，行从表中消失。
- **判定**：⚠️ 冲突（规格措辞 vs 实现语义）
  - 规格使用"未删除数据中唯一"，暗示存在软删除行；
  - 实现中字典类型与条目**只有硬删除路径**，不存在软删除行；
  - 因此规格描述的是一个**在当前实现下不可能出现的状态**。
  - 唯一性的实际保证来自数据库索引（含已硬删除以外的全部行），不是规格所描述的作用域。

### 契约 C｜级联删除

- **规格**：`spec:25` — "**AND** 系统删除该类型下的字典条目"
- **代码**：`service.go:117-124` 在同一事务内先 `DeleteItemsByTypeCode`（`repository.go:96-98`，`Unscoped()`）再 `DeleteType`（`repository.go:101`，`Unscoped()`）
- **测试**：`http_test.go:145-158` 断言删除类型后 `typesAfterDelete.Data.Total == 0` **且** `itemsAfterDelete.Data.Total == 0`
- **判定**：✅ 一致

---

## 三、本模块判断

| 判定 | 数量 | 明细 |
| --- | --- | --- |
| ✅ 一致 | 6 | 5 条路由 + 契约 A + 契约 C（其中契约 A、C 为行为契约） |
| 📄 未记录 | 4 | 路由 4、5、8、9 |
| 🧪 未验证 | 0 | — |
| ⚠️ 冲突 | 1 | 契约 B（唯一性语义） |
| ❌ 未实现 | 0 | — |

**未发现**：规格要求但代码缺失的端点；路由层与描述符不一致；公开错误码与实际返回状态不一致。

---

## 四、待确认问题（需人工判断，代码无法回答）

1. ~~4 条 📄 未记录路由是有意还是漏写？~~ **已确认：漏写。**
   依据：`spec:10-11` 与 `spec:27-28` 的 Requirement 正文均承诺"创建、查询、修改和删除"，
   但 Scenario 只落实其中 2-3 个；规格全文中 PUT、DELETE 零出现。
   项目所有者确认规格应覆盖完整 CRUD。
   **影响**：路由层核对对全部 11 个模块均为必要工作，方法不变。

2. **契约 B 应以哪一方为准？**（仍未确认）
   - 若以实现为准（硬删除，索引保证唯一）→ 应改规格措辞，去掉"未删除数据中"。
   - 若以规格为准（未来要引入软删除）→ 应改模型索引为不含 `deleted_at` 的唯一约束，
     并确认 `repository.go:43-51` 的查询作用域与之一致。
   现状是两者都未错到会立即出错，但**含义不同**。

---

## 五、证据缺口（本次未验证，不应据此下结论）

1. **唯一性在并发下是否真正成立**：
   `service.go:24-30` 的存在性检查在事务**之外**，`CreateType` 的写入在事务**之内**。
   两个并发同名创建请求可能同时通过检查。
   若数据库唯一索引生效，第二个写入会失败并返回 `DICT_INTERNAL_ERROR`（500），
   而非规格语义上的 `DICT_CONFLICT`（409）。
   **本次未做并发验证，因此不作为发现记录。**
   验证方式：SQLite 并发受限，需在 MySQL 门禁下补一个并发同名创建测试。

2. **`total` 与筛选条件的交互**：契约 A 未涉及 `keyword`/`status` 筛选，规格亦未声明。
   本次未核对筛选行为是否需要规格化。

---

## 六、调研成本实测

| 项目 | 数量 |
| --- | --- |
| 阅读文件 | `spec.md` 45 行 · `http.go` 224 行 · `service.go` 285 行 · `repository.go` 167 行 · `model.go` 28 行 · `http_test.go` 第 1-70、145-214 行 · `errors_http.go` 40 行 |
| 工具调用 | 约 14 次 |
| 产出 | 4 条 📄 未记录 · 1 条 ⚠️ 冲突 · 2 条待确认 · 2 条证据缺口 |

**外推**：全项目 15 个规格文件、11 个模块、120 条路由。
字典是**最小**的模块之一（45 行规格、9 条路由）；
`api-management`（249 行）、`user-management`（278 行）、`internal-messaging` 规模远大于字典。
按字典实测外推，全量调研需按模块拆分为多轮，不适合单次执行。

---

## 七、调研过程中已发生的误报（保留记录）

为确保后续模块不重复同类错误，保留本次调研中**发生并被推翻**的结论：

| 误报 | 推翻依据 |
| --- | --- |
| 字典模块将所有业务错误返回 400 | `errors_http.go:22-36` 有完整分类；`http_test.go` 4 处断言 409 |
| 缺少管理员创建用户端点，规格有要求 | 规格 `user-management:212-274` 无创建用户要求；`rbac:35-47` 是"替换角色下用户列表" |
| `DELETE /api/admin/users/:id` 不存在 | `internal/identity/adapters/http/routes.go:83` 存在 |
| 字典软删除行与唯一索引冲突 | `repository.go:97,101,135` 全部为 `Unscoped()` 硬删除 |
| `go test` 47 个包全部 ok | 实测 `ok` 43 行、`[no test files]` 10 行、合计 53 行 |
