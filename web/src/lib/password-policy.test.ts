import { describe, expect, it } from 'vitest';
import { validateNewPassword } from './password-policy';

describe('new-password input boundaries', () => {
  it('accepts exactly 72 UTF-8 bytes and rejects 73 without a character-count shortcut', () => {
    expect(validateNewPassword(`Aa1!${'x'.repeat(68)}`)).toBeNull();
    expect(validateNewPassword(`Aa1!${'x'.repeat(69)}`)).not.toBeNull();
    expect(validateNewPassword(`Aa1!${'界'.repeat(22)}ab`)).toBeNull();
    expect(validateNewPassword(`Aa1!${'界'.repeat(23)}`)).not.toBeNull();
    expect(validateNewPassword(`Aa1!${'😀'.repeat(17)}`)).toBeNull();
    expect(validateNewPassword(`Aa1!${'😀'.repeat(17)}a`)).not.toBeNull();
  });

  it('counts Unicode code points for the effective HTTP six-character minimum', () => {
    expect(validateNewPassword('Aa1😀😀')).not.toBeNull();
    expect(validateNewPassword('Aa1😀😀😀')).toBeNull();
  });

  it('requires three Unicode letter, decimal digit, and punctuation-or-symbol categories', () => {
    expect(validateNewPassword('Ää١!!!')).toBeNull();
    expect(validateNewPassword('Ａａ１xyz')).toBeNull();
    expect(validateNewPassword('Ää    ')).not.toBeNull();
    expect(validateNewPassword('Ää一二三四')).not.toBeNull();
    expect(validateNewPassword('abcdef')).not.toBeNull();
    expect(validateNewPassword('123456')).not.toBeNull();
  });

  it('does not trim password whitespace or normalize full-width input', () => {
    expect(validateNewPassword(' Aa1! ')).toBeNull();
    expect(validateNewPassword(`Aa1!${' '.repeat(68)}`)).toBeNull();
    expect(validateNewPassword(`Aa1!${' '.repeat(69)}`)).not.toBeNull();
    expect(validateNewPassword('Ａａ１'.repeat(8))).toBeNull();
    expect(validateNewPassword('Ａａ１'.repeat(9))).not.toBeNull();
  });
});
