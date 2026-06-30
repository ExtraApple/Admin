# Organization CRUD Design

## Model

| 字段 | 说明 |
|---|---|
| `parent_id` | 父组织 ID，0 表示根组织 |
| `name` | 组织名称 |
| `code` | 组织编码，全局唯一 |
| `remark` | 备注 |
| `sort` | 排序 |
| `status` | 状态，1 启用，0 禁用 |

## Routes

- `GET /api/admin/organizations`
- `GET /api/admin/organizations/tree`
- `POST /api/admin/organizations`
- `PUT /api/admin/organizations/:id`
- `DELETE /api/admin/organizations/:id`

## Tree Safety

修改 `parent_id` 时必须防止循环：

```text
A
└── B
    └── C
```

不允许把 `A.parent_id` 设置为 `C.id`。
