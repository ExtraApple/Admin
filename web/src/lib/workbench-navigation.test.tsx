// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Menu, UserContext } from './models';
import type { ManagementModule } from './navigation';
import { getInitialOpenGroups, getWorkbenchBreadcrumbs, getWorkbenchGroups, resolveWorkbenchModule, useWorkbenchNavigation, type WorkbenchGroupId } from './workbench-navigation';

function moduleMenu(module: ManagementModule, overrides: Partial<Menu> = {}): Menu {
  return { id: 1, parent_id: 0, name: '后端菜单名称', path: `/system/${module}`, component: `system/${module}/index`, icon: '', permission_code: `admin.${module}.get`, sort: 10, type: 2, status: 1, ...overrides };
}
const restrictedContext: UserContext = {
  user: { id: 7, username: 'operator', nickname: '', avatar: '', email: '', role: 'admin', status: 1 },
  roles: ['reader'], permissions: [], menus: [],
};
const administrator: UserContext = {
  ...restrictedContext, roles: ['admin'], permissions: ['admin.authorization.overview.get'],
  menus: [moduleMenu('roles'), moduleMenu('users'), moduleMenu('organizations')],
};
const configurationAdministrator: UserContext = {
  ...administrator,
  permissions: [...administrator.permissions, 'admin.menus.get', 'admin.apis.get'],
  menus: [...administrator.menus, moduleMenu('menus'), moduleMenu('apis')],
};
const allClosed = { 'authorization-review': false, 'identity-organization': false, 'configuration-management': false, 'audit': false };

function visibleEntries(context: UserContext | null) {
  return getWorkbenchGroups(context).map(group => ({ id: group.id, routes: group.items.map(item => item.to) }));
}

describe('permission-filtered workbench groups', () => {

  it.each([
    { roles: ['admin'], permissions: [], expected: ['/authorization-risks'] },
    { roles: ['admin'], permissions: ['admin.authorization.overview.get'], expected: ['/authorization-overview', '/authorization-risks'] },
    { roles: ['reader'], permissions: ['admin.authorization.overview.get'], expected: [] },
    { roles: ['reader'], permissions: ['admin.authorization.risks.get'], expected: ['/authorization-risks'] },
    { roles: ['reader'], permissions: ['admin.authorization.overview.get', 'admin.authorization.risks.get'], expected: ['/authorization-risks'] },
    { roles: [], permissions: [], expected: [] },
  ])('preserves the overview explicit-admin guard and separate risk permission: $roles / $permissions', ({ roles, permissions, expected }) => {
    const groups = getWorkbenchGroups({ ...restrictedContext, roles, permissions });
    expect(groups.find(group => group.id === 'authorization-review')?.items.map(item => item.to) ?? []).toEqual(expected);
    expect(groups.some(group => group.id === 'authorization-review')).toBe(expected.length > 0);
  });

  it('requires both module read permission and its real visible menu, then removes empty groups', () => {
    const usersOnly = { ...restrictedContext, permissions: ['admin.users.get', 'admin.roles.get'], menus: [moduleMenu('users'), moduleMenu('organizations')] };
    expect(visibleEntries(usersOnly)).toEqual([
      { id: 'identity-organization', routes: ['/users'] },
    ]);
    expect(visibleEntries(restrictedContext)).toEqual([]);
    expect(getWorkbenchGroups(null)).toEqual([]);
  });

  it('exposes configuration management only with the real menu and module permissions', () => {
    expect(visibleEntries(configurationAdministrator)).toEqual([
      { id: 'authorization-review', routes: ['/authorization-overview', '/authorization-risks'] },
      { id: 'identity-organization', routes: ['/roles', '/users', '/organizations'] },
      { id: 'configuration-management', routes: ['/menus', '/apis'] },
      { id: 'audit', routes: ['/audit-logs'] },
    ]);
    expect(visibleEntries({ ...configurationAdministrator, menus: [] }).some(entry => entry.id === 'configuration-management')).toBe(false);
  });


  it.each([
    { description: 'missing read permission', permissions: [], menus: [moduleMenu('users')] },
    { description: 'missing real menu', permissions: ['admin.users.get'], menus: [] },
    { description: 'disabled menu', permissions: ['admin.users.get'], menus: [moduleMenu('users', { status: 0 })] },
    { description: 'button instead of page', permissions: ['admin.users.get'], menus: [moduleMenu('users', { type: 3 })] },
    { description: 'lookalike backend path', permissions: ['admin.users.get'], menus: [moduleMenu('users', { path: '/system/users-disabled' })] },
    { description: 'ungranted menu permission', permissions: ['admin.users.get'], menus: [moduleMenu('users', { permission_code: 'admin.users.private.get' })] },
  ])('hides the identity group for $description', ({ permissions, menus }) => {
    expect(getWorkbenchGroups({ ...restrictedContext, permissions, menus }).map(group => group.id)).toEqual([]);
  });

  it('keeps backend admin fallback without inventing menus, and honors existing nested-menu access', () => {
    expect(getWorkbenchGroups({ ...administrator, menus: [] }).map(group => group.id)).toEqual(['authorization-review', 'audit']);
    const nested = { ...restrictedContext, permissions: ['admin.users.get'], menus: [moduleMenu('users', { path: '/system', type: 1, permission_code: '', children: [moduleMenu('users')] })] };
    expect(getWorkbenchGroups(nested).find(group => group.id === 'identity-organization')?.items.map(item => item.to)).toEqual(['/users']);
  });
});

