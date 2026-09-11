## MODIFIED Requirements

### Requirement: 字典类型 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除字典类型。

#### Scenario: 查询字典类型列表
- **WHEN** 管理员请求 `GET /api/admin/dict-types`
- **THEN** 系统分页返回字典类型列表

#### Scenario: 查询字典类型详情
- **WHEN** 管理员请求 `GET /api/admin/dict-types/:id`
- **THEN** 系统返回该字典类型的详情
- **AND** 类型不存在时系统拒绝请求

#### Scenario: 创建字典类型
- **WHEN** 管理员请求 `POST /api/admin/dict-types`
- **THEN** 系统创建字典类型
- **AND** 字典类型编码 SHALL 在未删除数据中唯一

#### Scenario: 修改字典类型
- **WHEN** 管理员请求 `PUT /api/admin/dict-types/:id` 并提交名称、编码、备注、排序或状态
- **THEN** 系统更新提交的字段
- **AND** 编码变更后 SHALL 仍满足未删除数据中的唯一性
- **AND** 编码变更 SHALL 在同一事务中同步该类型下条目的类型编码
- **AND** 未提交任何可更新字段时系统 SHALL 拒绝请求

#### Scenario: 删除字典类型
- **WHEN** 管理员请求 `DELETE /api/admin/dict-types/:id`
- **THEN** 系统删除该字典类型
- **AND** 系统在同一事务中删除该类型下的字典条目
- **AND** 类型不存在时系统拒绝请求

### Requirement: 字典条目 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除字典条目。

#### Scenario: 查询字典条目列表
- **WHEN** 管理员请求 `GET /api/admin/dict-items`
- **THEN** 系统分页返回字典条目列表

#### Scenario: 创建字典条目
- **WHEN** 管理员请求 `POST /api/admin/dict-items`
- **THEN** 系统创建字典条目
- **AND** 同一字典类型下条目值 SHALL 唯一
- **AND** 所属字典类型不存在时系统拒绝请求

#### Scenario: 修改字典条目
- **WHEN** 管理员请求 `PUT /api/admin/dict-items/:id` 并提交所属类型、显示文本、值、备注、排序或状态
- **THEN** 系统更新提交的字段
- **AND** 变更后的类型与值组合 SHALL 仍满足唯一性
- **AND** 目标类型不存在时系统拒绝请求
- **AND** 未提交任何可更新字段时系统 SHALL 拒绝请求

#### Scenario: 删除字典条目
- **WHEN** 管理员请求 `DELETE /api/admin/dict-items/:id`
- **THEN** 系统删除该字典条目
- **AND** 条目不存在时系统拒绝请求

### Requirement: 按类型编码获取字典条目
系统 SHALL 支持按字典类型编码获取启用状态的字典条目。

#### Scenario: 前端获取下拉框条目
- **WHEN** 请求 `GET /api/dicts/:type_code/items`
- **THEN** 系统返回该类型下状态为启用的条目
- **AND** 返回结果按 sort asc, id asc 排序

#### Scenario: 类型编码不存在或未启用
- **WHEN** 请求的 `:type_code` 不存在，或对应字典类型未启用
- **THEN** 系统返回空列表而非错误
- **AND** 响应 SHALL 使用 HTTP 200 与空 `data` 数组
