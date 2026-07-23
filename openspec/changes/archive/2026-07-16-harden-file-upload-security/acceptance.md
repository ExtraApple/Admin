# 12.4 Delta Spec 验收记录

验收日期：2026-07-16

补充校正日期：2026-07-22

验收范围：

- `specs/file-management/spec.md`
- `specs/user-management/spec.md`
- `specs/logging/spec.md`

验收方法：

1. 对每个 Scenario 核对生产实现入口。
2. 对每个 Scenario 核对可重复执行的自动化测试证据。
3. 对历史兼容边界单独确认：保留 `model.User.Avatar`，迁移只回填数据库状态，不访问历史 MinIO 对象。
4. 运行相关包测试和全量 Go 回归。

结论标记：以下条目均为 **通过**。

## file-management

### 管理员上传文件

实现入口：`handler/upload.go`、`handler/file.go`、`service/file.go`、`service/uploadsecurity/`、`service/objectstorage/`。

- ✅ **上传成功**：`TestFileServiceStoresOnlyValidatedUploadResult`、`TestManagedFileUploadEndToEndUsesOnlyValidatedSystemObjectKeys`、`TestFileHandlerUploadPassesBoundedInputToFileService`。
- ✅ **multipart 请求不合法**：`TestFileHandlerUploadRejectsMultipartWithoutFilePart`、`TestFileHandlerUploadRejectsMultipleFileParts`、`TestFileHandlerUploadRejectsExtraFilePartUnderAnotherFieldName`、`TestFileHandlerUploadRejectsEmptyFile`、`TestFileHandlerUploadRejectsDamagedMultipartBody`。
- ✅ **上传文件超过大小限制**：`TestFileHandlerUploadRejectsBodyAboveHardLimitBeforeMultipartParsing`、`TestFileHandlerUploadRejectsFileAboveConfiguredLimit`、`TestStageRejectsContentBeyondLimit`。
- ✅ **文件类型不受支持或不一致**：`TestManagedFileV1ForbiddenTypeMatrix`、`TestManagedFileV1RejectsMIMEAndDangerousDoubleExtensionMatrix`、`TestFileServiceUsesManagedFileValidationBeforeStorage`。
- ✅ **文件内容损坏或危险**：`TestValidateContentRejectsCorruptOrMismatchedImages`、`TestValidateContentChecksPDFHeaderAndEOFMarker`、`TestValidateOOXMLRejectsDangerousEntriesContentTypesAndRelationships`、`TestToHTTPErrorMapsStableCodesCentrally`。
- ✅ **MinIO 上传失败**：`TestFileServiceDoesNotCreateRecordWhenStorageWriteFails`。
- ✅ **对象上传后元数据写入失败**：`TestFileServiceDeletesStoredObjectWhenRecordCreationFails`。

### 管理员文件列表

实现入口：`service/file.go` 的 `ListFiles`、`dto/file.go`。

- ✅ **查询文件列表**：`TestListFilesOrdersPaginatesFiltersAndPreservesValidationStates` 验证创建时间倒序和分页；`TestListFilesReturnsValidationMetadata` 验证完整状态元数据。
- ✅ **按对象名前缀查询文件**：`TestListFilesOrdersPaginatesFiltersAndPreservesValidationStates` 验证 `object_name LIKE prefix%` 语义。
- ✅ **列表包含历史或封锁文件**：`TestListFilesOrdersPaginatesFiltersAndPreservesValidationStates` 验证 `legacy_unverified`、`validation_error`、`blocked` 原样返回。

### 管理员文件详情

实现入口：`service/file.go` 的 `FileDetailService`、`handler/file.go`。

- ✅ **验证通过的文件存在**：`TestFileDetailServiceGeneratesBoundAccessURLsForValidatedImage`、`TestFileHandlerGetFileReturnsControlledApplicationURLs`。
- ✅ **历史未验证或临时验证错误文件存在**：`TestFileDetailServiceRestrictsURLsByValidationStatusAndType`、`TestFileValidationStateMatrix`。
- ✅ **策略封锁文件存在**：`TestFileDetailServiceRestrictsURLsByValidationStatusAndType`、`TestFileValidationStateMatrix`。
- ✅ **文件不存在**：`TestFileDetailServiceClassifiesMissingRecordWithoutExposingQueryDetails`、`TestFileHandlerGetFileReturnsStableErrorsWithoutExposingServiceDetails`。

### 管理员修改文件元数据

实现入口：`service/file.go` 的 `UpdateFile`、`service/uploadsecurity/filename.go`。

