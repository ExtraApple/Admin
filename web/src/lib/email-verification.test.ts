// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest';
import { clearEmailTokenHash, emailTokenFromHash } from './email-verification';

const token = 'A'.repeat(43);

afterEach(() => {
  window.history.replaceState(null, '', '/profile');
});

describe('email verification hash token', () => {
  it('accepts only a 43 character base64url token', () => {
    expect(emailTokenFromHash(`#email_token=${token}`)).toBe(token);
    expect(emailTokenFromHash(`#source=mail&email_token=${token}`)).toBe(token);
    expect(emailTokenFromHash('#email_token=short')).toBeNull();
    expect(emailTokenFromHash(`#email_token=${'A'.repeat(42)}`)).toBeNull();
    expect(emailTokenFromHash(`#email_token=${'A'.repeat(44)}`)).toBeNull();
    expect(emailTokenFromHash(`#email_token=${'A'.repeat(42)}+`)).toBeNull();
    expect(emailTokenFromHash('')).toBeNull();
  });

  it('clears the fragment without changing path or query', () => {
    window.history.replaceState(null, '', `/profile?source=mail#email_token=${token}`);
    clearEmailTokenHash();
    expect(window.location.pathname).toBe('/profile');
    expect(window.location.search).toBe('?source=mail');
    expect(window.location.hash).toBe('');
  });
});
