## MODIFIED Requirements

### Requirement: 敏感请求和审计数据脱敏
系统 SHALL 不在普通请求日志、审计日志、错误响应或交付证据中保存 Authorization、Cookie、Set-Cookie、原始 Access／Refresh Token、可重建凭据的签发材料、WebSocket ticket、消息正文、图片内容、外链 URL、对象存储路径或基础设施原始错误。新增会话协议 SHALL 复用既有受控记录与脱敏边界，不扩大运行日志白名单，不重写历史审计记录。

#### Scenario: 敏感 HTTP 请求
- **WHEN** 请求包含 Authorization、Cookie、ticket、消息正文、图片或外链参数
- **THEN** 请求日志和审计记录 SHALL 省略或替换敏感值
- **AND** HTTP 错误响应 SHALL 只返回稳定错误码和安全提示

#### Scenario: Cookie 签发和续期恢复
- **WHEN** 登录、续期或退出产生 Set-Cookie，或系统处理短期幂等签发材料
- **THEN** 日志、审计、失败响应和持久化验收证据 SHALL NOT 包含这些凭据或可重建内容
- **AND** 错误 SHALL NOT 回显原始 Cookie、Header、Redis／数据库错误或连接信息

#### Scenario: 既有审计语义不扩大
- **WHEN** 会话控制成功、被拒绝或发生真实基础设施故障
- **THEN** 系统 SHALL 继续使用现有异步 Audit 与受控安全信息，不为本 Change 创建独立观测系统
- **AND** 新会话 key SHALL NOT 使用原始 JWT，脱敏变更 SHALL NOT 触发历史审计数据迁移
