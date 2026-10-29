import { localizedError, safeFallback } from './errors';
import type {
  ApiMetadata,
  ApiMetadataPage,
  ApiGroupOption,
  ApiMethodOption,
  ApiPermissionSyncResult,
  ApiSyncResult,
  ApiSyncRequest,
  ApiUpdateInput,
  AuthorizationOverview,
  AuthorizationRiskPage,
  AuthorizationRiskQuery,
  GenerateMenuButtonInput,
  ImpactPreview,
  ImpactPreviewRequest,
  Menu,
  MenuApi,
  MenuBindingRequest,
  MenuClearBindingRequest,
  MenuCreateInput,
  MenuSyncRequest,
  MenuUpdateInput,
} from './models';
export interface FieldError { field: string; error_code: string; message: string }
export interface SafeErrorData { remaining_attempts?: number; retry_after_seconds?: number }
interface Envelope { code: number; error_code: string; msg: string; data: unknown }
interface AuthBridge { token(path: string): string | null; rejected(token: string | null, status: number, code: string): void }
export interface RequestOptions {
  refreshAttemptId?: string;
  cache?: RequestCache;
  signal?: AbortSignal;
  headers?: HeadersInit;
}
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
function isPublicPath(path: string, method: string) {
  const pathname = path.split(/[?#]/)[0];
  return !!publicPaths[pathname] || (method === 'POST' && pathname === '/api/user/logout') || /^\/api\/avatars\/(?:default|[1-9]\d*)$/.test(pathname) || pathname === '/docs' || pathname.startsWith('/docs/');
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
async function request<T>(path: string, method: string, body: unknown, mode: 'json' | 'blob' | 'raw', options?: RequestOptions): Promise<T> {
  // Restrict bearer credentials to same-origin API/resources, never an arbitrary URL.
  if (!path.startsWith('/') || path.startsWith('//') || path.includes('\\')) throw new ApiError(0, 'HTTP_REQUEST_INVALID', 'Request path is invalid.');
  const pathname = path.split(/[?#]/)[0];
  const publicRequest = isPublicPath(path, method);
  const token = publicRequest ? null : auth?.token(pathname) ?? null;
  const control = method === 'POST' && (pathname === '/api/login' || pathname === '/api/refresh' || pathname === '/api/user/logout');
  const headers = new Headers({ Accept: mode === 'blob' ? '*/*' : 'application/json' });
  if (options?.headers) {
    new Headers(options.headers).forEach((value, name) => {
      if (name.toLowerCase() !== 'authorization') headers.set(name, value);
    });
  }
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const multipart = body instanceof FormData;
  if (body !== undefined && !multipart) headers.set('Content-Type', 'application/json');
  if (control) headers.set('X-Admin-Request', '1');
  if (options?.refreshAttemptId) headers.set('X-Refresh-Attempt-ID', options.refreshAttemptId);
  const controller = control || pathname === '/api/user/context' ? new AbortController() : null;
  // Cancel the real fetch, including body reads, before releasing an authentication lock.
  const timeout = controller ? globalThis.setTimeout(() => controller.abort(), 15_000) : undefined;
  try {
    let response: Response;
    try { response = await fetch(path, { method, headers, body: body === undefined ? undefined : multipart ? body : JSON.stringify(body), credentials: 'same-origin', signal: controller?.signal ?? options?.signal, cache: options?.cache }); }
    catch { throw new ApiError(0, 'CLIENT_NETWORK_ERROR', 'Network request failed.'); }
    if (response.ok && mode === 'blob') return await response.blob() as T;
    let value: unknown;
    try { value = await response.json(); }
    catch { throw controller?.signal.aborted ? new ApiError(0, 'CLIENT_NETWORK_ERROR', 'Network request failed.') : invalid(response.status); }
    if (response.ok && mode === 'raw') return value as T;
    const payload = envelope(value, response.status);
    if (!response.ok) {
      if (!publicRequest && (response.status === 401 || payload.error_code === 'IDENTITY_ACCOUNT_DISABLED')) auth?.rejected(token, response.status, payload.error_code);
      throw failure(response, payload);
    }
    return payload.data as T;
  } finally { clearTimeout(timeout); }
}
function authorizationRiskPath(query: AuthorizationRiskQuery = {}): string {
  const params = new URLSearchParams();
  params.set('page', String(query.page ?? 1));
  params.set('size', String(query.size ?? 10));
  if (query.kind) params.set('kind', query.kind);
  if (query.resource) params.set('resource', query.resource);
  if (query.keyword?.trim()) params.set('keyword', query.keyword.trim());
  return `/api/admin/authorization-risks?${params.toString()}`;
}

function menuPath(id: number): string {
  return `/api/admin/menus/${encodeURIComponent(String(id))}`;
}
function apiPath(id: number): string {
  return `/api/admin/apis/${encodeURIComponent(String(id))}`;
}

export const api = {
  get: <T>(path: string) => request<T>(path, 'GET', undefined, 'json'),
  post: <T>(path: string, body: unknown, options?: RequestOptions) => request<T>(path, 'POST', body, 'json', options),
  put: <T>(path: string, body?: unknown, options?: RequestOptions) => request<T>(path, 'PUT', body, 'json', options),
  delete: <T = void>(path: string, options?: RequestOptions) => request<T>(path, 'DELETE', undefined, 'json', options),
  blob: (path: string) => request<Blob>(path, 'GET', undefined, 'blob'),
  image: (path: string, options?: Pick<RequestOptions, 'cache' | 'signal'>) => request<Blob>(path, 'GET', undefined, 'blob', options),
  raw: <T>(path: string) => request<T>(path, 'GET', undefined, 'raw'),
  authorizationOverview: () => request<AuthorizationOverview>('/api/admin/authorization-overview', 'GET', undefined, 'json'),
  authorizationRisks: (query: AuthorizationRiskQuery = {}) => request<AuthorizationRiskPage>(authorizationRiskPath(query), 'GET', undefined, 'json'),
  menus: {
    list: () => request<Menu[]>('/api/admin/menus', 'GET', undefined, 'json'),
    get: (id: number) => request<Menu>(menuPath(id), 'GET', undefined, 'json'),
    preview: <TCandidate>(body: ImpactPreviewRequest<TCandidate>) => request<ImpactPreview>('/api/admin/menus/impact-preview', 'POST', body, 'json'),
    create: (body: MenuCreateInput) => request<Menu>('/api/admin/menus', 'POST', body, 'json'),
    update: (id: number, body: MenuUpdateInput) => request<Menu>(menuPath(id), 'PUT', body, 'json'),
    delete: (id: number, expectedFingerprint: string) => request<void>(menuPath(id), 'DELETE', undefined, 'json', {
      headers: { 'X-Expected-Fingerprint': expectedFingerprint },
    }),
    listApis: (id: number) => request<MenuApi[]>(`${menuPath(id)}/apis`, 'GET', undefined, 'json'),
    bindApis: (id: number, body: MenuBindingRequest) => request<void>(`${menuPath(id)}/apis`, 'POST', body, 'json'),
    clearApis: (id: number, body: MenuClearBindingRequest) => request<void>(`${menuPath(id)}/apis`, 'POST', body, 'json'),
    sync: (body: MenuSyncRequest) => request<{ created: number }>('/api/admin/menus/sync', 'POST', body, 'json'),
  },
  apis: {
    list: (path = '/api/admin/apis') => request<ApiMetadataPage>(path, 'GET', undefined, 'json'),
    get: (id: number) => request<ApiMetadata>(apiPath(id), 'GET', undefined, 'json'),
    preview: <TCandidate>(body: ImpactPreviewRequest<TCandidate>) => request<ImpactPreview>('/api/admin/apis/impact-preview', 'POST', body, 'json'),
    update: (id: number, body: ApiUpdateInput) => request<ApiMetadata>(apiPath(id), 'PUT', body, 'json'),
    delete: (id: number, expectedFingerprint: string) => request<void>(apiPath(id), 'DELETE', undefined, 'json', {
      headers: { 'X-Expected-Fingerprint': expectedFingerprint },
    }),
    groups: () => request<ApiGroupOption[]>('/api/admin/api-groups', 'GET', undefined, 'json'),
    methods: () => request<ApiMethodOption[]>('/api/admin/api-methods', 'GET', undefined, 'json'),
    sync: (body: ApiSyncRequest) => request<ApiSyncResult>('/api/admin/apis/sync', 'POST', body, 'json'),
    syncPermissions: (body: ApiSyncRequest) => request<ApiPermissionSyncResult>('/api/admin/apis/sync-permissions', 'POST', body, 'json'),
    generateMenuButton: (id: number, body: GenerateMenuButtonInput) => request<Menu>(`${apiPath(id)}/menu-button`, 'POST', body, 'json'),
  },
};
