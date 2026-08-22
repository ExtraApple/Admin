# API 响应契约分阶段实施设计

## 背景

Admin 的目标是形成完整的前后端链路，而不是仅提供一组后端 HTTP 接口。当前系统的业务 JSON 响应存在多种形态，错误通常被压缩为 HTTP 400，客户端无法稳定依赖机器可读的错误码，OpenAPI 也无法逐路由表达错误契约。

本 Change 同时覆盖后端和前端，但当前仓库还没有根目录 `web/`。因此实施不能被设计成“前端存在后后端才能开始”，也不能把后端协议切换误当成可以独立发布的后端功能。正确边界是：后端可以先开发和验收，前端随后开发，双方最后进行一次真实联合验收和统一发布。

当前 Change 采用以下状态模型：

```text
                    ┌────────────────────┐
                    │ 统一响应契约事实    │
                    │ error_code/OpenAPI │
                    └─────────┬──────────┘
                              │
              ┌───────────────┴────────────────┐
              ▼                                ▼
    ┌──────────────────┐              ┌──────────────────┐
    │ Backend Part      │              │ Frontend Part     │
    │ 后端实现           │              │ 前端实现          │
    └────────┬─────────┘              └────────┬─────────┘
             ▼                                  ▼
    ┌──────────────────┐              ┌──────────────────┐
    │ Backend Acceptance│              │ Frontend Acceptance│
    │ 后端验收           │              │ 前端验收          │
    └────────┬─────────┘              └────────┬─────────┘
             └───────────────┬──────────────────┘
                             ▼
                    ┌────────────────────┐
                    │ 联合验收            │
                    │ 真实前端 + 后端     │
                    └─────────┬──────────┘
                              ▼
                    ┌────────────────────┐
                    │ Change 完成/发布   │
                    └────────────────────┘
```

## 目标与非目标

### 目标

- 统一所有后端业务 JSON 成功和错误响应。
- 为后端 Application 错误建立模块所有、稳定、可记录 Cause 的分类模型。
- 让 Route Catalog、运行时错误映射和 OpenAPI 使用同一组公开错误定义。
- 让后端在没有前端时可以独立完成实现和后端验收。
- 让前端 API Client 依赖 `error_code`，而不是解析英文消息或旧响应形态。
- 让前端正确处理字段校验、登录锁定、二进制、图片和原始 OpenAPI 响应。
- 通过真实前后端联合验收证明完整链路，而不是只验证孤立 Fixture。
- 保持后端和前端一次正式协议切换，不留下长期兼容分支。

### 非目标

- 不改变 HTTP Method、Path、Access Level、Permission Code 或业务授权逻辑。
- 不改变数据库表、数据迁移、Redis Key、MinIO Bucket、对象命名和事务边界。
- 不在后端和前端之间引入双协议、版本协商、兼容 Header 或旧字段别名。
- 不在 Backend Acceptance 通过后允许后端独立生产发布。
- 不把 Gin、HTTP Response 或前端类型引入 Domain/Application。
- 不把数据库错误、堆栈、对象路径、Token、密码、验证码或拒绝值返回客户端。

## 设计决策

### 1. 一个 Change，两个实施部分，三个验收门槛

不拆成两个独立 Change。响应信封、错误码、OpenAPI 和 Fixture 必须只有一个事实来源；拆成两个 Change 会产生重复契约、错误码漂移以及后端 Change 被过早归档的风险。

一个 Change 内部拆成：

```text
Backend Part
  ├── 后端实现
  └── Backend Acceptance

Frontend Part
  ├── web/ 前置确认
  ├── 前端 API Client 和页面迁移
  └── Frontend Acceptance

Integration Acceptance
  └── 真实后端 + 真实前端 + 浏览器链路
```

三个门槛含义不同：

| 门槛 | 证明内容 | 是否允许独立生产发布 |
| --- | --- | --- |
| Backend Acceptance | 后端输出、日志、OpenAPI 和后端测试正确 | 否 |
| Frontend Acceptance | 前端 Client、错误状态和页面消费正确 | 否 |
| Integration Acceptance | 真实前端与真实后端完整链路正确 | 是，满足其他发布条件时 |

