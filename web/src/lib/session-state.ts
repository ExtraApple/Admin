import { api, configureApiAuth, ApiError } from './api';
import type { LoginInput, TokenPair, UserContext, UserInfo } from './models';

let context: UserContext | null = null;
let tokens: TokenPair | null = null;
let generation = 0;
let refreshTimer: number | undefined;
let refreshFlight: Promise<void> | null = null;
const listeners = new Set<() => void>();
function publish() { for (const listener of listeners) listener(); }
function clear() {
  generation++;
  tokens = null;
  context = null;
  clearTimeout(refreshTimer);
  refreshFlight = null;
  publish();
}
export function accessExpiry(token: string): number | null {
  try {
    const part = token.split('.')[1];
    const payload: unknown = JSON.parse(atob(part.replace(/-/g, '+').replace(/_/g, '/')));
    if (payload && typeof payload === 'object' && 'exp' in payload && typeof payload.exp === 'number' && Number.isFinite(payload.exp)) return payload.exp * 1000;
  } catch { /* A missing expiry cannot be used to schedule a refresh. */ }
  return null;
}
function schedule() {
  clearTimeout(refreshTimer);
  if (!tokens) return;
  const expires = accessExpiry(tokens.access_token);
  if (expires === null) return;
  const remaining = expires - Date.now();
  const delay = Math.max(0, remaining - Math.min(30_000, remaining / 2));
  refreshTimer = globalThis.setTimeout(() => { void refresh().catch(() => { /* refresh clears the session before rejecting */ }); }, Math.min(delay, 2_147_483_647)) as unknown as number;
}
function validateTokens(value: TokenPair) {
  if (!value || typeof value.access_token !== 'string' || !value.access_token || typeof value.refresh_token !== 'string' || !value.refresh_token) {
    throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
  }
}
function assertGeneration(expected: number) {
  if (generation !== expected) throw new ApiError(401, 'AUTHN_TOKEN_INVALID', 'Authentication token is invalid.');
}
async function reloadContext() {
  const expected = generation;
  const value = await api.get<UserContext>('/api/user/context');
  assertGeneration(expected);
  if (!value || !value.user || !Array.isArray(value.roles) || !Array.isArray(value.permissions) || !Array.isArray(value.menus)) {
    throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
  }
  context = value;
  publish();
}
async function login(input: LoginInput) {
  clear();
  const expected = generation;
  try {
    const result = await api.post<TokenPair & { user: UserInfo }>('/api/login', input);
    assertGeneration(expected);
    validateTokens(result);
    tokens = { access_token: result.access_token, refresh_token: result.refresh_token };
    await reloadContext();
    assertGeneration(expected);
    schedule();
  } catch (error) { if (generation === expected) clear(); throw error; }
}
function refresh(): Promise<void> {
  if (refreshFlight) return refreshFlight;
  if (!tokens) return Promise.reject(new ApiError(401, 'AUTHN_REFRESH_TOKEN_INVALID', 'Refresh token is invalid.'));
  const expected = generation;
  const refreshToken = tokens.refresh_token;
  const flight = (async () => {
    try {
      const result = await api.post<TokenPair>('/api/refresh', { refresh_token: refreshToken });
      assertGeneration(expected);
      validateTokens(result);
      tokens = { access_token: result.access_token, refresh_token: result.refresh_token };
      schedule();
    } catch (error) { if (generation === expected) clear(); throw error; }
    finally { if (generation === expected) refreshFlight = null; }
  })();
  refreshFlight = flight;
  return flight;
}
async function logout() {
  try { if (tokens) await api.post<null>('/api/user/logout', {}); }
  finally { clear(); }
}
configureApiAuth({ token: () => tokens?.access_token ?? null, unauthorized: (token) => { if (tokens && token === tokens.access_token) clear(); } });
export const sessionState = {
  getSnapshot: () => context,
  subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
  login, logout, refresh, reloadContext,
};
