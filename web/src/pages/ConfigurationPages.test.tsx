// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApisPage, MenusPage } from './ConfigurationPages';

const fingerprint = 'b'.repeat(64);

interface Call { path: string; method: string; headers: Headers; body: unknown }

function envelope(data: unknown, status = 200, errorCode = '', msg = 'success') {
  const headers = new Headers({ 'Content-Type': 'application/json' });
  return new Response(JSON.stringify({ code: status, data, error_code: errorCode, msg }), { status, headers });
}

const menuTree = [
  { id: 1, parent_id: 0, name: '系统管理', path: '', component: '', icon: 'Settings', permission_code: '', sort: 10, type: 1, status: 1, children: [
    { id: 2, parent_id: 1, name: '用户按钮', path: '', component: '', icon: '', permission_code: 'admin.users.get', sort: 10, type: 3, status: 1 },
    { id: 3, parent_id: 1, name: '用户页面', path: '/system/users', component: 'system/users/index', icon: 'Users', permission_code: 'admin.users.get', sort: 20, type: 2, status: 1 },
  ] },
];
const apiRecord = { id: 7, name: 'List users', method: 'GET', path: '/api/admin/users', group: 'user', permission_code: 'admin.users.get', remark: '', sort: 0, status: 1, need_auth: 1, need_audit: 1, route_state: 'access_mismatch' };

function preview(summary: Record<string, unknown>) {
  return { operation: 'update', normalized_target: {}, preview_fingerprint: fingerprint, summary: { affected_role_count: 0, affected_user_count: 0, access_version_change: true, requires_confirmation: true, ...summary } };
}

let container: HTMLDivElement;
let root: Root;
let calls: Call[];
let routes: Record<string, (call: Call) => Response>;

function respond(path: string, call: Call): Response {
  const handler = routes[path];
  if (!handler) throw new Error(`Unexpected request ${call.method} ${call.path}`);
  return handler(call);
}

function installFetch() {
  vi.stubGlobal('fetch', vi.fn(async (path: string, init: RequestInit = {}) => {
    const call: Call = {
      path, method: init.method ?? 'GET',
      headers: new Headers(init.headers),
      body: typeof init.body === 'string' ? JSON.parse(init.body) : init.body,
    };
    calls.push(call);
    return respond(path.split('?')[0], call);
  }));
}

async function mount(path: string) {
  const router = createMemoryRouter([
    { path: '/menus', element: <MenusPage /> },
    { path: '/apis', element: <ApisPage /> },
  ], { initialEntries: [path] });
  await act(async () => root.render(<RouterProvider router={router} />));
  await act(async () => { await Promise.resolve(); });
  return router;
}

function button(text: string) {
  const found = [...document.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent?.trim() === text)
    ?? [...document.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent?.includes(text));
  if (!found) throw new Error(`Missing button ${text}: ${container.textContent}`);
  return found;
}

async function click(target: HTMLElement) {
  await act(async () => { target.click(); await Promise.resolve(); });
}

