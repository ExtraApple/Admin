## 1. 范围与前置

- [ ] 1.1 确认以下前置已实施、验证并完成相应规格同步后再实施本change：`enforce-authorization-reference-lifecycle`、`fix-menu-api-code-consistency`；区分design中的功能与发布依赖，不能乱序用完整Requirement覆盖。

## 2. Model / DTO / Contract

- [ ] 2.1 将角色description、权限group、菜单icon/component/path及需拒显式空的必填更新字段做局部存在性输入，按LSP完整迁移HTTP/Application caller，不留旧签名。

## 3. Application / Adapter实现

- [ ] 3.1 Role/Permission按遗漏不写、合法空明确写入构造更新，group只是标签，不删资源。
- [ ] 3.2 Menu路径空值沿现有规范化/唯一性及适用约束，icon/component合法清空；permission_code仍独立绑定契约。
- [ ] 3.3 非法清空整次拒绝；真实清空仍遵当前配置事务/版本，后续Noop目标再处理同值，不抢改新触发。

## 4. HTTP / App装配

- [ ] 4.1 对齐三个现有PUT请求说明与实际Route Catalog schema，空可选字段不误判无字段，空name/code不被当省略。

## 5. 行为回归与原程序验证

- [ ] 5.1 角色description、Permission.group、菜单icon/component/path各有省略保持、显式空合法清空、非空替换的HTTP/真实存储断言。
- [ ] 5.2 仅提交合法空可选字段不被当成“无可更新字段”；明确空name/code或路径冲突等非法组合整次拒绝。
- [ ] 5.3 其他字段、关系及Permission资源不被误删；Role/Menu清空的版本处理失败回滚全部本次变化。

## 6. 文档与交付

- [ ] 6.1 在产品行为实现后只保留能捕获消费者可见错误的确定性回归；运行本切片适用门禁及原程序smoke，保存去敏响应/事务前后状态/必要交错证据，清理自建资源，不把未运行检查标通过。
- [ ] 6.2 依据实际验收更新现有实现说明、审计状态与必要changelog；保留历史JSON原样，记录本次新证据和局部覆盖，不把其他C6风险标完成。
- [ ] 6.3 核对delta完整保留前置Requirement与新场景，并完成OpenSpec检查；规格同步/归档按独立流程及design发布依赖执行，不在apply中自动归档。
