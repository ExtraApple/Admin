## 1. 范围与前置

- [x] 1.1 固定本change现行条款、审计来源与适用前提；复用已取得反例，不为重新确认已知失败重跑旧程序。

## 2. Model / DTO / Contract

- [x] 2.1 保持HTTP与内部api_ids输入原貌直到用例校验，不新增全局unique helper策略。

## 3. Application / Adapter实现

- [x] 3.1 AssignAPIs在去重前拒绝原始0，再保留非空、存在/启用/认证校验与现有事务顺序。

## 4. HTTP / App装配

- [x] 4.1 保持POST /api/admin/menus/:id/apis及NAV校验错误映射，真实HTTP与直接Application调用使用同一规则。

## 5. 行为回归与原程序验证

- [x] 5.1 有效ID加0返回既有Navigation校验拒绝且Menu/API/MenuAPI/Permission/RolePermission/版本全不变。
- [x] 5.2 重复有效ID成功且关系无重复，证明没有禁止合法规范化。
- [x] 5.3 不存在、停用、无需认证API与空集合仍按原前提拒绝；内部调用同样拒绝混合0。

## 6. 文档与交付

- [x] 6.1 在产品行为实现后只保留能捕获消费者可见错误的确定性回归；运行本切片适用门禁及原程序smoke，保存去敏响应/事务前后状态/必要交错证据，清理自建资源，不把未运行检查标通过。
- [x] 6.2 依据实际验收更新现有实现说明、审计状态与必要changelog；保留历史JSON原样，记录本次新证据和局部覆盖，不把其他C6风险标完成。
- [x] 6.3 核对delta完整保留前置Requirement与新场景，并完成OpenSpec检查；规格同步/归档按独立流程及design发布依赖执行，不在apply中自动归档。
