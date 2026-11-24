import { describe, expect, it } from 'vitest';
import type { Menu, UserContext } from './models';
import type { ManagementModule } from './navigation';
import {
  applyNavigationToTabs,
  canCloseTab,
  canPinTab,
  getBatchCloseState,
  planBatchCloseTabs,
  planCloseTab,
  pruneUnauthorizedTabs,
  resolveWorkbenchTab,
  syncVisitedTab,
  togglePinTab,
  type WorkbenchTab,
} from './workbench-tabs';

function moduleMenu(module: ManagementModule): Menu {
  return {
    id: 1,
    parent_id: 0,
    name: module,
    path: `/system/${module}`,
    component: `system/${module}/index`,
    icon: '',
    permission_code: `admin.${module}.get`,
    sort: 10,
    type: 2,
    status: 1,
  };
}

const fullAdmin: UserContext = {
  user: { id: 1, username: 'admin', nickname: '管理员', avatar: '', email: '', role: 'admin', status: 1 },
  roles: ['admin'],
  permissions: ['admin.authorization.overview.get'],
  menus: [moduleMenu('roles'), moduleMenu('users'), moduleMenu('organizations'), moduleMenu('menus'), moduleMenu('apis')],
};

const plainUser: UserContext = {
  user: { id: 2, username: 'viewer', nickname: '访客', avatar: '', email: '', role: 'user', status: 1 },
  roles: ['user'],
  permissions: [],
  menus: [],
};

