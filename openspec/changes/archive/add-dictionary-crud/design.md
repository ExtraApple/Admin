# Dictionary CRUD Design

## Models

| 模型 | 表 | 说明 |
|---|---|---|
| `DictType` | `dict_types` | 字典类型，例如 `user_status` |
| `DictItem` | `dict_items` | 字典条目，例如 `1=启用` |

## Routes

管理员管理接口：

- `GET /api/admin/dict-types`
- `POST /api/admin/dict-types`
- `PUT /api/admin/dict-types/:id`
- `DELETE /api/admin/dict-types/:id`
- `GET /api/admin/dict-items`
- `POST /api/admin/dict-items`
- `PUT /api/admin/dict-items/:id`
- `DELETE /api/admin/dict-items/:id`

前端读取接口：

- `GET /api/dicts/:type_code/items`

## Status

| status | 说明 |
|---|---|
| `1` | 启用 |
| `0` | 禁用 |

前端读取接口只返回启用的字典类型和启用的字典条目。
