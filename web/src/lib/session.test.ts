// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserContext } from './models';

const user = { id: 7, username: 'operator', nickname: '操作者', avatar: '/api/avatars/default', email: 'operator@example.com', role: 'user', status: 1 };
const context: UserContext = { user, roles: ['reader'], permissions: ['admin.users.get'], menus: [] };
const input = { username: 'operator', password: 'TestOnly123!', captcha_id: 'real-id', captcha_code: '123456' };
const pendingKey = 'admin-workbench.auth.logout-pending';
const attemptKey = 'admin-workbench.auth.refresh-attempt';
const eventKey = 'admin-workbench.auth.event';
function envelope(status: number, data: unknown, errorCode = '') { return new Response(JSON.stringify({ code: status, error_code: errorCode, msg: errorCode ? 'request failed' : 'success', data }), { status }); }
function token(tag: string, seconds = 60) { return `${tag}.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + seconds }))}.signature`; }
// ES2022 is the suite's supported runtime/type baseline; it has no Promise.withResolvers.
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(finish => { resolve = finish; }); return { promise, resolve }; }

beforeEach(() => {
  vi.resetModules(); vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-25T08:00:00Z'));
  localStorage.clear();
  const locks = new Map<string, Promise<unknown>>();
  vi.stubGlobal('isSecureContext', true);
  vi.stubGlobal('window', new EventTarget());
  vi.stubGlobal('navigator', { cookieEnabled: true, locks: {
    request<T>(name: string, work: () => Promise<T> | T) {
      const next = (locks.get(name) ?? Promise.resolve()).catch(() => {}).then(work);
      locks.set(name, next.then(() => undefined, () => undefined));
      return next;
    },
  } });
});
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
async function setup() {
  let serial = 0;
  let sid = 'session-one';
  const fetch = vi.fn(async (path: string, _init?: RequestInit): Promise<Response> => {
    if (path === '/api/login') { sid = `session-login-${++serial}`; return envelope(200, { access_token: token(`login-${serial}`), session_id: sid, user }); }
    if (path === '/api/refresh') return envelope(200, { access_token: token(`renew-${++serial}`), session_id: sid });
    if (path === '/api/user/context') return envelope(200, context);
    return envelope(200, null);
  });
  vi.stubGlobal('fetch', fetch);
  // A fresh module models a page reload; a static import would retain the previous Access.
  const { sessionState } = await import('./session-state');
  const { api } = await import('./api');
  return { state: sessionState, api, fetch };
}

