import type { AuditLogQueryState, AuditLogType } from './models';

export const AUDIT_PAGE_SIZES = [10, 20, 50] as const;
export const AUDIT_MAX_RANGE_DAYS = 90;
export const AUDIT_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const;
export const AUDIT_TAB_ORDER: readonly AuditLogType[] = ['all', 'login', 'operation', 'permission', 'data_access'];
export const AUDIT_TAB_LABELS: Record<AuditLogType, string> = {
  all: '全部',
  login: '登录',
  operation: '操作',
  permission: '权限变更',
  data_access: '数据访问',
};

const TIME_PATTERN = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2}):(\d{2})$/;
const LOCAL_INPUT_PATTERN = /^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})(?::(\d{2}))?$/;
const DAY_MS = 24 * 60 * 60 * 1000;

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

export function formatAuditTime(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

export function parseAuditTime(value: string): Date | null {
  const match = TIME_PATTERN.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  const date = new Date(year, month - 1, day, hour, minute, second, 0);
  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day
    || date.getHours() !== hour || date.getMinutes() !== minute || date.getSeconds() !== second) return null;
  return date;
}

export function defaultAuditRange(now: Date = new Date()): { start: string; end: string } {
  return { start: formatAuditTime(new Date(now.getTime() - DAY_MS)), end: formatAuditTime(now) };
}

export function presetAuditRange(hours: number, now: Date = new Date()): { start: string; end: string } {
  return { start: formatAuditTime(new Date(now.getTime() - hours * 60 * 60 * 1000)), end: formatAuditTime(now) };
}

export function validateAuditRange(start: string, end: string): string | null {
  const startDate = parseAuditTime(start);
  if (!startDate) return '开始时间格式无效。';
  const endDate = parseAuditTime(end);
  if (!endDate) return '结束时间格式无效。';
  if (startDate.getTime() > endDate.getTime()) return '开始时间不能晚于结束时间。';
  if (endDate.getTime() - startDate.getTime() > AUDIT_MAX_RANGE_DAYS * DAY_MS) return `单次查询范围不能超过 ${AUDIT_MAX_RANGE_DAYS} 天。`;
  return null;
}

export function toDateTimeLocal(value: string): string {
  const date = parseAuditTime(value);
  if (!date) return '';
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

export function fromDateTimeLocal(value: string): string {
  const match = LOCAL_INPUT_PATTERN.exec(value);
  if (!match) return '';
  const canonical = `${match[1]} ${match[2]}:${match[3] ?? '00'}`;
  return parseAuditTime(canonical) ? canonical : '';
}

export function auditEndpoint(type: AuditLogType): string {
  switch (type) {
    case 'login': return '/api/admin/login-logs';
    case 'operation': return '/api/admin/operation-logs';
    case 'permission': return '/api/admin/permission-logs';
    case 'data_access': return '/api/admin/data-access-logs';
    default: return '/api/admin/audit-logs';
  }
}

export function parseAuditLogQuery(search: string | URLSearchParams, now: Date = new Date()): AuditLogQueryState {
  const params = typeof search === 'string'
    ? new URLSearchParams(search.startsWith('?') ? search.slice(1) : search)
    : new URLSearchParams(search);
  const rawType = params.get('type') as AuditLogType | null;
  const type = rawType && AUDIT_TAB_ORDER.includes(rawType) ? rawType : 'all';
  const rawPage = Number(params.get('page'));
  const page = Number.isSafeInteger(rawPage) && rawPage > 0 ? rawPage : 1;
  const rawSize = Number(params.get('size'));
  const size = AUDIT_PAGE_SIZES.includes(rawSize as (typeof AUDIT_PAGE_SIZES)[number]) ? rawSize : 20;
  const rawMethod = (params.get('method') ?? '').toUpperCase();
  const method = AUDIT_METHODS.includes(rawMethod as (typeof AUDIT_METHODS)[number]) ? rawMethod : '';
  const rawStatus = params.get('status') ?? '';
  const status = /^[1-5]\d\d$/.test(rawStatus) ? rawStatus : '';
  const rawUserId = params.get('user_id') ?? '';
  const userId = /^[1-9]\d*$/.test(rawUserId) ? rawUserId : '';
  const start = params.get('start') ?? '';
  const end = params.get('end') ?? '';
  const range = start && end ? { start, end } : defaultAuditRange(now);
  return {
    type,
    page,
    size,
    start: range.start,
    end: range.end,
    method,
    status,
    path: params.get('path') ?? '',
    username: params.get('username') ?? '',
    userId,
  };
}

export function encodeAuditLogQuery(query: AuditLogQueryState): string {
  const params = new URLSearchParams();
  params.set('type', query.type);
  params.set('page', String(query.page));
  params.set('size', String(query.size));
  params.set('start', query.start);
  params.set('end', query.end);
  if (query.method) params.set('method', query.method);
  if (query.status) params.set('status', query.status);
  if (query.path.trim()) params.set('path', query.path.trim());
  if (query.username.trim()) params.set('username', query.username.trim());
  if (query.userId) params.set('user_id', query.userId);
  return params.toString();
}

export function auditLogPath(query: AuditLogQueryState): string {
  const params = new URLSearchParams();
  params.set('page', String(query.page));
  params.set('size', String(query.size));
  if (query.userId) params.set('user_id', query.userId);
  if (query.username.trim()) params.set('username', query.username.trim());
  if (query.method) params.set('method', query.method);
  if (query.path.trim()) params.set('path', query.path.trim());
  if (query.status) params.set('status', query.status);
  params.set('start_time', query.start);
  params.set('end_time', query.end);
  return `${auditEndpoint(query.type)}?${params.toString()}`;
}
