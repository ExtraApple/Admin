## MODIFIED Requirements

### Requirement: API 权限同步
系统 SHALL 能够将 API 元数据中的权限码同步到权限表。

一次管理端API权限同步请求的全部数据库写入 SHALL 在一个最外层事务内提交或回滚，包括API码、权限、授权关系和全部适用用户版本；正常skip不是失败。响应统计 SHALL 仅反映最终成功提交的attempt，不得报告已回滚的部分完成结果。

#### Scenario: 同步 API 权限码
- **WHEN** 管理员调用 `POST /api/admin/apis/sync-permissions`
- **THEN** 系统读取需要认证的 API 记录
- **AND** 为不存在于权限表的 API 权限码创建权限记录
- **AND** 跳过不需要认证的公开 API
- **AND** 已存在权限码不会重复创建

#### Scenario: 第二项失败回滚第一项
- **WHEN** 同一请求的前项已在事务内写入，后项API/权限/授权关系/用户版本发生失败
- **THEN** 系统 SHALL 回滚本请求全部写入并返回既有安全错误
- **AND** 不得留下前项新权限码或版本提升，也不得返回部分成功统计

#### Scenario: 正常skip与整请求成功
- **WHEN** 请求同时包含不需认证、已有权限及需创建权限的API
- **THEN** 系统 SHALL 按现行规则跳过前两类并原子提交适用写入，返回实际created/created_count/updated_api

#### Scenario: 成功attempt统计不重复
- **WHEN** 既有Runner按其既有策略回滚某attempt并重试后提交
- **THEN** 系统 SHALL 仅返回已提交attempt的创建列表及计数，不累加已回滚attempt
- **AND** 本场景不新增重试次数或所有错误都重试的承诺
