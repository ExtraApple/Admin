## 1. 范围与前置

- [ ] 1.1 确认以下前置已实施、验证并完成相应规格同步后再实施本change：`fix-menu-api-code-consistency`；区分design中的功能与发布依赖，不能乱序用完整Requirement覆盖。

## 2. Model / DTO / Contract

- [ ] 2.1 复用API Metadata已有transactions与协调Contract，明确所有Adapter消费同一ctx；必需装配缺失不得fallback逐项提交。

## 3. Application / Adapter实现

- [ ] 3.1 SyncPermissions将候选读取及整个遍历置入一个最外层Run，API码/权限/授权关联/全部版本共同提交。
- [ ] 3.2 callback内创建attempt局部created/updated结果，只在最外层成功后发布统计；内层协调外部副作用不能早于真正提交。
- [ ] 3.3 保留NeedAuth与已存在权限skip及既有码派生，其他sync/Seed用途不改变，不加新重试次数。

## 4. HTTP / App装配

- [ ] 4.1 HTTP失败不返回部分完成成功数据，成功created/created_count/updated_api匹配实际提交；保持原路径/权限码与安全错误。

## 5. 行为回归与原程序验证

- [ ] 5.1 全新/已存在/无需认证混合请求仅适用项提交，返回created/created_count/updated_api与实际库对应。
- [ ] 5.2 第二项Core/权限/关联失败和后续用户版本失败时整个请求回滚，不返回部分成功。
- [ ] 5.3 在真实存储按既有可重试错误触发重执行，只计最终成功attempt；最终失败时全部本请求变化无残留。
- [ ] 5.4 原程序HTTP验收保持权限码、公开错误和现有同步用途分离。

## 6. 文档与交付

- [ ] 6.1 在产品行为实现后只保留能捕获消费者可见错误的确定性回归；运行本切片适用门禁及原程序smoke，保存去敏响应/事务前后状态/必要交错证据，清理自建资源，不把未运行检查标通过。
- [ ] 6.2 依据实际验收更新现有实现说明、审计状态与必要changelog；保留历史JSON原样，记录本次新证据和局部覆盖，不把其他C6风险标完成。
- [ ] 6.3 核对delta完整保留前置Requirement与新场景，并完成OpenSpec检查；规格同步/归档按独立流程及design发布依赖执行，不在apply中自动归档。
