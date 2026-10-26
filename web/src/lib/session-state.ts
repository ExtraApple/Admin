import { api, configureApiAuth, ApiError, errorMessage } from './api';
import { checkBrowserCapabilities, isAuthStorageEvent, readAuthSignals, unavailable, withAuthenticationLock, withAuthState, type LogoutIntent } from './auth-coordination';
import type { LoginInput, SessionGrant, UserContext, UserInfo } from './models';

export type SessionPhase = 'bootstrapping' | 'anonymous' | 'authenticated' | 'recovery-required' | 'logout-pending' | 'unavailable';
export interface SessionSnapshot { phase: SessionPhase; context: UserContext | null; sessionId: string | null; notice: string | null }
const initialSnapshot: SessionSnapshot = { phase: 'bootstrapping', context: null, sessionId: null, notice: null };
let snapshot = initialSnapshot;
let grant: SessionGrant | null = null;
let verifying: SessionGrant | null = null;
let verificationPermit: { pendingId: string | null; epoch: number } | null = null;
let generation = 0;
let eventEpoch = 0;
let refreshTimer: ReturnType<typeof setTimeout> | undefined;
let expiryTimer: ReturnType<typeof setTimeout> | undefined;
let refreshFlight: Promise<void> | null = null;
let logoutFlight: Promise<void> | null = null;
let initializeFlight: Promise<void> | null = null;
let initialized = false;
let watching = false;
const listeners = new Set<() => void>();
function publish(next: SessionSnapshot) { snapshot = next; for (const listener of listeners) listener(); }
function clear(phase: SessionPhase, notice: string | null = null) {
  generation++;
  grant = null; verifying = null; verificationPermit = null;
  clearTimeout(refreshTimer); clearTimeout(expiryTimer);
  publish({ phase, context: null, sessionId: null, notice });
}
function superseded() { return new ApiError(0, 'CLIENT_AUTH_OPERATION_SUPERSEDED', 'Authentication operation was superseded.'); }
function assertGeneration(expected: number) { if (generation !== expected) throw superseded(); }
export function accessExpiry(token: string): number | null {
  try {
    const part = token.split('.')[1];
    const payload: unknown = JSON.parse(atob(part.replace(/-/g, '+').replace(/_/g, '/')));
    if (payload && typeof payload === 'object' && 'exp' in payload && typeof payload.exp === 'number' && Number.isFinite(payload.exp)) return payload.exp * 1000;
  } catch { /* Expiry is scheduling information, never proof of authentication. */ }
  return null;
}
function validateGrant(value: SessionGrant) {
  if (!value || typeof value.access_token !== 'string' || !value.access_token || typeof value.session_id !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(value.session_id) || accessExpiry(value.access_token) === null) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
}
function validateContext(value: UserContext) {
  if (!value || !value.user || !Number.isSafeInteger(value.user.id) || !Array.isArray(value.roles) || !Array.isArray(value.permissions) || !Array.isArray(value.menus)) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
}
function pause(notice = '会话需要重新确认。恢复前已暂停受保护操作，当前未保存修改仍保留。') {
  clearTimeout(refreshTimer);
  publish({ ...snapshot, phase: 'recovery-required', notice });
}
function schedule() {
  clearTimeout(refreshTimer); clearTimeout(expiryTimer);
  if (!grant) return;
  const remaining = (accessExpiry(grant.access_token) ?? 0) - Date.now();
  if (remaining <= 0) { pause(); return; }
  refreshTimer = setTimeout(() => { void refresh().catch(() => { /* The failure is represented in the snapshot. */ }); }, Math.min(Math.max(0, remaining - Math.min(30_000, remaining / 2)), 2_147_483_647));
  expiryTimer = setTimeout(() => {
    if (grant && (accessExpiry(grant.access_token) ?? 0) <= Date.now()) pause();
    else schedule();
  }, Math.min(remaining, 2_147_483_647));
}
function observeSignals(): boolean {
  const signals = readAuthSignals();
  if (signals.pending) {
    if (snapshot.phase !== 'logout-pending') clear('logout-pending', '服务端退出尚未确认，请重试退出或显式重新登录。');
    return false;
  }
  if (signals.event && signals.event.epoch > eventEpoch) {
    eventEpoch = signals.event.epoch;
    if ((snapshot.sessionId && (signals.event.kind !== 'login' || signals.event.sessionId !== snapshot.sessionId)) || snapshot.phase === 'logout-pending') {
      clear('anonymous', signals.event.kind === 'login' ? '其他标签已切换登录身份，旧编辑已清除。请重新确认当前身份。' : '当前会话已在其他标签退出或失效。');
      return false;
    }
  }
  return true;
}
function syncBrowserState() {
  try {
    if (!observeSignals()) return;
    if (grant && snapshot.phase === 'authenticated' && (accessExpiry(grant.access_token) ?? 0) <= Date.now()) pause();
  } catch { clear('unavailable', errorMessage(unavailable())); }
}
function watch() {
  if (watching) return;
  watching = true;
  window.addEventListener('storage', event => { if (isAuthStorageEvent(event.key)) syncBrowserState(); });
  window.addEventListener('focus', syncBrowserState);
  document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'visible') syncBrowserState(); });
}
async function rejectIdentity(sessionId: string | null, notice: string) {
  clear('anonymous', notice);
  try {
    await withAuthState(state => {
      if (sessionId && state.event?.sessionId === sessionId && !state.pending) eventEpoch = state.emit('invalidated', sessionId).epoch;
    });
  } catch { clear('unavailable', errorMessage(unavailable())); }
}
async function recoveryFailure(error: unknown, expected: number) {
  if (generation !== expected) return;
  if (error instanceof ApiError && error.errorCode === 'CLIENT_SESSION_UNAVAILABLE') { clear('unavailable', error.message); return; }
  try { if (!observeSignals()) return; } catch { clear('unavailable', errorMessage(unavailable())); return; }
  if (error instanceof ApiError && (error.status === 401 || error.errorCode === 'IDENTITY_ACCOUNT_DISABLED')) {
    if (snapshot.phase === 'bootstrapping' && !readAuthSignals().event) clear('anonymous');
    else await rejectIdentity(snapshot.sessionId, error.message);
  } else if (snapshot.context && grant && (accessExpiry(grant.access_token) ?? 0) > Date.now()) {
    publish({ ...snapshot, phase: 'authenticated', notice: `会话续期暂未完成：${errorMessage(error)} 当前页面和未保存修改仍保留。` });
  } else {
    pause(errorMessage(error));
  }
}
function refresh(): Promise<void> {
  if (refreshFlight) return refreshFlight;
  const expected = generation;
  let ownedAttempt: string | null = null;
  const flight = (async () => {
    try {
      checkBrowserCapabilities(); watch();
      await withAuthenticationLock(async () => {
        const permit = await withAuthState(state => {
          assertGeneration(expected);
          if (state.pending) { clear('logout-pending', '服务端退出尚未确认，请重试退出。'); throw superseded(); }
          if (!observeSignals()) throw superseded();
          let attempt = state.attempt;
          if (attempt && (Date.now() - attempt.startedAt >= 30_000 || attempt.startedAt > Date.now())) {
            // Only the server can tell an unconsumed current Cookie from an old credential.
            // A manual retry may start a new attempt; replay/reuse rules remain server-enforced.
            state.saveAttempt(null); attempt = null;
          }
          if (!attempt) {
            attempt = { id: crypto.randomUUID(), startedAt: Date.now(), epoch: state.event?.epoch ?? 0 };
            state.saveAttempt(attempt);
          }
          return { attempt, epoch: state.event?.epoch ?? 0 };
        });
        ownedAttempt = permit.attempt.id;
        const next = await api.post<SessionGrant>('/api/refresh', {}, { refreshAttemptId: permit.attempt.id });
        assertGeneration(expected); validateGrant(next);
        verifying = next;
        verificationPermit = { pendingId: null, epoch: permit.epoch };
        const context = await api.get<UserContext>('/api/user/context');
        validateContext(context);
        await withAuthState(state => {
          assertGeneration(expected);
          if (state.pending || (state.event?.epoch ?? 0) !== permit.epoch) throw superseded();
          const changed = snapshot.sessionId !== null && (snapshot.sessionId !== next.session_id || snapshot.context?.user.id !== context.user.id);
          state.saveAttempt(null);
          if (state.event?.kind !== 'login' || state.event.sessionId !== next.session_id) eventEpoch = state.emit('login', next.session_id).epoch;
          else eventEpoch = state.event.epoch;
          grant = next; verifying = null;
          publish({ phase: 'authenticated', context, sessionId: next.session_id, notice: changed ? '登录身份已变化，旧编辑已清除。' : null });
          schedule();
        });
      });
    } catch (error) {
      if (error instanceof ApiError && error.errorCode === 'AUTHN_REFRESH_SUPERSEDED') {
        try { await withAuthState(state => { if (state.attempt?.id === ownedAttempt) state.saveAttempt(null); }); }
        catch { clear('unavailable', errorMessage(unavailable())); throw unavailable(); }
      }
      await recoveryFailure(error, expected);
      throw error;
    } finally { verifying = null; }
  })();
  refreshFlight = flight;
  void flight.finally(() => { if (refreshFlight === flight) refreshFlight = null; }).catch(() => { /* Preserve rejection for the caller. */ });
  return flight;
}
async function initialize() {
  if (initialized) return;
  if (initializeFlight) return initializeFlight;
  initializeFlight = (async () => {
    try { await refresh(); } catch { /* Startup failures are explicit snapshot states. */ }
    finally { initialized = true; initializeFlight = null; }
  })();
  return initializeFlight;
}
async function login(input: LoginInput) {
  clear('anonymous');
  const expected = generation;
  try {
    checkBrowserCapabilities(); watch();
    // Capture the user's intent before waiting for a network lock: a later logout wins.
    const intent = await withAuthState(state => ({ pendingId: state.pending?.id ?? null, epoch: state.event?.epoch ?? 0 }));
    await withAuthenticationLock(async () => {
      await withAuthState(state => { assertGeneration(expected); if ((state.pending?.id ?? null) !== intent.pendingId || (state.event?.epoch ?? 0) !== intent.epoch) throw superseded(); });
      const next = await api.post<SessionGrant & { user: UserInfo }>('/api/login', input);
      assertGeneration(expected); validateGrant(next);
      verifying = next;
      verificationPermit = intent;
      const context = await api.get<UserContext>('/api/user/context');
      validateContext(context);
      await withAuthState(state => {
        assertGeneration(expected);
        if ((state.pending?.id ?? null) !== intent.pendingId || (state.event?.epoch ?? 0) !== intent.epoch) {
          // A logout accepted while Login was in flight must revoke the unconfirmed result.
          if (state.pending && state.pending.id !== intent.pendingId) state.savePending({ ...state.pending, sessionId: next.session_id });
          throw superseded();
        }
        eventEpoch = state.emit('login', next.session_id).epoch;
        state.saveAttempt(null); state.savePending(null);
        grant = next; verifying = null;
        publish({ phase: 'authenticated', context, sessionId: next.session_id, notice: null });
        schedule();
      });
    });
  } catch (error) {
    if (generation === expected) {
      try {
        if (readAuthSignals().pending) clear('logout-pending', '服务端退出尚未确认。新的登录未完成，退出意图仍保留。');
        else clear(error instanceof ApiError && error.errorCode === 'CLIENT_SESSION_UNAVAILABLE' ? 'unavailable' : 'anonymous', errorMessage(error));
      } catch { clear('unavailable', errorMessage(unavailable())); }
    }
    throw error;
  } finally { verifying = null; }
}
function performLogout(retry: boolean): Promise<void> {
  if (logoutFlight) return logoutFlight;
  const target = snapshot.sessionId;
  const flight = (async () => {
    let intent: LogoutIntent | null = null;
    let durable = true;
    try {
      intent = await withAuthState(state => {
        if (retry && !state.pending) throw superseded();
        if (target && state.event?.kind === 'login' && state.event.sessionId !== target) throw superseded();
        const value = state.pending ?? { id: crypto.randomUUID(), sessionId: target ?? state.event?.sessionId ?? null, epoch: state.event?.epoch ?? 0 };
        state.savePending(value);
        clear('logout-pending', '正在撤销当前浏览器会话。');
        return value;
      });
    } catch (error) {
      if (!(error instanceof ApiError) || error.errorCode !== 'CLIENT_SESSION_UNAVAILABLE') { syncBrowserState(); throw error; }
      durable = false;
      clear('unavailable', '本地退出意图无法保存。仍将尝试服务端撤销；未确认前不能保证跨刷新退出。');
    }
    try {
      await withAuthenticationLock(async () => {
        if (durable) await withAuthState(state => {
          const pending = state.pending;
          if (!pending || pending.id !== intent?.id || (state.event?.kind === 'login' && state.event.epoch > pending.epoch && state.event.sessionId !== pending.sessionId)) throw superseded();
        });
        await api.post<null>('/api/user/logout', {});
        if (!durable) throw unavailable();
        await withAuthState(state => {
          const pending = state.pending;
          if (!pending || pending.id !== intent?.id) throw superseded();
          eventEpoch = state.emit('logout', pending.sessionId).epoch;
          state.saveAttempt(null); state.savePending(null);
          clear('anonymous', '已退出登录。');
        });
      });
    } catch (error) {
      try {
        if (durable && readAuthSignals().pending?.id === intent?.id) clear('logout-pending', `服务端退出未确认：${errorMessage(error)}`);
        else if (!durable) clear('unavailable', `退出尚未可靠完成：${errorMessage(error)}`);
        else syncBrowserState();
      } catch { clear('unavailable', '退出尚未可靠完成，无法保存或读取本地退出意图。'); }
      throw error;
    }
  })();
  logoutFlight = flight;
  void flight.finally(() => { if (logoutFlight === flight) logoutFlight = null; }).catch(() => { /* State reports retryable failure. */ });
  return flight;
}
async function reloadContext() {
  const expected = generation;
  const context = await api.get<UserContext>('/api/user/context');
  validateContext(context); assertGeneration(expected);
  if (!observeSignals() || snapshot.phase !== 'authenticated') throw superseded();
  publish({ ...snapshot, context });
}
configureApiAuth({
  token(path) {
    try {
      if (path === '/api/user/context' && verifying && verificationPermit) {
        const signals = readAuthSignals();
        if ((signals.pending?.id ?? null) !== verificationPermit.pendingId || (signals.event?.epoch ?? 0) !== verificationPermit.epoch) throw superseded();
        return verifying.access_token;
      }
      if (!observeSignals()) throw superseded();
      if (!grant) return null;
      if (snapshot.phase !== 'authenticated' || (accessExpiry(grant.access_token) ?? 0) <= Date.now()) { pause(); throw new ApiError(0, 'CLIENT_SESSION_RECOVERY_REQUIRED', 'Session recovery is required.'); }
      return grant.access_token;
    } catch (error) {
      if (error instanceof ApiError && error.errorCode === 'CLIENT_SESSION_UNAVAILABLE') clear('unavailable', error.message);
      throw error;
    }
  },
  rejected(token, status, code) {
    if (!grant || token !== grant.access_token) return;
    if (status === 401 && (accessExpiry(grant.access_token) ?? 0) <= Date.now()) { pause(); return; }
    void rejectIdentity(snapshot.sessionId, errorMessage(new ApiError(status, code, 'Authentication failed.')));
  },
});
export const sessionState = {
  getSnapshot: () => snapshot,
  getServerSnapshot: () => initialSnapshot,
  subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
  initialize, login, refresh, reloadContext,
  logout: () => performLogout(false),
  retryLogout: () => performLogout(true),
  beginLogin: () => {
    try { clear(readAuthSignals().pending ? 'logout-pending' : 'anonymous', '请重新输入凭据登录。'); }
    catch { clear('unavailable', errorMessage(unavailable())); }
  },
};
