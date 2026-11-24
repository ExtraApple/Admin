// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Organization, UserContext } from '@/lib/models';
import { OrganizationCreate, OrganizationDetail } from './OrganizationPages';

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
    'admin.organizations.get',
    'admin.organizations.tree.get',
    'admin.organizations.post',
    'admin.organizations.id.put',
    'admin.organizations.id.delete',
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
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

const scopedTree: Organization[] = [
  {
    id: 1,
    parent_id: 0,
    name: '集团总部',
    code: 'hq',
    remark: '',
    sort: 0,
    status: 1,
    manageable: false,
    children: [
      {
        id: 2,
        parent_id: 1,
        name: '华东大区',
        code: 'east',
        remark: '',
        sort: 1,
        status: 1,
        manageable: true,
        children: [
          {
            id: 3,
            parent_id: 2,
            name: '上海分部',
            code: 'shanghai',
            remark: '',
            sort: 2,
            status: 1,
            manageable: true,
          },
        ],
      },
    ],
  },
];

describe('OrganizationPages create and delete', () => {
  it('preselects manageable parentId, disallows root when readonly ancestors exist, and navigates to created org', async () => {
    const requests: { url: string; method: string; body: unknown }[] = [];
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method || 'GET';
      const body = init?.body ? JSON.parse(String(init.body)) : null;
      requests.push({ url, method, body });
      if (url === '/api/admin/organizations/tree') return envelope(scopedTree);
      if (url === '/api/admin/organizations' && method === 'POST') {
        return envelope({ id: 9, parent_id: 2, name: '杭州分部', code: 'hangzhou', remark: '', sort: 0, status: 1 });
      }
      throw new Error(`Unexpected ${method} ${url}`);
    }));

    const router = createMemoryRouter([
      { path: '/organizations/new', element: <OrganizationCreate /> },
      { path: '/organizations/:id', element: <div>org detail</div> },
    ], { initialEntries: ['/organizations/new?parentId=2'] });
    await act(async () => root.render(<RouterProvider router={router} />));
    await flush();
    await flush();

    const parentSelect = container.querySelector<HTMLSelectElement>('select')!;
    expect(parentSelect.value).toBe('2');
    expect(parentSelect.options[0].textContent).toContain('请选择可管理的上级组织');

    const inputs = container.querySelectorAll<HTMLInputElement>('input');
    await act(async () => {
      setNativeValue(inputs[0], '杭州分部');
      setNativeValue(inputs[1], 'hangzhou');
    });
    const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    await act(async () => { submit.click(); });
    await flush();
    await flush();

    expect(requests.find(req => req.method === 'POST')).toEqual({
      url: '/api/admin/organizations',
      method: 'POST',
      body: { parent_id: 2, name: '杭州分部', code: 'hangzhou', remark: '', sort: 0, status: 1 },
    });
    expect(router.state.location.pathname).toBe('/organizations/9');
  });

  it('pre-blocks deletion on org with visible children and exposes create-child link', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      if (url === '/api/admin/organizations/tree') return envelope(scopedTree);
      throw new Error(`Unexpected ${url}`);
    }));

    const router = createMemoryRouter([
      { path: '/organizations/:id', element: <OrganizationDetail /> },
    ], { initialEntries: ['/organizations/2'] });
    await act(async () => root.render(<RouterProvider router={router} />));
    await flush();
    await flush();

    expect(container.textContent).toContain('当前组织下仍有 1 个可见下级组织，无法直接删除');
    expect(container.querySelector('a[href="/organizations/new?parentId=2"]')?.textContent).toBe('新建子组织');
    expect(Array.from(container.querySelectorAll('button')).some(btn => btn.textContent === '删除组织')).toBe(false);
  });

  it('deletes a manageable leaf org after confirmation and navigates back to list', async () => {
    let deletedId = 0;
    vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method || 'GET';
      if (url === '/api/admin/organizations/tree') return envelope(scopedTree);
      if (url === '/api/admin/organizations/3' && method === 'DELETE') {
        deletedId = 3;
        return envelope(null);
      }
      throw new Error(`Unexpected ${method} ${url}`);
    }));

    const router = createMemoryRouter([
      { path: '/organizations/:id', element: <OrganizationDetail /> },
      { path: '/organizations', element: <div>orgs list</div> },
    ], { initialEntries: ['/organizations/3'] });
    await act(async () => root.render(<RouterProvider router={router} />));
    await flush();
    await flush();

    const deleteBtn = Array.from(container.querySelectorAll('button')).find(btn => btn.textContent === '删除组织')!;
    await act(async () => { deleteBtn.click(); });
    await flush();
    const confirmBtn = Array.from(document.body.querySelectorAll('button')).find(btn => btn.textContent === '确认删除')!;
    await act(async () => { confirmBtn.click(); });
    await flush();
    await flush();

    expect(deletedId).toBe(3);
    expect(router.state.location.pathname).toBe('/organizations');
  });
});