- ✅ **修改文件名主体**：`TestUpdateFileSanitizesNameBodyAndPreservesTrustedStorageMetadata`。
- ✅ **修改扩展名或提交危险名称**：`TestUpdateFileRejectsUnsafeNamesWithoutMutatingRecord`。
- ✅ **清洗后名称为空**：`TestUpdateFileRejectsUnsafeNamesWithoutMutatingRecord` 的空主体用例。

### MinIO 文件浏览

实现入口：`service/file.go` 的 `FileBrowseService`、`service/objectstorage/`。

- ✅ **浏览文件**：`TestFileBrowseServiceReturnsOnlyObjectMetadataWithoutClaimingObjects`。
- ✅ **浏览到孤立对象**：`TestFileBrowseServiceReturnsOnlyObjectMetadataWithoutClaimingObjects` 验证不创建记录、不赋状态、不提供访问 URL。

### 文件上传安全配置

实现入口：`initialize/server.go`、`config.yaml`。

- ✅ **使用合法配置**：`TestInitConfigLoadsFileUploadConfig`、`TestInitConfigAcceptsFileUploadBoundaryValues`。
- ✅ **使用非法配置**：`TestInitConfigRejectsInvalidFileUploadConfig`。

### 普通文件 V1 类型策略

实现入口：`service/uploadsecurity/types.go`、`managed_file.go`、`content.go`、`filename.go`。

- ✅ **上传允许的栅格图片**：`TestManagedFileV1AllowedTypeMatrix`、`TestValidateContentAcceptsCompleteStaticImages`。
- ✅ **上传 PDF**：`TestManagedFileV1AllowedTypeMatrix`、`TestValidateContentChecksPDFHeaderAndEOFMarker`。
- ✅ **上传 UTF-8 文本**：`TestManagedFileV1AllowedTypeMatrix`、`TestValidateContentAcceptsStreamingUTF8TextWithOptionalBOM`。
- ✅ **上传非 UTF-8 文本**：`TestValidateContentRejectsInvalidTextEncodingAndNUL`。
- ✅ **上传危险或不支持格式**：`TestManagedFileV1ForbiddenTypeMatrix`。
- ✅ **上传危险双扩展名**：`TestValidateExtensionChain`、`TestManagedFileV1RejectsMIMEAndDangerousDoubleExtensionMatrix`。

### Office Open XML 容器校验

实现入口：`service/uploadsecurity/ooxml.go`、`ooxml_zip.go`。

- ✅ **合法 Office Open XML 文件**：`TestValidateOOXMLAcceptsRequiredDOCXXLSXAndPPTXStructures`。
- ✅ **普通 ZIP 伪装为 Office**：`TestValidateOOXMLRejectsMissingOrInvalidRequiredStructure`、`TestValidateOOXMLRejectsDisguisedOrMixedOfficePackages`。
- ✅ **Office 文件包含主动或嵌入内容**：`TestValidateOOXMLRejectsDangerousEntriesContentTypesAndRelationships`、`TestValidateOOXMLRejectsDangerousSignaturesHiddenByNeutralEntryNames`。
- ✅ **Office 文件包含外部关系**：`TestValidateOOXMLAllowsOnlyHTTPHyperlinkExternalRelationships`、`TestValidateOOXMLRejectsOtherExternalRelationships`。
- ✅ **Office 文件加密或密码保护**：`TestValidateOOXMLRejectsEncryptedOLEContainer`。
- ✅ **Office 容器路径不安全**：`TestOpenRestrictedZIPRejectsUnsafeEntryNamesAndNestedArchives`、`TestOpenRestrictedZIPRejectsCorruptCentralDirectory`。
- ✅ **Office 容器超过资源限制**：`TestOpenRestrictedZIPEnforcesDeclaredResourceLimits`、`TestOpenRestrictedZIPEnforcesCompressionRatioLimits`。
- ✅ **Office XML 解析不安全**：`TestValidateOOXMLRejectsDTDExternalEntitiesAndExcessiveDepth`。

### 管理员文件下载和预览

实现入口：`service/file.go`、`service/file_content.go`、`service/fileaccess/`、`handler/file.go`、`router/router.go`。

