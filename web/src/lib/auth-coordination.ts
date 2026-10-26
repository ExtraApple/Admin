import { ApiError } from './api';

const authLock = 'admin-workbench.auth';
const stateLock = `${authLock}.state`;
const keys = {
  event: `${authLock}.event`,
  pending: `${authLock}.logout-pending`,
  attempt: `${authLock}.refresh-attempt`,
};
export interface AuthEvent { epoch: number; kind: 'login' | 'logout' | 'invalidated'; sessionId: string | null }
export interface LogoutIntent { id: string; sessionId: string | null; epoch: number }
export interface RefreshAttempt { id: string; startedAt: number; epoch: number }
interface StoredState { event: AuthEvent | null; pending: LogoutIntent | null; attempt: RefreshAttempt | null }
interface StateEditor extends StoredState {
  savePending(value: LogoutIntent | null): void;
  saveAttempt(value: RefreshAttempt | null): void;
  emit(kind: AuthEvent['kind'], sessionId: string | null): AuthEvent;
}
export function unavailable(): ApiError { return new ApiError(0, 'CLIENT_SESSION_UNAVAILABLE', 'Required browser capabilities are unavailable.'); }
function record(value: unknown): value is Record<string, unknown> { return !!value && typeof value === 'object' && !Array.isArray(value); }
function integer(value: unknown): value is number { return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0; }
function identifier(value: unknown): value is string { return typeof value === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(value); }
function sessionId(value: unknown) { return value === null || identifier(value); }
function parse<K extends keyof StoredState>(kind: K): StoredState[K] {
  try {
    const raw = localStorage.getItem(keys[kind]);
    if (raw === null) return null;
    const value: unknown = JSON.parse(raw);
    if (!record(value) || !integer(value.epoch)) throw unavailable();
    if (kind === 'event' && sessionId(value.sessionId) && ['login', 'logout', 'invalidated'].includes(String(value.kind)) && Object.keys(value).length === 3) return value as unknown as StoredState[K];
    if (kind === 'pending' && identifier(value.id) && sessionId(value.sessionId) && Object.keys(value).length === 3) return value as unknown as StoredState[K];
    if (kind === 'attempt' && identifier(value.id) && integer(value.startedAt) && Object.keys(value).length === 3) return value as unknown as StoredState[K];
  } catch { throw unavailable(); }
  throw unavailable();
}
function store<K extends keyof StoredState>(kind: K, value: StoredState[K]) {
  try {
    if (value === null) localStorage.removeItem(keys[kind]);
    else localStorage.setItem(keys[kind], JSON.stringify(value));
  } catch { throw unavailable(); }
}
export function checkBrowserCapabilities() {
  try {
    if (!globalThis.isSecureContext || !navigator.cookieEnabled || !navigator.locks || !crypto.randomUUID) throw unavailable();
    const probe = `${authLock}.probe.${crypto.randomUUID()}`;
    try {
      localStorage.setItem(probe, '1');
      if (localStorage.getItem(probe) !== '1') throw unavailable();
    } finally { localStorage.removeItem(probe); }
    const cookie = `admin_session_probe_${crypto.randomUUID().replaceAll('-', '')}`;
    const secure = location.protocol === 'https:' ? '; Secure' : '';
    try {
      document.cookie = `${cookie}=1; Path=/; SameSite=Strict${secure}`;
      if (!document.cookie.split(';').some(item => item.trim() === `${cookie}=1`)) throw unavailable();
    } finally { document.cookie = `${cookie}=; Path=/; Max-Age=0; SameSite=Strict${secure}`; }
  } catch { throw unavailable(); }
}
// Synchronous reads may deny an existing Access, never establish identity.
export function readAuthSignals() { return { event: parse('event'), pending: parse('pending') }; }
export function isAuthStorageEvent(key: string | null) { return key === null || Object.values(keys).includes(key); }
export async function withAuthenticationLock<T>(work: () => Promise<T>): Promise<T> {
  if (!navigator.locks) throw unavailable();
  return await navigator.locks.request(authLock, work);
}
// The callback is synchronous. Never hold this short lock across HTTP or another lock.
export function withAuthState<T>(work: (state: StateEditor) => T): Promise<T> {
  if (!navigator.locks) return Promise.reject(unavailable());
  return navigator.locks.request(stateLock, () => {
    const state: StateEditor = {
      event: parse('event'), pending: parse('pending'), attempt: parse('attempt'),
      savePending(value) { store('pending', value); state.pending = value; },
      saveAttempt(value) { store('attempt', value); state.attempt = value; },
      emit(kind, sessionId) {
        const epoch = (state.event?.epoch ?? 0) + 1;
        if (!Number.isSafeInteger(epoch)) throw unavailable();
        const event: AuthEvent = { epoch, kind, sessionId };
        store('event', event); state.event = event;
        return event;
      },
    };
    return work(state);
  });
}
