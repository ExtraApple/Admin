// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AdminUser, Page, UserContext } from '@/lib/models';
import { UserCreate, UsersList } from './UserPages';

const session = vi.hoisted(() => ({
  context: null as UserContext | null,
}));

vi.mock('@/lib/session', () => ({
  useSession: () => ({
    context: session.context,
    can: (permission: string) => session.context?.permissions.includes(permission) ?? false,
  }),
}));

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
  menus: [],
};

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

function envelope(data: unknown, status = 200) {
  return new Response(JSON.stringify({ code: status, data, error_code: '', msg: 'success' }), {
    status,
    headers: new Headers({ 'Content-Type': 'application/json' }),
  });
}

let container: HTMLDivElement;
let root: Root;
let compactMatches: boolean;
let mediaListeners: Set<() => void>;

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

async function mount() {
  const router = createMemoryRouter([{ path: '/users', element: <UsersList /> }], { initialEntries: ['/users'] });
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
  session.context = { ...baseContext, permissions: [...baseContext.permissions] };
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
  vi.stubGlobal('fetch', vi.fn(async (path: string) => {
    const pathname = new URL(path, 'http://localhost').pathname;
    if (pathname === '/api/admin/users') return envelope(usersPage);
    throw new Error(`Unexpected request ${path}`);
  }));
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('UsersList action menu', () => {
  it('opens bottom end-aligned menu in desktop table, separates destructive delete, and closes on scroll', async () => {
    await mount();

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

  it('opens bottom start-aligned menu in compact cards and transitions into confirmation dialog on destructive action', async () => {
    compactMatches = true;
    await mount();

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
    expect(dialog?.textContent).toContain('此操作删除用户，不是停用，无法撤销。');
  });

  it('omits separator when delete is the only permitted action and closes menu on viewport switch', async () => {
    session.context = {
      ...baseContext,
      permissions: ['admin.users.get', 'admin.users.id.delete'],
    };
    await mount();

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

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

describe('UserCreate and create button gating', () => {
  it('shows create user link only when admin.users.post permission is granted', async () => {
    await mount();
    expect(container.querySelector('a[href="/users/new"]')).toBeNull();

    session.context = { ...baseContext, permissions: [...baseContext.permissions, 'admin.users.post'] };
    await mount();
    expect(container.querySelector('a[href="/users/new"]')?.textContent).toBe('新建用户');
  });

  it('validates password confirmation locally, submits pure identity payload, and redirects to detail', async () => {
    session.context = { ...baseContext, permissions: [...baseContext.permissions, 'admin.users.post'] };
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

    const router = createMemoryRouter([
      { path: '/users/new', element: <UserCreate /> },
      { path: '/users/:id', element: <div>user detail</div> },
    ], { initialEntries: ['/users/new'] });
    await act(async () => root.render(<RouterProvider router={router} />));
    await flush();

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
