import { describe, expect, it } from 'vitest';
import {
  auditEndpoint,
  auditLogPath,
  defaultAuditRange,
  encodeAuditLogQuery,
  formatAuditTime,
  fromDateTimeLocal,
  parseAuditLogQuery,
  presetAuditRange,
  toDateTimeLocal,
  validateAuditRange,
} from './audit-query';
import type { AuditLogQueryState } from './models';

const now = new Date(2026, 9, 30, 12, 0, 0);

const query: AuditLogQueryState = {
  type: 'login',
  page: 2,
  size: 50,
  start: '2026-10-01 00:00:00',
  end: '2026-10-02 00:00:00',
  method: 'GET',
  status: '403',
  path: '/api/admin',
  username: 'alice',
  userId: '7',
};

describe('audit URL query state', () => {
  it('defaults to the last 24 hours and parses supported filters', () => {
    expect(defaultAuditRange(now)).toEqual({ start: '2026-10-29 12:00:00', end: '2026-10-30 12:00:00' });
    expect(presetAuditRange(168, now)).toEqual({ start: '2026-10-23 12:00:00', end: '2026-10-30 12:00:00' });
    expect(parseAuditLogQuery(new URLSearchParams({
      type: 'login', page: '2', size: '50', start: query.start, end: query.end,
      method: 'get', status: '403', path: '/api/admin', username: 'alice', user_id: '7',
    }), now)).toEqual(query);
  });

  it('round-trips URL state and drops unsupported values', () => {
    expect(parseAuditLogQuery(encodeAuditLogQuery(query), now)).toEqual(query);
    expect(parseAuditLogQuery(new URLSearchParams({
      type: 'bogus', page: '0', size: '100', method: 'TRACE', status: '99', user_id: '-1',
    }), now)).toMatchObject({ type: 'all', page: 1, size: 20, method: '', status: '', userId: '' });
  });

  it('validates the local time range and input conversions', () => {
    expect(validateAuditRange('2026-10-01 00:00:00', '2026-10-02 00:00:00')).toBeNull();
    expect(validateAuditRange('2026-10-02 00:00:00', '2026-10-01 00:00:00')).toContain('晚于');
    expect(validateAuditRange('2026-01-01 00:00:00', '2026-04-02 00:00:00')).toContain('90');
    expect(validateAuditRange('bad', '2026-10-02 00:00:00')).toContain('开始');
    expect(toDateTimeLocal('2026-10-30 00:00:00')).toBe('2026-10-30T00:00:00');
    expect(fromDateTimeLocal('2026-10-30T00:00')).toBe('2026-10-30 00:00:00');
    expect(fromDateTimeLocal('bad')).toBe('');
    expect(formatAuditTime(now)).toBe('2026-10-30 12:00:00');
  });

  it('maps each tab to its endpoint and sends server query parameters', () => {
    expect(auditEndpoint('all')).toBe('/api/admin/audit-logs');
    expect(auditEndpoint('login')).toBe('/api/admin/login-logs');
    expect(auditEndpoint('operation')).toBe('/api/admin/operation-logs');
    expect(auditEndpoint('permission')).toBe('/api/admin/permission-logs');
    expect(auditEndpoint('data_access')).toBe('/api/admin/data-access-logs');
    const url = new URL(auditLogPath(query), 'http://localhost');
    expect(url.pathname).toBe('/api/admin/login-logs');
    expect(url.searchParams.get('page')).toBe('2');
    expect(url.searchParams.get('size')).toBe('50');
    expect(url.searchParams.get('user_id')).toBe('7');
    expect(url.searchParams.get('username')).toBe('alice');
    expect(url.searchParams.get('method')).toBe('GET');
    expect(url.searchParams.get('path')).toBe('/api/admin');
    expect(url.searchParams.get('status')).toBe('403');
    expect(url.searchParams.get('start_time')).toBe('2026-10-01 00:00:00');
    expect(url.searchParams.get('end_time')).toBe('2026-10-02 00:00:00');
  });
});