- ✅ **下载验证通过文件**：`TestResolveDownloadAccessUsesCanonicalMIMEForValidatedFile`、`TestFileHandlerDownloadStreamsControlledAttachmentWithSecurityHeaders`、`TestFileHandlerSurfacesFirstStorageReadFailureBeforeCommittingSuccess`。
- ✅ **下载历史未验证文件**：`TestResolveDownloadAccessForcesUnverifiedFilesToBinaryAttachment`、`TestFileValidationStateMatrix`。
- ✅ **下载零字节历史文件**：`TestFileContentServiceAllowsEmptyHistoricalDownload`。
- ✅ **对象首批字节伴随存储错误**：`TestFileContentServicePreservesErrorReturnedWithFirstByte`、`TestFileHandlerSurfacesFirstStorageReadFailureBeforeCommittingSuccess`。
- ✅ **预览验证通过图片**：`TestResolvePreviewAccessAllowsOnlyValidatedImagesInline`、`TestFileHandlerPreviewStreamsControlledImageInlineWithSecurityHeaders`。
- ✅ **预览非图片或未验证文件**：`TestResolvePreviewAccessRejectsNonImagesAndDisallowedStates`、`TestFileHandlerPreviewMapsRestrictionsAndInvalidSignaturesToStableErrors`。
- ✅ **临时 URL 无效**：`TestDerivedSignerRejectsEveryBoundClaimTamper`、`TestDerivedSignerRejectsDifferentJWTSecretAndExpiredAccess`、`TestFileContentServiceRejectsInvalidSignatureBeforeOpeningObject`。

### 历史文件验证状态和重新验证

实现入口：`initialize/mysql.go`、`service/file.go` 的 `FileRevalidationService`。

- ✅ **历史记录迁移**：`TestMigrateDatabaseBackfillsUploadValidationStatusIdempotently`、`TestMigrateDatabaseDoesNotRequireObjectStorage`。
- ✅ **重新验证成功**：`TestFileRevalidationServiceMarksPassingHistoricalFileValidatedWithoutRewritingObject`。
- ✅ **重新验证确认不合规**：`TestFileRevalidationServiceMarksPolicyRejectionBlockedWithoutDeletingObject`。
- ✅ **重新验证遇到临时错误**：`TestFileRevalidationServiceMarksStorageFailureValidationErrorForRetry`、`TestFileRevalidationServiceMarksTemporaryValidationFailureForRetry`、`TestFileRevalidationServicePreservesStorageFailureFromObjectRead`、`TestStagePreservesClassifiedStorageReadFailures`。
- ✅ **对已验证或封锁文件重复重新验证**：`TestFileRevalidationServiceRejectsFinalStatesBeforeOpeningObject`。
- ✅ **系统启动时存在历史文件**：`TestMigrateDatabaseDoesNotRequireObjectStorage`；`initialize/mysql.go` 的迁移路径只有 GORM 回填，不导入对象存储依赖。

## user-management

### 当前用户资料

实现入口：`service/user.go`、`service/user_info.go`、`handler/user.go`、`dto/user.go`。

- ✅ **查询当前用户资料**：`TestUserInfoFromModelUsesControlledAvatarURLForTrustedAvatar`、`TestUserInfoFromModelFallsBackForUntrustedAvatarMetadata`、`TestGetUserInfoFallsBackToDefaultForLegacyAvatarURL`。
- ✅ **修改当前用户资料**：`TestUpdateSelfUpdatesOnlyNicknameAndEmail`、`TestUpdateSelfRejectsEmailUsedByAnotherUser`、`TestUpdateSelfClassifiesEmptyProfileRequest`。
- ✅ **资料接口提交头像字段**：`TestUpdateSelfRejectsAvatarFieldBeforePersistingOtherChanges`、`TestUpdateSelfRejectsNullAvatarField`、`TestUserHandlerUpdateSelfRejectsAvatarBeforeProfilePersistence`。

### 头像上传

实现入口：`service/uploadsecurity/avatar.go`、`service/avatar.go`、`handler/user.go`。

