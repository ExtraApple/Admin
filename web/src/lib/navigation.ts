import type { AuditLogType, Menu, MenuSyncItem, UserContext } from './models';
import { AUDIT_TAB_ORDER } from './audit-query';
export type ManagementModule = 'roles' | 'users' | 'organizations' | 'menus' | 'apis';

// Mirrors App.tsx/ManagementPages.tsx: the menu catalog a sync must create for
// canAccessModule to authorize these pages. Directories carry no path, so every
// entry is a root-level page menu.
const managementMenuCatalog: { module: ManagementModule; label: string; icon: string }[] = [
  { module: 'roles', label: '角色管理', icon: 'Shield' },
  { module: 'users', label: '用户管理', icon: 'Users' },
  { module: 'organizations', label: '组织管理', icon: 'Network' },
  { module: 'menus', label: '菜单管理', icon: 'Menu' },
  { module: 'apis', label: '接口管理', icon: 'Route' },
];

export const managementMenuRoutes: MenuSyncItem[] = managementMenuCatalog.map((entry, index) => ({
  name: entry.label,
  path: `/system/${entry.module}`,
  component: '',
  icon: entry.icon,
  permission_code: `admin.${entry.module}.get`,
  parent_path: '',
  sort: (index + 1) * 10,
  type: 2,
}));
export const authorizationOverviewPermission = 'admin.authorization.overview.get';
export const authorizationRisksPermission = 'admin.authorization.risks.get';
export function canAccessAuthorizationOverview(context: UserContext | null): boolean {
  return !!context && context.roles.includes('admin') && context.permissions.includes(authorizationOverviewPermission);
}
export function canAccessAuthorizationRisks(context: UserContext | null): boolean {
  return hasPermission(context, authorizationRisksPermission);
}

export function hasPermission(context: UserContext | null, permission: string): boolean {
  return !!context && (context.roles.includes('admin') || context.permissions.includes(permission));
}

export const auditLogPermissions: Record<AuditLogType, string> = {
  all: 'admin.audit-logs.get',
  login: 'admin.login-logs.get',
  operation: 'admin.operation-logs.get',
  permission: 'admin.permission-logs.get',
  data_access: 'admin.data-access-logs.get',
};

export function canAccessAuditLogs(context: UserContext | null): boolean {
  return !!context && Object.values(auditLogPermissions).some(permission => hasPermission(context, permission));
}

export function allowedAuditLogTabs(context: UserContext | null): AuditLogType[] {
  return AUDIT_TAB_ORDER.filter(type => hasPermission(context, auditLogPermissions[type]));
}

export function canAccessModule(context: UserContext | null, module: ManagementModule): boolean {
  if (!context || !hasPermission(context, `admin.${module}.get`)) return false;
  const visible = (menus: Menu[]): boolean => menus.some(menu => menu.status === 1 &&
    ((!menu.permission_code || hasPermission(context, menu.permission_code)) && menu.type === 2 && menu.path === `/system/${module}` || visible(menu.children ?? [])));
  return visible(context.menus);
}
export function safeDestination(value: unknown): string {
  return typeof value === 'string' && /^\/(?!\/)/.test(value) && !value.includes('\\') && !/^\/(login|register)(?:[/?#]|$)/.test(value) ? value : '/';
}
