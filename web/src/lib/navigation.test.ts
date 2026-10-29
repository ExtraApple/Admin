import { describe, expect, it } from 'vitest';
import { canAccessAuthorizationOverview, canAccessAuthorizationRisks, canAccessModule, hasPermission, managementMenuRoutes, safeDestination } from './navigation';
import type { Menu, UserContext } from './models';
const menu: Menu = { id: 2, parent_id: 1, name: '用户管理', path: '/system/users', component: 'system/users/index', icon: 'Users', permission_code: 'admin.users.get', sort: 10, type: 2, status: 1 };
const context: UserContext = { user: { id: 7, username: 'operator', nickname: '', avatar: '', email: '', role: 'admin', status: 1 }, roles: ['reader'], permissions: ['admin.users.get'], menus: [menu] };
describe('real menus and permissions jointly authorize entry', () => {
  it('exposes only the supported, enabled menu with its read permission', () => {
    expect(canAccessModule(context, 'users')).toBe(true);
    expect(canAccessModule(context, 'roles')).toBe(false);
    expect(canAccessModule({ ...context, permissions: [] }, 'users')).toBe(false);
    expect(canAccessModule({ ...context, menus: [{ ...menu, status: 0 }] }, 'users')).toBe(false);
    expect(canAccessModule({ ...context, menus: [{ ...menu, type: 3 }] }, 'users')).toBe(false);
    expect(canAccessModule({ ...context, menus: [{ ...menu, path: '/system/users-disabled' }] }, 'users')).toBe(false);
  });
  it('recognizes backend admin fallback from real roles, never legacy user.role or invented menus', () => {
    const administrator = { ...context, roles: ['admin'], permissions: [] };
    expect(hasPermission(administrator, 'admin.users.id.roles.put')).toBe(true);
    expect(canAccessModule(administrator, 'users')).toBe(true);
    expect(canAccessModule({ ...administrator, menus: [] }, 'users')).toBe(false);
    expect(hasPermission({ ...context, roles: [], permissions: [] }, 'admin.users.get')).toBe(false);
  });
  it('finds an enabled visible entry nested beneath a real directory and rejects absent context', () => {
    expect(canAccessModule({ ...context, menus: [{ ...menu, id: 1, path: '/system', type: 1, permission_code: '', children: [menu] }] }, 'users')).toBe(true);
    expect(canAccessModule(null, 'users')).toBe(false);
  });
});
describe('frontend menu route catalog', () => {
  it('declares one sync-valid page menu per gated module', () => {
    const paths = managementMenuRoutes.map(item => item.path);
    expect(new Set(paths).size).toBe(paths.length);
    expect(managementMenuRoutes).toEqual([
      { name: '角色管理', path: '/system/roles', component: '', icon: 'Shield', permission_code: 'admin.roles.get', parent_path: '', sort: 10, type: 2 },
      { name: '用户管理', path: '/system/users', component: '', icon: 'Users', permission_code: 'admin.users.get', parent_path: '', sort: 20, type: 2 },
      { name: '组织管理', path: '/system/organizations', component: '', icon: 'Network', permission_code: 'admin.organizations.get', parent_path: '', sort: 30, type: 2 },
      { name: '菜单管理', path: '/system/menus', component: '', icon: 'Menu', permission_code: 'admin.menus.get', parent_path: '', sort: 40, type: 2 },
      { name: '接口管理', path: '/system/apis', component: '', icon: 'Route', permission_code: 'admin.apis.get', parent_path: '', sort: 50, type: 2 },
    ]);
  });

  it('keeps every catalog path exactly what canAccessModule requires', () => {
    for (const module of ['roles', 'users', 'organizations', 'menus', 'apis'] as const) {
      const entry = managementMenuRoutes.find(item => item.permission_code === `admin.${module}.get`);
      expect(entry).toBeDefined();
      const menu = { id: 1, parent_id: 0, name: entry!.name, path: entry!.path, component: '', icon: '', permission_code: entry!.permission_code, sort: 0, type: 2, status: 1 };
      expect(canAccessModule({ ...context, permissions: [`admin.${module}.get`], menus: [menu] }, module)).toBe(true);
    }
  });
});
describe('authorization overview entry points', () => {
  const administrator: UserContext = { ...context, roles: ['admin'], permissions: ['admin.authorization.overview.get', 'admin.authorization.risks.get'] };
  it('requires the protected admin role and the explicit overview permission', () => {
    expect(canAccessAuthorizationOverview(administrator)).toBe(true);
    expect(canAccessAuthorizationOverview({ ...administrator, permissions: [] })).toBe(false);
    expect(canAccessAuthorizationOverview({ ...context, permissions: ['admin.authorization.overview.get'] })).toBe(false);
  });
  it('allows the separately protected risk list whenever its read permission is present', () => {
    expect(canAccessAuthorizationRisks(administrator)).toBe(true);
    expect(canAccessAuthorizationRisks({ ...context, permissions: ['admin.authorization.risks.get'] })).toBe(true);
    expect(canAccessAuthorizationRisks({ ...administrator, permissions: [] })).toBe(true);
    expect(canAccessAuthorizationRisks(null)).toBe(false);
  });
});
describe('login destination', () => {
  it('preserves direct detail/editor URLs including filters and hash', () => {
    expect(safeDestination('/users/7/roles?source=users#selection')).toBe('/users/7/roles?source=users#selection');
  });
  it.each([undefined, 'https://example.com', '//example.com', '/\\example.com', '/login', '/register?from=users'])('rejects external or auth-loop destinations: %s', value => {
    expect(safeDestination(value)).toBe('/');
  });
});
