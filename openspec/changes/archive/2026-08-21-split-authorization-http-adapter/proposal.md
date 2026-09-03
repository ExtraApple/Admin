## Why

`internal/authorization/adapters/http/routes.go` currently combines route aggregation, request and response DTOs, role CRUD, permission CRUD, permission groups, user-role operations, data-scope operations and shared Gin helpers in one large file. This makes a local Authorization HTTP change harder to review and increases merge conflicts even though the module boundary itself is already correct.

## What Changes

- Split the Authorization HTTP adapter by cohesive capability while retaining a small route aggregation entry point.
- Keep shared Handler state and genuinely shared HTTP binding, pagination and response helpers in one local shared file; do not create a new cross-module abstraction.
- Keep role-permission assignment with permission handling and place each request/response DTO next to the capability that owns it.
- Preserve the exact Route Descriptor order so Seed, API Metadata synchronization, RBAC synchronization, OpenAPI generation and route snapshots remain stable.
- Add structural and route-snapshot tests that prevent the adapter from collapsing back into a single mixed-responsibility file.
- Make no HTTP contract, authorization behavior, application-layer, persistence, route, permission-code, database, Redis or MinIO change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `modular-layered-architecture`: require a complex module's HTTP adapter to keep route aggregation, capability handlers and shared transport helpers cohesive without creating new business modules or framework dependencies in Application/Domain.

## Impact

- Affected code is limited to `internal/authorization/adapters/http` and its focused tests.
- HTTP Method, Path, Access Level, middleware behavior, Permission Code, request/response JSON, status code, message text and OpenAPI output remain byte-for-byte or structurally compatible with the current route and contract snapshots.
- Authorization Domain, Application services, GORM adapters, App composition, tables, Redis keys and external clients are unaffected.
- The archived `2026-08-09-restructure-layered-monolith` change is the architecture baseline for this refactor.
