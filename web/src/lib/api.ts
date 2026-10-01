import { localizedError, safeFallback } from './errors';

export interface FieldError { field: string; error_code: string; message: string }
export interface SafeErrorData { remaining_attempts?: number; retry_after_seconds?: number }
interface Envelope { code: number; error_code: string; msg: string; data: unknown }
interface AuthBridge { token(): string | null; unauthorized(token: string | null): void }
let auth: AuthBridge | null = null;
export function configureApiAuth(bridge: AuthBridge) { auth = bridge; }

export class ApiError extends Error {
  constructor(
    public status: number,
    public errorCode: string,
    public fallback: string,
    public fields: FieldError[] = [],
    public safeData: SafeErrorData | null = null,
    public retryAfterSeconds = 0,
  ) { super(localizedError(errorCode, fallback)); this.name = 'ApiError'; }
}
export function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : '操作未完成，请稍后手动重试。';
}
export function fieldMessages(error: unknown): Record<string, string[]> {
  const result: Record<string, string[]> = Object.create(null);
  if (error instanceof ApiError) for (const field of error.fields) {
    (result[field.field] ??= []).push(localizedError(field.error_code, field.message));
  }
  return result;
}
const publicPaths: Readonly<Record<string, true>> = { '/api/captcha': true, '/api/login': true, '/api/register': true, '/api/refresh': true };
function isPublicPath(path: string) {
  const pathname = path.split(/[?#]/)[0];
  return !!publicPaths[pathname] || /^\/api\/avatars\/(?:default|[1-9]\d*)$/.test(pathname) || pathname === '/docs' || pathname.startsWith('/docs/');
}
function object(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}
function invalid(status: number): ApiError {
  return new ApiError(status, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
}
function envelope(value: unknown, status: number): Envelope {
  if (!object(value) || Object.keys(value).sort().join(',') !== 'code,data,error_code,msg'
    || value.code !== status || typeof value.error_code !== 'string' || typeof value.msg !== 'string') throw invalid(status);
  if ((status >= 200 && status < 300) ? value.error_code !== '' || value.msg !== 'success' : !/^[A-Z][A-Z0-9_]*$/.test(value.error_code)) throw invalid(status);
  return value as unknown as Envelope;
}
function seconds(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? Math.ceil(value) : undefined;
}
function retryAfter(response: Response): number {
  const value = response.headers.get('Retry-After');
  if (!value) return 0;
  if (/^\d+$/.test(value)) return Number(value);
  const date = Date.parse(value);
  return Number.isFinite(date) ? Math.max(0, Math.ceil((date - Date.now()) / 1000)) : 0;
}
function failure(response: Response, value: Envelope): ApiError {
  const data = object(value.data) ? value.data : null;
  const fields: FieldError[] = [];
  if (response.status === 422 && Array.isArray(data?.fields)) for (const item of data.fields) {
    if (object(item) && typeof item.field === 'string' && /^[a-z][a-z0-9_]*$/.test(item.field)
      && typeof item.error_code === 'string' && /^[A-Z][A-Z0-9_]*$/.test(item.error_code)
      && typeof item.message === 'string') fields.push({ field: item.field, error_code: item.error_code, message: safeFallback(item.message) });
  }
  const safeData: SafeErrorData = {};
  if (value.error_code === 'AUTHN_CREDENTIALS_INVALID') {
    const remaining = seconds(data?.remaining_attempts);
    if (remaining !== undefined) safeData.remaining_attempts = remaining;
  }
  if (value.error_code === 'AUTHN_LOGIN_LOCKED' || value.error_code === 'IDENTITY_EMAIL_VERIFICATION_RATE_LIMITED') {
    const retry = seconds(data?.retry_after_seconds);
    if (retry !== undefined) safeData.retry_after_seconds = retry;
  }
  return new ApiError(response.status, value.error_code, safeFallback(value.msg), fields,
    Object.keys(safeData).length ? safeData : null,
    Math.max(retryAfter(response), safeData.retry_after_seconds ?? 0));
}
async function request<T>(path: string, method: string, body: unknown, mode: 'json' | 'blob' | 'raw'): Promise<T> {
  // Restrict bearer credentials to same-origin API/resources, never an arbitrary URL.
  if (!path.startsWith('/') || path.startsWith('//') || path.includes('\\')) throw new ApiError(0, 'HTTP_REQUEST_INVALID', 'Request path is invalid.');
  const publicRequest = isPublicPath(path);
  const token = publicRequest ? null : auth?.token() ?? null;
  const headers = new Headers({ Accept: mode === 'blob' ? '*/*' : 'application/json' });
  if (token) headers.set('Authorization', `Bearer ${token}`);
  if (body !== undefined) headers.set('Content-Type', 'application/json');
  let response: Response;
  try { response = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin' }); }
  catch { throw new ApiError(0, 'CLIENT_NETWORK_ERROR', 'Network request failed.'); }
  if (response.status === 401 && !publicRequest) auth?.unauthorized(token);
  if (response.ok && mode === 'blob') return await response.blob() as T;
  let value: unknown;
  try { value = await response.json(); } catch { throw invalid(response.status); }
  if (response.ok && mode === 'raw') return value as T;
  const payload = envelope(value, response.status);
  if (!response.ok) throw failure(response, payload);
  return payload.data as T;
}
export const api = {
  get: <T>(path: string) => request<T>(path, 'GET', undefined, 'json'),
  post: <T>(path: string, body: unknown) => request<T>(path, 'POST', body, 'json'),
  put: <T>(path: string, body?: unknown) => request<T>(path, 'PUT', body, 'json'),
  delete: <T = void>(path: string) => request<T>(path, 'DELETE', undefined, 'json'),
  blob: (path: string) => request<Blob>(path, 'GET', undefined, 'blob'),
  image: (path: string) => request<Blob>(path, 'GET', undefined, 'blob'),
  raw: <T>(path: string) => request<T>(path, 'GET', undefined, 'raw'),
};
