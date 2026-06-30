# Organization Members Design

## Model

`user_organizations`

| 字段 | 说明 |
|---|---|
| `user_id` | 用户 ID |
| `organization_id` | 组织 ID |

使用联合主键，避免重复绑定。

## Routes

- `POST /api/admin/organizations/:id/users`
- `GET /api/admin/organizations/:id/users`

## Assignment Behavior

`POST /api/admin/organizations/:id/users` 使用覆盖式分配：

```text
删除该组织旧成员关联
→ 写入新的 user_ids 关联
```

空数组表示清空该组织成员。
