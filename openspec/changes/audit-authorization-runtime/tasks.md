## 1. 审计基线与隔离环境

- [ ] 1.1 固定当前代码基线、Go 构建信息、运行配置形状和去敏环境记录；确认本 change 不修改产品代码、主规格或永久测试。
- [ ] 1.2 创建独占 MySQL schema、Redis 命名空间和临时配置，编译并启动当前真实 Go 服务；验证真实 HTTP、MySQL、Redis、JWT 链路可用。
- [ ] 1.3 创建固定隔离夹具：root／child／sibling／outside 组织、超级管理员、受限管理员、普通用户、范围外用户、受保护 admin、禁用用户、软删用户及 all/self/org/org_and_children/custom 角色范围。
- [ ] 1.4 创建真实 Access／Refresh Token、当前版本、旧版本和缺版本状态；为数据库锁等待、版本冲突和受控故障准备可清理的确定性 Runner 辅助。

## 2. W1 Token 与授权版本

- [ ] 2.1 通过真实 `POST /api/login`、`POST /api/refresh`、`GET /api/user/info` 和 `GET /api/user/context` 执行 T01-T03，记录当前版本允许、旧 Access 拒绝和旧 Refresh 拒绝。
- [ ] 2.2 执行 T04-T05，验证 Access／Refresh 缺版本的精确 HTTP 状态、错误码、不补行、不回退旧存储和不签发 Token。
- [ ] 2.3 执行 T06-T08，验证版本读取故障、禁用／软删／不存在用户和首次登录初始化的安全响应、持久化事实及 Token 签发边界。

## 3. W2 授权变更、no-op 与事务

- [ ] 3.1 通过用户资料、状态、Kick、删除、密码及逐人角色／组织入口执行 M01-M07，验证真实变化只提升受影响用户一次并使旧 Token 失效。
- [ ] 3.2 通过角色权限、角色数据范围和角色成员入口执行 M01-M03，验证角色级变化的影响集合、共享版本推进和保护对象拒绝。
- [ ] 3.3 执行 M08-M10，验证当前版本合法 no-op 不提升版本、过期 no-op 仍返回冲突，以及本人、admin、范围外和无权限目标无部分写入。
- [ ] 3.4 执行 M11，注入业务写入、版本初始化／锁定／提升和提交失败，核对关系、身份状态、组织关系和授权版本全部回滚。

## 4. W3 数据范围与授权投影

- [ ] 4.1 通过用户详情、用户列表、组织成员和角色授权代表入口执行 S01-S05，分别验证 all、self、org、org_and_children、custom 的读取和写入边界。
- [ ] 4.2 执行 S06-S08，验证范围外对象、`manageable=false` 定位祖先、本人和受保护 admin 的拒绝结果及关系／版本不变。
- [ ] 4.3 执行 S09，验证用户详情不泄露范围外组织、角色或授权版本，并分别记录 API 安全真值与 `/api/user/context` 菜单投影。
- [ ] 4.4 验证菜单隐藏或可见不改变 API 授权结果；将菜单投影偏差与 API 越权分别归类，不把菜单结果当作安全通过依据。

## 5. W4 确定性并发与版本不变量

- [ ] 5.1 使用数据库锁等待和两条真实 HTTP 请求执行 C01，验证角色／组织关系竞争同一旧版本时最多一项真实修改提交，另一项返回对应冲突且无部分写入。
- [ ] 5.2 执行 C02，验证 `EnsureVersion` 与 `EnsureAndIncrement` 并发时只保留一条版本记录、最终版本不倒退且 Token 只使用已提交版本。
- [ ] 5.3 执行 C03-C04，验证角色／组织跨维度竞争、失败重试和已完成请求不会重复提交、重复失效或推进错误版本。

## 6. 证据与审计结论

- [ ] 6.1 写入 `docs/reviews/evidence/c6b-auth-runtime-token.json`，保存去敏 Token／版本、HTTP 状态／错误码、存储故障和签发结果。
- [ ] 6.2 写入 `docs/reviews/evidence/c6b-auth-runtime-mutations.json`，保存去敏关系、版本、no-op、CAS、回滚和事务事实。
- [ ] 6.3 写入 `docs/reviews/evidence/c6b-auth-runtime-scope.json`，保存范围、保护对象、菜单投影和确定性并发结果。
- [ ] 6.4 更新 `docs/reviews/contract-readiness-c6b-authorization-runtime.md`，为每个风险域登记通过、发现缺陷或证据缺口，并列明未覆盖边界；不把未执行路径写成通过。
- [ ] 6.5 对发现的明确产品偏差只保存原始去敏证据并提出独立修复 change，不在本 change 内修改产品代码或永久测试；对环境障碍保留证据缺口说明。
- [ ] 6.6 清理本 change 拥有的服务进程、容器、schema、Redis 数据、临时 Runner、二进制、配置和凭据，确认没有写入共享业务库。

## 7. 最终验证

- [ ] 7.1 运行覆盖审计入口的定向 Go／HTTP 门禁和必要的现有授权测试，确认真实服务结果与证据文件一致。
- [ ] 7.2 复核所有证据不包含密码、JWT、验证码答案、Authorization header、DSN 或其他凭据，并确认 change 工件、主规格引用和报告链接可解析。
- [ ] 7.3 运行 OpenSpec 严格校验，确认 `auth` delta 完整保留原 Token 实时失效场景，未改变本 change 排除的候选设计和其他能力边界。