describe('workbench initialization, breadcrumbs, and route resolution', () => {
  it('opens all visible groups by default and keeps hidden groups closed regardless of route', () => {
    expect(getInitialOpenGroups(getWorkbenchGroups(administrator), '/users/7')).toEqual({
      'authorization-review': true,
      'identity-organization': true,
      'configuration-management': false,
      audit: true,
    });
    expect(getInitialOpenGroups(getWorkbenchGroups(configurationAdministrator), '/profile')).toEqual({
      'authorization-review': true,
      'identity-organization': true,
      'configuration-management': true,
      audit: true,
    });
    expect(getInitialOpenGroups(getWorkbenchGroups(restrictedContext), '/unknown')).toEqual(allClosed);
  });

  it.each([
    ['/authorization-overview', [{ label: '工作台', to: '/' }, { label: '授权总览' }]],
    ['/authorization-risks', [{ label: '工作台', to: '/' }, { label: '风险清单' }]],
    ['/roles', [{ label: '工作台', to: '/' }, { label: '角色管理' }]],
    ['/roles/new', [{ label: '工作台', to: '/' }, { label: '角色管理', to: '/roles' }, { label: '新建角色' }]],
    ['/roles/3', [{ label: '工作台', to: '/' }, { label: '角色管理', to: '/roles' }, { label: '角色详情 #3' }]],
    ['/roles/3/permissions', [{ label: '工作台', to: '/' }, { label: '角色管理', to: '/roles' }, { label: '角色详情 #3', to: '/roles/3' }, { label: '编辑权限' }]],
    ['/roles/3/menus', [{ label: '工作台', to: '/' }, { label: '角色管理', to: '/roles' }, { label: '角色详情 #3', to: '/roles/3' }, { label: '编辑菜单' }]],
    ['/roles/3/data-scope', [{ label: '工作台', to: '/' }, { label: '角色管理', to: '/roles' }, { label: '角色详情 #3', to: '/roles/3' }, { label: '编辑数据范围' }]],
    ['/users/new', [{ label: '工作台', to: '/' }, { label: '用户管理', to: '/users' }, { label: '新建用户' }]],
    ['/users/12', [{ label: '工作台', to: '/' }, { label: '用户管理', to: '/users' }, { label: '用户详情 #12' }]],
    ['/users/12/roles', [{ label: '工作台', to: '/' }, { label: '用户管理', to: '/users' }, { label: '用户详情 #12', to: '/users/12' }, { label: '编辑角色' }]],
    ['/users/12/organizations', [{ label: '工作台', to: '/' }, { label: '用户管理', to: '/users' }, { label: '用户详情 #12', to: '/users/12' }, { label: '编辑组织' }]],
    ['/organizations/new', [{ label: '工作台', to: '/' }, { label: '组织管理', to: '/organizations' }, { label: '新建组织' }]],
    ['/organizations/5', [{ label: '工作台', to: '/' }, { label: '组织管理', to: '/organizations' }, { label: '组织详情 #5' }]],
    ['/organizations/5/move', [{ label: '工作台', to: '/' }, { label: '组织管理', to: '/organizations' }, { label: '组织详情 #5', to: '/organizations/5' }, { label: '调整上级' }]],
    ['/menus', [{ label: '工作台', to: '/' }, { label: '菜单管理' }]],
    ['/apis', [{ label: '工作台', to: '/' }, { label: '接口管理' }]],
    ['/audit-logs', [{ label: '工作台', to: '/' }, { label: '审计日志' }]],
    ['/profile', [{ label: '工作台', to: '/' }, { label: '个人资料' }]],
    ['/no-management', [{ label: '工作台', to: '/' }, { label: '访问说明' }]],
    ['/roles/3/unknown', [{ label: '工作台', to: '/' }, { label: '页面不存在' }]],
    ['/apis/42', [{ label: '工作台', to: '/' }, { label: '页面不存在' }]],
  ])('builds static hierarchical breadcrumbs for %s', (pathname, expected) => {
    expect(getWorkbenchBreadcrumbs(pathname)).toEqual(expected);
  });

  it('rejects 404 paths and unauthorized detail routes when resolving modules', () => {
    const listOnlyRoles: UserContext = {
      ...restrictedContext,
      permissions: ['admin.roles.get'],
      menus: [moduleMenu('roles')],
    };
    expect(resolveWorkbenchModule('/roles', listOnlyRoles)?.key).toBe('roles');
    expect(resolveWorkbenchModule('/roles/3', listOnlyRoles)).toBeNull();
    expect(resolveWorkbenchModule('/roles/3/unknown', administrator)).toBeNull();
    expect(resolveWorkbenchModule('/apis/42', configurationAdministrator)).toBeNull();
    expect(resolveWorkbenchModule('/profile', restrictedContext)?.key).toBe('profile');
    expect(resolveWorkbenchModule('/roles/new', listOnlyRoles)).toBeNull();
    expect(resolveWorkbenchModule('/roles/new', { ...listOnlyRoles, permissions: ['admin.roles.get', 'admin.roles.post'] })?.key).toBe('roles');
    expect(resolveWorkbenchModule('/users/new', { ...restrictedContext, permissions: ['admin.users.get'], menus: [moduleMenu('users')] })).toBeNull();
    expect(resolveWorkbenchModule('/users/new', { ...restrictedContext, permissions: ['admin.users.get', 'admin.users.post'], menus: [moduleMenu('users')] })?.key).toBe('users');
    expect(resolveWorkbenchModule('/organizations/new', { ...restrictedContext, permissions: ['admin.organizations.get', 'admin.organizations.post'], menus: [moduleMenu('organizations')] })).toBeNull();
    expect(resolveWorkbenchModule('/organizations/new', { ...restrictedContext, permissions: ['admin.organizations.get', 'admin.organizations.tree.get', 'admin.organizations.post'], menus: [moduleMenu('organizations')] })?.key).toBe('organizations');
  });
});

