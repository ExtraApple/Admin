// @vitest-environment jsdom
import { act, useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Menu, UserContext } from '@/lib/models';
import type { ManagementModule } from '@/lib/navigation';
import { UnsavedGuard } from '@/components/management/shared';
import { UserAvatar, WorkbenchShell } from './WorkbenchShell';

const sessionMock = vi.hoisted(() => ({
  context: null as UserContext | null,
  phase: 'authenticated' as 'authenticated' | 'anonymous' | 'recovering' | 'reconnecting',
  sessionId: 1,
  notice: '',
  userRevision: 0,
}));

vi.mock('@/lib/session', () => ({
  useSession: () => ({
    ...sessionMock,
    can: () => true,
    refresh: vi.fn(async () => {}),
    reloadContext: vi.fn(async () => sessionMock.context!),
    logout: vi.fn(async () => {}),
    retryLogout: vi.fn(async () => {}),
    login: vi.fn(async () => {}),
    beginLogin: vi.fn(),
  }),
}));
// ES2022 is the suite's supported type baseline; Promise.withResolvers is unavailable.
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(finish => { resolve = finish; }); return { promise, resolve }; }
function image(value: string) { return new Response(value, { headers: { 'Content-Type': 'image/png' } }); }

let container: HTMLDivElement;
let root: Root;
let blobs: Blob[];
let revoked: string[];
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  blobs = []; revoked = [];
  class BrowserURL extends URL {
    static createObjectURL(blob: Blob) { blobs.push(blob); return `blob:avatar-${blobs.length}`; }
    static revokeObjectURL(url: string) { revoked.push(url); }
  }
  vi.stubGlobal('URL', BrowserURL);
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('current-user avatar display lifecycle', () => {
  it('replaces cached avatar contents on a new revision and releases replaced and unmounted URLs', async () => {
    let stored = 'original avatar';
    let cached = stored;
    vi.stubGlobal('fetch', vi.fn(async (_path: string, init: RequestInit) => {
      if (init.cache === 'reload') cached = stored;
      return image(cached);
    }));
    await act(async () => root.render(<UserAvatar id={7} name="操作者" />));
    expect(await blobs[0].text()).toBe('original avatar');
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-1');
    stored = 'normalized new avatar';
    await act(async () => root.render(<UserAvatar id={7} name="操作者" revision={1} />));
    expect(await blobs[1].text()).toBe('normalized new avatar');
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-2');
    expect(revoked).toContain('blob:avatar-1');
    act(() => root.render(null));
    expect(revoked).toContain('blob:avatar-2');
  });

  it('discards a late prior revision without creating a retained URL or replacing the new image', async () => {
    const old = deferred<Response>();
    const fetch = vi.fn().mockImplementationOnce(() => old.promise).mockResolvedValueOnce(image('new image'));
    vi.stubGlobal('fetch', fetch);
    await act(async () => root.render(<UserAvatar id={7} name="操作者" revision={1} />));
    await act(async () => root.render(<UserAvatar id={7} name="操作者" revision={2} />));
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-1');
    await act(async () => { old.resolve(image('obsolete image')); });
    expect(blobs).toHaveLength(1);
    expect(await blobs[0].text()).toBe('new image');
    expect(container.querySelector('img')?.getAttribute('src')).toBe('blob:avatar-1');
    expect((fetch.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true);
  });

  it('lets a consumer distinguish image read failure and retry display without issuing another write', async () => {
    function Display({ revision }: { revision: number }) {
      const [status, setStatus] = useState('loading');
      return <><UserAvatar id={7} name="操作者" revision={revision} onLoad={() => setStatus('ready')} onError={() => setStatus('error')} /><output>{status}</output></>;
    }
    const fetch = vi.fn().mockRejectedValueOnce(new TypeError('offline')).mockResolvedValueOnce(image('saved server avatar'));
    vi.stubGlobal('fetch', fetch);
    await act(async () => root.render(<Display revision={1} />));
    expect(container.querySelector('output')?.textContent).toBe('error');
    await act(async () => root.render(<Display revision={2} />));
    act(() => container.querySelector('img')!.dispatchEvent(new Event('load')));
    expect(container.querySelector('output')?.textContent).toBe('ready');
    expect(fetch.mock.calls).toHaveLength(2);
    expect(fetch.mock.calls.every(([, init]) => init.method === 'GET')).toBe(true);
  });
});

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

const adminContext: UserContext = {
  user: { id: 1, username: 'admin', nickname: '系统管理员', avatar: '', email: '', role: 'admin', status: 1 },
  roles: ['admin'],
  permissions: ['admin.authorization.overview.get'],
  menus: [moduleMenu('roles'), moduleMenu('users'), moduleMenu('organizations'), moduleMenu('menus'), moduleMenu('apis')],
};

function DirtyEditorPage() {
  const [dirty, setDirty] = useState(true);
  return (
    <div>
      <UnsavedGuard dirty={dirty} />
      <h1>编辑角色权限</h1>
      <button type="button" data-clean onClick={() => setDirty(false)}>标记已保存</button>
    </div>
  );
}

function buildRouter(initialEntries: string[]) {
  return createMemoryRouter([
    {
      path: '/',
      element: <WorkbenchShell />,
      children: [
        { path: 'authorization-overview', element: <h1>授权总览内容</h1> },
        { path: 'roles', element: <h1>角色列表内容</h1> },
        { path: 'roles/:id', element: <h1>角色详情内容</h1> },
        { path: 'roles/:id/permissions', element: <DirtyEditorPage /> },
        { path: 'users', element: <h1>用户列表内容</h1> },
        { path: 'audit-logs', element: <h1>审计日志内容</h1> },
        { path: 'profile', element: <h1>个人资料内容</h1> },
        { path: '*', element: <h1>页面不存在</h1> },
      ],
    },
  ], { initialEntries });
}

describe('WorkbenchShell navigation, breadcrumbs, and opened-page tab bar', () => {
  let shellContainer: HTMLDivElement;
  let shellRoot: Root;

  let mobileMatches = false;
  let mediaListeners: Array<() => void> = [];

  function setViewport(mobile: boolean) {
    mobileMatches = mobile;
    for (const listener of [...mediaListeners]) listener();
  }

  const themeSnapshot = { mode: 'light' as const, appearance: 'light' as const };

  beforeEach(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    vi.stubGlobal('fetch', vi.fn(async () => new Response('avatar', { headers: { 'Content-Type': 'image/png' } })));
    window.AdminWorkbenchTheme = {
      getSnapshot: () => themeSnapshot,
      subscribe: () => () => {},
      setMode: vi.fn(),
    };
    sessionMock.context = adminContext;
    sessionMock.phase = 'authenticated';
    sessionMock.sessionId = 1;
    mobileMatches = false;
    mediaListeners = [];
    vi.stubGlobal('matchMedia', (query: string) => ({
      get matches() { return mobileMatches && query.includes('850px'); },
      media: query,
      addEventListener(_event: string, listener: () => void) { mediaListeners.push(listener); },
      removeEventListener(_event: string, listener: () => void) { mediaListeners = mediaListeners.filter(fn => fn !== listener); },
    }));
    shellContainer = document.createElement('div');
    document.body.append(shellContainer);
    shellRoot = createRoot(shellContainer);
  });

  afterEach(() => {
    act(() => shellRoot.unmount());
    shellContainer.remove();
    delete window.AdminWorkbenchTheme;
  });

  function tabKeys() {
    return Array.from(shellContainer.querySelectorAll<HTMLElement>('.workbench-tab')).map(el => el.dataset.tabKey);
  }

  it('renders hierarchical breadcrumbs, keeps tab URL vs sidebar root navigation distinct, and hides footer account block', async () => {
    const router = buildRouter(['/roles/3']);
    await act(async () => shellRoot.render(<RouterProvider router={router} />));

    // No duplicated sidebar footer account block; desktop sidebar is expanded with all visible groups open.
    expect(shellContainer.querySelector('[data-sidebar="footer"]')).toBeNull();
    expect(shellContainer.querySelector('[data-sidebar="sidebar"]')).not.toBeNull();

    // Static hierarchical breadcrumb shows clickable parent link + #ID.
    const crumbs = Array.from(shellContainer.querySelectorAll('.ui-breadcrumb-list li:not([aria-hidden="true"])')).map(el => el.textContent);
    expect(crumbs).toEqual(['工作台', '角色管理', '角色详情 #3']);

    // Single tab cannot be closed (close button hidden).
    expect(tabKeys()).toEqual(['roles']);
    expect(shellContainer.querySelector('.workbench-tab-close')).toBeNull();

    // Navigate to users with query params -> second tab opens; both tabs now have close buttons.
    await act(async () => router.navigate('/users?keyword=alice&page=2'));
    expect(tabKeys()).toEqual(['roles', 'users']);
    expect(shellContainer.querySelectorAll('.workbench-tab-close')).toHaveLength(2);

    // Clicking the roles tab resumes the last visited subroute (/roles/3).
    const rolesTabLink = shellContainer.querySelector<HTMLAnchorElement>('[data-tab-key="roles"] .workbench-tab-link')!;
    await act(async () => { rolesTabLink.click(); });
    expect(router.state.location.pathname).toBe('/roles/3');

    // Clicking the roles sidebar link navigates back to the module root (/roles).
    const rolesSidebarLink = shellContainer.querySelector<HTMLAnchorElement>('[data-sidebar="sidebar"] a[href="/roles"]')!;
    await act(async () => { rolesSidebarLink.click(); });
    expect(router.state.location.pathname).toBe('/roles');

    // Clicking breadcrumb parent link from /roles/3 navigates to /roles.
    await act(async () => router.navigate('/roles/3'));
    const rolesBreadcrumbLink = shellContainer.querySelector<HTMLAnchorElement>('.ui-breadcrumb-list a[href="/roles"]')!;
    await act(async () => { rolesBreadcrumbLink.click(); });
    expect(router.state.location.pathname).toBe('/roles');

    // Middle-click closes inactive tab without navigating away from /roles.
    const usersTab = shellContainer.querySelector<HTMLElement>('[data-tab-key="users"]')!;
    await act(async () => {
      usersTab.dispatchEvent(new MouseEvent('auxclick', { bubbles: true, cancelable: true, button: 1 }));
    });
    expect(tabKeys()).toEqual(['roles']);
    expect(router.state.location.pathname).toBe('/roles');
  });

  it('protects dirty active tab closure with UnsavedGuard and commits removal only after confirmation', async () => {
    const router = buildRouter(['/roles/3/permissions']);
    await act(async () => shellRoot.render(<RouterProvider router={router} />));

    // Mark clean briefly to open a second tab cleanly, then return to dirty editor.
    await act(async () => { shellContainer.querySelector<HTMLButtonElement>('[data-clean]')!.click(); });
    await act(async () => router.navigate('/users'));
    await act(async () => router.navigate('/roles/3/permissions'));
    expect(tabKeys()).toEqual(['roles', 'users']);
    expect(router.state.location.pathname).toBe('/roles/3/permissions');

    // Attempt to close active dirty roles tab -> UnsavedGuard blocks navigation; roles tab remains open.
    const closeRoles = shellContainer.querySelector<HTMLButtonElement>('[data-tab-key="roles"] .workbench-tab-close')!;
    await act(async () => { closeRoles.click(); });
    const dialog = document.querySelector('[role="alertdialog"]');
    expect(dialog).not.toBeNull();
    expect(tabKeys()).toEqual(['roles', 'users']);

    // Cancel departure -> stays on dirty page and keeps roles tab intact.
    const cancelBtn = Array.from(dialog!.querySelectorAll('button')).find(btn => btn.textContent?.includes('取消'))!;
    await act(async () => { cancelBtn.click(); });
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
    expect(router.state.location.pathname).toBe('/roles/3/permissions');
    expect(tabKeys()).toEqual(['roles', 'users']);

    // Retry closing roles tab and confirm departure -> navigates to neighbor /users and removes roles tab.
    await act(async () => { closeRoles.click(); });
    const confirmBtn = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="alertdialog"] button')).find(btn => btn.textContent?.includes('放弃修改并离开'))!;
    await act(async () => { confirmBtn.click(); });
    expect(router.state.location.pathname).toBe('/users');
    expect(tabKeys()).toEqual(['users']);
  });

  it('supports right-click context menu batch close and hides tab bar on narrow viewports', async () => {
    const router = buildRouter(['/authorization-overview']);
    await act(async () => shellRoot.render(<RouterProvider router={router} />));
    await act(async () => router.navigate('/roles'));
    await act(async () => router.navigate('/users'));
    await act(async () => router.navigate('/audit-logs'));
    expect(tabKeys()).toEqual(['authorization-overview', 'roles', 'users', 'audit-logs']);

    // Right-click roles tab and choose "关闭右侧页签" (active tab audit-logs is to the right -> navigates to roles and removes users & audit-logs).
    const rolesTab = shellContainer.querySelector<HTMLElement>('[data-tab-key="roles"]')!;
    await act(async () => {
      rolesTab.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    });
    const closeRightItem = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(el => el.textContent?.includes('关闭右侧页签'))!;
    await act(async () => { closeRightItem.click(); });
    expect(router.state.location.pathname).toBe('/roles');
    expect(tabKeys()).toEqual(['authorization-overview', 'roles']);

    // Resize to narrow viewport (<=850px) -> tab bar hides; resize back to desktop -> accumulated tabs restore.
    await act(async () => { setViewport(true); });
    expect(shellContainer.querySelector('.workbench-tabbar')).toBeNull();
    await act(async () => { setViewport(false); });
    expect(tabKeys()).toEqual(['authorization-overview', 'roles']);
  });

  it('renders an icon-only tab actions dropdown with pin/unpin support, pins tabs left, and protects pinned tabs from closing', async () => {
    const router = buildRouter(['/authorization-overview']);
    await act(async () => shellRoot.render(<RouterProvider router={router} />));
    await act(async () => router.navigate('/roles'));
    await act(async () => router.navigate('/users'));
    await act(async () => router.navigate('/audit-logs'));
    expect(tabKeys()).toEqual(['authorization-overview', 'roles', 'users', 'audit-logs']);

    // Rightmost dropdown trigger is icon-only (no visible "页签操作" label) while keeping accessible name and title.
    const actionsTrigger = shellContainer.querySelector<HTMLButtonElement>('.workbench-tabbar-actions')!;
    expect(actionsTrigger.textContent?.trim()).toBe('');
    expect(actionsTrigger.getAttribute('aria-label')).toBe('页签操作');
    expect(actionsTrigger.getAttribute('title')).toBe('页签操作');

    // Open rightmost dropdown and pin active tab (audit-logs) -> moves to leftmost pinned zone, shows pin icon, hides close button.
    await act(async () => {
      actionsTrigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    });
    const pinCurrentItem = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(el => el.textContent?.trim() === '固定当前页签')!;
    expect(pinCurrentItem).toBeDefined();
    expect(pinCurrentItem.getAttribute('aria-disabled')).toBeNull();
    await act(async () => { pinCurrentItem.click(); });

    expect(tabKeys()).toEqual(['audit-logs', 'authorization-overview', 'roles', 'users']);
    const pinnedAuditTab = shellContainer.querySelector<HTMLElement>('[data-tab-key="audit-logs"]')!;
    expect(pinnedAuditTab.dataset.pinned).toBe('true');
    expect(pinnedAuditTab.querySelector('.workbench-tab-pin')).not.toBeNull();
    expect(pinnedAuditTab.querySelector('.workbench-tab-close')).toBeNull();

    // Middle-click on pinned tab is ignored.
    await act(async () => {
      pinnedAuditTab.dispatchEvent(new MouseEvent('auxclick', { bubbles: true, cancelable: true, button: 1 }));
    });
    expect(tabKeys()).toEqual(['audit-logs', 'authorization-overview', 'roles', 'users']);

    // Right-click inactive tab (roles) -> pin it after existing pinned tab, then run "关闭其他页签" anchored on users -> both pinned tabs survive.
    const rolesTab = shellContainer.querySelector<HTMLElement>('[data-tab-key="roles"]')!;
    await act(async () => {
      rolesTab.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    });
    const pinRolesItem = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(el => el.textContent?.trim() === '固定页签')!;
    await act(async () => { pinRolesItem.click(); });
    expect(tabKeys()).toEqual(['audit-logs', 'roles', 'authorization-overview', 'users']);

    const usersTab = shellContainer.querySelector<HTMLElement>('[data-tab-key="users"]')!;
    await act(async () => {
      usersTab.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    });
    const closeOthersItem = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(el => el.textContent?.trim() === '关闭其他页签')!;
    await act(async () => { closeOthersItem.click(); });
    expect(tabKeys()).toEqual(['audit-logs', 'roles', 'users']);
    // Active tab was pinned audit-logs, so batch close kept it active without navigating away.
    expect(router.state.location.pathname).toBe('/audit-logs');

    // Unpin active tab via rightmost dropdown -> moves right after remaining pinned tab (roles) and regains close button.
    await act(async () => {
      actionsTrigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    });
    const unpinCurrentItem = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(el => el.textContent?.trim() === '取消固定当前页签')!;
    await act(async () => { unpinCurrentItem.click(); });
    expect(tabKeys()).toEqual(['roles', 'audit-logs', 'users']);
    expect(shellContainer.querySelector('[data-tab-key="audit-logs"] .workbench-tab-close')).not.toBeNull();

    // Navigating to 404 leaves no active tab -> rightmost dropdown disables "固定当前页签".
    await act(async () => router.navigate('/nonexistent-404'));
    await act(async () => {
      actionsTrigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    });
    const disabledPinItem = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(el => el.textContent?.trim() === '固定当前页签')!;
    expect(disabledPinItem.getAttribute('aria-disabled')).toBe('true');
  });
});
