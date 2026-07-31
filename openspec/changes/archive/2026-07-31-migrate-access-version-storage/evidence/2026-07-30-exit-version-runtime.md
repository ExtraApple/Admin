# 2026-07-30 授权版本退出版本运行时证据

状态：通过

## TDD 证据

Red 阶段证明退出前制品仍存在以下旧字段运行时行为：

- `EnsureVersion` 初始化后镜像更新 `users.token_version`；
- `EnsureAndIncrement` 提升后镜像更新 `users.token_version`；
- MySQL 启动流程仍执行旧字段迁移；
- Admin 初始化和 Seed 创建用户时仍显式声明 `TokenVersion`。

Green 阶段完成：

- Repository 初始化和提升只写 `user_access_versions`；
- 旧用户行不存在或拒绝更新旧列时，授权版本初始化和提升仍可成功；
- MySQL 启动不再运行授权版本旧字段迁移；
- Admin 初始化和 Seed 创建用户不再显式写旧字段；
- 原观察期镜像失败测试已改为验证退出版本不会触发旧列写入。

## 验证结果

目标退出测试：

```text
go test ./service ./initialize \
  -run '^(TestAccessVersionRepositoryEnsureVersionDoesNotWriteLegacyColumn|TestAccessVersionRepositoryEnsureAndIncrementDoesNotWriteLegacyColumn|TestAccessVersionRepositoryEnsureVersionDoesNotRequireLegacyUserRow|TestAccessVersionRepositoryEnsureAndIncrementDoesNotRequireLegacyUserRow|TestAccessVersionExitRuntimeDoesNotReadOrWriteLegacyUserColumn)$' \
  -count=1
```

结果：通过。

相关包回归：

```text
go test ./service ./initialize ./seed -count=1
```

结果：通过。

## 运行时引用复核

应用认证和授权失效运行时只通过 `user_access_versions` 读取、初始化和提升版本。
剩余旧列引用限于下一阶段待移除的 User GORM Model，以及任务 9.9 待清理的迁移、
一致性扫描和加速资格运行器，不再由正常应用启动或请求链路执行。
