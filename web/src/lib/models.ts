export interface Page<T> { list: T[]; total: number; page: number; size: number }
export interface UserInfo { id: number; username: string; nickname: string; avatar: string; email: string; role: string; status: number; pending_email?: string; email_verified?: boolean }
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
export interface TokenPair { access_token: string; refresh_token: string }
export interface Captcha { captcha_id: string; captcha_img: string }
