## MODIFIED Requirements

### Requirement: 登录锁定
系统 SHALL 基于用户名使用 Redis 记录登录凭据错误累计，并在达到五次阈值后进行渐进式锁定，同时不暴露用户名是否存在。有效验证码之后、未锁定且进入凭据校验时，用户名不存在或密码错误 SHALL 计入累计；锁定期间的重试 SHALL NOT 再次计入凭据错误。

本次失败计入后的累计次数少于 5 次 SHALL 不锁定，5～9 次 SHALL 锁定 1 分钟，10～14 次 SHALL 锁定 5 分钟，15～19 次 SHALL 锁定 15 分钟，20 次及以上 SHALL 锁定 1 小时。第 10 次失败和锁定到期 SHALL NOT 清零累计；每次记录凭据错误 SHALL 将累计记录的有效期续为 24 小时，累计记录到期自动清除，成功登录清除累计与锁定。

#### Scenario: 未达到锁定阈值时登录失败
- **WHEN** 用户提交有效验证码且未处于锁定状态，用户名不存在或启用用户密码错误，且本次错误计入后的累计失败次数少于 5 次
- **THEN** 系统递增 `fail:<username>`
- **AND** 系统返回 HTTP 401、`AUTHN_CREDENTIALS_INVALID` 和相同的模糊英文提示
- **AND** 系统 MAY 在 `data.remaining_attempts` 返回剩余尝试次数
- **AND** 响应 SHALL NOT 通过状态、错误码、消息或详情暴露用户名是否存在

#### Scenario: 登录失败达到锁定阈值
- **WHEN** 本次登录凭据错误使累计失败次数达到 5 次或以上
- **THEN** 系统在 Redis 中设置 `lock:<username>`，采用该累计次数对应的锁定档位
- **AND** 系统返回 HTTP 429 和 `AUTHN_LOGIN_LOCKED`
- **AND** 系统在 `data.retry_after_seconds` 和 `Retry-After` Header 返回锁定时间
- **AND** 锁定存在期间拒绝继续登录

#### Scenario: 账号处于锁定状态
- **WHEN** Redis 中存在 `lock:<username>`
- **THEN** 系统拒绝登录请求
- **AND** 系统返回 HTTP 429、`AUTHN_LOGIN_LOCKED`、`data.retry_after_seconds` 和 `Retry-After` Header

#### Scenario: 登录成功
- **WHEN** 验证码正确、账号启用、密码匹配
- **THEN** 系统清理 `fail:<username>` 和 `lock:<username>`
- **AND** 系统签发 access token 和 refresh token
- **AND** JSON 响应 SHALL 使用 `code=200`、空 `error_code`、`msg=success` 和登录结果 `data`

#### Scenario: 锁定到期保留凭据错误累计
- **WHEN** `lock:<username>` 到期，而 `fail:<username>` 尚未到期
- **THEN** 系统 SHALL 保留已有累计次数
- **AND** 下一次满足计入条件的凭据错误 SHALL 在已有次数上递增，而不是从第一次重新开始

#### Scenario: 第十次失败继续累计
- **WHEN** 已累计 9 次错误、对应锁定已到期，且累计记录仍有效，用户再次发生可计入的凭据错误
- **THEN** 系统 SHALL 将累计次数递增为 10 并锁定 5 分钟
- **AND** 系统 SHALL NOT 在该次锁定后重置累计

#### Scenario: 更高档位继续升级
- **WHEN** 用户未成功登录，累计记录仍有效，后续可计入的凭据错误达到 15 次或 20 次
- **THEN** 系统 SHALL 分别锁定 15 分钟或 1 小时，而不是因第 10 次重置而停留在较低档位

#### Scenario: 累计记录续期与到期
- **WHEN** 系统记录一次新的登录凭据错误
- **THEN** 系统 SHALL 将 `fail:<username>` 的有效期续为该次记录后的 24 小时
- **AND** 在无新计入错误且该记录到期后，系统 SHALL 不保留此前累计，后续凭据错误从新的累计开始
