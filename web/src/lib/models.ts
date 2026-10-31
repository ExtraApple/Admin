export interface Page<T> { list: T[]; total: number; page: number; size: number }
export type AuditLogType = 'all' | 'login' | 'operation' | 'permission' | 'data_access';
export type AuditCategory = 'api' | 'login' | 'operation' | 'permission' | 'data_access' | 'message';
export interface AuditLog {
  id: number;
  user_id: number;
  username: string;
  method: string;
  path: string;
  query: string;
  body: string;
  metadata: unknown;
  status: number;
  duration: number;
  client_ip: string;
  user_agent: string;
  category: AuditCategory;
  created_at: string;
}
export interface AuditLogPage extends Page<AuditLog> {}
export interface AuditLogQueryState {
  type: AuditLogType;
  page: number;
  size: number;
  start: string;
  end: string;
  method: string;
  status: string;
  path: string;
  username: string;
  userId: string;
}
export type MenuType = 1 | 2 | 3;
export type RouteState = 'matched' | 'metadata_only' | 'catalog_only' | 'access_mismatch';
export type MenuImpactOperation = 'create' | 'update' | 'delete' | 'bind-apis' | 'clear-apis' | 'sync';
export type ApiImpactOperation = 'update' | 'delete' | 'sync' | 'sync-permissions' | 'generate-menu-button';
export type ManagementConflictCode = 'NAV_CONFLICT' | 'API_META_CONFLICT';
export interface ImpactSummary {
  affected_menu_ids?: number[];
  affected_api_ids?: number[];
  affected_role_count: number;
  affected_user_count: number;
  access_version_change: boolean;
  requires_confirmation: boolean;
  created_count?: number;
  skipped_count?: number;
  conflict_count?: number;
}
export interface ImpactPreview<TTarget = unknown> {
  operation: string;
  normalized_target: TTarget;
  preview_fingerprint: string;
  summary: ImpactSummary;
}
export interface MenuSyncItem {
  name: string;
  path: string;
  component: string;
  icon: string;
  permission_code: string;
  parent_path: string;
  sort: number;
  type: number;
}
export interface ApiMetadata {
  id: number;
  name: string;
  method: string;
  path: string;
  group: string;
  permission_code: string;
  remark: string;
  sort: number;
  status: number;
  need_auth: number;
  need_audit: number;
  route_state?: RouteState;
}
export interface ApiMetadataPage extends Page<ApiMetadata> {}
export interface MenuApi {
  id: number;
  name: string;
  method: string;
  path: string;
  group: string;
  permission_code: string;
  remark: string;
  sort: number;
  status: number;
  need_auth: number;
  need_audit: number;
}
export interface ApiGroupOption { group: string; count: number }
export interface ApiMethodOption { label: string; value: string }
export interface MenuCreateInput {
  parent_id: number;
  name: string;
  path?: string;
  component?: string;
  icon?: string;
  permission_code?: string;
  sort?: number;
  type?: number;
  status?: number;
  expected_fingerprint: string;
}
export interface MenuUpdateInput {
  parent_id?: number;
  name?: string;
  path?: string;
  component?: string;
  icon?: string;
  permission_code?: string;
  sort?: number;
  type?: number;
  status?: number;
  expected_fingerprint: string;
}
export interface MenuSyncRequest {
  routes: MenuSyncItem[];
  expected_fingerprint: string;
}
export interface MenuBindingRequest {
  api_ids: number[];
  permission_code?: string;
  expected_fingerprint: string;
}
export interface MenuClearBindingRequest {
  clear: true;
  expected_fingerprint: string;
}
export interface ApiUpdateInput {
  name?: string;
  group?: string;
  permission_code?: string;
  remark?: string;
  sort?: number;
  status?: number;
  need_audit?: number;
  expected_fingerprint: string;
}
export interface ApiSyncRequest {
  expected_fingerprint: string;
}
export interface ApiPermissionSyncResult {
  created: string[];
  created_count: number;
  updated_api: number;
}
export interface ApiSyncResult {
  created: ApiMetadata[];
  count: number;
}
export interface GenerateMenuButtonInput {
  parent_id: number;
  name: string;
  sort: number;
  expected_fingerprint: string;
}
export interface ImpactPreviewRequest<TCandidate = unknown> {
  operation: string;
  candidate: TCandidate;
}
export interface UserInfo { id: number; username: string; nickname: string; avatar: string; email: string; role: string; status: number; pending_email?: string; email_verified?: boolean }
export interface EmailVerificationPolicy { enabled: boolean }
export interface EmailVerificationInput { token: string }
export interface CancelEmailVerificationInput { pending_email: string }
export interface AvatarUploadPolicy { max_bytes: number }
export interface Role { id: number; name: string; code: string; description: string; sort: number; status: number; data_scope: string }
export interface RoleSummary { id: number; code: string; name: string; status: number }
export interface OrganizationSummary { id: number; name: string }
export interface AdminUser { id: number; username: string; nickname: string; avatar: string; email: string; status: number; roles: RoleSummary[]; organizations: OrganizationSummary[]; has_unmanaged_organizations: boolean; access_version: number }
export interface Permission { id: number; name: string; code: string; group: string; sort: number }
export interface Menu { id: number; parent_id: number; name: string; path: string; component: string; icon: string; permission_code: string; sort: number; type: number; status: number; children?: Menu[] }
export interface Organization { id: number; parent_id: number; name: string; code: string; remark: string; sort: number; status: number; manageable: boolean; children?: Organization[] }
export interface DataScope { role_id: number; data_scope: string; organization_ids: number[] }
export interface UserContext { user: UserInfo; roles: string[]; permissions: string[]; menus: Menu[] }
export interface LoginInput { username: string; password: string; captcha_id: string; captcha_code: string }
export interface SessionGrant { access_token: string; session_id: string }
export interface Captcha { captcha_id: string; captcha_img: string }

export type AuthorizationRiskKind = 'disabled_assigned_menu' | 'missing_menu_permission' | 'used_role_without_permissions' | 'managed_user_without_role';
export type AuthorizationRiskResource = 'role' | 'user';
export interface AuthorizationSessionPolicy { revalidate_on_next_request: boolean }
export interface AuthorizationOverviewScope { data_scope: string; organization_count: number }
export interface AuthorizationOverviewCount { total: number; enabled: number; used?: number; without_role?: number; manageable?: number }
export interface AuthorizationOverviewSummary { roles: AuthorizationOverviewCount; users: AuthorizationOverviewCount; organizations: AuthorizationOverviewCount }
export interface AuthorizationRisk {
  resource: AuthorizationRiskResource;
  resource_id: number;
  resource_name: string;
  issue_count: number;
  issue_kinds: AuthorizationRiskKind[];
  potentially_affected_users: number;
  potentially_affected_organizations: number;
  session_policy: AuthorizationSessionPolicy;
}
export interface AuthorizationRiskSummary { items: AuthorizationRisk[]; total: number; limit: number; has_more: boolean }
export interface AuthorizationOverview { scope: AuthorizationOverviewScope; summary: AuthorizationOverviewSummary; risks: AuthorizationRiskSummary }
export interface AuthorizationRiskPage { scope: AuthorizationOverviewScope; list: AuthorizationRisk[]; total: number; page: number; size: number }
export interface AuthorizationRiskQuery { page?: number; size?: number; kind?: AuthorizationRiskKind; resource?: AuthorizationRiskResource; keyword?: string }