function NavigationConsumer({ context, pathname, layout }: { context: UserContext | null; pathname: string; layout: 'desktop' | 'mobile' }) {
  const { groups, openGroups, setGroupOpen } = useWorkbenchNavigation(context, pathname);
  return <nav aria-label="工作台导航" data-layout={layout}>{groups.map(group => <section key={group.id}>
    <button data-group={group.id} aria-expanded={openGroups[group.id]} onClick={() => setGroupOpen(group.id, !openGroups[group.id])}>{group.label}</button>
    {openGroups[group.id] && <ul>{group.items.map(item => <li key={item.to}><a href={item.to}>{item.label}</a></li>)}</ul>}
  </section>)}</nav>;
}

describe('mounted workbench navigation state', () => {
  let container: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    container = document.createElement('div');
    document.body.append(container);
    root = createRoot(container);
  });
  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  function render(context: UserContext | null, pathname: string, layout: 'desktop' | 'mobile' = 'desktop') {
    act(() => root.render(<NavigationConsumer context={context} pathname={pathname} layout={layout} />));
  }
  function button(id: WorkbenchGroupId) {
    const control = container.querySelector<HTMLButtonElement>(`button[data-group="${id}"]`);
    if (!control) throw new Error(`Missing visible group ${id}`);
    return control;
  }
  function expectExpanded(id: WorkbenchGroupId, expanded: boolean) {
    expect(button(id).getAttribute('aria-expanded')).toBe(String(expanded));
  }

  it('opens all visible groups on mount and keeps user-collapsed groups closed across routes and desktop/mobile rerenders', () => {
    render(administrator, '/users/7');
    expectExpanded('identity-organization', true);
    expectExpanded('authorization-review', true);
    expectExpanded('audit', true);
    expect(container.querySelector('a[href="/users"]')).not.toBeNull();

    act(() => button('identity-organization').click());
    expectExpanded('identity-organization', false);
    expect(container.querySelector('a[href="/users"]')).toBeNull();
    render(administrator, '/users/8');
    expectExpanded('identity-organization', false);
    render(administrator, '/authorization-overview');
    expectExpanded('authorization-review', true);
    expectExpanded('identity-organization', false);

    act(() => {
      button('authorization-review').click();
    });
    expectExpanded('authorization-review', false);
    render(administrator, '/profile', 'mobile');
    expectExpanded('authorization-review', false);
    expectExpanded('identity-organization', false);
    render(administrator, '/roles/12', 'desktop');
    expectExpanded('authorization-review', false);
    expectExpanded('identity-organization', false);
  });

  it('keeps configuration management open by default and preserves manual collapse on narrow layouts', () => {
    render(configurationAdministrator, '/apis');
    expectExpanded('configuration-management', true);
    expect(container.querySelector('a[href="/apis"]')).not.toBeNull();
    act(() => button('configuration-management').click());
    expectExpanded('configuration-management', false);
    render(configurationAdministrator, '/menus', 'mobile');
    expectExpanded('configuration-management', false);
    render(configurationAdministrator, '/apis', 'desktop');
    expectExpanded('configuration-management', false);
  });

  it('retains manual collapse choices when permissions temporarily disappear and return', () => {
    render(restrictedContext, '/profile');
    expect(container.querySelector('button[data-group]')).toBeNull();
    render(administrator, '/users/7');
    expectExpanded('authorization-review', true);
    expectExpanded('identity-organization', true);
    act(() => button('identity-organization').click());
    expectExpanded('identity-organization', false);

    render(restrictedContext, '/profile');
    expect(container.querySelector('button[data-group="identity-organization"]')).toBeNull();
    expect(container.querySelector('button[data-group="authorization-review"]')).toBeNull();
    render({ ...administrator, permissions: [...administrator.permissions] }, '/authorization-overview', 'mobile');
    expectExpanded('identity-organization', false);
    expectExpanded('authorization-review', true);
  });

  it('resets manual collapse choices only on a fresh mount', () => {
    render(administrator, '/unknown');
    expectExpanded('authorization-review', true);
    expectExpanded('identity-organization', true);
    act(() => button('identity-organization').click());
    expectExpanded('identity-organization', false);
    render(administrator, '/profile');
    expectExpanded('identity-organization', false);

    act(() => root.unmount());
    root = createRoot(container);
    render(administrator, '/profile');
    expectExpanded('authorization-review', true);
    expectExpanded('identity-organization', true);
  });
});
