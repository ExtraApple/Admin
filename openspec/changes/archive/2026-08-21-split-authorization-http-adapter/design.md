## Context

The modular-monolith migration is complete and archived as `2026-08-09-restructure-layered-monolith`. Authorization already owns its Domain, Application, GORM adapter and HTTP adapter correctly, but `internal/authorization/adapters/http/routes.go` is 528 lines and mixes twenty Route Descriptors, eleven request/response DTO families, role operations, user-role operations, data-scope operations, permission operations, route-permission synchronization, permission groups and shared Gin helpers.

The problem is local cohesion, not a missing business boundary. Splitting this file must not create new packages, change HTTP behavior or disturb Descriptor ordering consumed by Seed, API Metadata, RBAC synchronization, OpenAPI and App route snapshots. The later `standardize-api-response-contract` change owns response and error-contract changes; this change deliberately preserves the current contract.

## Goals / Non-Goals

**Goals:**

- Make each Authorization HTTP capability readable and changeable in one file.
- Keep `Routes` as the single, obvious aggregation point for the Authorization-owned HTTP surface.
- Preserve all twenty descriptors in their current order and preserve every descriptor field and OpenAPI schema.
- Keep request/response DTOs close to their owning handler capability.
- Keep only genuinely shared transport helpers in a local shared file.
- Add focused structural and route-contract checks that prevent accidental behavior drift.

**Non-Goals:**

- No response-envelope, error-code, message, status-code or validation semantic changes.
- No changes to HTTP Method, Path, Access Level, Permission Code, audit category or OpenAPI output.
- No changes to Authorization Domain, Application services, contracts, repositories, transactions or App composition.
- No new first-level module, subpackage, generic framework or shared cross-module HTTP abstraction.
- No database, Redis, MinIO, migration or Seed data changes.

## Decisions

### 1. Split by existing capability, not by technical artifact type

The final package remains `internal/authorization/adapters/http` with this layout:

```text
routes.go
roles.go
permissions.go
permission_groups.go
shared.go
```

- `routes.go` contains only the exported `Routes` aggregation and its stable descriptor sequence.
- `roles.go` contains role CRUD, role-user assignment/listing, role data-scope assignment/read, the associated request/response DTOs and `roleInfoOf`.
- `permissions.go` contains permission CRUD, permission-code listing, Route Catalog permission synchronization, role-permission assignment/listing, associated DTOs and `permissionInfoOf`.
- `permission_groups.go` contains permission-group CRUD and associated DTOs.
- `shared.go` contains `Handler`, `authRoute`, shared success/error response schemas, pagination/path/JSON binding helpers and the current bad-request writer.

Role-permission operations remain in `permissions.go`: they change and return permission assignments, while role-user and data-scope operations remain in `roles.go`. This is a smaller and more stable seam than creating separate files for every relation.

Alternative considered: separate `requests.go`, `responses.go` and `handlers.go`. Rejected because it would preserve the current technical scattering inside the package and force a reader to cross several files for one capability.

Alternative considered: create subpackages per capability. Rejected because the handler shares one Application service and one route source, the package has only twenty routes, and new package boundaries would add imports without protecting a distinct business invariant.

### 2. Preserve the descriptor list as one literal ordered sequence

`Routes` continues to construct one `Handler` and return one literal `[]routecatalog.Descriptor` in the existing order:

1. eight role/user/data-scope routes;
2. eight permission/synchronization/role-permission routes;
3. four permission-group routes.

The split does not concatenate independently returned slices. A single literal keeps the order visible and avoids accidental reordering caused by helper registration order. Each entry retains the same `authRoute` arguments, handler method and request/response schema type.

Alternative considered: each capability returns its own descriptor slice and `Routes` appends them. Rejected because it makes the externally consumed order depend on multiple functions and provides no benefit for twenty static routes.

### 3. Move declarations without renaming or reshaping them

Request/response type names, JSON tags, Gin binding tags, handler method names and mapping functions remain unchanged. Existing inline `gin.H` responses also remain unchanged. This avoids coupling this structural change to the later global response-contract migration and allows current HTTP compatibility tests to prove a pure move.

`authRoute`, `queryPage`, `pathID`, `bindJSON` and `badRequest` remain package-private. They are not promoted to `internal/platform`, because the current behavior is Authorization-specific and will be replaced deliberately by `standardize-api-response-contract` rather than generalized prematurely.

### 4. Verify structure and behavior separately

A package structural test uses Go AST inspection to assert that `routes.go` does not contain handler method bodies or request/response type declarations and that the expected capability files exist. It does not enforce line counts or formatting.

A route-contract test snapshots the ordered descriptors by Method, Path, Access Level, Name, Group, Default Permission Code, audit category, handler presence and OpenAPI request/response schema identity. Existing App route-snapshot and Authorization HTTP tests remain the behavioral safety net.

The structural check protects the purpose of the change; the route-contract check protects all downstream consumers of descriptor ordering and metadata.

### 5. Apply this change before the response-contract change

`split-authorization-http-adapter` is implementation-ready now. `standardize-api-response-contract` may later edit the newly separated files, but this change must first pass with the current external response contract unchanged. No compatibility shim or temporary duplicate route implementation is introduced.

## Risks / Trade-offs

- **Risk: declaration moves accidentally reorder descriptors or change schemas.** Mitigation: keep one literal ordered list and compare the complete ordered descriptor contract.
- **Risk: a helper is placed in `shared.go` merely because two handlers could use it later.** Mitigation: only move helpers already shared today; capability-specific mappers remain with their capability.
- **Risk: AST structural tests become formatting-sensitive.** Mitigation: inspect declarations through `go/parser`, not source text or line counts.
- **Trade-off: the package still has shared Handler state.** Accepted because there is one Authorization Application service and one Route Permission source; splitting that state would add constructors without creating a stronger boundary.
- **Trade-off: response code remains duplicated and ad hoc during this change.** Accepted temporarily and explicitly owned by the separate clean-cutover response-contract change.
