import { describe, expect, it } from 'vitest';
import { canAccessAuthorizationOverview, canAccessAuthorizationRisks, canAccessModule, hasPermission, safeDestination } from './navigation';
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