Backend Acceptance 通过后，后端可以进入等待前端的开发分支，但仍标记为 `backend-accepted / release-blocked`。

### 2. 后端可以先实施，但不能先发布

`web/` 不存在时允许执行 Backend Part：

- 创建 `internal/platform/httpresponse`。
- 扩展 Route Catalog 和 API Doc。
- 迁移后端错误分类和 HTTP Adapter。
- 编写后端契约 Fixture 和后端测试。
- 运行后端专项测试和 `go test ./... -count=1`。

但统一响应是破坏性协议切换。后端切换后，旧客户端可能无法读取新响应。因此：

```text
后端实现完成 ≠ 后端可以部署
后端验收通过 ≠ Change 完成
```

在联合验收之前，禁止将该协议切换作为独立生产发布物。这样既满足后端先行开发，又不引入双协议过渡实现。

### 3. `web/` 只阻塞 Frontend Part 和联合验收

前置条件调整为：

```text
web/ 不存在时：
  Backend Part             可开始
  Backend Acceptance       可完成
  Frontend Part            阻塞
  Frontend Acceptance      阻塞
  Integration Acceptance   阻塞
  Change 最终完成          阻塞
```

`web/` 创建后必须确认：

- 前端框架和包管理器。
- 集中 API Client 位置。
- 错误本地化存储位置。
- 下载、头像和原始 OpenAPI 的原生请求入口。
- 前端测试、类型检查、构建命令。

### 4. 后端统一响应信封

共享技术包放在 `internal/platform/httpresponse`，只负责传输，不拥有业务错误语义：

```text
Envelope[T]
  code
  error_code
  msg
  data

ValidationErrorData
  fields: []FieldError

FieldError
  field
  error_code
  message
```

成功响应不变量：

- `code` 等于 HTTP Status。
- `error_code` 为空字符串。
- `msg` 固定为 `success`。
- 无业务返回值时 `data: null`。
- 空集合序列化为 `[]`。
- 原有分页和列表内容保留在 `data` 内。

错误响应不变量：

- `code` 等于 HTTP Status。
- `error_code` 非空且稳定。
- `msg` 为安全英文 fallback。
- `data` 默认为 `null`，只有白名单详情可以非空。

Domain 和 Application 不依赖该技术包；模块 HTTP Adapter 将模块错误映射为公开定义后再写入信封。

### 5. 错误定义由模块拥有，运行时和 OpenAPI 共用

模块沿用 `internal/uploadsecurity` 的成熟模式：

```text
Code
Error{Code, private cause}
NewError
Unwrap
CodeOf
```

模块 HTTP Adapter 拥有公开定义：

```text
owner
code
status
message
safe data schema
field error definitions
```

同一公开定义同时服务于：

```text
Application Error
      ↓
HTTP Runtime Mapping
      ↓
Route Descriptor
      ↓
OpenAPI
```

不使用进程级可变全局错误注册表。App 负责把认证和权限中间件的公开错误定义加入实际路由组合，因为 App 才知道实际的中间件链。

### 6. 后端 OpenAPI 是前端的契约输入，但不是前端实现替代品

OpenAPI 继续保持成功时的原始 JSON，不使用四字段信封包装。每条业务 JSON 路由描述：

- 成功信封。
- 错误信封。
- 对应状态下允许的顶层 `error_code`。
- 422 的 `data.fields` 和字段级错误码。
- 安全错误详情 Schema。
- 实际 Access Level 引入的认证和权限错误。

OpenAPI 可以在没有 `web/` 时先完成并验收，但它只能作为前端实现输入，不能证明前端已经正确消费。

### 7. Frontend Part 使用单一 API Client

`web/` 创建后，前端只允许通过集中 Client 处理业务 JSON：

```text
HTTP Response
      │
      ▼
检查 HTTP status 与 envelope.code
      │
      ├── success → 返回 data
      │
      └── failure → Typed API Error
                         ├── error_code
                         ├── field errors
                         ├── safe data
                         └── msg fallback
```

