// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Page, Role, UserContext } from '@/lib/models';
import { RoleCreate, RoleDetail, RolesList } from './RolePages';

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
  user: { id: 1, username: 'admin', nickname: '超管', avatar: '', email: 'admin@example.com', role: 'user', status: 1 },
  roles: ['admin'],
  permissions: [
    'admin.roles.get',
    'admin.roles.id.get',
    'admin.roles.post',
    'admin.roles.id.delete',
    'admin.roles.id.users.get',
  ],
  menus: [],
};

function envelope(data: unknown, status = 200, errorCode = '', msg = 'success') {
  return new Response(JSON.stringify({ code: status, data, error_code: errorCode, msg }), {
    status,
    headers: new Headers({ 'Content-Type': 'application/json' }),
  });
}

function setNativeValue(element: HTMLInputElement | HTMLSelectElement, value: string) {
  const proto = element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
  setter?.call(element, value);
  element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }));
}

let container: HTMLDivElement;
let root: Root;

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  session.context = { ...baseContext, permissions: [...baseContext.permissions] };
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches: false,
    media: '',
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })));
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('RolePages create and delete', () => {
  it('creates a role with safe defaults (status=1, data_scope=self) and redirects to detail', async () => {
    const requests: { url: string; method: string; body: unknown }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method || 'GET';
      const body = init?.body ? JSON.parse(String(init.body)) : null;
      requests.push({ url, method, body });
      if (url === '/api/admin/roles' && method === 'POST') {
        return envelope({ id: 19, name: '运维', code: 'ops', description: '', sort: 0, status: 1, data_scope: 'self' });
      }
      if (url === '/api/admin/roles/19' && method === 'GET') {
        return envelope({ id: 19, name: '运维', code: 'ops', description: '', sort: 0, status: 1, data_scope: 'self' });
      }
      throw new Error(`Unexpected ${method} ${url}`);
    }));

    const router = createMemoryRouter([
      { path: '/roles/new', element: <RoleCreate /> },
      { path: '/roles/:id', element: <RoleDetail /> },
    ], { initialEntries: ['/roles/new'] });
    await act(async () => root.render(<RouterProvider router={router} />));
    await flush();

    const inputs = container.querySelectorAll<HTMLInputElement>('input');
    await act(async () => {
      setNativeValue(inputs[0], '运维');
      setNativeValue(inputs[1], 'ops');
    });
    const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    await act(async () => { submit.click(); });
    await flush();
    await flush();

    expect(requests[0]).toEqual({
      url: '/api/admin/roles',
      method: 'POST',
      body: { name: '运维', code: 'ops', description: '', sort: 0, status: 1, data_scope: 'self' },
    });
    expect(router.state.location.pathname).toBe('/roles/19');
  });

  it('protects admin role, loads affected user count on delete, and steps back page when last item deleted', async () => {
    const page2: Page<Role> = {
      list: [
        { id: 1, name: '超管', code: 'admin', description: '系统管理员', sort: 0, status: 1, data_scope: 'all' },
      ],
      total: 21,
      page: 2,
      size: 20,
    };
    const singleDeletablePage2: Page<Role> = {
      list: [
        { id: 8, name: '审阅员', code: 'reviewer', description: '', sort: 1, status: 1, data_scope: 'self' },
      ],
      total: 21,
      page: 2,
      size: 20,
    };
    let deleted = false;
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method || 'GET';
      if (url.startsWith('/api/admin/roles?page=2')) return envelope(singleDeletablePage2);
      if (url.startsWith('/api/admin/roles?page=1')) return envelope({ ...page2, page: 1, total: 20 });
      if (url === '/api/admin/roles/8/users') return envelope([{ id: 2 }, { id: 3 }]);
      if (url === '/api/admin/roles/8' && method === 'DELETE') {
        deleted = true;
        return envelope(null);
      }
      throw new Error(`Unexpected ${method} ${url}`);
    }));

    const router = createMemoryRouter([{ path: '/roles', element: <RolesList /> }], { initialEntries: ['/roles?page=2&size=20'] });
    await act(async () => root.render(<RouterProvider router={router} />));
    await flush();
    await flush();

    expect(container.querySelector('a[href="/roles/new"]')?.textContent).toBe('新建角色');
    const trigger = container.querySelector<HTMLButtonElement>('button[aria-label="审阅员的操作"]')!;
    await act(async () => {
      trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    });
    await flush();
    const deleteItem = Array.from(document.body.querySelectorAll('[role="menuitem"]')).find(node => node.textContent?.includes('删除角色')) as HTMLElement;
    await act(async () => { deleteItem.click(); });
    await flush();
    await flush();

    expect(document.body.textContent).toContain('当前绑定用户：2 人');
    const confirmBtn = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent === '确认删除')!;
    await act(async () => { confirmBtn.click(); });
    await flush();
    await flush();

    expect(deleted).toBe(true);
    expect(router.state.location.search).toContain('page=1');
  });

  it('shows warning instead of 0 when affected user lookup fails during role deletion', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (url === '/api/admin/roles/8') return envelope({ id: 8, name: '审阅员', code: 'reviewer', description: '', sort: 1, status: 1, data_scope: 'self' });
      if (url === '/api/admin/roles/8/users') return envelope(null, 500, 'AUTHZ_INTERNAL_ERROR', 'failed');
      throw new Error(`Unexpected ${url}`);
    }));

    const router = createMemoryRouter([
      { path: '/roles/:id', element: <RoleDetail /> },
      { path: '/roles', element: <div>roles</div> },
    ], { initialEntries: ['/roles/8'] });
    await act(async () => root.render(<RouterProvider router={router} />));
    await flush();
    await flush();

    const deleteBtn = Array.from(container.querySelectorAll('button')).find(btn => btn.textContent === '删除角色')!;
    await act(async () => { deleteBtn.click(); });
    await flush();
    await flush();

    expect(document.body.textContent).toContain('暂时无法核对当前绑定用户数');
    expect(document.body.textContent).not.toContain('当前绑定用户：0 人');
  });
});
