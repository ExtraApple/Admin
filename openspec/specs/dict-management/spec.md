# dict-management Specification

## Purpose

字典管理用于维护系统可配置枚举，为后台表单、状态展示和前端下拉框提供统一数据源。

## Requirements

### Requirement: 字典类型 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除字典类型。

#### Scenario: 查询字典类型列表
- **WHEN** 管理员请求 `GET /api/admin/dict-types`
- **THEN** 系统分页返回字典类型列表

#### Scenario: 创建字典类型
- **WHEN** 管理员请求 `POST /api/admin/dict-types`
- **THEN** 系统创建字典类型
- **AND** 字典类型编码 SHALL 在未删除数据中唯一

#### Scenario: 删除字典类型
- **WHEN** 管理员删除字典类型
- **THEN** 系统删除该字典类型
- **AND** 系统删除该类型下的字典条目

### Requirement: 字典条目 CRUD
系统 SHALL 支持管理员创建、查询、修改和删除字典条目。

#### Scenario: 查询字典条目列表
- **WHEN** 管理员请求 `GET /api/admin/dict-items`
- **THEN** 系统分页返回字典条目列表

#### Scenario: 创建字典条目
- **WHEN** 管理员请求 `POST /api/admin/dict-items`
- **THEN** 系统创建字典条目
- **AND** 同一字典类型下条目值 SHALL 唯一

### Requirement: 按类型编码获取字典条目
系统 SHALL 支持按字典类型编码获取启用状态的字典条目。

#### Scenario: 前端获取下拉框条目
- **WHEN** 请求 `GET /api/dicts/:type_code/items`
- **THEN** 系统返回该类型下状态为启用的条目
- **AND** 返回结果按 sort asc, id asc 排序
