// @vitest-environment jsdom
import { act, useSyncExternalStore } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { createMemoryRouter, Link, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { api } from '@/lib/api';
import type { UserContext } from '@/lib/models';
import type { SessionSnapshot } from '@/lib/session-state';
import { ProfilePage } from './ProfilePage';

interface FixtureSession extends SessionSnapshot {
  logout(reason?: 'password-changed'): Promise<void>;
  reloadContext(): Promise<UserContext>;
  can(): boolean;
}
// The placeholder is replaced in beforeEach, before any consumer can mount.
const sessionFixture = vi.hoisted(() => ({ current: {} as FixtureSession, listeners: new Set<() => void>() }));
vi.mock('@/lib/session', () => {
  return { useSession: () => useSyncExternalStore((listener: () => void) => { sessionFixture.listeners.add(listener); return () => sessionFixture.listeners.delete(listener); }, () => sessionFixture.current) };
});
vi.mock('@/lib/session-state', () => ({ sessionState: { getSnapshot: () => sessionFixture.current } }));

const original: UserContext = {
  user: { id: 7, username: 'operator', nickname: '已保存昵称', avatar: '/api/avatars/7', email: 'read@example.test', role: 'user', status: 1, pending_email: '', email_verified: true },
  roles: ['reader'], permissions: [], menus: [],
};
function envelope(data: unknown, status = 200, errorCode = '', fields?: unknown) {
  return new Response(JSON.stringify({ code: status, error_code: errorCode, msg: status === 200 ? 'success' : 'Request failed.', data: fields ?? data }), { status, headers: { 'Content-Type': 'application/json' } });
}
function deferred<T>() {
  // This project targets ES2022, which does not provide Promise.withResolvers.
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}

describe('rendered profile self-service', () => {
  let container: HTMLDivElement;
  let root: Root;
  let disposeRouter: (() => void) | undefined;
  let navigate: (to: string | number) => Promise<void>;
  let pathname: () => string;
  let saved: UserContext;
  let fetchMock: Mock<(path: string, init?: RequestInit) => Promise<Response>>;
  let logout: Mock<(reason?: 'password-changed') => Promise<void>>;
  let write: ((path: string, init: RequestInit) => Promise<Response>) | null;
  let policy: unknown;
  let emailPolicyEnabled: boolean;
  let contextFailure: boolean;
  let policyUnavailable: boolean;
  function publish(patch: Partial<FixtureSession>) {
    sessionFixture.current = { ...sessionFixture.current, ...patch };
    sessionFixture.listeners.forEach(listener => listener());
  }
  beforeEach(() => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
    vi.stubGlobal('URL', class extends URL { static createObjectURL = vi.fn(() => 'blob:profile-test'); static revokeObjectURL = vi.fn(); });
    saved = structuredClone(original); write = null; policy = { max_bytes: 4096 }; emailPolicyEnabled = true; contextFailure = false; policyUnavailable = false;
    logout = vi.fn(async (_reason?: 'password-changed') => {});
    sessionFixture.current = {
      phase: 'authenticated', context: structuredClone(original), sessionId: 'session-one', userRevision: 0,
      notice: null, logout, can: () => false,
      reloadContext: async () => {
        const context = await api.get<UserContext>('/api/user/context');
        publish({ context, userRevision: sessionFixture.current.userRevision + 1 });
        return context;
      },
    };
    fetchMock = vi.fn(async (path: string, init: RequestInit = {}) => {
      if (init.method === 'PUT' || init.method === 'POST' || init.method === 'DELETE') {
        if (write) return write(path, init);
        if (path === '/api/user/info') {
          const body = JSON.parse(init.body as string);
          if (body.email) {
            if (saved.user.email_verified) saved.user.pending_email = body.email;
            else {
              saved.user.email = body.email;
              saved.user.pending_email = '';
              saved.user.email_verified = false;
            }
          }
          if (body.nickname !== undefined) saved.user.nickname = body.nickname;
          return envelope(saved.user);
        }
        if (path === '/api/user/email-verifications') return envelope(null);
        if (path === '/api/user/email-verifications/confirm') {
          if (saved.user.pending_email) saved.user.email = saved.user.pending_email;
          saved.user.pending_email = '';
          saved.user.email_verified = true;
          return envelope(saved.user);
        }
        if (path === '/api/user/email-verifications/cancel') {
          saved.user.pending_email = '';
          return envelope(saved.user);
        }
        return envelope(saved.user);
      }
      if (path === '/api/user/info') return envelope(saved.user);
      if (path === '/api/user/email-verification-policy') return envelope({ enabled: emailPolicyEnabled });
      if (path === '/api/user/context') return contextFailure ? envelope(null, 500, 'IDENTITY_INTERNAL_ERROR') : envelope(saved);
      if (path === '/api/user/avatar-upload-policy') return policyUnavailable ? envelope(null, 500, 'IDENTITY_INTERNAL_ERROR') : envelope(policy);
      if (path.startsWith('/api/avatars/')) return new Response(new Blob(['image'], { type: 'image/png' }));
      throw new Error(`Unexpected request ${path}`);
    });
    vi.stubGlobal('fetch', fetchMock);
    container = document.createElement('div'); document.body.append(container); root = createRoot(container);
  });
  afterEach(() => {
    act(() => root.unmount()); disposeRouter?.(); container.remove();
    sessionFixture.listeners.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks();
  });
  async function mount(entry = '/profile') {
    const router = createMemoryRouter([
      { path: '/profile', element: <><Link to="/away">其他页面</Link><Link to="/logout">退出登录</Link><ProfilePage /></> },
      { path: '/away', element: <p>已离开资料页</p> },
      { path: '/logout', element: <p>退出入口</p> },
      { path: '/login', element: <p>登录入口</p> },
    ], { initialEntries: ['/away', entry] });
    disposeRouter = () => router.dispose();
    navigate = to => typeof to === 'number' ? router.navigate(to) : router.navigate(to);
    pathname = () => router.state.location.pathname;
    await act(async () => root.render(<RouterProvider router={router} />));
  }
  async function settle() {
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
  }
  function control(text: string) {
    const button = [...document.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent === text);
    if (!button) throw new Error(`Missing button ${text}`);
    return button;
  }
  async function click(text: string) { await act(async () => control(text).click()); }
  function input(id: string, value: string) {
    const field = container.querySelector<HTMLInputElement>(`#${id}`);
    if (!field) throw new Error(`Missing input ${id}`);
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(field, value);
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
  }
  function writes() { return fetchMock.mock.calls.filter(([, init]) => ['PUT', 'POST', 'DELETE'].includes(init?.method ?? '')); }
  function selectFile(file: File) {
    const field = container.querySelector<HTMLInputElement>('#profile-file');
    if (!field) throw new Error('Missing file input');
    act(() => {
      Object.defineProperty(field, 'files', { configurable: true, value: [file] });
      field.dispatchEvent(new Event('change', { bubbles: true }));
    });
  }
  function fillPasswords(newPassword = 'NewPass1!', oldPassword = 'OldPass1!') {
    input('profile-old_password', oldPassword);
    input('profile-new_password', newPassword);
    input('profile-confirm_password', newPassword);
  }

  it('counts Unicode nickname code points, rejects blank/101, and submits only the raw nickname', async () => {
    await mount(); await click('编辑昵称');
    input('profile-nickname', ' \t '); await click('保存昵称');
    expect(container.textContent).toContain('昵称不能为空或全为空白'); expect(writes()).toHaveLength(0);
    input('profile-nickname', '😀'.repeat(101)); await click('保存昵称');
    expect(container.textContent).toContain('昵称最多 100 个 Unicode 字符'); expect(writes()).toHaveLength(0);
    const nickname = ` Ab${'😀'.repeat(95)}  `;
    input('profile-nickname', nickname); await click('保存昵称');
    expect(writes()).toHaveLength(1);
    expect(JSON.parse(writes()[0][1]!.body as string)).toEqual({ nickname });
    expect(container.querySelector('#profile-nickname')).toBeNull();
    expect(container.textContent).toContain(nickname);
  });

  it('requires real dirtiness for switches, preserves canceled drafts, and clears only after confirmed switch', async () => {
    await mount(); await click('编辑昵称'); await click('修改密码');
    expect(document.querySelector('[role="alertdialog"]')).toBeNull();
    fillPasswords(); await click('编辑头像');
    expect(document.querySelector('[role="alertdialog"]')).not.toBeNull();
    await click('取消');
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('NewPass1!');
    expect(container.querySelector('#profile-avatar-editor')).toBeNull();
    await click('编辑头像'); await click('放弃修改并切换');
    expect(container.querySelector('#profile-password-editor')).toBeNull();
    expect(container.querySelector('#profile-avatar-editor')).not.toBeNull();
    await click('修改密码');
    expect(container.querySelector<HTMLInputElement>('#profile-old_password')?.value).toBe('');
    expect(writes()).toHaveLength(0);
  });

  it.each(['navigation', 'back', 'logout'] as const)('preserves passwords when %s is canceled and clears on actual departure', async action => {
    await mount(); await click('修改密码'); fillPasswords();
    await act(async () => {
      if (action === 'back') await navigate(-1);
      else container.querySelector<HTMLAnchorElement>(`a[href="${action === 'logout' ? '/logout' : '/away'}"]`)!.click();
    });
    await click('取消');
    expect(pathname()).toBe('/profile');
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('NewPass1!');
    expect(logout).not.toHaveBeenCalled();
    const unloading = new Event('beforeunload', { cancelable: true });
    act(() => { window.dispatchEvent(unloading); });
    expect(unloading.defaultPrevented).toBe(true);
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('NewPass1!');
    await act(async () => container.querySelector<HTMLAnchorElement>('a[href="/away"]')!.click());
    await click('放弃修改并离开');
    expect(pathname()).toBe('/away');
    await act(async () => navigate('/profile')); await click('修改密码');
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('');
    expect(writes()).toHaveLength(0);
  });

  it.each([undefined, null, '4096', 0, -1, 2.5, Number.MAX_SAFE_INTEGER + 1])('isolates invalid upload max_bytes=%s and retries the real policy without disabling reset', async maxBytes => {
    policy = { max_bytes: maxBytes };
    await mount(); await click('编辑头像');
    expect(container.querySelector<HTMLInputElement>('#profile-file')?.disabled).toBe(true);
    expect(control('恢复默认头像').disabled).toBe(false);
    expect(control('编辑昵称').disabled).toBe(false);
    expect(control('修改密码').disabled).toBe(false);
    policy = { max_bytes: 4096 };
    await click('重试读取上传规则');
    expect(container.querySelector<HTMLInputElement>('#profile-file')?.disabled).toBe(false);
    expect(container.textContent).toContain('4096 字节');
    expect(writes()).toHaveLength(0);
  });

  it('previews a boundary-sized file without writing, preserves canceled reset, and uploads real multipart only on Save', async () => {
    await mount(); await click('编辑头像');
    selectFile(new File([new Uint8Array(4097)], 'too-large.png', { type: 'image/png' }));
    expect(container.textContent).toContain('文件超过当前 4096 字节上限');
    expect(container.querySelector('img[alt="待保存的头像预览"]')).toBeNull();
    const file = new File([new Uint8Array(4096)], 'boundary.webp', { type: 'image/webp' });
    selectFile(file);
    expect(container.querySelector('img[alt="待保存的头像预览"]')).not.toBeNull();
    expect(writes()).toHaveLength(0);
    await click('恢复默认头像'); await click('取消');
    expect(container.querySelector('img[alt="待保存的头像预览"]')).not.toBeNull();
    expect(writes()).toHaveLength(0);
    await click('保存头像');
    expect(writes()).toHaveLength(1);
    const body = writes()[0][1]!.body;
    expect(body).toBeInstanceOf(FormData);
    expect([...((body as FormData).keys())]).toEqual(['file']);
    expect((body as FormData).get('file')).toBe(file);
    expect(new Headers(writes()[0][1]!.headers).has('Content-Type')).toBe(false);
    expect(container.querySelector('img[alt="待保存的头像预览"]')).toBeNull();
    expect(URL.revokeObjectURL).toHaveBeenCalled();
  });

  it('resets immediately after confirmation and retains preview on unconfirmed reset', async () => {
    await mount(); await click('编辑头像');
    selectFile(new File(['image'], 'avatar.png', { type: 'image/png' }));
    write = async () => envelope(null, 500, 'IDENTITY_INTERNAL_ERROR');
    await click('恢复默认头像'); await click('确认恢复默认头像');
    expect(writes()).toHaveLength(1);
    expect(writes()[0][1]!.method).toBe('DELETE');
    expect(writes()[0][1]!.body).toBeUndefined();
    expect(container.textContent).toContain('保存结果尚未确认');
    expect(container.querySelector('img[alt="待保存的头像预览"]')).not.toBeNull();
    await click('取消头像编辑'); await click('编辑头像');
    write = null;
    await click('恢复默认头像'); await click('确认恢复默认头像');
    expect(container.textContent).toContain('默认头像已恢复');
    expect(writes()).toHaveLength(2);
  });

  it('keeps confirmed write success separate from display reload failure and retries only reads', async () => {
    await mount(); await click('编辑昵称'); input('profile-nickname', '新昵称');
    contextFailure = true;
    await click('保存昵称');
    expect(container.textContent).toContain('昵称已保存');
    expect(container.textContent).toContain('已保存，显示更新未完成');
    expect(container.querySelector('#profile-nickname')).toBeNull();
    expect(writes()).toHaveLength(1);
    contextFailure = false; await click('重新读取显示');
    expect(container.textContent).not.toContain('已保存，显示更新未完成');
    expect(writes()).toHaveLength(1);
  });

  it('keeps unknown-write drafts during read-only reconciliation without replaying the write', async () => {
    await mount(); await click('编辑昵称'); input('profile-nickname', '本地草稿');
    write = async () => envelope(null, 500, 'IDENTITY_INTERNAL_ERROR');
    await click('保存昵称');
    expect(container.textContent).toContain('保存结果尚未确认');
    await click('重新读取已保存资料核对');
    expect(container.querySelector<HTMLInputElement>('#profile-nickname')?.value).toBe('本地草稿');
    expect(container.textContent).toContain('已保存昵称');
    expect(writes()).toHaveLength(1);
  });

  it('blocks duplicate writes and conflicting switches while a write is unresolved', async () => {
    const pending = deferred<Response>(); write = () => pending.promise;
    await mount(); await click('编辑昵称'); input('profile-nickname', '新昵称');
    await click('保存昵称'); await click('正在保存…'); await click('修改密码');
    expect(writes()).toHaveLength(1);
    expect(control('修改密码').disabled).toBe(true);
    expect(container.querySelector('#profile-password-editor')).toBeNull();
    await act(async () => pending.resolve(envelope({ ...saved.user, nickname: '新昵称' })));
  });

  it('localizes backend password fields and preserves all inputs after a definite rejection', async () => {
    await mount(); await click('修改密码'); fillPasswords();
    write = async () => envelope(null, 422, 'IDENTITY_VALIDATION_INVALID', { fields: [{ field: 'password', error_code: 'IDENTITY_PASSWORD_INVALID', message: 'password is invalid' }] });
    await click('修改密码并退出');
    expect(container.querySelector('#profile-new_password')?.getAttribute('aria-invalid')).toBe('true');
    expect(container.querySelector('#profile-new_password-errors')?.textContent).toContain('密码');
    expect(container.querySelector<HTMLInputElement>('#profile-old_password')?.value).toBe('OldPass1!');
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('NewPass1!');
    expect(logout).not.toHaveBeenCalled();
  });

  it('rejects 73 UTF-8 bytes, accepts 72, does not constrain the old password, and logs out only on confirmed success', async () => {
    await mount(); await click('修改密码');
    fillPasswords(`Aa1${'界'.repeat(23)}a`, 'o'.repeat(90)); await click('修改密码并退出');
    expect(writes()).toHaveLength(0); expect(container.textContent).toContain('72');
    const accepted = `Aa1${'界'.repeat(23)}`;
    fillPasswords(accepted, 'o'.repeat(90)); await click('修改密码并退出');
    expect(writes()).toHaveLength(1);
    expect(JSON.parse(writes()[0][1]!.body as string)).toEqual({ old_password: 'o'.repeat(90), new_password: accepted, confirm_password: accepted });
    expect(logout).toHaveBeenCalledExactlyOnceWith('password-changed');
    expect(container.querySelector('#profile-password-editor')).toBeNull();
  });

  it('retains passwords during a valid-access notice but clears them on suspension and never restores them', async () => {
    await mount(); await click('修改密码'); fillPasswords();
    act(() => publish({ notice: '续期暂不可用，Access 仍有效。' }));
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('NewPass1!');
    await act(async () => publish({ phase: 'recovery-required' }));
    expect(container.querySelector<HTMLInputElement>('#profile-old_password')?.value).toBe('');
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('');
    expect(container.querySelector<HTMLInputElement>('#profile-confirm_password')?.value).toBe('');
    await act(async () => publish({ phase: 'authenticated' }));
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('');
    expect(writes()).toHaveLength(0);
  });

  it.each(['nickname', 'avatar'] as const)('retains the %s draft across same-SID recovery but not a new SID', async kind => {
    await mount(); await click(kind === 'nickname' ? '编辑昵称' : '编辑头像');
    if (kind === 'nickname') input('profile-nickname', '恢复后保留的草稿');
    else selectFile(new File(['image'], 'draft.png', { type: 'image/png' }));
    await act(async () => publish({ phase: 'recovery-required' }));
    await act(async () => publish({ phase: 'authenticated' }));
    if (kind === 'nickname') expect(container.querySelector<HTMLInputElement>('#profile-nickname')?.value).toBe('恢复后保留的草稿');
    else expect(container.querySelector('img[alt="待保存的头像预览"]')).not.toBeNull();
    await act(async () => publish({ sessionId: 'new-same-user-session' }));
    expect(container.querySelector('#profile-nickname')).toBeNull();
    expect(container.querySelector('img[alt="待保存的头像预览"]')).toBeNull();
    expect(writes()).toHaveLength(0);
  });

  it.each([false, true])('discards late password 200 after %s actor change without logging out the new session', async differentUser => {
    const pending = deferred<Response>(); write = () => pending.promise;
    await mount(); await click('修改密码'); fillPasswords(); await click('修改密码并退出');
    if (differentUser) saved = { ...saved, user: { ...saved.user, id: 8, username: 'new-user', nickname: '新身份' } };
    await act(async () => publish({ sessionId: 'new-login-session', context: saved }));
    await click('修改密码'); fillPasswords('AnotherPass2!');
    await act(async () => pending.resolve(envelope(null)));
    expect(logout).not.toHaveBeenCalled();
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('AnotherPass2!');
    expect(container.textContent).not.toContain('密码已修改');
  });

  it('does not report unknown password writes as success or trigger logout/replay', async () => {
    await mount(); await click('修改密码'); fillPasswords();
    write = async () => { throw new TypeError('Network interrupted'); };
    await click('修改密码并退出');
    expect(container.textContent).toContain('密码修改结果尚未确认');
    expect(container.querySelector('a[href="/login"]')).not.toBeNull();
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('NewPass1!');
    expect(logout).not.toHaveBeenCalled(); expect(writes()).toHaveLength(1);
  });

  it('still restores default and permits other editors after the real policy request fails', async () => {
    policyUnavailable = true;
    await mount(); await click('编辑头像');
    expect(container.querySelector<HTMLInputElement>('#profile-file')?.disabled).toBe(true);
    expect(control('编辑昵称').disabled).toBe(false);
    expect(control('修改密码').disabled).toBe(false);
    await click('恢复默认头像'); await click('确认恢复默认头像');
    expect(container.textContent).toContain('默认头像已恢复');
    expect(writes()).toHaveLength(1);
  });

  it('discards old-actor user and policy reads instead of overwriting a new identity or its draft', async () => {
    const oldUser = deferred<Response>();
    const oldPolicy = deferred<Response>();
    const fallback = fetchMock.getMockImplementation()!;
    let firstUser = true; let firstPolicy = true;
    fetchMock.mockImplementation((path, init) => {
      if (path === '/api/user/info' && firstUser) { firstUser = false; return oldUser.promise; }
      if (path === '/api/user/avatar-upload-policy' && firstPolicy) { firstPolicy = false; return oldPolicy.promise; }
      return fallback(path, init);
    });
    await mount();
    saved = { ...saved, user: { ...saved.user, id: 8, username: 'new-user', nickname: '新身份' } };
    await act(async () => publish({ sessionId: 'new-login-session', context: saved }));
    await click('编辑昵称'); input('profile-nickname', '新身份的草稿');
    await act(async () => {
      oldUser.resolve(envelope(original.user));
      oldPolicy.resolve(envelope({ max_bytes: 1 }));
    });
    expect(container.querySelector<HTMLInputElement>('#profile-nickname')?.value).toBe('新身份的草稿');
    expect(container.textContent).toContain('新身份');
    expect(container.textContent).not.toContain('已保存昵称');
    await click('取消昵称编辑'); await click('编辑头像');
    expect(container.textContent).toContain('4096 字节');
    expect(writes()).toHaveLength(0);
  });

  it('retains a selected file on 413 and rechecks it against a manually refreshed policy', async () => {
    await mount(); await click('编辑头像');
    selectFile(new File([new Uint8Array(4096)], 'avatar.png', { type: 'image/png' }));
    write = async () => envelope(null, 413, 'FILE_TOO_LARGE');
    await click('保存头像');
    expect(container.querySelector('img[alt="待保存的头像预览"]')).not.toBeNull();
    expect(container.textContent).toContain('文件超过大小限制');
    policy = { max_bytes: 1024 };
    await click('重新读取上传规则'); await click('保存头像');
    expect(container.textContent).toContain('当前 1024 字节上限');
    expect(writes()).toHaveLength(1);
  });

  it('reports controlled-image failure after a confirmed save without relabeling the write or replaying it', async () => {
    await mount(); await click('编辑昵称'); input('profile-nickname', '新昵称'); await click('保存昵称');
    const avatar = container.querySelector<HTMLImageElement>('img[alt="新昵称的头像"]');
    expect(avatar).not.toBeNull();
    act(() => { avatar!.dispatchEvent(new Event('error')); });
    expect(container.textContent).toContain('昵称已保存');
    expect(container.textContent).toContain('已保存，显示更新未完成');
    await click('重新读取显示');
    const retried = container.querySelector<HTMLImageElement>('img[alt="新昵称的头像"]');
    expect(retried).not.toBeNull();
    act(() => { retried!.dispatchEvent(new Event('load')); });
    expect(container.textContent).not.toContain('已保存，显示更新未完成');
    expect(writes()).toHaveLength(1);
  });

  it('discards a password response from the pre-suspension turn even after the same actor recovers', async () => {
    const pending = deferred<Response>(); write = () => pending.promise;
    await mount(); await click('修改密码'); fillPasswords(); await click('修改密码并退出');
    await act(async () => publish({ phase: 'recovery-required' }));
    await act(async () => publish({ phase: 'authenticated' }));
    fillPasswords('RefilledPass2!');
    await act(async () => pending.resolve(envelope(null)));
    expect(logout).not.toHaveBeenCalled();
    expect(container.querySelector<HTMLInputElement>('#profile-new_password')?.value).toBe('RefilledPass2!');
    expect(container.textContent).not.toContain('密码已修改');
  });

  it('submits a verified email change and shows the pending candidate', async () => {
    await mount(); await click('编辑邮箱');
    input('profile-email', 'new@example.test');
    input('profile-current-password', 'secret');
    await click('提交换绑');
    const request = writes().find(([path]) => path === '/api/user/info');
    expect(request).toBeDefined();
    expect(JSON.parse(request?.[1]?.body as string)).toEqual({ email: 'new@example.test', current_password: 'secret' });
    await settle();
    expect(container.textContent).toContain('待验证的新邮箱：new@example.test');
    expect(container.textContent).toContain('换绑请求已提交');
  });

  it('warns and directly replaces an unverified current email', async () => {
    saved.user.email_verified = false;
    await mount(); await click('编辑邮箱');
    expect(container.textContent).toContain('保存后会立即替换当前邮箱');
    input('profile-email', 'changed@example.test');
    input('profile-current-password', 'secret');
    await click('提交换绑');
    await settle();
    expect(container.textContent).toContain('邮箱已更换为未验证状态');
    expect(container.textContent).toContain('changed@example.test');
  });

  it('confirms a pending email token and updates the verification state', async () => {
    saved.user.pending_email = 'candidate@example.test';
    await mount(); await click('验证待验证邮箱');
    input('profile-email-token', 'A'.repeat(43));
    await click('确认验证');
    await settle();
    const request = writes().find(([path]) => path.endsWith('/confirm'));
    expect(request).toBeDefined();
    expect(JSON.parse(request?.[1]?.body as string)).toEqual({ token: 'A'.repeat(43) });
    expect(container.textContent).toContain('邮箱已验证');
    expect(container.textContent).toContain('candidate@example.test');
    expect(container.textContent).not.toContain('待验证的新邮箱');
  });

  it('auto-confirms a fragment token and clears the fragment', async () => {
    saved.user.pending_email = 'linked@example.test';
    const token = 'B'.repeat(43);
    window.history.replaceState(null, '', `/profile#email_token=${token}`);
    await mount(`/profile#email_token=${token}`);
    await settle();
    const request = writes().find(([path]) => path.endsWith('/confirm'));
    expect(request).toBeDefined();
    expect(JSON.parse(request?.[1]?.body as string)).toEqual({ token });
    expect(window.location.hash).toBe('');
    expect(container.textContent).toContain('邮箱已验证');
  });

  it('disables sending actions when the deployment policy is disabled', async () => {
    emailPolicyEnabled = false;
    saved.user.pending_email = 'pending@example.test';
    await mount();
    await settle();
    await click('验证待验证邮箱');
    expect(container.textContent).toContain('当前部署未启用邮件服务');
    expect(control('提交换绑').disabled).toBe(true);
    expect(control('重发验证邮件').disabled).toBe(true);
    expect(control('确认验证').disabled).toBe(false);
    expect(control('取消待验证邮箱').disabled).toBe(false);
  });

  it('reports resend throttling without a precise countdown', async () => {
    saved.user.pending_email = 'pending@example.test';
    write = async () => envelope(null, 429, 'IDENTITY_EMAIL_VERIFICATION_RATE_LIMITED');
    await mount();
    await settle();
    await click('验证待验证邮箱');
    await click('重发验证邮件');
    await settle();
    expect(container.textContent).toContain('请求过于频繁');
    expect(container.textContent).not.toContain('3600');
  });
});
