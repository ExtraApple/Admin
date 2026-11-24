// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuditLog, UserContext } from '@/lib/models';
import { AuditLogsPage } from './AuditLogPages';

const session = vi.hoisted(() => ({ context: null as UserContext | null }));
vi.mock('@/lib/session', () => ({ useSession: () => ({ context: session.context }) }));

const allPermissions = [
  'admin.audit-logs.get',
  'admin.login-logs.get',
  'admin.operation-logs.get',
  'admin.permission-logs.get',
  'admin.data-access-logs.get',
];
const baseContext: UserContext = {
  user: { id: 1, username: 'operator', nickname: '', avatar: '', email: '', role: 'operator', status: 1 },
  roles: ['reader'],
  permissions: [],
  menus: [],
};
const log: AuditLog = {
  id: 9,
  user_id: 7,
  username: 'alice',
  method: 'GET',
  path: '/api/admin/users',
  query: 'page=2&size=10',
  body: 'x'.repeat(300),
  metadata: { purpose: 'managed_file' },
  status: 403,
  duration: 12,
  client_ip: '127.0.0.1',
  user_agent: 'test-agent',
  category: 'data_access',
  created_at: '2026-10-30 10:00:00',
};
const loginLog: AuditLog = { ...log, id: 10, category: 'login', status: 200, path: '/api/login', method: 'POST' };
const range = 'start=2026-10-29+10%3A00%3A00&end=2026-10-30+10%3A00%3A00';

interface Call { path: string; method: string; headers: Headers; body: unknown }

function envelope(data: unknown, status = 200, errorCode = '', msg = 'success') {
  return new Response(JSON.stringify({ code: status, data, error_code: errorCode, msg }), {
    status,
    headers: new Headers({ 'Content-Type': 'application/json' }),
  });
}

let container: HTMLDivElement;
let root: Root;
let calls: Call[];
let routes: Record<string, () => Response>;

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

async function mount(path: string) {
  const router = createMemoryRouter([{ path: '/audit-logs', element: <AuditLogsPage /> }], { initialEntries: [path] });
  await act(async () => root.render(<RouterProvider router={router} />));
  await flush();
  await flush();
  return router;
}

function button(text: string) {
  const found = [...container.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent?.trim() === text)
    ?? [...container.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent?.includes(text));
  if (!found) throw new Error(`Missing button ${text}: ${container.textContent}`);
  return found;
}

async function click(target: HTMLElement) {
  await act(async () => { target.click(); await Promise.resolve(); });
  await flush();
}

