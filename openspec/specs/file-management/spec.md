# file-management Specification

## Purpose

文件管理允许管理员上传、查询、查看详情、改名、删除、浏览和轮转存储在 MinIO 中的文件，并在 MySQL 中维护文件元数据。

## Requirements

### Requirement: 管理员上传文件
系统 SHALL 允许管理员上传文件到 MinIO，并持久化文件元数据。

#### Scenario: 上传成功
- **WHEN** 管理员向 `POST /api/admin/files` 上传不超过 50 MB 的文件
- **THEN** 系统使用生成的对象名将文件存入 `files` bucket
- **AND** 系统创建包含原始文件名、bucket、对象名、Content-Type、大小和上传者 ID 的数据库记录
- **AND** 响应返回文件元数据

#### Scenario: 上传文件超过大小限制
- **WHEN** 上传文件大于 50 MB
- **THEN** 系统拒绝上传

#### Scenario: 对象上传后元数据写入失败
- **WHEN** MinIO 上传成功，但数据库元数据创建失败
- **THEN** 系统删除刚上传的 MinIO 对象
- **AND** 系统返回错误

### Requirement: 管理员文件列表
系统 SHALL 提供已上传文件的分页元数据列表。

#### Scenario: 查询文件列表
- **WHEN** 管理员调用 `GET /api/admin/files`
- **THEN** 系统按创建时间倒序返回文件元数据
- **AND** 响应包含总数、页码和每页数量

#### Scenario: 按对象名前缀查询文件
- **WHEN** 请求提供 prefix 查询参数
- **THEN** 系统只返回 `object_name` 以前缀开头的文件

### Requirement: 管理员文件详情
系统 SHALL 提供文件元数据和短期有效的下载链接。

#### Scenario: 文件存在
- **WHEN** 管理员查询存在的文件
- **THEN** 系统返回持久化的文件元数据
- **AND** 系统返回有效期为 300 秒的 MinIO 预签名下载 URL

#### Scenario: 文件不存在
- **WHEN** 请求的文件 ID 不存在
- **THEN** 系统返回文件不存在错误

### Requirement: 管理员修改文件元数据
系统 SHALL 允许管理员修改文件展示名称，但不修改 MinIO 对象名。

#### Scenario: 修改文件名
- **WHEN** 管理员提交非空 `name`
- **THEN** 系统更新数据库中的 `name` 字段
- **AND** 系统保持 `bucket` 和 `object_name` 不变

### Requirement: 管理员删除文件
系统 SHALL 同时删除文件的 MinIO 对象和数据库元数据。

#### Scenario: 删除文件成功
- **WHEN** 管理员删除存在的文件
- **THEN** 系统删除 MinIO 对象
- **AND** 系统硬删除数据库元数据记录

#### Scenario: MinIO 删除失败
- **WHEN** MinIO 对象无法删除
- **THEN** 系统不删除数据库元数据记录
- **AND** 系统返回错误

### Requirement: MinIO 文件浏览
系统 SHALL 允许管理员按前缀浏览 `files` bucket 中的原始对象。

#### Scenario: 浏览文件
- **WHEN** 管理员调用浏览接口并提供可选 prefix
- **THEN** 系统返回 `files` bucket 中的 MinIO 对象信息

### Requirement: 文件轮转
系统 SHALL 支持按配置将旧文件从热 bucket 轮转到冷 bucket。

#### Scenario: 文件轮转未启用
- **WHEN** 配置未启用文件轮转
- **THEN** 系统跳过轮转

#### Scenario: 轮转符合条件的文件
- **WHEN** 文件轮转已启用
- **AND** 热 bucket 中存在超过配置天数阈值的文件
- **THEN** 系统最多处理配置的 batch size 数量
- **AND** 系统将文件从热 bucket 移动到冷 bucket
- **AND** 移动成功后更新数据库元数据中的 bucket 字段

#### Scenario: 轮转时数据库更新失败
- **WHEN** 对象移动成功，但数据库 bucket 字段更新失败
- **THEN** 系统尝试将对象移回热 bucket
- **AND** 系统继续处理后续文件