前端必须：

- 依据 `error_code` 做业务判断和本地化。
- 只把英文 `msg` 作为未知错误码 fallback。
- 将 `data.fields` 映射到 JSON 字段名，而不是 Go 字段名。
- 显式区分 JSON、Blob、Image 和原始 OpenAPI 响应。
- 不保留旧 `code/msg/data` 组合的兼容分支。

### 8. 三种验收必须使用不同证据

#### Backend Acceptance

使用后端代码、`httptest`、契约 Fixture、OpenAPI 和日志观察：

- 所有后端路由输出满足信封规则。
- 错误码、状态和 OpenAPI 一致。
- 内部 Cause 不泄漏。
- 认证、权限、字段校验、文件和流式响应边界正确。
- 后端全量测试通过。

#### Frontend Acceptance

使用前端单元测试、类型检查、构建和 Fixture：

- Client 正确解包成功响应。
- Typed API Error 正确保留错误码和安全详情。
- 字段错误、锁定时间和未知错误码 fallback 正确。
- 原生响应不进入信封解析器。

#### Integration Acceptance

启动真实后端和真实 `web/`，使用浏览器验证：

- 登录凭据错误不可区分用户名是否存在。
- 验证码失败显示正确错误。
- 登录锁定展示剩余等待时间。
- 受保护路由正确处理 401/403。
- 字段校验展示在对应表单字段。
- 空列表保持空集合语义。
- 文件下载、头像和 OpenAPI 原生响应正常。
- 未知 `error_code` 使用 fallback 而不崩溃。

### 9. 发布状态机

```text
Backend Pending
      │ Backend tests + backend acceptance
      ▼
Backend Accepted / Release Blocked
      │ web/ ready
      ▼
Frontend Pending
      │ frontend tests + frontend acceptance
      ▼
Frontend Accepted / Integration Required
      │ real backend + real frontend
      ▼
Integration Accepted / Release Eligible
      │ final OpenSpec acceptance
      ▼
Change Complete
```

任何中间状态都不能被标记为 Change 完成，也不能以新协议独立发布生产。

## 实施顺序

```text
1. 固化后端响应和错误码基线
2. 实现后端响应基础设施
3. 扩展 Route Catalog 和 OpenAPI
4. 实现 App 日志与恢复
5. 完成模块错误分类
6. 迁移后端所有 HTTP Adapter
7. 执行 Backend Acceptance
8. 创建并确认 web/
9. 实现前端 API Client 和业务迁移
10. 执行 Frontend Acceptance
11. 执行 Frontend/Backend Integration Acceptance
12. 删除旧协议路径并完成最终验收
```

第 7 步完成后只表示后端可交付给前端开发，不表示可部署。第 11 步完成后才具备统一发布条件。

## 风险与取舍

- **风险：后端先完成后被误部署。** 通过 `Backend Accepted / Release Blocked` 状态和最终联合验收门禁阻止。
- **风险：前端缺失导致 Change 长期阻塞。** 接受该事实，因为 Admin 的目标是完整前后端链路，而不是后端孤立完成。
- **风险：前后端错误码漂移。** 使用同一 OpenSpec 契约、OpenAPI 和共享 Fixture；运行时和文档引用同一公开定义。
- **风险：错误分类隐藏诊断。** 保留 Cause 在服务端错误链中，仅由受控日志记录。
- **风险：字段详情泄漏密码或 Token。** 只返回字段名、稳定错误码和安全英文消息，不返回拒绝值。
- **风险：流式响应提交后无法改写 JSON。** 在提交前完成校验；提交后只记录错误并终止连接。
- **取舍：不提供后端独立发布的兼容窗口。** 代价是必须等待前端联合验收；收益是避免双协议和临时兼容代码长期残留。
- **取舍：一个 Change 而不是两个 Change。** 通过两个实施部分和三个验收门槛解决阶段性，而不复制契约事实来源。
