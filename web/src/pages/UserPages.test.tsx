// @vitest-environment jsdom
import { act, useSyncExternalStore } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import type { AdminUser, DeletedAdminUser, Menu, Page, UserContext } from '@/lib/models';
import type { SessionSnapshot } from '@/lib/session-state';
import { managementRoutes } from './ManagementPages';
import { DeletedUsersList, UserCreate, UsersList } from './UserPages';

interface FixtureSession extends SessionSnapshot {
  logout(): Promise<void>;
  reloadContext(): Promise<UserContext>;
  can(permission: string): boolean;
}

const sessionFixture = vi.hoisted(() => ({ current: {} as FixtureSession, listeners: new Set<() => void>() }));

vi.mock('@/lib/session', () => ({
  useSession: () => useSyncExternalStore(
    (listener: () => void) => { sessionFixture.listeners.add(listener); return () => sessionFixture.listeners.delete(listener); },
    () => sessionFixture.current,
  ),
}));
vi.mock('@/lib/session-state', () => ({ sessionState: { getSnapshot: () => sessionFixture.current } }));

const usersMenu: Menu = {
  id: 2,
  parent_id: 1,
  name: '用户管理',
  path: '/system/users',
  component: 'system/users/index',
  icon: '',
  permission_code: 'admin.users.get',
  sort: 10,
  type: 2,
  status: 1,
};

const baseContext: UserContext = {
  user: { id: 1, username: 'operator', nickname: '操作员', avatar: '', email: 'op@example.com', role: 'operator', status: 1 },
  roles: ['operator'],
  permissions: [
    'admin.users.get',
    'admin.users.id.get',
    'admin.users.id.status.put',
    'admin.users.id.kick.put',
    'admin.users.id.delete',
  ],
  menus: [usersMenu],
};

function setSessionContext(context: UserContext | null) {
  sessionFixture.current = {
    phase: context ? 'authenticated' : 'anonymous',
    context,
    sessionId: context ? 'sess-1' : null,
    userRevision: 1,
    notice: null,
    logout: async () => {},
    reloadContext: async () => context!,
    can: (permission: string) => !!context && (context.roles.includes('admin') || context.permissions.includes(permission)),
  };
  sessionFixture.listeners.forEach(listener => listener());
}

const usersPage: Page<AdminUser> = {
  list: [
    {
      id: 1,
      username: 'operator',
      nickname: '操作员',
      avatar: '',
      email: 'op@example.com',
      status: 1,
      access_version: 1,
      roles: [{ id: 2, name: '运营', code: 'operator', status: 1 }],
      organizations: [{ id: 1, name: '总部' }],
      has_unmanaged_organizations: false,
    },
    {
      id: 2,
      username: 'rootadmin',
      nickname: '超管',
      avatar: '',
      email: 'admin@example.com',
      status: 1,
      access_version: 1,
      roles: [{ id: 1, name: '超级管理员', code: 'admin', status: 1 }],
      organizations: [{ id: 1, name: '总部' }],
      has_unmanaged_organizations: false,
    },
    {
      id: 3,
      username: 'alice',
      nickname: '爱丽丝',
      avatar: '',
      email: 'alice@example.com',
      status: 1,
      access_version: 4,
      roles: [{ id: 3, name: '成员', code: 'member', status: 1 }],
      organizations: [{ id: 1, name: '总部' }],
      has_unmanaged_organizations: false,
    },
  ],
  total: 3,
  page: 1,
  size: 20,
};

const sampleDeleted: DeletedAdminUser[] = [
  {
    id: 16,
    username: 'alice_del',
    nickname: '爱丽丝',
    avatar: '/api/avatars/default',
    email: 'alice@example.test',
    status: 1,
    deleted_at: '2026-11-25T10:53:06Z',
    protected: false,
    roles: [{ id: 2, code: 'user', name: '普通用户', status: 1 }],
    organizations: [{ id: 5, name: '研发部' }],
    access_version: 3,
  },
  {
    id: 2,
    username: 'superadmin',
    nickname: '超管',
    avatar: '/api/avatars/default',
    email: 'admin@example.test',
    status: 0,
    deleted_at: '2026-11-25T09:00:00Z',
    protected: true,
    roles: [{ id: 1, code: 'admin', name: '超级管理员', status: 1 }],
    organizations: [],
    access_version: 7,
  },
];

