## 1. 策略契约测试

- [x] 1.1 先修改 managed-file V1 类型矩阵测试：只允许 PDF、UTF-8 TXT、UTF-8 CSV，并稳定拒绝 JPEG、PNG、WebP、Office、压缩包和其他未列入白名单的格式
- [x] 1.2 补充目的隔离测试：managed-file 验证器拒绝图片，avatar 验证器继续接受 JPEG、PNG、WebP 且拒绝 PDF/TXT/CSV
- [x] 1.3 补充管理员文件详情和预览服务测试：详情不生成 `preview_url`，预览访问在打开 MinIO 对象前返回文件状态冲突

## 2. Model、DTO 与迁移

- [x] 2.1 先为 `files.content_sha256`、`users.avatar_content_sha256` 和管理员文件 JSON 元数据编写 model/dto 测试，确认摘要为 64 位小写十六进制且用户响应不暴露头像摘要
- [x] 2.2 先为数据库迁移编写测试：新增摘要列、既有空摘要记录保持可用、非 PDF/TXT/CSV 的已验证管理员文件幂等降级为 `legacy_unverified`
- [x] 2.3 在 `model.File`、`model.User`、`dto.FileInfo` 和 GORM 迁移中实现摘要字段及非破坏性状态回填，不添加唯一索引

## 3. Upload Security 核心实现

- [x] 3.1 为上传安全 `Result` 增加服务端摘要契约，并用失败优先测试约束摘要格式和“摘要对应 Result.Reader 完整字节”
- [x] 3.2 在管理员文件有界暂存过程中同步计算 SHA-256，确保验证完成后的 Reader 仍从起点提供同一组已计算摘要的字节
- [x] 3.3 对头像标准化重新编码后的 JPEG/PNG 输出计算 SHA-256，验证摘要不对应原始上传图片
- [x] 3.4 在 managed-file 验证器增加用途专属白名单判断，保留头像所需的共享图片类型定义

## 4. 管理员文件 Service

- [x] 4.1 先补充文件上传 service 测试：持久化验证结果摘要、返回管理员元数据摘要，并保持 MinIO 或数据库失败时现有补偿边界
- [x] 4.2 实现管理员文件上传摘要持久化和 DTO 映射，确保摘要不进入 object key、URL、普通日志或审计元数据
- [x] 4.3 先补充重新验证测试：允许类型成功保存摘要，不允许的历史图片/Office 变为 `blocked`，临时存储错误保持 `validation_error`
- [x] 4.4 实现重新验证摘要更新，并仅在完整格式验证成功后替换摘要
- [x] 4.5 收窄管理员文件下载访问判断为 PDF/TXT/CSV，移除详情预览授权并让保留的预览路由稳定拒绝所有管理员普通文件

## 5. 用户头像 Service

- [x] 5.1 先补充头像 service/repository 测试：保存标准化输出摘要、数据库更新明确失败时补偿删除、提交结果不确定时不误删对象
- [x] 5.2 实现头像摘要与 object name、MIME、验证状态、验证时间的同次数据库更新，并保持用户 DTO 不暴露摘要
- [x] 5.3 先补充恢复默认头像测试并实现摘要清理，保证重复恢复默认头像仍然幂等且不会误删对象

## 6. Handler、Router 与文档同步

- [x] 6.1 更新 handler/router 行为测试，验证普通用户不能进入管理员文件上传路由、管理员图片上传返回 HTTP 415、管理员文件预览返回 HTTP 409
- [x] 6.2 同步 OpenAPI 注释与响应模型：管理员文件类型只列 PDF/TXT/CSV、文件元数据包含 `content_sha256`、详情不再承诺预览 URL、头像响应不包含摘要
- [x] 6.3 更新 `docs/` 和 `docs/modify/` 中当前上传策略说明，明确头像与管理员普通文件分流、Office/外部扫描属于后续版本、SHA-256 只是完整性基线
- [x] 6.4 复核本 change 的 proposal、design、delta specs 与实际实现一致，并保持主规格只通过后续 OpenSpec sync/archive 流程更新

## 7. 验证

- [x] 7.1 运行上传安全、文件 service、头像 service、迁移和路由的定向测试，确认新增测试经历红灯后通过
- [x] 7.2 运行 `gofmt`、`go test ./...`、`go vet ./...` 和 `git diff --check`
- [x] 7.3 运行 OpenSpec change 校验并人工核对：没有 Office 实现、没有外部扫描依赖、没有摘要唯一索引、没有启动批量对象扫描
