import { ApiError } from './api';
export function lockDeadline(error: unknown, now = Date.now()): number {
  return error instanceof ApiError && error.status === 429 ? now + error.retryAfterSeconds * 1000 : 0;
}
export function lockSeconds(deadline: number, now = Date.now()): number { return Math.max(0, Math.ceil((deadline - now) / 1000)); }
export function captchaSource(value: string): string { return value.startsWith('data:image/') ? value : `data:image/png;base64,${value}`; }
