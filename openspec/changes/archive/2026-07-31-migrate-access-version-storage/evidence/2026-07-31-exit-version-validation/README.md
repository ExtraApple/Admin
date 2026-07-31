# 2026-07-31 退出版本全链路验证

状态：通过

对应任务：9.6

## TDD Red

先新增 `TestExitVersionRegressionDatabaseHasNoLegacyUserColumn`，它通过长期认证授权
回归共用的数据库入口检查物理 schema。测试先按预期失败：

```text
exit-version regression database still has users.token_version
FAIL admin/service
```

该失败证明原长期回归 fixture 仍创建旧列，无法证明退出版本在旧列不存在时可运行。

## Green 实现

- 新增退出版用户测试 fixture，保留 JWT Claim 所需的测试期望版本，但通过
  `gorm:"-"` 明确不持久化该值。
- Service 授权版本、用户生命周期和授权失效长期回归全部切换到物理无
  `users.token_version` 的数据库。
- Router 登录、Refresh Token、JWT 和管理员用户契约回归切换到同一退出版
  schema，并在 fixture 建立后显式断言旧列不存在。
- 删除长期回归自身对旧列的更新和读取断言；版本结果只从
  `user_access_versions` 验证。
- 启动、Model 与 AutoMigrate 继续由退出版本测试证明不会声明或重建旧列。

迁移、观察和回滚专用代码及测试仍保留到任务 9.9；它们不在正常启动或请求链路
执行。

## 验证范围

- 启动与 AutoMigrate；
- 登录、Access Token、Refresh Token、JWT、中间件用户状态顺序；
- 密码变化、管理员修改用户、状态切换、Kick、软删除和恢复；
- 角色、用户角色、角色菜单、权限和数据范围；
- 菜单创建、修改、删除、同步、Menu API 与 API 权限码联动；
- 组织成员变更和组织删除；
- 首次登录前发生失效操作、主写失败回滚和懒初始化。

## Green 结果

```text
go test ./initialize ./service ./router -count=1
ok admin/initialize
ok admin/service
ok admin/router
```

详细日志：

```text
startup.log
http-auth.log
authorization-entrypoints.log
package-regression.log
runtime-reference-scan.log
```

运行时文件定向扫描未发现数据库旧列引用。JWT Claim 的 `TokenVersion` 按 ADR
继续保留，它不是 `users.token_version` 数据库字段依赖。

## 结论

退出版本在物理无旧列的测试数据库上可以完成启动、登录、刷新、JWT 校验和全部
授权失效入口，正常应用链路不再访问 `users.token_version`。