describe('workbench opened-page tab state machine', () => {
  it('deduplicates tabs by first-level module and remembers the latest subroute and query', () => {
    let tabs: WorkbenchTab[] = [];
    tabs = syncVisitedTab(tabs, resolveWorkbenchTab('/roles', '', fullAdmin));
    tabs = syncVisitedTab(tabs, resolveWorkbenchTab('/users', '?keyword=alice&page=2', fullAdmin));
    tabs = syncVisitedTab(tabs, resolveWorkbenchTab('/roles/3/permissions', '', fullAdmin));

    expect(tabs.map(tab => ({ key: tab.key, url: tab.url }))).toEqual([
      { key: 'roles', url: '/roles/3/permissions' },
      { key: 'users', url: '/users?keyword=alice&page=2' },
    ]);
  });

  it('includes /profile, isolates /no-management as sole fallback, and excludes 404 or redirecting root', () => {
    expect(resolveWorkbenchTab('/', '', fullAdmin)).toBeNull();
    expect(resolveWorkbenchTab('/not-found', '', fullAdmin)).toBeNull();
    expect(resolveWorkbenchTab('/roles/3/nonexistent', '', fullAdmin)).toBeNull();

    let tabs: WorkbenchTab[] = [];
    tabs = syncVisitedTab(tabs, resolveWorkbenchTab('/', '', plainUser));
    expect(tabs).toEqual([{ key: 'no-management', label: '访问说明', rootPath: '/no-management', url: '/', pinned: false }]);

    // Entering a real module replaces the fallback no-management tab.
    tabs = syncVisitedTab(tabs, resolveWorkbenchTab('/profile', '', plainUser));
    expect(tabs).toEqual([{ key: 'profile', label: '个人资料', rootPath: '/profile', url: '/profile', pinned: false }]);

    // Revisiting /no-management while real tabs exist does not mix it alongside them.
    tabs = syncVisitedTab(tabs, resolveWorkbenchTab('/no-management', '', plainUser));
    expect(tabs).toEqual([{ key: 'profile', label: '个人资料', rootPath: '/profile', url: '/profile', pinned: false }]);
  });

  it('prevents closing the last remaining tab and disables batch close', () => {
    const single: WorkbenchTab[] = [{ key: 'roles', label: '角色管理', rootPath: '/roles', url: '/roles', pinned: false }];
    expect(canCloseTab(single, 'roles')).toBe(false);
    expect(getBatchCloseState(single, 'roles')).toEqual({
      canCloseOthers: false,
      canCloseRight: false,
      otherKeys: [],
      rightKeys: [],
    });
    expect(planCloseTab(single, 'roles', 'roles')).toEqual({
      nextTabs: single,
      navigateTo: null,
      pendingRemoval: null,
    });
  });

  it('closes inactive tabs immediately and defers active tab removal with right-then-left neighbor selection', () => {
    const tabs: WorkbenchTab[] = [
      { key: 'authorization-overview', label: '授权总览', rootPath: '/authorization-overview', url: '/authorization-overview', pinned: false },
      { key: 'roles', label: '角色管理', rootPath: '/roles', url: '/roles/3/permissions', pinned: false },
      { key: 'users', label: '用户管理', rootPath: '/users', url: '/users?page=2', pinned: false },
    ];

    // Closing inactive tab updates immediately without navigation.
    const closeInactive = planCloseTab(tabs, 'roles', 'users');
    expect(closeInactive.navigateTo).toBeNull();
    expect(closeInactive.pendingRemoval).toBeNull();
    expect(closeInactive.nextTabs.map(t => t.key)).toEqual(['authorization-overview', 'roles']);

    // Closing active middle tab navigates to right neighbor and defers removal until commit.
    const closeActiveMiddle = planCloseTab(tabs, 'roles', 'roles');
    expect(closeActiveMiddle.nextTabs).toBe(tabs);
    expect(closeActiveMiddle.navigateTo?.key).toBe('users');
    expect(closeActiveMiddle.pendingRemoval).toEqual({ removeKeys: ['roles'], expectedTargetKey: 'users' });

    // Committing navigation to the expected target applies the deferred removal.
    const committed = applyNavigationToTabs(
      tabs,
      resolveWorkbenchTab('/users', '?page=2', fullAdmin),
      closeActiveMiddle.pendingRemoval,
      fullAdmin,
    );
    expect(committed.pendingRemoval).toBeNull();
    expect(committed.tabs.map(t => t.key)).toEqual(['authorization-overview', 'users']);

    // Closing active rightmost tab falls back to left neighbor.
    const closeRightmost = planCloseTab(tabs, 'users', 'users');
    expect(closeRightmost.navigateTo?.key).toBe('roles');
    expect(closeRightmost.pendingRemoval).toEqual({ removeKeys: ['users'], expectedTargetKey: 'roles' });
  });

  it('plans batch close operations relative to the anchor tab and protects active dirty routes', () => {
    const tabs: WorkbenchTab[] = [
      { key: 'authorization-overview', label: '授权总览', rootPath: '/authorization-overview', url: '/authorization-overview', pinned: false },
      { key: 'roles', label: '角色管理', rootPath: '/roles', url: '/roles', pinned: false },
      { key: 'users', label: '用户管理', rootPath: '/users', url: '/users', pinned: false },
      { key: 'audit-logs', label: '审计日志', rootPath: '/audit-logs', url: '/audit-logs', pinned: false },
    ];

    // Close right of active tab: active tab stays, immediate removal.
    const closeRightSafe = planBatchCloseTabs(tabs, 'roles', 'roles', 'right');
    expect(closeRightSafe.navigateTo).toBeNull();
    expect(closeRightSafe.nextTabs.map(t => t.key)).toEqual(['authorization-overview', 'roles']);

    // Close others anchored on an inactive tab: active tab would close, so navigate first and defer removal.
    const closeOthersDeferred = planBatchCloseTabs(tabs, 'users', 'roles', 'others');
    expect(closeOthersDeferred.nextTabs).toBe(tabs);
    expect(closeOthersDeferred.navigateTo?.key).toBe('roles');
    expect(closeOthersDeferred.pendingRemoval).toEqual({
      removeKeys: ['authorization-overview', 'users', 'audit-logs'],
      expectedTargetKey: 'roles',
    });

    // If navigation commits elsewhere instead of the expected target, deferred removal is discarded.
    const diverted = applyNavigationToTabs(
      tabs,
      resolveWorkbenchTab('/audit-logs', '', fullAdmin),
      closeOthersDeferred.pendingRemoval,
      fullAdmin,
    );
    expect(diverted.pendingRemoval).toBeNull();
    expect(diverted.tabs.map(t => t.key)).toEqual(['authorization-overview', 'roles', 'users', 'audit-logs']);

    // Committing to anchor applies the batch removal cleanly.
    const confirmed = applyNavigationToTabs(
      tabs,
      resolveWorkbenchTab('/roles', '', fullAdmin),
      closeOthersDeferred.pendingRemoval,
      fullAdmin,
    );
    expect(confirmed.tabs.map(t => t.key)).toEqual(['roles']);
  });

  it('prunes unauthorized tabs when permissions change in the current session', () => {
    const tabs: WorkbenchTab[] = [
      { key: 'roles', label: '角色管理', rootPath: '/roles', url: '/roles', pinned: true },
      { key: 'users', label: '用户管理', rootPath: '/users', url: '/users', pinned: false },
      { key: 'profile', label: '个人资料', rootPath: '/profile', url: '/profile', pinned: true },
    ];
    const downgraded: UserContext = {
      ...plainUser,
      permissions: ['admin.users.get'],
      menus: [moduleMenu('users')],
    };
    // Revoked pinned tabs are removed alongside unpinned ones; surviving pinned tabs keep their state.
    expect(pruneUnauthorizedTabs(tabs, downgraded)).toEqual([
      { key: 'users', label: '用户管理', rootPath: '/users', url: '/users', pinned: false },
      { key: 'profile', label: '个人资料', rootPath: '/profile', url: '/profile', pinned: true },
    ]);
  });

  it('pins tabs into a leftmost zone in pin order, preserves pinned on subroute updates, and protects pinned tabs from single and batch close', () => {
    let tabs: WorkbenchTab[] = [
      { key: 'authorization-overview', label: '授权总览', rootPath: '/authorization-overview', url: '/authorization-overview', pinned: false },
      { key: 'roles', label: '角色管理', rootPath: '/roles', url: '/roles', pinned: false },
      { key: 'users', label: '用户管理', rootPath: '/users', url: '/users', pinned: false },
      { key: 'audit-logs', label: '审计日志', rootPath: '/audit-logs', url: '/audit-logs', pinned: false },
    ];

    // Fallback no-management tab cannot be pinned.
    const fallbackTab = resolveWorkbenchTab('/no-management', '', plainUser);
    expect(canPinTab(fallbackTab)).toBe(false);
    expect(togglePinTab(fallbackTab ? [fallbackTab] : [], 'no-management')).toEqual(fallbackTab ? [fallbackTab] : []);

    // Pinning moves tabs into the leftmost pinned zone in pin order.
    tabs = togglePinTab(tabs, 'users');
    expect(tabs.map(t => [t.key, t.pinned])).toEqual([
      ['users', true],
      ['authorization-overview', false],
      ['roles', false],
      ['audit-logs', false],
    ]);

    tabs = togglePinTab(tabs, 'audit-logs');
    expect(tabs.map(t => [t.key, t.pinned])).toEqual([
      ['users', true],
      ['audit-logs', true],
      ['authorization-overview', false],
      ['roles', false],
    ]);

    // Updating URL on a pinned module keeps pinned status and position intact.
    tabs = syncVisitedTab(tabs, resolveWorkbenchTab('/users/9/roles', '', fullAdmin));
    expect(tabs[0]).toEqual({ key: 'users', label: '用户管理', rootPath: '/users', url: '/users/9/roles', pinned: true });

    // Pinned tabs cannot be closed individually.
    expect(canCloseTab(tabs, 'users')).toBe(false);
    expect(canCloseTab(tabs, 'roles')).toBe(true);
    expect(planCloseTab(tabs, 'users', 'users')).toEqual({ nextTabs: tabs, navigateTo: null, pendingRemoval: null });

    // Batch close right of first pinned tab skips the second pinned tab and targets only unpinned tabs.
    expect(getBatchCloseState(tabs, 'users')).toEqual({
      canCloseOthers: true,
      canCloseRight: true,
      otherKeys: ['authorization-overview', 'roles'],
      rightKeys: ['authorization-overview', 'roles'],
    });

    // Batch close others anchored on roles while active tab is pinned (users): active pinned tab is untouched, no navigation needed.
    const closeOthersKeepPinned = planBatchCloseTabs(tabs, 'users', 'roles', 'others');
    expect(closeOthersKeepPinned.navigateTo).toBeNull();
    expect(closeOthersKeepPinned.nextTabs.map(t => [t.key, t.pinned])).toEqual([
      ['users', true],
      ['audit-logs', true],
      ['roles', false],
    ]);

    // When only pinned tabs remain alongside the anchor, batch close is disabled.
    const onlyPinnedOthers = closeOthersKeepPinned.nextTabs;
    expect(getBatchCloseState(onlyPinnedOthers, 'roles')).toEqual({
      canCloseOthers: false,
      canCloseRight: false,
      otherKeys: [],
      rightKeys: [],
    });

    // Closing the last unpinned tab while pinned tabs exist navigates to the left pinned neighbor.
    const closeLastUnpinned = planCloseTab(onlyPinnedOthers, 'roles', 'roles');
    expect(closeLastUnpinned.navigateTo?.key).toBe('audit-logs');
    expect(closeLastUnpinned.pendingRemoval).toEqual({ removeKeys: ['roles'], expectedTargetKey: 'audit-logs' });

    // Unpinning moves a tab immediately after the remaining pinned zone.
    const afterUnpin = togglePinTab(tabs, 'users');
    expect(afterUnpin.map(t => [t.key, t.pinned])).toEqual([
      ['audit-logs', true],
      ['users', false],
      ['authorization-overview', false],
      ['roles', false],
    ]);
  });
});
