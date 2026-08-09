# 字典管理

> 本文只保留字典模块边界。接口契约以本地 `http://localhost:8080/docs` 与 `http://localhost:8080/docs/openapi.json` 为准；当前行为以 [`openspec/specs/dict-management/spec.md`](../../openspec/specs/dict-management/spec.md) 为准。

## 模块边界

`internal/dictionary` 负责字典类型、字典条目和公开读取接口。

## 关键规则

- 字典类型编码在未删除数据中唯一。
- 同一类型下的条目值唯一。
- 删除字典类型时同步删除其条目。
- 修改类型编码时同步更新条目的 `type_code`。
- 管理端接口支持类型和条目 CRUD、分页和筛选。
- 公开读取接口只返回启用的字典类型或条目，并按 `sort asc, id asc` 排序。

## 接口入口

Swagger UI 的 `dict` 标签提供管理端接口；公开字典读取接口为：

```text
GET /api/dicts/:type_code/items
```

字段、参数和响应 Schema 不在本页重复维护。