async function setInput(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

function lastURL(): URL {
  return new URL(calls[calls.length - 1].path, 'http://localhost');
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  session.context = { ...baseContext, permissions: allPermissions };
  calls = [];
  routes = {};
  container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
  vi.stubGlobal('fetch', vi.fn(async (path: string, init: RequestInit = {}) => {
    calls.push({
      path,
      method: init.method ?? 'GET',
      headers: new Headers(init.headers),
      body: typeof init.body === 'string' ? JSON.parse(init.body) : init.body,
    });
    const route = routes[new URL(path, 'http://localhost').pathname];
    if (!route) throw new Error(`Unexpected request ${path}`);
    return route();
  }));
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe('audit log page', () => {
  beforeEach(() => {
    routes['/api/admin/audit-logs'] = () => envelope({ list: [log], total: 21, page: 1, size: 20 });
    routes['/api/admin/login-logs'] = () => envelope({ list: [loginLog], total: 1, page: 1, size: 20 });
    routes['/api/admin/data-access-logs'] = () => envelope({ list: [log], total: 1, page: 1, size: 20 });
  });

  it('renders only permitted tabs and falls back from an unauthorized type', async () => {
    session.context = { ...baseContext, permissions: ['admin.login-logs.get'] };
    const router = await mount(`/audit-logs?type=all&page=1&size=20&${range}`);
    expect([...container.querySelectorAll('.mg-audit-tabs button')].map(item => item.textContent)).toEqual(['登录']);
    expect(router.state.location.search).toContain('type=login');
    expect(calls.some(call => call.path.startsWith('/api/admin/login-logs'))).toBe(true);
    expect(calls.some(call => call.path.startsWith('/api/admin/audit-logs'))).toBe(false);
  });

  it('applies filters and server pagination through the URL', async () => {
    const router = await mount(`/audit-logs?type=all&page=1&size=20&${range}`);
    await setInput(container.querySelector<HTMLInputElement>('input[placeholder="如 /api/admin"]')!, '/api/users');
    await click(button('查询'));
    expect(lastURL().searchParams.get('path')).toBe('/api/users');
    expect(lastURL().searchParams.get('page')).toBe('1');
    await click(button('下一页'));
    expect(lastURL().searchParams.get('page')).toBe('2');
    expect(router.state.location.search).toContain('page=2');
  });

  it('restores filters from the URL and requests the matching server parameters', async () => {
    await mount(`/audit-logs?type=all&page=1&size=20&${range}&method=GET&status=403&path=%2Fapi%2Fadmin&username=alice&user_id=7`);
    expect(container.querySelector<HTMLInputElement>('input[placeholder="如 /api/admin"]')!.value).toBe('/api/admin');
    expect(container.querySelector<HTMLInputElement>('input[placeholder="如 alice"]')!.value).toBe('alice');
    expect(lastURL().searchParams.get('username')).toBe('alice');
    expect(lastURL().searchParams.get('user_id')).toBe('7');
  });

  it('opens the detail sheet with semantic metadata, compact actions, and closes with a descriptive label', async () => {
    await mount(`/audit-logs?type=all&page=1&size=20&${range}`);
    await click(button('2026-10-30 10:00:00'));
    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('审计日志 #9');
    expect(dialog?.textContent).toContain('test-agent');
    const meta = dialog?.querySelector('dl.mg-audit-meta');
    expect(meta?.querySelectorAll('dt').length).toBe(4);
    expect(meta?.querySelectorAll('dd').length).toBe(4);
    expect(meta?.querySelector('li')).toBeNull();
    const bodyBlock = [...(dialog?.querySelectorAll('.mg-audit-block') ?? [])].find(block => block.querySelector('h3')?.textContent === 'Body');
    const pre = bodyBlock?.querySelector('.mg-audit-pre');
    expect(pre?.className).toContain('mg-audit-pre-collapsed');
    const actionButtons = [...(bodyBlock?.querySelectorAll<HTMLButtonElement>('.mg-audit-block-actions button') ?? [])];
    expect(actionButtons.map(item => item.textContent?.trim())).toEqual(['复制', '展开']);
    expect(actionButtons.every(item => item.className.includes('ui-button-sm'))).toBe(true);
    await click(actionButtons[1]);
    expect(pre?.className).not.toContain('mg-audit-pre-collapsed');
    expect(document.querySelector('button[aria-label="关闭审计日志详情"]')).not.toBeNull();
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    await flush();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it('formats JSON body, falls back on truncated JSON, copies displayed text with auto-reset, and shows non-expanding copy errors', async () => {
    const jsonBody = '{"username":"alice","role_ids":[1,2],"org_ids":[3,4],"status":1,"remark":"ok"}';
    const formatted = JSON.stringify(JSON.parse(jsonBody), null, 2);
    const truncatedBody = '{"username":"bob","note":"truncated...';
    const jsonLog: AuditLog = { ...log, id: 21, created_at: '2026-10-30 10:05:00', body: jsonBody };
    const truncatedLog: AuditLog = { ...log, id: 22, created_at: '2026-10-30 10:06:00', body: truncatedBody };
    routes['/api/admin/audit-logs'] = () => envelope({ list: [jsonLog, truncatedLog], total: 2, page: 1, size: 20 });

    const writeText = vi.fn<(text: string) => Promise<void>>()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('clipboard blocked'));
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });

    await mount(`/audit-logs?type=all&page=1&size=20&${range}`);
    await click(button('2026-10-30 10:05:00'));

    const dialog = document.querySelector('[role="dialog"]');
    const bodyBlock = [...(dialog?.querySelectorAll('.mg-audit-block') ?? [])].find(block => block.querySelector('h3')?.textContent === 'Body');
    const pre = bodyBlock?.querySelector('.mg-audit-pre');
    expect(pre?.textContent).toBe(formatted);
    expect(pre?.className).toContain('mg-audit-pre-collapsed');

    vi.useFakeTimers();
    try {
      const copyBtn = [...(bodyBlock?.querySelectorAll<HTMLButtonElement>('.mg-audit-block-actions button') ?? [])]
        .find(item => item.textContent?.trim() === '复制');
      if (!copyBtn) throw new Error('Missing copy button');
      await act(async () => { copyBtn.click(); await Promise.resolve(); });
      expect(writeText).toHaveBeenCalledWith(formatted);
      expect(copyBtn.textContent?.trim()).toBe('已复制');
      await act(async () => { vi.advanceTimersByTime(2000); });
      expect(copyBtn.textContent?.trim()).toBe('复制');

      await act(async () => { copyBtn.click(); await Promise.resolve(); });
      expect(copyBtn.textContent?.trim()).toBe('复制');
      expect(bodyBlock?.querySelector('[role="alert"]')?.textContent).toContain('复制失败');
    } finally {
      vi.useRealTimers();
    }

    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    await flush();
    await click(button('2026-10-30 10:06:00'));
    const secondDialog = document.querySelector('[role="dialog"]');
    const secondBody = [...(secondDialog?.querySelectorAll('.mg-audit-block') ?? [])].find(block => block.querySelector('h3')?.textContent === 'Body');
    expect(secondBody?.querySelector('.mg-audit-pre')?.textContent).toBe(truncatedBody);
  });

  it('resets expanded and copy state when switching logs or reopening the same log', async () => {
    const secondLog: AuditLog = { ...log, id: 11, created_at: '2026-10-30 09:30:00', body: 'y'.repeat(300) };
    routes['/api/admin/audit-logs'] = () => envelope({ list: [log, secondLog], total: 2, page: 1, size: 20 });
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } });

    await mount(`/audit-logs?type=all&page=1&size=20&${range}`);
    await click(button('2026-10-30 10:00:00'));
    const firstBody = () => [...(document.querySelector('[role="dialog"]')?.querySelectorAll('.mg-audit-block') ?? [])]
      .find(block => block.querySelector('h3')?.textContent === 'Body');
    const buttonsOf = (block: Element | undefined) => [...(block?.querySelectorAll<HTMLButtonElement>('.mg-audit-block-actions button') ?? [])];

    await click(buttonsOf(firstBody())[0]);
    await click(buttonsOf(firstBody())[1]);
    expect(buttonsOf(firstBody())[0].textContent?.trim()).toBe('已复制');
    expect(firstBody()?.querySelector('.mg-audit-pre')?.className).not.toContain('mg-audit-pre-collapsed');

    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    await flush();

    await click(button('2026-10-30 09:30:00'));
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('审计日志 #11');
    expect(buttonsOf(firstBody())[0].textContent?.trim()).toBe('复制');
    expect(firstBody()?.querySelector('.mg-audit-pre')?.className).toContain('mg-audit-pre-collapsed');

    await click(buttonsOf(firstBody())[1]);
    expect(firstBody()?.querySelector('.mg-audit-pre')?.className).not.toContain('mg-audit-pre-collapsed');
    await act(async () => { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    await flush();

    await click(button('2026-10-30 09:30:00'));
    expect(firstBody()?.querySelector('.mg-audit-pre')?.className).toContain('mg-audit-pre-collapsed');
  });

  it('shows an empty state and a permission state instead of false success', async () => {
    routes['/api/admin/audit-logs'] = () => envelope({ list: [], total: 0, page: 1, size: 20 });
    await mount(`/audit-logs?type=all&page=1&size=20&${range}`);
    expect(container.textContent).toContain('暂无日志');

    session.context = { ...baseContext, permissions: ['admin.data-access-logs.get'] };
    routes['/api/admin/data-access-logs'] = () => envelope(null, 403, 'AUTHZ_FORBIDDEN', 'forbidden');
    await mount(`/audit-logs?type=data_access&page=1&size=20&${range}`);
    expect(container.textContent).toContain('没有访问权限');
  });
});