describe('Cookie-backed browser session lifecycle', () => {
  it('does not publish identity until both startup refresh and real context read succeed', async () => {
    const { state, fetch } = await setup();
    const response = deferred<Response>();
    fetch.mockImplementation(async path => path === '/api/refresh' ? envelope(200, { access_token: token('restored'), session_id: 'restored-session' }) : response.promise);
    const boot = state.initialize();
    await vi.waitFor(() => expect(fetch.mock.calls.some(([path]) => path === '/api/user/context')).toBe(true));
    expect(state.getSnapshot()).toMatchObject({ phase: 'bootstrapping', context: null });
    response.resolve(envelope(200, context)); await boot;
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', context, sessionId: 'restored-session' });
    expect(localStorage.getItem(attemptKey)).toBeNull();
  });

  it('keeps initial network failure unconfirmed and retries only on explicit action', async () => {
    const { state, fetch } = await setup();
    fetch.mockRejectedValueOnce(new Error('offline'));
    await state.initialize();
    expect(state.getSnapshot()).toMatchObject({ phase: 'recovery-required', context: null });
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch).toHaveBeenCalledTimes(1);
    // Only an explicit action starts a fresh attempt after the bounded recovery window.
    await state.refresh();
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', context });
  });

  it('uses the same attempt for an unknown result and sends no JSON Refresh credential', async () => {
    const { state, fetch } = await setup();
    fetch.mockRejectedValueOnce(new Error('response lost'));
    await state.initialize();
    const first = new Headers(fetch.mock.calls[0][1]?.headers).get('X-Refresh-Attempt-ID');
    await state.refresh();
    const calls = fetch.mock.calls.filter(([path]) => path === '/api/refresh');
    expect(new Headers(calls[1][1]?.headers).get('X-Refresh-Attempt-ID')).toBe(first);
    expect(calls.map(([, init]) => init?.body)).toEqual(['{}', '{}']);
    expect(state.getSnapshot().phase).toBe('authenticated');
    expect(JSON.stringify(Object.entries(localStorage))).not.toContain('signature');
  });

  it('preserves a valid page on temporary renewal failure, suspends at expiry, then restores the same session', async () => {
    const { state, api, fetch } = await setup();
    fetch.mockResolvedValueOnce(envelope(200, { access_token: token('short', 20), session_id: 'session-one', user }));
    await state.login(input);
    const sid = state.getSnapshot().sessionId;
    fetch.mockResolvedValueOnce(envelope(500, null, 'IDENTITY_INTERNAL_ERROR'));
    await vi.advanceTimersByTimeAsync(10_000);
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', context, sessionId: sid });
    await vi.advanceTimersByTimeAsync(10_000);
    expect(state.getSnapshot()).toMatchObject({ phase: 'recovery-required', context, sessionId: sid });
    const count = fetch.mock.calls.length;
    await expect(api.put('/api/admin/users/8/roles', { role_ids: [] })).rejects.toMatchObject({ errorCode: 'CLIENT_SESSION_RECOVERY_REQUIRED' });
    expect(fetch).toHaveBeenCalledTimes(count);
    await state.refresh();
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', context, sessionId: sid });
  });

  it('resumes the same context after an in-window temporary renewal failure without replaying a business operation', async () => {
    const { state, api, fetch } = await setup(); await state.login(input);
    const sid = state.getSnapshot().sessionId;
    fetch.mockResolvedValueOnce(envelope(500, null, 'IDENTITY_INTERNAL_ERROR'));
    await expect(state.refresh()).rejects.toMatchObject({ status: 500 });
    await state.refresh();
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', context, sessionId: sid });
    fetch.mockResolvedValueOnce(envelope(403, null, 'API_META_PERMISSION_DENIED'));
    await expect(api.put('/api/admin/users/8/roles', {})).rejects.toMatchObject({ status: 403 });
    expect(state.getSnapshot().phase).toBe('authenticated');
    expect(fetch.mock.calls.filter(([path]) => path === '/api/admin/users/8/roles')).toHaveLength(1);
  });

  it('does not let a late protected 401 clear a newer explicit login', async () => {
    const { state, api, fetch } = await setup(); await state.login(input);
    const old = deferred<Response>();
    fetch.mockImplementationOnce(() => old.promise);
    const request = api.get('/api/user/info');
    const rejected = expect(request).rejects.toMatchObject({ status: 401 });
    await state.login(input);
    const next = state.getSnapshot().sessionId;
    old.resolve(envelope(401, null, 'AUTHN_TOKEN_INVALID')); await rejected;
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', sessionId: next });
  });
  it('returns the freshly read context and refreshes display revision without renewing Access', async () => {
    const { state, api, fetch } = await setup(); await state.login(input);
    const revision = state.getSnapshot().userRevision;
    const updated = { ...context, user: { ...user, nickname: '已保存的昵称' } };
    fetch.mockResolvedValueOnce(envelope(200, updated));
    await expect(state.reloadContext()).resolves.toEqual(updated);
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', context: updated, userRevision: revision + 1 });
    await api.get('/api/user/info');
    expect(new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization')).toBe('Bearer ' + token('login-1'));
    expect(fetch.mock.calls.filter(([path]) => path === '/api/refresh')).toHaveLength(0);
  });

  it('rejects an older reload instead of replacing a newer confirmed display', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    const old = deferred<Response>();
    fetch.mockImplementationOnce(() => old.promise);
    const loading = state.reloadContext();
    const obsolete = expect(loading).rejects.toMatchObject({ errorCode: 'CLIENT_AUTH_OPERATION_SUPERSEDED' });
    const updated = { ...context, user: { ...user, nickname: '最新读取' } };
    fetch.mockResolvedValueOnce(envelope(200, updated));
    await state.reloadContext();
    const revision = state.getSnapshot().userRevision;
    old.resolve(envelope(200, context)); await obsolete;
    expect(state.getSnapshot()).toMatchObject({ context: updated, userRevision: revision });
  });

  it('rejects a reload from a previous login generation even for the same user', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    const old = deferred<Response>();
    fetch.mockImplementationOnce(() => old.promise);
    const loading = state.reloadContext();
    const obsolete = expect(loading).rejects.toMatchObject({ errorCode: 'CLIENT_AUTH_OPERATION_SUPERSEDED' });
    await state.login(input);
    const current = state.getSnapshot();
    old.resolve(envelope(200, { ...context, user: { ...user, nickname: '旧会话' } })); await obsolete;
    expect(state.getSnapshot()).toBe(current);
  });

  it('does not let a reload begun before renewal overwrite the newer renewal context', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    const old = deferred<Response>();
    fetch.mockImplementationOnce(() => old.promise);
    const loading = state.reloadContext();
    const obsolete = expect(loading).rejects.toMatchObject({ errorCode: 'CLIENT_AUTH_OPERATION_SUPERSEDED' });
    const updated = { ...context, user: { ...user, nickname: '续期后读取' } };
    fetch.mockResolvedValueOnce(envelope(200, { access_token: token('renewed'), session_id: 'session-login-1' }))
      .mockResolvedValueOnce(envelope(200, updated));
    await state.refresh();
    old.resolve(envelope(200, context)); await obsolete;
    expect(state.getSnapshot()).toMatchObject({ context: updated });
  });

  it('installs renewed Access without rolling back a later confirmed profile reload', async () => {
    const { state, api, fetch } = await setup(); await state.login(input);
    const old = deferred<Response>();
    fetch.mockResolvedValueOnce(envelope(200, { access_token: token('renewed'), session_id: 'session-login-1' }))
      .mockImplementationOnce(() => old.promise);
    const refreshing = state.refresh();
    await vi.waitFor(() => expect(fetch.mock.calls.filter(([path]) => path === '/api/user/context')).toHaveLength(2));
    const updated = { ...context, user: { ...user, nickname: '更新的资料读取' } };
    fetch.mockResolvedValueOnce(envelope(200, updated));
    await state.reloadContext();
    const revision = state.getSnapshot().userRevision;
    old.resolve(envelope(200, context)); await refreshing;
    expect(state.getSnapshot()).toMatchObject({ context: updated, userRevision: revision });
    await api.get('/api/user/info');
    expect(new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization')).toBe('Bearer ' + token('renewed'));
  });

  it('keeps confirmed display and revision when a read fails without an authentication rejection', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    const before = state.getSnapshot();
    fetch.mockResolvedValueOnce(envelope(500, null, 'IDENTITY_INTERNAL_ERROR'));
    await expect(state.reloadContext()).rejects.toMatchObject({ status: 500 });
    expect(state.getSnapshot()).toBe(before);
    expect(fetch.mock.calls.filter(([path]) => path === '/api/refresh')).toHaveLength(0);
  });

  it('clears a definitively invalid identity without replay or automatic renewal', async () => {
    const { state, api, fetch } = await setup(); await state.login(input);
    fetch.mockResolvedValueOnce(envelope(401, null, 'AUTHN_TOKEN_INVALID'));
    await expect(api.put('/api/admin/users/8/roles', {})).rejects.toMatchObject({ status: 401 });
    expect(state.getSnapshot()).toMatchObject({ phase: 'anonymous', context: null });
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch.mock.calls.filter(([path]) => path === '/api/admin/users/8/roles')).toHaveLength(1);
    expect(fetch.mock.calls.filter(([path]) => path === '/api/refresh')).toHaveLength(0);
  });

  it('keeps failed password-change logout durable across a fresh page, and retry does not require Access', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    fetch.mockResolvedValueOnce(envelope(500, null, 'IDENTITY_INTERNAL_ERROR'));
    await expect(state.logout('password-changed')).rejects.toMatchObject({ status: 500 });
    expect(state.getSnapshot()).toMatchObject({ phase: 'logout-pending', context: null });
    expect(JSON.parse(localStorage.getItem(pendingKey)!)).toMatchObject({ sessionId: 'session-login-1' });
    vi.resetModules();
    // Reload must lose module memory while retaining the browser's durable logout intent.
    const fresh = (await import('./session-state')).sessionState;
    const before = fetch.mock.calls.length;
    await fresh.initialize();
    expect(fresh.getSnapshot().phase).toBe('logout-pending');
    expect(fetch).toHaveBeenCalledTimes(before);
    await fresh.retryLogout();
    expect(new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization')).toBeNull();
    expect(fresh.getSnapshot()).toMatchObject({ phase: 'anonymous', context: null });
    expect(localStorage.getItem(pendingKey)).toBeNull();
  });

  it('clears a pending logout only after explicit login and confirmed context', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    fetch.mockResolvedValueOnce(envelope(500, null, 'IDENTITY_INTERNAL_ERROR'));
    await expect(state.logout()).rejects.toMatchObject({ status: 500 });
    const contextRead = deferred<Response>();
    fetch.mockResolvedValueOnce(envelope(200, { access_token: token('new-login'), session_id: 'new-login', user })).mockImplementationOnce(() => contextRead.promise);
    const loggingIn = state.login(input);
    await vi.waitFor(() => expect(fetch.mock.calls.at(-1)?.[0]).toBe('/api/user/context'));
    expect(localStorage.getItem(pendingKey)).not.toBeNull();
    expect(state.getSnapshot().context).toBeNull();
    contextRead.resolve(envelope(200, context)); await loggingIn;
    expect(localStorage.getItem(pendingKey)).toBeNull();
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', sessionId: 'new-login' });
    await expect(state.retryLogout()).rejects.toMatchObject({ errorCode: 'CLIENT_AUTH_OPERATION_SUPERSEDED' });
    expect(state.getSnapshot().phase).toBe('authenticated');
  });

  it('records logout during an in-flight renewal and waits for its body before sending the exit', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    const response = deferred<Response>();
    fetch.mockImplementationOnce(() => response.promise);
    const refreshing = state.refresh();
    const refused = expect(refreshing).rejects.toMatchObject({ errorCode: 'CLIENT_AUTH_OPERATION_SUPERSEDED' });
    await vi.waitFor(() => expect(fetch.mock.calls.at(-1)?.[0]).toBe('/api/refresh'));
    const exit = state.logout();
    await vi.waitFor(() => expect(state.getSnapshot().phase).toBe('logout-pending'));
    expect(fetch.mock.calls.filter(([path]) => path === '/api/user/logout')).toHaveLength(0);
    response.resolve(envelope(200, { access_token: token('late'), session_id: 'session-login-1' }));
    await refused; await exit;
    expect(state.getSnapshot()).toMatchObject({ phase: 'anonymous', context: null });
    expect(localStorage.getItem(pendingKey)).toBeNull();
  });

  it('removes old context when another tab changes identity, including a missed-event focus check', async () => {
    const { state } = await setup(); await state.login(input);
    localStorage.setItem(eventKey, JSON.stringify({ epoch: 9, kind: 'login', sessionId: 'another-user-session' }));
    window.dispatchEvent(new Event('focus'));
    expect(state.getSnapshot()).toMatchObject({ phase: 'anonymous', context: null, sessionId: null });
  });

  it('does not retain pending logout or revoke a new login after another tab completes the switch', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    fetch.mockResolvedValueOnce(envelope(500, null, 'IDENTITY_INTERNAL_ERROR'));
    await expect(state.logout()).rejects.toMatchObject({ status: 500 });
    localStorage.setItem(eventKey, JSON.stringify({ epoch: 9, kind: 'login', sessionId: 'new-browser-session' }));
    localStorage.removeItem(pendingKey);
    window.dispatchEvent(new Event('focus'));
    expect(state.getSnapshot()).toMatchObject({ phase: 'anonymous', context: null });
    const count = fetch.mock.calls.length;
    await expect(state.retryLogout()).rejects.toMatchObject({ errorCode: 'CLIENT_AUTH_OPERATION_SUPERSEDED' });
    expect(fetch).toHaveBeenCalledTimes(count);
  });

  it('does not let a queued old logout cancel or revoke a newer explicit login', async () => {
    const { state, fetch } = await setup(); await state.login(input);
    const gate = deferred<void>();
    const holding = navigator.locks.request('admin-workbench.auth.state', () => gate.promise);
    const exiting = state.logout('password-changed');
    const obsolete = expect(exiting).rejects.toMatchObject({ errorCode: 'CLIENT_AUTH_OPERATION_SUPERSEDED' });
    const loggingIn = state.login(input);
    gate.resolve(); await holding;
    await obsolete; await loggingIn;
    expect(state.getSnapshot()).toMatchObject({ phase: 'authenticated', sessionId: 'session-login-2' });
    expect(localStorage.getItem(pendingKey)).toBeNull();
    expect(fetch.mock.calls.filter(([path]) => path === '/api/user/logout')).toHaveLength(0);
  });

  it('reports unavailable storage rather than silently falling back to memory-only login', async () => {
    const { state, fetch } = await setup();
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new DOMException('blocked', 'SecurityError'); });
    await state.initialize();
    expect(state.getSnapshot()).toMatchObject({ phase: 'unavailable', context: null });
    expect(fetch).not.toHaveBeenCalled();
  });
});