function dialog(): HTMLElement | null {
  return document.querySelector('[role="alertdialog"]');
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  calls = [];
  routes = {};
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
  installFetch();
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('menu management page', () => {
  beforeEach(() => {
    routes['/api/admin/menus'] = () => envelope(menuTree);
    routes['/api/admin/api-groups'] = () => envelope([]);
    routes['/api/admin/apis'] = () => envelope({ list: [apiRecord], total: 1, page: 1, size: 50 });
  });

  it('restores the selected menu from menu_id and applies type-specific fields', async () => {
    const router = await mount('/menus?menu_id=2');
    expect(button('用户按钮').getAttribute('aria-current')).toBe('page');
    expect(router.state.location.search).toBe('?menu_id=2');
    // A button menu has no path/component controls, while the form still shows the shared fields.
    expect(container.querySelector('#api-keyword')).toBeNull();
    expect([...container.querySelectorAll('label')].map(label => label.textContent)).not.toContain('前端路径');

    await click(button('用户页面'));
    expect(router.state.location.search).toBe('?menu_id=3');
    expect([...container.querySelectorAll('label')].map(label => label.textContent)).toEqual(expect.arrayContaining(['前端路径', '组件路径']));
    expect((container.querySelector('input[value="/system/users"]') as HTMLInputElement).value).toBe('/system/users');
  });

  it('previews before saving and submits the preview fingerprint', async () => {
    routes['/api/admin/menus/impact-preview'] = () => envelope(preview({ affected_menu_ids: [2], affected_role_count: 1, affected_user_count: 3 }));
    routes['/api/admin/menus/2'] = () => envelope({ ...menuTree[0].children[0], name: '改名后的按钮' });
    await mount('/menus?menu_id=2');

    const nameInput = [...container.querySelectorAll<HTMLInputElement>('input')].find(input => input.value === '用户按钮')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(nameInput, '改名后的按钮');
      nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await click(button('保存菜单'));
    expect(calls.filter(call => call.method === 'PUT')).toHaveLength(0);
    expect(dialog()?.textContent).toContain('服务器已生成只读影响预览');
    expect(calls.find(call => call.path === '/api/admin/menus/impact-preview')?.body).toMatchObject({
      operation: 'update',
      candidate: { menu_id: 2, name: '改名后的按钮' },
    });

    await click(button('确认提交'));
    const update = calls.find(call => call.method === 'PUT');
    expect(update?.path).toBe('/api/admin/menus/2');
    expect(update?.body).toMatchObject({ name: '改名后的按钮', expected_fingerprint: fingerprint });
    expect(calls.some(call => call.method === 'GET' && call.path === '/api/admin/menus')).toBe(true);
  });

  it('blocks navigation away from an unsaved draft', async () => {
    await mount('/menus?menu_id=2');
    const nameInput = [...container.querySelectorAll<HTMLInputElement>('input')].find(input => input.value === '用户按钮')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(nameInput, '草稿');
      nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await click(button('用户页面'));
    expect(dialog()?.textContent).toContain('放弃未保存的修改');
    await click(button('取消'));
    expect(dialog()).toBeNull();
  });

  it('binds and clears APIs without ever sending an empty id list', async () => {
    routes['/api/admin/menus/2/apis'] = call => call.method === 'POST'
      ? envelope(null)
      : envelope([{ ...apiRecord, id: 7, name: 'List users' }]);
    routes['/api/admin/menus/impact-preview'] = () => envelope(preview({ affected_menu_ids: [2], affected_api_ids: [7] }));
    routes['/api/admin/apis/7'] = () => envelope(apiRecord);

    await mount('/menus?menu_id=2');
    expect(container.textContent).toContain('已绑定接口（1）');

    await click(button('预览清空绑定'));
    await click(button('确认提交'));
    const clear = calls.filter(call => call.method === 'POST' && call.path === '/api/admin/menus/2/apis').at(-1);
    expect(clear?.body).toEqual({ clear: true, expected_fingerprint: fingerprint });
    const emptyIdListSent = calls.some(call => {
      const body = call.body;
      return !!body && typeof body === 'object' && 'api_ids' in body && Array.isArray(body.api_ids) && body.api_ids.length === 0;
    });
    expect(emptyIdListSent).toBe(false);
  });

  it('forbids committing a route sync preview that reports conflicts', async () => {
    routes['/api/admin/menus/impact-preview'] = () => envelope(preview({ created_count: 0, skipped_count: 5, conflict_count: 1 }));
    routes['/api/admin/menus/sync'] = () => envelope({ created: 0 });

    await mount('/menus');
    await click(button('预览同步'));
    const action = [...document.querySelectorAll<HTMLButtonElement>('[role="alertdialog"] button')].find(item => item.textContent === '确认提交')!;
    expect(action.disabled).toBe(true);
    expect(dialog()?.textContent).toContain('禁止提交');
    expect(calls.some(call => call.path === '/api/admin/menus/sync')).toBe(false);
  });
});

describe('API management page', () => {
  beforeEach(() => {
    routes['/api/admin/menus'] = () => envelope(menuTree);
    routes['/api/admin/api-groups'] = () => envelope([{ group: 'user', count: 3 }]);
    routes['/api/admin/apis'] = () => envelope({ list: [apiRecord], total: 1, page: 2, size: 10 });
  });

  it('sends the URL filters and pagination to the server', async () => {
    await mount('/apis?page=2&size=10&keyword=users&group=user&method=GET&status=1&need_auth=1&need_audit=0&route_state=access_mismatch');
    const list = calls.find(call => call.path.startsWith('/api/admin/apis?'))!;
    expect(list.path).toBe('/api/admin/apis?page=2&size=10&keyword=users&group=user&method=GET&status=1&need_auth=1&need_audit=0&route_state=access_mismatch');
    expect(container.textContent).toContain('授权漂移');
    expect(container.textContent).toContain('ID 7');
  });

  it('keeps route facts read-only while editing only metadata', async () => {
    routes['/api/admin/apis/7'] = () => envelope(apiRecord);
    routes['/api/admin/apis/impact-preview'] = () => envelope(preview({ affected_api_ids: [7] }));
    routes['/api/admin/apis/7-update'] = () => envelope(apiRecord);
    routes['/api/admin/apis/7'] = call => call.method === 'PUT' ? envelope({ ...apiRecord, name: 'Renamed API' }) : envelope(apiRecord);

    await mount('/apis');
    await click(button('List users'));
    expect(container.textContent).toContain('GET /api/admin/users');
    expect(container.textContent).toContain('只读');

    const values = [...container.querySelectorAll<HTMLInputElement>('input')].map(input => input.value);
    expect(values).not.toContain('GET');
    expect(values).not.toContain('/api/admin/users');

    const nameInput = [...container.querySelectorAll<HTMLInputElement>('input')].find(input => input.value === 'List users')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(nameInput, 'Renamed API');
      nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await click(button('预览并保存'));
    await click(button('确认提交'));
    const update = calls.find(call => call.method === 'PUT');
    expect(update?.body).toMatchObject({ name: 'Renamed API', expected_fingerprint: fingerprint });
    expect(update?.body).not.toHaveProperty('method');
    expect(update?.body).not.toHaveProperty('path');
    expect(update?.body).not.toHaveProperty('need_auth');
  });

  it('deletes through the fingerprint header after preview', async () => {
    routes['/api/admin/apis/7'] = call => call.method === 'DELETE' ? envelope(null) : envelope(apiRecord);
    routes['/api/admin/apis/impact-preview'] = () => envelope(preview({ affected_api_ids: [7] }));

    await mount('/apis');
    await click(button('List users'));
    await click(button('预览删除'));
    await click(button('确认提交'));
    const remove = calls.find(call => call.method === 'DELETE');
    expect(remove?.path).toBe('/api/admin/apis/7');
    expect(remove?.headers.get('X-Expected-Fingerprint')).toBe(fingerprint);
  });

  it('previews route sync without sending route facts and reports the created count', async () => {
    routes['/api/admin/apis/impact-preview'] = () => envelope(preview({ created_count: 2, skipped_count: 4 }));
    routes['/api/admin/apis/sync'] = () => envelope({ created: [], count: 2 });

    await mount('/apis');
    await click(button('预览路由同步'));
    expect(calls.find(call => call.path === '/api/admin/apis/impact-preview')?.body).toEqual({ operation: 'sync', candidate: {} });
    await click(button('确认提交'));
    const sync = calls.find(call => call.path === '/api/admin/apis/sync');
    expect(sync?.body).toEqual({ expected_fingerprint: fingerprint });
    expect(container.textContent).toContain('服务器创建 2 条 API Metadata');
  });
});
