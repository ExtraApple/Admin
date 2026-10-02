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