- ✅ **无透明头像上传成功**：`TestNormalizeAvatarReencodesOpaqueAndTransparentPixelsToTrustedFormats`、`TestAvatarServiceStoresNormalizedOutputAndTrustedMetadata`。
- ✅ **透明头像上传成功**：`TestNormalizeAvatarReencodesOpaqueAndTransparentPixelsToTrustedFormats`、`TestNormalizeAvatarConvertsWebPToTrustedJPEGOrPNGOutput`。
- ✅ **头像请求体、原文件或输出过大**：`TestUserHandlerUploadAvatarRejectsBodyAboveHardLimitBeforeMultipartParsing`、`TestNormalizeAvatarEnforcesReencodedOutputSizeBoundary`。
- ✅ **头像尺寸或像素超限**：`TestProbeAvatarHeaderRejectsDimensionLimitsBeforeFullDecode`。
- ✅ **头像格式不受支持或损坏**：`TestNormalizeAvatarRejectsCorruptMismatchedAndAnimatedImages`。
- ✅ **新头像对象写入失败**：`TestAvatarServicePreservesOriginalWhenNewObjectWriteFails`。
- ✅ **新头像写入后数据库确认未提交**：`TestAvatarServiceDeletesNewObjectWhenDatabaseUpdateIsNotCommitted`。
- ✅ **新头像数据库提交结果不确定**：`TestAvatarServiceKeepsNewObjectWhenDatabaseCommitOutcomeIsZeroValue`。
- ✅ **新头像替换成功**：`TestAvatarServiceDeletesOldTrustedObjectAfterCommittedReplacement`、`TestAvatarServiceKeepsSuccessfulReplacementWhenOldObjectCleanupFails`、`TestAvatarServiceDoesNotDeleteNewObjectAfterCommittedDatabaseUpdate`。

### 恢复默认头像

实现入口：`service/avatar.go`、`handler/user.go`。

- ✅ **恢复默认头像成功**：`TestAvatarServiceRestoresDefaultBeforeDeletingOldTrustedObject`、`TestUserHandlerRestoreDefaultAvatarReturnsControlledDefaultURL`。
- ✅ **恢复默认头像时数据库更新失败**：`TestAvatarServicePreservesTrustedAvatarWhenRestoreUpdateFails`。
- ✅ **重复恢复默认头像**：`TestAvatarServiceRestoreDefaultIsIdempotentWhenAlreadyDefault`。
- ✅ **删除旧头像对象失败**：`TestAvatarServiceKeepsDefaultWhenRestoreCleanupFails`、`TestUserHandlerAvatarMutationsReturnStableErrorsWithoutLeakingDetails`。

### 历史头像来源过渡

实现入口：`model/user.go`、`initialize/mysql.go`、`service/user_info.go`。

- ✅ **用户存在历史自定义头像**：`TestUserInfoFromModelFallsBackForUntrustedAvatarMetadata`、`TestGetUserInfoFallsBackToDefaultForLegacyAvatarURL`、`TestGetRoleUsersUsesControlledAvatarURLs`、`TestGetAllUsersUsesControlledAvatarURLs`、`TestGetOrganizationUsersUsesControlledAvatarURLs`。
- ✅ **历史头像迁移**：`TestMigrateDatabaseBackfillsUploadValidationStatusIdempotently`、`TestMigrateDatabaseDoesNotRequireObjectStorage`。
- ✅ **用户重新上传头像**：`TestAvatarServiceStoresNormalizedOutputAndTrustedMetadata` 验证只保存新系统对象；`TestAvatarServiceDeletesOldTrustedObjectAfterCommittedReplacement` 只清理可确认归属的旧系统对象。

### 受控头像读取

实现入口：`service/avatar_content.go`、`handler/user.go`、`router/router.go`。

- ✅ **读取可信用户头像**：`TestAvatarContentServiceOpensOnlyTrustedNormalizedAvatar`、`TestUserHandlerGetAvatarStreamsControlledImageWithPublicHeaders`。
- ✅ **读取不可信或不可用的用户头像**：`TestAvatarContentServiceFallsBackToSameBuiltInPNG`、`TestUserHandlerGetAvatarUsesIndistinguishableDefaultForInvalidUserID`、`TestUserHandlerGetAvatarFallsBackWhenStoredObjectFailsOnFirstRead`。
- ✅ **读取默认头像**：`TestUserHandlerGetDefaultAvatarReturnsBuiltInPNGWithLongCache`。
- ✅ **未携带 JWT 读取头像**：`TestInitRouterRegistersAPIRoutes`、`TestSyncAPIsGeneratesFileAccessPermissionsAndKeepsAvatarRoutesPublic`。

## logging

### API 审计日志

实现入口：`middleware/audit_log.go`、`middleware/audit_log_category.go`、`service/upload_audit.go`。

