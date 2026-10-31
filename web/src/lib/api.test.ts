import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, fieldMessages, errorMessage } from './api';
import { lockDeadline, lockSeconds, captchaSource } from './auth-form';
import fixture from '../../../testsupport/testdata/api-response-contract.json';

interface ContractCase {
  name: string;
  status: number;
  headers?: Record<string, string>;
  envelope?: { code: number; error_code: string; msg: string; data: unknown };
  native?: { kind: 'image' | 'blob' | 'raw'; body?: string; base64?: string; json?: unknown };
  expected: {
    result?: unknown;
    status?: number;
    errorCode?: string;
    fallback?: string;
    message?: string;
    fields?: unknown[];
    fieldMessages?: Record<string, string[]>;
    safeData?: unknown;
    retryAfterSeconds?: number;
    contentType?: string;
    base64?: string;
    body?: string;
    json?: unknown;
  };
}
const contractCases = fixture.cases as ContractCase[];

describe('shared frontend/backend contract fixture', () => {
  it.each(contractCases)('$name', async test => {
    const native = test.native;
    const body = native
      ? native.base64 ? Uint8Array.from(atob(native.base64), char => char.charCodeAt(0))
        : native.body ?? JSON.stringify(native.json)
      : JSON.stringify(test.envelope);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { status: test.status, headers: test.headers })));
    if (native) {
      if (native.kind === 'raw') {
        expect(await api.raw('/docs/openapi.json')).toEqual(test.expected.json);
      } else {
        const value = await api[native.kind](native.kind === 'image' ? '/api/avatars/7' : '/api/admin/files/7/download');
        expect(value.type).toBe(test.expected.contentType);
        if (test.expected.base64) {
          expect(new Uint8Array(await value.arrayBuffer())).toEqual(Uint8Array.from(atob(test.expected.base64), char => char.charCodeAt(0)));
        } else {
          expect(await value.text()).toBe(test.expected.body);
        }
      }
    } else if (test.status < 400) {
      const value = test.name === 'success-null' ? await api.put('/api/admin/users/7/kick') : await api.get('/api/admin/users');
      expect(value).toEqual(test.expected.result);
    } else {
      const error = await api.post('/api/login', {}).catch(value => value);
      expect(error).toBeInstanceOf(ApiError);
      expect(error).toMatchObject({
        status: test.expected.status, errorCode: test.expected.errorCode, fallback: test.expected.fallback,
        message: test.expected.message, fields: test.expected.fields, safeData: test.expected.safeData,
        retryAfterSeconds: test.expected.retryAfterSeconds,
      });
      expect(errorMessage(error)).toBe(test.expected.message);
      expect(fieldMessages(error)).toEqual(test.expected.fieldMessages);

      // A server must not send these details; the consumer must not retain them either.
      const data = { ...((test.envelope!.data ?? {}) as object), cause: 'database password=secret', username_exists: true, password: 'secret' };
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response(test.status, test.envelope!.error_code, data, test.envelope!.msg, test.headers)));
      const unsafe = await api.post('/api/login', {}).catch(value => value);
      expect(unsafe).toBeInstanceOf(ApiError);
      if (!(unsafe instanceof ApiError)) throw unsafe;
      expect(unsafe.safeData).toEqual(test.expected.safeData);
      expect(unsafe.fields).toEqual(test.expected.fields);
      expect(JSON.stringify(unsafe)).not.toMatch(/secret|username_exists|cause/);
    }
  });
});