function envelope(data: unknown, status = 200, errorCode = '', msg = status === 200 ? 'success' : 'Request failed.') {
  return new Response(JSON.stringify({ code: status, data, error_code: errorCode, msg }), {
    status,
    headers: new Headers({ 'Content-Type': 'application/json' }),
  });
}

let container: HTMLDivElement;
let root: Root;
let compactMatches: boolean;
let mediaListeners: Set<() => void>;
let fetchMock: Mock<(path: string, init?: RequestInit) => Promise<Response>>;

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

async function mountUsers(initialEntry = '/users') {
  const router = createMemoryRouter([
    { path: '/users', element: <UsersList /> },
    { path: '/users/deleted', element: <DeletedUsersList /> },
    { path: '/users/new', element: <UserCreate /> },
    { path: '/users/:id', element: <div data-testid="user-detail">详情</div> },
  ], { initialEntries: [initialEntry] });
  await act(async () => root.render(<RouterProvider router={router} />));
  await flush();
  await flush();
  return router;
}

async function openMenu(label: string) {
  const trigger = container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  if (!trigger) throw new Error(`Missing trigger ${label}`);
  await act(async () => {
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  });
  await flush();
  return trigger;
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  setSessionContext({ ...baseContext, permissions: [...baseContext.permissions] });
  compactMatches = false;
  mediaListeners = new Set();
  vi.stubGlobal('matchMedia', vi.fn((query: string) => ({
    get matches() { return query.includes('1100px') ? compactMatches : false; },
    media: query,
    onchange: null,
    addEventListener: (_event: string, listener: () => void) => mediaListeners.add(listener),
    removeEventListener: (_event: string, listener: () => void) => mediaListeners.delete(listener),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })));
  fetchMock = vi.fn(async (path: string) => {
    const pathname = new URL(path, 'http://localhost').pathname;
    if (pathname === '/api/admin/users') return envelope(usersPage);
    if (pathname === '/api/admin/deleted-users') return envelope({ list: sampleDeleted, total: sampleDeleted.length, page: 1, size: 20 });
    throw new Error(`Unexpected request ${path}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = '';
  vi.unstubAllGlobals();
});

describe('UsersList action menu', () => {
  it('opens bottom end-aligned menu in desktop table, separates destructive delete, and closes on scroll', async () => {
    await mountUsers();

    expect(container.querySelector('.mg-table-desktop')).not.toBeNull();
    expect(container.querySelector('.mg-action-reserve')).toBeNull();
    expect(container.querySelector('button[aria-label="操作员的操作"]')).toBeNull();
    expect(container.querySelector('button[aria-label="超管的操作"]')).toBeNull();

    await openMenu('爱丽丝的操作');
    const menu = document.querySelector<HTMLElement>('.mg-action-menu[role="menu"]');
    expect(menu).not.toBeNull();
    expect(menu?.dataset.side).toBe('bottom');
    expect(menu?.dataset.align).toBe('end');

    const items = Array.from(menu!.querySelectorAll<HTMLElement>('[role="menuitem"]'));
    expect(items.map(item => item.textContent?.trim())).toEqual(['停用用户', '强制下线', '删除用户']);
    expect(menu!.querySelector('[role="separator"]')).not.toBeNull();
    expect(items[2].classList.contains('mg-danger')).toBe(true);

    // Scrolling dismisses the open menu without triggering any confirmation dialog.
    await act(async () => {
      window.dispatchEvent(new Event('scroll'));
    });
    await flush();
    expect(document.querySelector('.mg-action-menu[role="menu"]')).toBeNull();
    expect(document.querySelector('.mg-dialog')).toBeNull();
  });

  it('opens bottom start-aligned menu in compact cards and transitions into confirmation dialog with synced soft-delete copy', async () => {
    compactMatches = true;
    await mountUsers();

    expect(container.querySelector('.mg-user-cards')).not.toBeNull();
    expect(container.querySelector('.mg-card-actions')).not.toBeNull();

    await openMenu('爱丽丝的操作');
    const menu = document.querySelector<HTMLElement>('.mg-action-menu[role="menu"]');
    expect(menu).not.toBeNull();
    expect(menu?.dataset.side).toBe('bottom');
    expect(menu?.dataset.align).toBe('start');

    const deleteItem = Array.from(menu!.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
      item => item.textContent?.trim() === '删除用户',
    )!;
    await act(async () => { deleteItem.click(); });
    await flush();

    expect(document.querySelector('.mg-action-menu[role="menu"]')).toBeNull();
    const dialog = document.querySelector<HTMLElement>('.mg-dialog[role="alertdialog"]');
    expect(dialog).not.toBeNull();
    expect(dialog?.textContent).toContain('确认删除爱丽丝？');
    expect(dialog?.textContent).toContain('账号会移入已删除用户回收站');
    expect(dialog?.textContent).toContain('在恢复或彻底注销前仍保留用户名与邮箱占用');
  });

  it('omits separator when delete is the only permitted action and closes menu on viewport switch', async () => {
    setSessionContext({
      ...baseContext,
      permissions: ['admin.users.get', 'admin.users.id.delete'],
    });
    await mountUsers();

    await openMenu('爱丽丝的操作');
    const menu = document.querySelector<HTMLElement>('.mg-action-menu[role="menu"]')!;
    expect(Array.from(menu.querySelectorAll('[role="menuitem"]')).map(el => el.textContent?.trim())).toEqual(['删除用户']);
    expect(menu.querySelector('[role="separator"]')).toBeNull();

    await act(async () => {
      compactMatches = true;
      mediaListeners.forEach(listener => listener());
    });
    await flush();
    expect(document.querySelector('.mg-action-menu[role="menu"]')).toBeNull();
    expect(container.querySelector('.mg-user-cards')).not.toBeNull();
  });
});

describe('UserCreate and create button gating', () => {
  it('shows create user link only when admin.users.post permission is granted', async () => {
    await mountUsers();
    expect(container.querySelector('a[href="/users/new"]')).toBeNull();

    setSessionContext({ ...baseContext, permissions: [...baseContext.permissions, 'admin.users.post'] });
    await mountUsers();
    expect(container.querySelector('a[href="/users/new"]')?.textContent).toBe('新建用户');
  });

  it('validates password confirmation locally, submits pure identity payload, and redirects to detail', async () => {
    setSessionContext({ ...baseContext, permissions: [...baseContext.permissions, 'admin.users.post'] });
    const calls: { url: string; method: string; body: unknown }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method || 'GET';
      const body = init?.body ? JSON.parse(String(init.body)) : null;
      calls.push({ url, method, body });
      if (url === '/api/admin/users' && method === 'POST') {
        return envelope({
          id: 55, username: 'new_ops', nickname: '新成员', avatar: '/api/avatars/default',
          email: 'new.ops@example.com', status: 1, roles: [], organizations: [],
          has_unmanaged_organizations: false, access_version: 1,
        });
      }
      throw new Error(`Unexpected ${method} ${url}`);
    }));

    const router = await mountUsers('/users/new');

    const inputs = container.querySelectorAll<HTMLInputElement>('input');
    await act(async () => {
      setInputValue(inputs[0], 'new_ops');
      setInputValue(inputs[1], 'new.ops@example.com');
      setInputValue(inputs[2], '新成员');
      setInputValue(inputs[3], 'Password1!');
      setInputValue(inputs[4], 'Mismatch1!');
    });
    const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    await act(async () => { submit.click(); });
    await flush();
    expect(calls).toHaveLength(0);
    expect(container.textContent).toContain('两次输入的密码不一致');

    await act(async () => { setInputValue(inputs[4], 'Password1!'); });
    await act(async () => { submit.click(); });
    await flush();
    await flush();

    expect(calls).toEqual([{
      url: '/api/admin/users',
      method: 'POST',
      body: { username: 'new_ops', email: 'new.ops@example.com', password: 'Password1!', nickname: '新成员', status: 1 },
    }]);
    expect(router.state.location.pathname).toBe('/users/55');
  });
});

describe('DeletedUsersList and recycle bin flows', () => {
  beforeEach(() => {
    setSessionContext({
      ...baseContext,
      permissions: [
        'admin.users.get',
        'admin.users.id.get',
        'admin.deleted-users.get',
        'admin.users.id.restore.post',
        'admin.users.id.anonymize.post',
      ],
    });
  });

  it('enforces admin.deleted-users.get route guard on /users/deleted before /users/:id', async () => {
    await act(async () => {
      setSessionContext({ ...baseContext, permissions: ['admin.users.get'] });
      const deniedRouter = createMemoryRouter([{ path: '/', children: managementRoutes }], { initialEntries: ['/users/deleted'] });
      root.render(<RouterProvider router={deniedRouter} />);
    });
    await flush();
    expect(container.textContent).toContain('没有访问权限');
    expect(fetchMock).not.toHaveBeenCalled();

    await act(async () => {
      setSessionContext({ ...baseContext, permissions: ['admin.users.get', 'admin.deleted-users.get'] });
      const allowedRouter = createMemoryRouter([{ path: '/', children: managementRoutes }], { initialEntries: ['/users/deleted'] });
      root.render(<RouterProvider router={allowedRouter} />);
    });
    await flush();
    expect(container.textContent).toContain('已删除用户');
    expect(container.textContent).toContain('alice_del');
  });

  it('syncs URL keyword and pagination, renders compact cards on narrow viewports, and steps back when tail page empties', async () => {
    compactMatches = true;
    let page2Items: DeletedAdminUser[] = [sampleDeleted[0]];
    fetchMock.mockImplementation(async (path, init) => {
      const url = new URL(path, 'http://localhost');
      if (url.pathname === '/api/admin/deleted-users') {
        const page = Number(url.searchParams.get('page') || '1');
        const keyword = url.searchParams.get('keyword') || '';
        if (keyword === 'alice') {
          return envelope({ list: [sampleDeleted[0]], total: 1, page: 1, size: 10 });
        }
        if (page === 2) {
          return envelope({ list: page2Items, total: 10 + page2Items.length, page: 2, size: 10 });
        }
        return envelope({ list: [sampleDeleted[1]], total: 10 + page2Items.length, page: 1, size: 10 });
      }
      if (url.pathname === '/api/admin/users/16/restore' && init?.method === 'POST') {
        page2Items = [];
        return envelope({
          id: 16, username: 'alice_del', nickname: '爱丽丝', avatar: '/api/avatars/default',
          email: 'alice@example.test', status: 1, roles: [], organizations: [],
          has_unmanaged_organizations: false, access_version: 3,
        });
      }
      throw new Error(`Unexpected ${path}`);
    });

    const router = await mountUsers('/users/deleted?page=2&size=10');
    expect(container.querySelector('.mg-user-cards')).not.toBeNull();
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/deleted-users?page=2&size=10', expect.anything());

    // Restoring the last item on page 2 steps URL back to page 1.
    const restoreBtn = Array.from(container.querySelectorAll('button')).find(btn => btn.textContent === '恢复用户') as HTMLButtonElement;
    await act(async () => { restoreBtn.click(); });
    const confirmBtn = Array.from(document.body.querySelectorAll('button')).find(btn => btn.textContent === '确认恢复') as HTMLButtonElement;
    await act(async () => { confirmBtn.click(); });
    await flush();
    expect(router.state.location.search).toContain('page=1');

    // Submitting search keyword updates URL query and resets to page 1.
    const searchInput = container.querySelector<HTMLInputElement>('#deleted-users-search')!;
    await act(async () => { setInputValue(searchInput, 'alice'); });
    const searchBtn = Array.from(container.querySelectorAll('button')).find(btn => btn.textContent === '搜索') as HTMLButtonElement;
    await act(async () => { searchBtn.click(); });
    await flush();
    expect(router.state.location.search).toContain('keyword=alice');
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/deleted-users?page=1&size=10&keyword=alice', expect.anything());
  });

  it('shows protected badge without restore/anonymize actions, confirms restore with feedback link, and surfaces restore failures', async () => {
    let failOnce = true;
    let items = [...sampleDeleted];
    fetchMock.mockImplementation(async (path, init) => {
      if (path.startsWith('/api/admin/deleted-users')) {
        return envelope({ list: items, total: items.length, page: 1, size: 20 });
      }
      if (path === '/api/admin/users/16/restore' && init?.method === 'POST') {
        if (failOnce) {
          failOnce = false;
          return envelope(null, 409, 'IDENTITY_CONFLICT', 'conflict');
        }
        items = items.filter(item => item.id !== 16);
        const restored: AdminUser = {
          id: 16, username: 'alice_del', nickname: '爱丽丝', avatar: '/api/avatars/default',
          email: 'alice@example.test', status: 1, roles: sampleDeleted[0].roles,
          organizations: sampleDeleted[0].organizations, has_unmanaged_organizations: false, access_version: 3,
        };
        return envelope(restored);
      }
      return envelope(null, 404, 'HTTP_NOT_FOUND');
    });

    await mountUsers('/users/deleted');

    expect(container.textContent).toContain('alice_del');
    expect(container.textContent).toContain('受保护 · admin（系统托管）');
    const restoreButtons = Array.from(container.querySelectorAll('button')).filter(btn => btn.textContent === '恢复用户');
    expect(restoreButtons).toHaveLength(1);
    const anonymizeButtons = Array.from(container.querySelectorAll('button')).filter(btn => btn.textContent === '彻底注销');
    expect(anonymizeButtons).toHaveLength(1);

    await act(async () => { restoreButtons[0].click(); });
    expect(document.body.textContent).toContain('删除前签发的登录会话与邮箱验证链接仍保持永久失效');
    expect(document.body.textContent).toContain('软删除期间已被清理的角色或组织归属不会自动恢复');

    // First attempt returns 409: error shown inside dialog, no fake success.
    const confirmBtn = Array.from(document.body.querySelectorAll('button')).find(btn => btn.textContent === '确认恢复') as HTMLButtonElement;
    await act(async () => { confirmBtn.click(); });
    await flush();
    expect(document.body.textContent).toContain('资料与现有记录冲突，请核对后重试。');
    expect(container.textContent).not.toContain('已恢复用户');

    // Retry succeeds and shows link to /users/16.
    await act(async () => { confirmBtn.click(); });
    await flush();
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/16/restore', expect.objectContaining({ method: 'POST' }));
    expect(container.textContent).toContain('已恢复用户 爱丽丝（alice_del）');
    expect(container.querySelector('a[href="/users/16"]')).not.toBeNull();
  });

  it('requires exact username input to unlock anonymize and surfaces server errors without faking success', async () => {
    let failOnce = true;
    let items = [sampleDeleted[0]];
    fetchMock.mockImplementation(async (path, init) => {
      if (path.startsWith('/api/admin/deleted-users')) {
        return envelope({ list: items, total: items.length, page: 1, size: 20 });
      }
      if (path === '/api/admin/users/16/anonymize' && init?.method === 'POST') {
        if (failOnce) {
          failOnce = false;
          return envelope({ fields: [{ field: 'confirm_username', error_code: 'IDENTITY_CONFIRM_USERNAME_MISMATCH', message: 'mismatch' }] }, 422, 'IDENTITY_VALIDATION_INVALID');
        }
        items = [];
        return envelope(null);
      }
      return envelope(null, 404, 'HTTP_NOT_FOUND');
    });

    await mountUsers('/users/deleted');

    const anonymizeBtn = Array.from(container.querySelectorAll('button')).find(btn => btn.textContent === '彻底注销') as HTMLButtonElement;
    await act(async () => { anonymizeBtn.click(); });

    const submitBtn = Array.from(document.body.querySelectorAll('button')).find(btn => btn.textContent === '确认彻底注销') as HTMLButtonElement;
    expect(submitBtn.disabled).toBe(true);

    const input = document.body.querySelector('input[placeholder="alice_del"]') as HTMLInputElement;
    await act(async () => { setInputValue(input, 'wrong_name'); });
    expect(submitBtn.disabled).toBe(true);

    await act(async () => { setInputValue(input, 'alice_del'); });
    expect(submitBtn.disabled).toBe(false);

    // First attempt returns 422: dialog remains open with localized field error, no fake success.
    await act(async () => { submitBtn.click(); });
    await flush();
    expect(document.body.textContent).toContain('输入的确认用户名与目标用户不一致');
    expect(container.textContent).not.toContain('已彻底注销');

    // Second attempt succeeds.
    await act(async () => { submitBtn.click(); });
    await flush();
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/16/anonymize', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ confirm_username: 'alice_del' }),
    }));
    expect(container.textContent).toContain('已彻底注销（匿名化）alice_del');
  });

  it('shows explicit full-scope permission notice on 403 IDENTITY_PERMISSION_DENIED instead of empty state', async () => {
    fetchMock.mockImplementation(async () => envelope(null, 403, 'IDENTITY_PERMISSION_DENIED'));
    await mountUsers('/users/deleted');
    expect(container.textContent).toContain('需要全量数据范围权限');
    expect(container.textContent).not.toContain('没有可恢复的已删除用户');
  });

  it('conditionally renders recycle bin link in UsersList according to admin.deleted-users.get permission', async () => {
    fetchMock.mockImplementation(async () => envelope(usersPage));
    await mountUsers('/users');
    expect(container.querySelector('a[href="/users/deleted"]')).not.toBeNull();

    setSessionContext({ ...baseContext, permissions: ['admin.users.get'] });
    await mountUsers('/users');
    expect(container.querySelector('a[href="/users/deleted"]')).toBeNull();
  });
});