- ✅ **普通 API 请求被记录**：`TestAuditLogPersistsRequestFieldsAndRecursivelyMasksJSON`。
- ✅ **请求体脱敏**：`TestAuditLogPersistsRequestFieldsAndRecursivelyMasksJSON` 验证对象和对象数组中的密码、token、验证码递归脱敏。
- ✅ **multipart 请求**：`TestAuditLogPersistsUploadMetadataAfterHandlerAndOmitsMultipartBody`。
- ✅ **上传验证成功被记录**：`TestFileHandlerUploadPassesBoundedInputToFileService`、`TestUserHandlerUploadAvatarPassesBoundedInputToAvatarService`、`TestUploadAuditMetadataJSONUsesOnlyControlledFields`。
- ✅ **上传被安全策略拒绝**：`TestFileHandlerUploadMapsServiceErrorAndRecordsRejectedAuditMetadata`、`TestUserHandlerUploadAvatarRecordsSanitizedRejectedAuditMetadata`、`TestAuditLogIgnoresUncontrolledUploadMetadataTypes`。
- ✅ **在文件名不可用前拒绝**：文件和头像 handler 的请求体超限、缺失文件和损坏 multipart 测试验证只写用途与稳定原因码，不写未清洗路径。

### 审计日志查询

实现入口：`handler/audit_log.go`、`service/audit_log.go`、`service/audit_log_helper.go`。

- ✅ **查询全部 API 日志**：`TestListAuditLogsReturnsStructuredMetadata`、`TestListAuditLogsByCategoriesReturnsEndpointSpecificResults`。
- ✅ **查询登录日志**：`TestAuditLogClassifiesLoginReadsOperationsAndPermissionMutations`、`TestListAuditLogsByCategoriesReturnsEndpointSpecificResults`。
- ✅ **查询操作日志**：`TestListAuditLogsByCategoriesReturnsEndpointSpecificResults` 验证同时返回 `operation` 和 `permission`。
- ✅ **查询权限变更日志**：`TestAuditLogClassifiesLoginReadsOperationsAndPermissionMutations` 验证 GET 权限查询归类为 `data_access`、写操作归类为 `permission`；分类查询测试验证只返回 `permission`。
- ✅ **查询数据访问日志**：`TestAuditLogClassifiesLoginReadsOperationsAndPermissionMutations`、`TestListAuditLogsByCategoriesReturnsEndpointSpecificResults`。

### 审计日志冷热归档

实现入口：`main.go` 的启用开关、`service/audit_log_archive.go`。

- ✅ **归档任务未启用**：`main.go` 只在 `audit_log_archive.enabled` 为 true 时启动定时任务；`TestArchiveAuditLogsDisabledLeavesHotRecordsUntouched` 验证关闭时不移动记录。
- ✅ **归档过期日志**：`TestArchiveAuditLogsMovesExpiredRecordWithExactMetadataAfterSuccessfulCopy` 验证 metadata 和其他字段原样复制、复制成功后删除热记录；`TestArchiveAuditLogsKeepsHotRecordWhenArchiveInsertFails` 验证事务失败不删除热记录。

## 历史兼容边界

- ✅ `model.User.Avatar` 仍保留为旧头像兼容字段，`model/user_test.go` 和迁移测试验证新增字段与历史字段共存。
- ✅ `initialize/mysql.go` 的 `backfillUploadValidationStatus` 只执行 SQL 状态回填。
- ✅ `TestMigrateDatabaseDoesNotRequireObjectStorage` 验证迁移不读取、解码、删除或重写 MinIO 对象。
- ✅ V1 没有自动批量扫描、自动认领孤立对象或删除历史对象。

## 代码审查缺陷回归

- ✅ **头像提交边界**：数据库更新成功即视为提交完成；仓储不再执行提交后查询，且 `TestAvatarServiceDoesNotDeleteNewObjectAfterCommittedDatabaseUpdate` 验证新对象不会被误删。
- ✅ **文件流首读失败**：`TestFileHandlerSurfacesFirstStorageReadFailureBeforeCommittingSuccess` 验证下载与预览在提交 `200` 前返回稳定的存储错误。
- ✅ **头像流首读失败**：`TestUserHandlerGetAvatarFallsBackWhenStoredObjectFailsOnFirstRead` 验证惰性 reader 首读失败时返回可解码的内置默认 PNG。
- ✅ **重新验证错误分类**：`TestFileRevalidationServicePreservesStorageFailureFromObjectRead` 和 `TestStagePreservesClassifiedStorageReadFailures` 验证对象不存在与存储不可用不会被覆盖为请求体错误。
- ✅ 上述针对性回归命令使用 `-count=2` 连续通过。

## 验收命令

```powershell
go test ./initialize ./service/uploadsecurity ./service/fileaccess ./service/objectstorage ./service ./handler ./middleware ./router ./dto ./model
go vet ./...
go test ./...
```