function response(status: number, errorCode: string, data: unknown, msg = errorCode ? 'request failed' : 'success', headers?: HeadersInit) {
  const responseHeaders = new Headers(headers);
  responseHeaders.set('Content-Type', 'application/json');
  return new Response(JSON.stringify({ code: status, error_code: errorCode, msg, data }), { status, headers: responseHeaders });
}
afterEach(() => { vi.unstubAllGlobals(); });
describe('single business response contract', () => {
  it.each([
    { code: 400, error_code: '', msg: 'success', data: {} },
    { code: 200, error_code: 'HTTP_INTERNAL_ERROR', msg: 'success', data: {} },
    { code: 200, error_code: '', msg: 'ok', data: {} },
    { code: 200, error_code: '', msg: 'success', data: {}, extra: true },
    { code: 200, msg: 'success', data: {} },
  ])('rejects inconsistent or legacy envelopes instead of accepting success: %j', async payload => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 })));
    await expect(api.get('/api/admin/roles')).rejects.toMatchObject({ errorCode: 'CLIENT_RESPONSE_INVALID', status: 200 });
  });
  it('uses the longer safe lock time and never replays credentials when countdown expires', async () => {
    const fetch = vi.fn().mockResolvedValue(response(429, 'AUTHN_LOGIN_LOCKED', { retry_after_seconds: 30 }, 'login is temporarily locked', { 'Retry-After': '37' }));
    vi.stubGlobal('fetch', fetch);
    const error = await api.post('/api/login', { password: 'private', captcha_code: '123456' }).catch(value => value);
    if (!(error instanceof ApiError)) throw error;
    expect(error.retryAfterSeconds).toBe(37);
    const deadline = lockDeadline(error, 1_000);
    expect(lockSeconds(deadline, 1_000)).toBe(37);
    expect(lockSeconds(deadline, 37_100)).toBe(1);
    expect(lockSeconds(deadline, 38_000)).toBe(0);
    expect(lockSeconds(deadline, 40_000)).toBe(0);
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it('handles network or non-JSON failure without leaking raw server bodies', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValueOnce(new Error('password=secret')).mockResolvedValueOnce(new Response('database password=secret', { status: 500 })));
    await expect(api.get('/api/admin/users')).rejects.toMatchObject({ errorCode: 'CLIENT_NETWORK_ERROR' });
    await expect(api.get('/api/admin/users')).rejects.toMatchObject({ errorCode: 'CLIENT_RESPONSE_INVALID' });
  });
  it('does not attach tokens or fetch arbitrary network destinations', async () => {
    const fetch = vi.fn(); vi.stubGlobal('fetch', fetch);
    await expect(api.get('https://example.com/api/users')).rejects.toMatchObject({ errorCode: 'HTTP_REQUEST_INVALID' });
    await expect(api.image('//example.com/avatar')).rejects.toMatchObject({ errorCode: 'HTTP_REQUEST_INVALID' });
    await expect(api.raw('/\\example.com/openapi')).rejects.toMatchObject({ errorCode: 'HTTP_REQUEST_INVALID' });
    expect(fetch).not.toHaveBeenCalled();
  });
  it('lets the browser encode a readable multipart file with its generated boundary', async () => {
    const received: { file: File | null; boundary: string | null } = { file: null, boundary: null };
    vi.stubGlobal('fetch', vi.fn(async (path: string, init: RequestInit) => {
      const request = new Request(`https://workbench.example${path}`, init);
      received.boundary = request.headers.get('Content-Type');
      const form = await request.formData();
      received.file = form.get('file') as File | null;
      return response(200, '', { id: 7 });
    }));
    const form = new FormData();
    form.append('file', new File([new Uint8Array([137, 80, 78, 71])], 'avatar.png', { type: 'image/png' }));
    await api.post('/api/user/avatar', form);
    expect(received.boundary).toMatch(/^multipart\/form-data; boundary=.+/);
    expect(received.file?.name).toBe('avatar.png');
    expect(received.file?.type).toBe('image/png');
    expect(new Uint8Array(await received.file!.arrayBuffer())).toEqual(new Uint8Array([137, 80, 78, 71]));
  });
});
describe('menu and API management write contract', () => {
  const fingerprint = 'a'.repeat(64);
  it('sends the menu fingerprint in the update body and delete header, never in the URL', async () => {
    const fetch = vi.fn().mockImplementation(async () => response(200, '', { id: 9 }));
    vi.stubGlobal('fetch', fetch);
    await api.menus.update(9, { name: 'Renamed', expected_fingerprint: fingerprint });
    await api.menus.delete(9, fingerprint);
    const [updatePath, updateInit] = fetch.mock.calls[0];
    expect(updatePath).toBe('/api/admin/menus/9');
    expect(updateInit.method).toBe('PUT');
    expect(JSON.parse(updateInit.body)).toEqual({ name: 'Renamed', expected_fingerprint: fingerprint });
    const [deletePath, deleteInit] = fetch.mock.calls[1];
    expect(deletePath).toBe('/api/admin/menus/9');
    expect(deleteInit.method).toBe('DELETE');
    expect(deleteInit.body).toBeUndefined();
    expect(new Headers(deleteInit.headers).get('X-Expected-Fingerprint')).toBe(fingerprint);
  });
  it('expresses clear as clear=true instead of an empty API id list', async () => {
    const fetch = vi.fn().mockImplementation(async () => response(200, '', null));
    vi.stubGlobal('fetch', fetch);
    await api.menus.clearApis(4, { clear: true, expected_fingerprint: fingerprint });
    const [path, init] = fetch.mock.calls[0];
    expect(path).toBe('/api/admin/menus/4/apis');
    expect(JSON.parse(init.body)).toEqual({ clear: true, expected_fingerprint: fingerprint });
  });
  it('keeps the API fingerprint in the delete header and update body', async () => {
    const fetch = vi.fn().mockImplementation(async () => response(200, '', { id: 3 }));
    vi.stubGlobal('fetch', fetch);
    await api.apis.update(3, { name: 'Renamed', need_audit: 0, expected_fingerprint: fingerprint });
    await api.apis.delete(3, fingerprint);
    const [, updateInit] = fetch.mock.calls[0];
    expect(JSON.parse(updateInit.body)).toEqual({ name: 'Renamed', need_audit: 0, expected_fingerprint: fingerprint });
    const [, deleteInit] = fetch.mock.calls[1];
    expect(new Headers(deleteInit.headers).get('X-Expected-Fingerprint')).toBe(fingerprint);
    expect(updateInit.signal).toBeUndefined();
  });
  it('maps stale fingerprints to the conflict codes the page uses for blocking retry', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(response(409, 'NAV_CONFLICT', null, 'navigation resource conflicts'))
      .mockResolvedValueOnce(response(409, 'API_META_CONFLICT', null, 'api metadata conflicts')));
    const menuError = await api.menus.update(9, { name: 'x', expected_fingerprint: fingerprint }).catch(value => value);
    const apiError = await api.apis.update(3, { name: 'x', expected_fingerprint: fingerprint }).catch(value => value);
    expect(menuError).toMatchObject({ status: 409, errorCode: 'NAV_CONFLICT' });
    expect(apiError).toMatchObject({ status: 409, errorCode: 'API_META_CONFLICT' });
  });
});
describe('explicit native resource paths', () => {
  it('parses pre-stream errors on all native paths through the same typed error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response(404, 'STORAGE_OBJECT_NOT_FOUND', null, 'storage object was not found')));
    await expect(api.image('/api/avatars/7')).rejects.toMatchObject({ status: 404, errorCode: 'STORAGE_OBJECT_NOT_FOUND' });
    await expect(api.blob('/api/files/7/download')).rejects.toMatchObject({ message: '文件不存在。' });
    await expect(api.raw('/docs/openapi.json')).rejects.toBeInstanceOf(ApiError);
  });
  it('uses actual captcha base64 and preserves backend data URLs', () => {
    expect(captchaSource('aGVsbG8=')).toBe('data:image/png;base64,aGVsbG8=');
    expect(captchaSource('data:image/png;base64,aGVsbG8=')).toBe('data:image/png;base64,aGVsbG8=');
  });
});
describe('authorization overview client contract', () => {
  it('uses the centralized typed paths and preserves server pagination filters', async () => {
    const payload = { list: [], total: 0, page: 2, size: 10 };
    const fetch = vi.fn().mockResolvedValueOnce(response(200, '', payload)).mockResolvedValueOnce(response(200, '', payload));
    vi.stubGlobal('fetch', fetch);
    await expect(api.authorizationOverview()).resolves.toEqual(payload);
    await expect(api.authorizationRisks({ page: 2, size: 10, kind: 'missing_menu_permission', resource: 'role', keyword: '审查' })).resolves.toEqual(payload);
    expect(fetch).toHaveBeenNthCalledWith(1, '/api/admin/authorization-overview', expect.any(Object));
    expect(fetch).toHaveBeenNthCalledWith(2, '/api/admin/authorization-risks?page=2&size=10&kind=missing_menu_permission&resource=role&keyword=%E5%AE%A1%E6%9F%A5', expect.any(Object));
  });
});
describe('audit log client contract', () => {
  it('loads the requested endpoint and maps validation errors through the shared typed error path', async () => {
    const payload = { list: [], total: 0, page: 1, size: 20 };
    const fetch = vi.fn()
      .mockResolvedValueOnce(response(200, '', payload))
      .mockResolvedValueOnce(response(422, 'AUDIT_VALIDATION_INVALID', { fields: [{ field: 'start_time', error_code: 'AUDIT_VALIDATION_INVALID', message: 'invalid time' }] }, 'audit query validation failed'));
    vi.stubGlobal('fetch', fetch);
    await expect(api.auditLogs.list('/api/admin/login-logs?page=1&size=20')).resolves.toEqual(payload);
    expect(fetch).toHaveBeenNthCalledWith(1, '/api/admin/login-logs?page=1&size=20', expect.any(Object));
    const error = await api.auditLogs.list('/api/admin/audit-logs?start_time=bad').catch(value => value);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 422, errorCode: 'AUDIT_VALIDATION_INVALID' });
    expect(Object.keys(fieldMessages(error))).toEqual(['start_time']);
  });
});
describe('email verification client contract', () => {
  it('uses the authenticated endpoints and payloads', async () => {
    const policy = { enabled: true };
    const fetch = vi.fn()
      .mockResolvedValueOnce(response(200, '', policy))
      .mockResolvedValueOnce(response(200, '', null))
      .mockResolvedValueOnce(response(200, '', { id: 7 }))
      .mockResolvedValueOnce(response(200, '', { id: 7 }));
    vi.stubGlobal('fetch', fetch);
    await expect(api.emailVerification.policy()).resolves.toEqual(policy);
    await expect(api.emailVerification.resend()).resolves.toBeNull();
    await api.emailVerification.confirm('verification-token');
    await api.emailVerification.cancel('new@example.com');
    expect(fetch).toHaveBeenNthCalledWith(1, '/api/user/email-verification-policy', expect.objectContaining({ method: 'GET' }));
    expect(fetch).toHaveBeenNthCalledWith(2, '/api/user/email-verifications', expect.objectContaining({ method: 'POST' }));
    expect(fetch.mock.calls[1][1].body).toBeUndefined();
    expect(JSON.parse(fetch.mock.calls[2][1].body)).toEqual({ token: 'verification-token' });
    expect(JSON.parse(fetch.mock.calls[3][1].body)).toEqual({ pending_email: 'new@example.com' });
  });

  it('maps disabled and stale pending email errors', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(response(503, 'IDENTITY_EMAIL_VERIFICATION_DISABLED', null, 'disabled'))
      .mockResolvedValueOnce(response(409, 'IDENTITY_EMAIL_PENDING_STALE', null, 'stale')));
    const disabled = await api.emailVerification.resend().catch(value => value);
    const stale = await api.emailVerification.cancel('new@example.com').catch(value => value);
    expect(disabled).toMatchObject({ status: 503, errorCode: 'IDENTITY_EMAIL_VERIFICATION_DISABLED' });
    expect(errorMessage(disabled)).toContain('未启用邮件服务');
    expect(stale).toMatchObject({ status: 409, errorCode: 'IDENTITY_EMAIL_PENDING_STALE' });
    expect(errorMessage(stale)).toContain('待验证邮箱已变化');
  });
});
