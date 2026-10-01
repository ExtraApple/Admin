import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { UserContext } from './models';
const user = { id: 7, username: 'operator', nickname: '操作者', avatar: '/api/avatars/default', email: 'operator@example.com', role: 'user', status: 1 };
const context: UserContext = { user, roles: ['reader'], permissions: ['admin.users.get'], menus: [] };
const input = { username: 'operator', password: 'Private123!', captcha_id: 'real-id', captcha_code: '123456' };
function envelope(status: number, data: unknown, errorCode = '') { return new Response(JSON.stringify({ code: status, error_code: errorCode, msg: errorCode ? 'request failed' : 'success', data }), { status }); }
function token(seconds = 60, tag = 'one') {
  return `${tag}.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + seconds }))}.signature`;
}
beforeEach(() => { vi.resetModules(); vi.useFakeTimers(); vi.setSystemTime(new Date('2026-09-30T08:00:00Z')); });
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals(); });
async function setup() {
  const access = token();
  const fetch = vi.fn(async (path: string, _init?: RequestInit) => {
    if (path === '/api/login') return envelope(200, { access_token: access, refresh_token: 'refresh-one', user });
    if (path === '/api/user/context') return envelope(200, context);
    if (path === '/api/refresh') return envelope(200, { access_token: token(60, 'two'), refresh_token: 'refresh-two' });
    return envelope(200, null);
  });
  vi.stubGlobal('fetch', fetch);
  // Runtime imports intentionally exercise the fresh-page module boundary and its empty memory session.
  const { sessionState } = await import('./session-state');
  const { api } = await import('./api');
  return { sessionState, api, fetch, access };
}
describe('memory-only authenticated lifecycle', () => {
  it('establishes context only after authenticated read, and never restores tokens across a page instance', async () => {
    const { sessionState, api, fetch, access } = await setup();
    expect(sessionState.getSnapshot()).toBeNull();
    await sessionState.login(input);
    expect(sessionState.getSnapshot()).toEqual(context);
    await api.get('/api/user/info');
    expect(new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization')).toBe(`Bearer ${access}`);
    vi.resetModules();
    // A new module instance models a full page reload; static imports would reuse its tokens.
    const fresh = await import('./session-state');
    expect(fresh.sessionState.getSnapshot()).toBeNull();
    const freshClient = await import('./api');
    await freshClient.api.get('/api/user/info');
    expect(new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization')).toBeNull();
  });
  it('refreshes proactively before expiry and uses rotated access credentials', async () => {
    const { sessionState, api, fetch, access } = await setup();
    await sessionState.login(input);
    await vi.advanceTimersByTimeAsync(29_000);
    expect(fetch.mock.calls.filter(call => call[0] === '/api/refresh')).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(fetch.mock.calls.filter(call => call[0] === '/api/refresh')).toHaveLength(1);
    expect(sessionState.getSnapshot()).toEqual(context);
    await api.get('/api/user/info');
    const nextToken = new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization');
    expect(nextToken).not.toBe(`Bearer ${access}`);
    expect(nextToken).toBe(`Bearer ${token(60, 'two')}`);
  });
  it('clears context and credentials on protected 401 without replaying the failed request', async () => {
    const { sessionState, api, fetch } = await setup(); await sessionState.login(input);
    fetch.mockResolvedValueOnce(envelope(401, null, 'AUTHN_TOKEN_INVALID'));
    await expect(api.put('/api/admin/users/8/roles', { role_ids: [], expected_access_version: 1 })).rejects.toMatchObject({ status: 401 });
    expect(sessionState.getSnapshot()).toBeNull();
    expect(fetch.mock.calls.filter(call => call[0] === '/api/admin/users/8/roles')).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch.mock.calls.filter(call => call[0] === '/api/refresh')).toHaveLength(0);
    await api.get('/api/user/info');
    expect(new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization')).toBeNull();
  });
  it('retains the authenticated page context on 403', async () => {
    const { sessionState, api, fetch } = await setup(); await sessionState.login(input);
    fetch.mockResolvedValueOnce(envelope(403, null, 'API_META_PERMISSION_DENIED'));
    await expect(api.get('/api/admin/roles')).rejects.toMatchObject({ status: 403 });
    expect(sessionState.getSnapshot()).toEqual(context);
    expect(fetch.mock.calls.filter(call => call[0] === '/api/admin/roles')).toHaveLength(1);
  });
  it('clears the session if proactive refresh is refused and never retries refresh automatically', async () => {
    const { sessionState, fetch } = await setup(); await sessionState.login(input);
    fetch.mockResolvedValueOnce(envelope(401, null, 'AUTHN_REFRESH_TOKEN_INVALID'));
    await vi.advanceTimersByTimeAsync(30_000);
    expect(sessionState.getSnapshot()).toBeNull();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch.mock.calls.filter(call => call[0] === '/api/refresh')).toHaveLength(1);
  });
  it('does not establish a session from rejected credentials or a refused context read', async () => {
    const { sessionState, fetch } = await setup();
    fetch.mockResolvedValueOnce(envelope(401, { remaining_attempts: 2 }, 'AUTHN_CREDENTIALS_INVALID'));
    await expect(sessionState.login(input)).rejects.toMatchObject({ errorCode: 'AUTHN_CREDENTIALS_INVALID' });
    expect(sessionState.getSnapshot()).toBeNull();
    expect(fetch).toHaveBeenCalledTimes(1);
    fetch.mockResolvedValueOnce(envelope(200, { access_token: token(), refresh_token: 'refresh', user })).mockResolvedValueOnce(envelope(403, null, 'IDENTITY_ACCOUNT_DISABLED'));
    await expect(sessionState.login(input)).rejects.toMatchObject({ status: 403 });
    expect(sessionState.getSnapshot()).toBeNull();
  });
  it('clears local credentials even when server logout fails', async () => {
    const { sessionState, api, fetch } = await setup(); await sessionState.login(input);
    fetch.mockResolvedValueOnce(envelope(500, null, 'IDENTITY_INTERNAL_ERROR'));
    await expect(sessionState.logout()).rejects.toMatchObject({ status: 500 });
    expect(sessionState.getSnapshot()).toBeNull();
    await api.get('/api/user/info');
    expect(new Headers(fetch.mock.calls.at(-1)?.[1]?.headers).get('Authorization')).toBeNull();
  });
  it('does not resurrect a logged-out session from an in-flight refresh', async () => {
    const { sessionState, fetch } = await setup(); await sessionState.login(input);
    let finish!: (value: Response) => void;
    fetch.mockImplementationOnce(() => new Promise<Response>(resolve => { finish = resolve; }));
    const refreshing = sessionState.refresh();
    const rejected = expect(refreshing).rejects.toMatchObject({ status: 401 });
    await sessionState.logout();
    finish(envelope(200, { access_token: token(60, 'late'), refresh_token: 'late-refresh' }));
    await rejected;
    expect(sessionState.getSnapshot()).toBeNull();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(fetch.mock.calls.filter(call => call[0] === '/api/refresh')).toHaveLength(1);
  });
});
