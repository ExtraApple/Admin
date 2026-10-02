import type { Menu, UserContext } from './models';
export type ManagementModule = 'roles' | 'users' | 'organizations';
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
export function canAccessModule(context: UserContext | null, module: ManagementModule): boolean {
  if (!context || !hasPermission(context, `admin.${module}.get`)) return false;
  const visible = (menus: Menu[]): boolean => menus.some(menu => menu.status === 1 &&
    ((!menu.permission_code || hasPermission(context, menu.permission_code)) && menu.type === 2 && menu.path === `/system/${module}` || visible(menu.children ?? [])));
  return visible(context.menus);
}
export function safeDestination(value: unknown): string {
  return typeof value === 'string' && /^\/(?!\/)/.test(value) && !value.includes('\\') && !/^\/(login|register)(?:[/?#]|$)/.test(value) ? value : '/';
}
