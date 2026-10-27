export const passwordRequirementsText = '至少 6 个字符，包含大写、小写、数字、标点或符号中的至少三类，且不超过 72 个 UTF-8 字节。';

const utf8 = new TextEncoder();
export function validateNewPassword(value: string): string | null {
  if ([...value].length < 6) return '密码至少需要 6 个字符。';
  if (utf8.encode(value).byteLength > 72) return '密码不能超过 72 个 UTF-8 字节。';
  const categories = [/[\p{Lu}]/u, /[\p{Ll}]/u, /[\p{Nd}]/u, /[\p{P}\p{S}]/u];
  if (categories.filter(category => category.test(value)).length < 3) return '密码需要包含大写、小写、数字、标点或符号中的至少三类。';
  return null;
}
