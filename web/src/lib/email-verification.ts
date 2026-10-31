const EMAIL_TOKEN_PATTERN = /^[A-Za-z0-9_-]{43}$/;
const EMAIL_ADDRESS_PATTERN = /^[^\s@]+@[^\s@]+$/;

export function isEmailAddress(value: string): boolean {
  return EMAIL_ADDRESS_PATTERN.test(value.trim());
}

export function isEmailToken(value: string): boolean {
  return EMAIL_TOKEN_PATTERN.test(value.trim());
}

export function emailTokenFromHash(hash: string): string | null {
  const value = hash.startsWith('#') ? hash.slice(1) : hash;
  if (!value) return null;
  const token = new URLSearchParams(value).get('email_token') ?? '';
  return EMAIL_TOKEN_PATTERN.test(token) ? token : null;
}

export function clearEmailTokenHash(): void {
  if (typeof window === 'undefined' || !window.location.hash) return;
  const url = `${window.location.pathname}${window.location.search}`;
  window.history.replaceState(window.history.state, '', url);
}
