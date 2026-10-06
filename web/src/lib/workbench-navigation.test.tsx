// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Menu, UserContext } from './models';
import type { ManagementModule } from './navigation';
import { getInitialOpenGroups, getWorkbenchGroups, useWorkbenchNavigation, type WorkbenchGroupId } from './workbench-navigation';

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
const allClosed = { 'authorization-review': false, 'identity-organization': false, account: false };

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
      { id: 'account', routes: ['/profile'] },
    ]);
    expect(visibleEntries(restrictedContext)).toEqual([{ id: 'account', routes: ['/profile'] }]);
    expect(getWorkbenchGroups(null)).toEqual([]);
  });

  it.each([
    { description: 'missing read permission', permissions: [], menus: [moduleMenu('users')] },
    { description: 'missing real menu', permissions: ['admin.users.get'], menus: [] },
    { description: 'disabled menu', permissions: ['admin.users.get'], menus: [moduleMenu('users', { status: 0 })] },
    { description: 'button instead of page', permissions: ['admin.users.get'], menus: [moduleMenu('users', { type: 3 })] },
    { description: 'lookalike backend path', permissions: ['admin.users.get'], menus: [moduleMenu('users', { path: '/system/users-disabled' })] },
    { description: 'ungranted menu permission', permissions: ['admin.users.get'], menus: [moduleMenu('users', { permission_code: 'admin.users.private.get' })] },
  ])('hides the identity group for $description', ({ permissions, menus }) => {
    expect(getWorkbenchGroups({ ...restrictedContext, permissions, menus }).map(group => group.id)).toEqual(['account']);
  });

  it('keeps backend admin fallback without inventing menus, and honors existing nested-menu access', () => {
    expect(getWorkbenchGroups({ ...administrator, menus: [] }).map(group => group.id)).toEqual(['authorization-review', 'account']);
    const nested = { ...restrictedContext, permissions: ['admin.users.get'], menus: [moduleMenu('users', { path: '/system', type: 1, permission_code: '', children: [moduleMenu('users')] })] };
    expect(getWorkbenchGroups(nested).find(group => group.id === 'identity-organization')?.items.map(item => item.to)).toEqual(['/users']);
  });
});

describe('workbench path matching and initialization', () => {
  it.each([
    ['/authorization-overview', 'authorization-review'], ['/authorization-risks', 'authorization-review'],
    ['/roles', 'identity-organization'], ['/roles/12', 'identity-organization'], ['/roles/12/permissions', 'identity-organization'],
    ['/users', 'identity-organization'], ['/users/7/roles', 'identity-organization'],
    ['/organizations', 'identity-organization'], ['/organizations/4', 'identity-organization'],
    ['/profile', 'account'],
  ] as const)('opens only the visible group for %s', (pathname, groupId) => {
    expect(getInitialOpenGroups(getWorkbenchGroups(administrator), pathname)).toEqual({ ...allClosed, [groupId]: true });
  });

  it.each(['/', '/no-management', '/unknown', '/roles-extra', '/usersettings', '/organizations-other/1', '/authorization-overview/extra', '/authorization-risks/extra', '/profile/edit', '/profile/'])('leaves all groups closed for unmatched pathname %s', pathname => {
    expect(getInitialOpenGroups(getWorkbenchGroups(administrator), pathname)).toEqual(allClosed);
  });

  it('does not open a hidden group or a visible group whose matching item is denied', () => {
    expect(getInitialOpenGroups(getWorkbenchGroups(restrictedContext), '/users/7')).toEqual(allClosed);
    const usersOnly = { ...restrictedContext, permissions: ['admin.users.get'], menus: [moduleMenu('users')] };
    expect(getInitialOpenGroups(getWorkbenchGroups(usersOnly), '/roles/12')).toEqual(allClosed);
    expect(getInitialOpenGroups(getWorkbenchGroups(usersOnly), '/users/7')).toEqual({ ...allClosed, 'identity-organization': true });
    expect(getInitialOpenGroups(getWorkbenchGroups({ ...administrator, permissions: [] }), '/authorization-overview')).toEqual(allClosed);
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

  it('keeps the user-collapsed active group closed across routes and desktop/mobile rerenders', () => {
    render(administrator, '/users/7');
    expectExpanded('identity-organization', true);
    expectExpanded('authorization-review', false);
    expectExpanded('account', false);
    expect(container.querySelector('a[href="/users"]')).not.toBeNull();

    act(() => button('identity-organization').click());
    expectExpanded('identity-organization', false);
    expect(container.querySelector('a[href="/users"]')).toBeNull();
    render(administrator, '/users/8');
    expectExpanded('identity-organization', false);
    render(administrator, '/authorization-overview');
    expectExpanded('authorization-review', false);
    expectExpanded('identity-organization', false);

    act(() => {
      button('authorization-review').click();
      button('account').click();
    });
    expectExpanded('authorization-review', true);
    expectExpanded('account', true);
    render(administrator, '/profile', 'mobile');
    expectExpanded('authorization-review', true);
    expectExpanded('identity-organization', false);
    expectExpanded('account', true);
    render(administrator, '/roles/12', 'desktop');
    expectExpanded('authorization-review', true);
    expectExpanded('identity-organization', false);
    expectExpanded('account', true);
  });

  it('updates visible permissions without resetting state, retaining choices when groups disappear and return', () => {
    render(restrictedContext, '/profile');
    expectExpanded('account', true);
    render(administrator, '/users/7');
    expectExpanded('account', true);
    expectExpanded('authorization-review', false);
    expectExpanded('identity-organization', false);
    act(() => button('identity-organization').click());
    expectExpanded('identity-organization', true);

    render(restrictedContext, '/profile');
    expect(container.querySelector('button[data-group="identity-organization"]')).toBeNull();
    expect(container.querySelector('button[data-group="authorization-review"]')).toBeNull();
    expectExpanded('account', true);
    render({ ...administrator, permissions: [...administrator.permissions] }, '/authorization-overview', 'mobile');
    expectExpanded('identity-organization', true);
    expectExpanded('authorization-review', false);
    expectExpanded('account', true);
  });

  it('initializes unmatched routes closed and resets choices only on a new mount', () => {
    render(administrator, '/unknown');
    expectExpanded('authorization-review', false);
    expectExpanded('identity-organization', false);
    expectExpanded('account', false);
    render(administrator, '/profile');
    expectExpanded('account', false);
    act(() => button('identity-organization').click());
    expectExpanded('identity-organization', true);

    act(() => root.unmount());
    root = createRoot(container);
    render(administrator, '/profile');
    expectExpanded('authorization-review', false);
    expectExpanded('identity-organization', false);
    expectExpanded('account', true);
  });
});
