import { useEffect, useRef, useState } from 'react';
import { api, ApiError, errorMessage, fieldMessages } from '@/lib/api';
import { useSession } from '@/lib/session';
import { sessionState } from '@/lib/session-state';
import type { AvatarUploadPolicy, UserInfo } from '@/lib/models';
import { validateNewPassword } from '@/lib/password-policy';
import { isEmailAddress, isEmailToken } from '@/lib/email-verification';

type Editor = 'nickname' | 'avatar' | 'email' | 'password' | null;
type Fields = Record<string, string[]>;
type Policy = { state: 'loading' | 'ready' | 'error'; maxBytes: number | null; error: unknown };
type EmailPolicy = { state: 'loading' | 'ready' | 'error'; enabled: boolean; error: unknown };
type Outcome = { kind: 'saved' | 'unknown' | 'error'; message: string; password?: boolean } | null;
const emptyPasswords = { old_password: '', new_password: '', confirm_password: '' };

export function useProfileEditor(userID: number, sessionID: string) {
  const session = useSession();
  const mounted = useRef(false);
  const turns = useRef({ read: 0, policy: 0, emailPolicy: 0, write: 0, editor: 0 });
  const writing = useRef(false);
  const previewURL = useRef('');
  const [user, setUser] = useState<UserInfo | null>(null);
  const [readError, setReadError] = useState<unknown>(null);
  const [reading, setReading] = useState(false);
  const [policy, setPolicy] = useState<Policy>({ state: 'loading', maxBytes: null, error: null });
  const [emailPolicy, setEmailPolicy] = useState<EmailPolicy>({ state: 'loading', enabled: false, error: null });
  const [activeEditor, setActiveEditor] = useState<Editor>(null);
  const [nickname, setNickname] = useState('');
  const [emailCandidate, setEmailCandidate] = useState('');
  const [emailCurrentPassword, setEmailCurrentPassword] = useState('');
  const [emailToken, setEmailToken] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState('');
  const [passwords, setPasswords] = useState(emptyPasswords);
  const [fields, setFields] = useState<Fields>({});
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<Outcome>(null);
  const [displayError, setDisplayError] = useState(false);
  const [imageError, setImageError] = useState(false);
  const [displayRefreshing, setDisplayRefreshing] = useState(false);
  const [switchTo, setSwitchTo] = useState<Editor>(null);
  const [resetOpen, setResetOpen] = useState(false);
  const confirmedSave = useRef(false);
  const dirty = activeEditor === 'nickname' ? nickname !== (user?.nickname ?? '')
    : activeEditor === 'avatar' ? file !== null
      : activeEditor === 'email' ? (emailCandidate.trim() !== '' || emailCurrentPassword !== '' || emailToken !== '')
        : activeEditor === 'password' && Object.values(passwords).some(Boolean);

  // Consult the store synchronously: a session can change before React commits its next render.
  function ownsActor() {
    const current = sessionState.getSnapshot();
    return mounted.current && current.phase === 'authenticated' && current.context?.user.id === userID && current.sessionId === sessionID;
  }
  function ownsTurn(channel: 'read' | 'policy' | 'emailPolicy' | 'write', turn: number) {
    return ownsActor() && turns.current[channel] === turn;
  }
  function releasePreview() {
    if (previewURL.current) URL.revokeObjectURL(previewURL.current);
    previewURL.current = ''; setPreview(''); setFile(null);
  }
  function discardEditor() {
    turns.current.editor++;
    setNickname(user?.nickname ?? ''); releasePreview(); setPasswords(emptyPasswords);
    setEmailCandidate(''); setEmailCurrentPassword(''); setEmailToken('');
    setFields({}); setActiveEditor(null); setSwitchTo(null); setResetOpen(false);
  }
  function openEditor(editor: Exclude<Editor, null>) {
    if (!ownsActor() || writing.current || reading || activeEditor === editor) return;
    if (dirty) { setSwitchTo(editor); return; }
    discardEditor(); setActiveEditor(editor);
  }
  function confirmSwitch() {
    if (!ownsActor() || writing.current || !switchTo) return;
    const next = switchTo; discardEditor(); setActiveEditor(next);
  }

  async function readDisplay() {
    if (!ownsActor() || writing.current) return;
    const turn = ++turns.current.read;
    setReading(true); setReadError(null); setDisplayRefreshing(true);
    try {
      const result = await api.get<UserInfo>('/api/user/info');
      if (!ownsTurn('read', turn)) return;
      if (!result || result.id !== userID) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      setUser(result);
      // Never replace an open draft during read-only reconciliation.
      if (turns.current.editor === 0) setNickname(result.nickname);
      const context = await session.reloadContext();
      if (!ownsTurn('read', turn)) return;
      if (context.user.id !== userID) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      setUser(context.user); setReadError(null); setDisplayError(false);
    } catch (error) {
      if (!ownsTurn('read', turn)) return;
      setReadError(error); setDisplayError(true);
    } finally {
      if (ownsTurn('read', turn)) { setReading(false); setDisplayRefreshing(false); }
    }
  }
  async function readPolicy() {
    if (!ownsActor()) return;
    const turn = ++turns.current.policy;
    setPolicy({ state: 'loading', maxBytes: null, error: null });
    try {
      const result = await api.get<AvatarUploadPolicy>('/api/user/avatar-upload-policy');
      if (!ownsTurn('policy', turn)) return;
      if (!result || !Number.isSafeInteger(result.max_bytes) || result.max_bytes <= 0) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      setPolicy({ state: 'ready', maxBytes: result.max_bytes, error: null });
    } catch (error) {
      if (ownsTurn('policy', turn)) setPolicy({ state: 'error', maxBytes: null, error });
    }
  }
  async function readEmailPolicy() {
    if (!ownsActor()) return;
    const turn = ++turns.current.emailPolicy;
    setEmailPolicy({ state: 'loading', enabled: false, error: null });
    try {
      const result = await api.emailVerification.policy();
      if (!ownsTurn('emailPolicy', turn)) return;
      if (!result || typeof result.enabled !== 'boolean') throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      setEmailPolicy({ state: 'ready', enabled: result.enabled, error: null });
    } catch (error) {
      if (ownsTurn('emailPolicy', turn)) setEmailPolicy({ state: 'error', enabled: false, error });
    }
  }

  useEffect(() => {
    mounted.current = true;
    void readDisplay(); void readPolicy(); void readEmailPolicy();
    return () => {
      mounted.current = false;
      turns.current.read++; turns.current.policy++; turns.current.emailPolicy++; turns.current.write++; turns.current.editor++;
      if (previewURL.current) URL.revokeObjectURL(previewURL.current);
      previewURL.current = '';
    };
    // Actor remounts own these operations; ordinary context refreshes must not reset drafts.
  }, [userID, sessionID]);

  const previousPhase = useRef(session.phase);
  useEffect(() => {
    const previous = previousPhase.current;
    previousPhase.current = session.phase;
    if (session.phase !== 'authenticated') {
      turns.current.read++; turns.current.policy++; turns.current.emailPolicy++; turns.current.write++; turns.current.editor++;
      writing.current = false; setBusy(false); setReading(false); setDisplayRefreshing(false);
      setPasswords(emptyPasswords); setFields({}); setSwitchTo(null); setResetOpen(false);
      setEmailCurrentPassword(''); setEmailToken('');
    } else if (previous !== 'authenticated') {
      // Same actor recovery keeps nonsecret drafts, but only replays reads, never writes.
      void readDisplay(); void readPolicy(); void readEmailPolicy();
    }
  }, [session.phase]);

  function chooseFile(selected: File | undefined) {
    if (!ownsActor() || writing.current || policy.state !== 'ready' || !selected) return;
    let message = '';
    if (!selected.size) message = '文件为空，请选择静态 JPEG、PNG 或 WebP 图片。';
    else if (selected.size > policy.maxBytes!) message = `文件超过当前 ${policy.maxBytes} 字节上限。`;
    else if (!/\.(?:jpe?g|png|webp)$/i.test(selected.name) || (selected.type && !['image/jpeg', 'image/png', 'image/webp'].includes(selected.type))) message = '请选择静态 JPEG、PNG 或 WebP 图片。';
    if (message) { setFields({ file: [message] }); return; }
    const nextURL = URL.createObjectURL(selected);
    if (!ownsActor()) { URL.revokeObjectURL(nextURL); return; }
    releasePreview(); previewURL.current = nextURL; setFile(selected); setPreview(nextURL); setFields({});
  }

  async function refreshSavedDisplay(turn: number) {
    setDisplayRefreshing(true);
    try {
      const context = await session.reloadContext();
      if (!ownsTurn('write', turn)) return;
      if (context.user.id !== userID) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      setUser(context.user); setNickname(context.user.nickname); setDisplayError(false);
    } catch {
      if (ownsTurn('write', turn)) setDisplayError(true);
    } finally {
      if (ownsTurn('write', turn)) setDisplayRefreshing(false);
    }
  }
  async function submit(kind: Exclude<Editor, null> | 'reset') {
    if (!ownsActor() || writing.current || reading || !user) return;
    const validation: Fields = {};
    if (kind === 'email' && !emailPolicy.enabled) {
      setOutcome({ kind: 'error', message: '当前部署未启用邮件服务，暂不能换绑邮箱。' });
      return;
    }
    if (kind === 'nickname') {
      if (!nickname.trim()) validation.nickname = ['昵称不能为空或全为空白。'];
      else if ([...nickname].length > 100) validation.nickname = ['昵称最多 100 个 Unicode 字符。'];
      if (nickname === user.nickname && !Object.keys(validation).length) { setFields({ nickname: ['昵称尚未修改。'] }); return; }
    } else if (kind === 'avatar') {
      if (policy.state !== 'ready' || !file) return;
      if (file.size > policy.maxBytes!) validation.file = [`文件超过当前 ${policy.maxBytes} 字节上限，请重新选择或读取规则。`];
    } else if (kind === 'email') {
      const candidate = emailCandidate.trim();
      if (!candidate) validation.email = ['请输入新邮箱。'];
      else if (!isEmailAddress(candidate)) validation.email = ['请输入有效的邮箱地址。'];
      else if (candidate === user.email) validation.email = ['新邮箱不能与当前邮箱相同。'];
      else if (candidate === user.pending_email) validation.email = ['该邮箱已在待验证，请直接重发或取消。'];
      if (!emailCurrentPassword) validation.current_password = ['请输入当前密码。'];
    } else if (kind === 'password') {
      if (!passwords.old_password) validation.old_password = ['请输入旧密码。'];
      const policyError = validateNewPassword(passwords.new_password);
      if (policyError) validation.new_password = [policyError];
      else if (passwords.new_password === passwords.old_password) validation.new_password = ['新密码不能与旧密码相同。'];
      if (passwords.confirm_password !== passwords.new_password) validation.confirm_password = ['两次输入的密码不一致。'];
    }
    setFields(validation);
    if (Object.keys(validation).length) return;
    const turn = ++turns.current.write;
    turns.current.read++;
    writing.current = true; setBusy(true); setOutcome(null); setReadError(null); setResetOpen(false);
    confirmedSave.current = false;
    let refreshAfterWrite = false;
    try {
      if (kind === 'password') {
        await api.put<null>('/api/user/password', passwords);
        if (!ownsTurn('write', turn)) return;
        setPasswords(emptyPasswords); setActiveEditor(null); setFields({});
        setOutcome({ kind: 'saved', message: '密码已修改，请重新登录。' });
        // This check must remain immediately before logout, not just before rendering success.
        if (ownsTurn('write', turn)) {
          try { await session.logout('password-changed'); }
          catch {
            if (ownsTurn('write', turn)) setOutcome({ kind: 'saved', message: '密码已修改；服务端退出未确认，请使用会话入口重试退出或显式重新登录。' });
          }
        }
      } else if (kind === 'email') {
        const result = await api.put<UserInfo>('/api/user/info', { email: emailCandidate.trim(), current_password: emailCurrentPassword });
        if (!ownsTurn('write', turn)) return;
        if (!result || result.id !== userID) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
        discardEditor(); setUser(result); setNickname(result.nickname);
        confirmedSave.current = true;
        setOutcome({ kind: 'saved', message: result.pending_email ? '换绑请求已提交。请查收验证邮件并完成验证。' : '邮箱已更换为未验证状态，请查收验证邮件。' });
        await refreshSavedDisplay(turn);
      } else {
        let result: UserInfo;
        if (kind === 'nickname') result = await api.put<UserInfo>('/api/user/info', { nickname });
        else if (kind === 'reset') result = await api.delete<UserInfo>('/api/user/avatar');
        else {
          const body = new FormData(); body.append('file', file!);
          result = await api.post<UserInfo>('/api/user/avatar', body);
        }
        if (!ownsTurn('write', turn)) return;
        if (!result || result.id !== userID) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
        discardEditor(); setUser(result); setNickname(result.nickname);
        confirmedSave.current = true;
        setOutcome({ kind: 'saved', message: kind === 'nickname' ? '昵称已保存。' : kind === 'reset' ? '默认头像已恢复。' : '头像已保存。' });
        await refreshSavedDisplay(turn);
      }
    } catch (error) {
      if (!ownsTurn('write', turn)) return;
      const localized = fieldMessages(error);
      if (kind === 'password' && localized.password) { localized.new_password = localized.password; delete localized.password; }
      if (kind === 'email' && error instanceof ApiError && error.errorCode === 'IDENTITY_CONFLICT') localized.email = ['该邮箱已被使用，请更换。'];
      const emailDelivery = kind === 'email' && error instanceof ApiError && (error.status === 503 || error.errorCode === 'IDENTITY_EMAIL_VERIFICATION_DELIVERY_FAILED' || error.errorCode === 'IDENTITY_EMAIL_VERIFICATION_DISABLED');
      if (emailDelivery) refreshAfterWrite = true;
      setFields(localized);
      const definite = error instanceof ApiError && error.status >= 400 && error.status < 500 && error.status !== 408;
      setOutcome(emailDelivery
        ? { kind: 'error', message: errorMessage(error) }
        : definite
          ? { kind: 'error', message: errorMessage(error) }
          : { kind: 'unknown', password: kind === 'password', message: kind === 'password' ? '密码修改结果尚未确认。不要自动重交或尝试新旧密码；可手动重新登录核对。' : '保存结果尚未确认。草稿仍保留，请重新读取已保存资料核对；不会自动重发写请求。' });
    } finally {
      if (ownsTurn('write', turn)) {
        writing.current = false; setBusy(false);
        if (refreshAfterWrite) { void readDisplay(); void readEmailPolicy(); }
      }
    }
  }
  async function resendEmail() {
    if (!ownsActor() || writing.current || !emailPolicy.enabled) return;
    const turn = ++turns.current.write;
    turns.current.read++;
    writing.current = true; setBusy(true); setOutcome(null); setFields({});
    let refreshAfterWrite = false;
    try {
      await api.emailVerification.resend();
      if (!ownsTurn('write', turn)) return;
      setOutcome({ kind: 'saved', message: '验证邮件已发送。' });
    } catch (error) {
      if (!ownsTurn('write', turn)) return;
      setFields(fieldMessages(error));
      refreshAfterWrite = error instanceof ApiError && (error.status === 503 || error.errorCode === 'IDENTITY_EMAIL_VERIFICATION_DISABLED');
      setOutcome({ kind: 'error', message: errorMessage(error) });
    } finally {
      if (ownsTurn('write', turn)) {
        writing.current = false; setBusy(false);
        if (refreshAfterWrite) { void readDisplay(); void readEmailPolicy(); }
      }
    }
  }

  async function confirmEmail(rawToken: string) {
    if (!ownsActor() || writing.current) return;
    const token = rawToken.trim();
    if (!token) { setFields({ token: ['请输入邮箱验证码。'] }); return; }
    if (!isEmailToken(token)) { setFields({ token: ['验证码格式不正确，请粘贴邮件中的完整验证码。'] }); return; }
    const turn = ++turns.current.write;
    turns.current.read++;
    writing.current = true; setBusy(true); setOutcome(null); setFields({});
    try {
      const result = await api.emailVerification.confirm(token);
      if (!ownsTurn('write', turn)) return;
      if (!result || result.id !== userID) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      discardEditor(); setUser(result); setNickname(result.nickname);
      confirmedSave.current = true;
      setOutcome({ kind: 'saved', message: '邮箱已验证。' });
      await refreshSavedDisplay(turn);
    } catch (error) {
      if (!ownsTurn('write', turn)) return;
      const localized = fieldMessages(error);
      if (error instanceof ApiError && error.errorCode === 'IDENTITY_EMAIL_VERIFICATION_INVALID') localized.token = ['验证码无效或已过期，请重新发送验证邮件。'];
      if (error instanceof ApiError && error.errorCode === 'IDENTITY_CONFLICT') localized.email = ['该邮箱已被其他账号使用，请更换邮箱后重新验证。'];
      setFields(localized);
      setOutcome({ kind: 'error', message: errorMessage(error) });
    } finally {
      if (ownsTurn('write', turn)) { writing.current = false; setBusy(false); }
    }
  }

  async function cancelPendingEmail() {
    const pending = user?.pending_email ?? '';
    if (!ownsActor() || writing.current || !pending) return;
    const turn = ++turns.current.write;
    turns.current.read++;
    writing.current = true; setBusy(true); setOutcome(null); setFields({});
    let refreshAfterWrite = false;
    try {
      const result = await api.emailVerification.cancel(pending);
      if (!ownsTurn('write', turn)) return;
      if (!result || result.id !== userID) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      discardEditor(); setUser(result); setNickname(result.nickname);
      confirmedSave.current = true;
      setOutcome({ kind: 'saved', message: '已取消待验证邮箱。' });
      await refreshSavedDisplay(turn);
    } catch (error) {
      if (!ownsTurn('write', turn)) return;
      const localized = fieldMessages(error);
      if (error instanceof ApiError && error.errorCode === 'IDENTITY_EMAIL_PENDING_STALE') localized.email = ['待验证邮箱已变化，请重新读取后再操作。'];
      refreshAfterWrite = error instanceof ApiError && error.errorCode === 'IDENTITY_EMAIL_PENDING_STALE';
      setFields(localized);
      setOutcome({ kind: 'error', message: errorMessage(error) });
    } finally {
      if (ownsTurn('write', turn)) {
        writing.current = false; setBusy(false);
        if (refreshAfterWrite) { void readDisplay(); void readEmailPolicy(); }
      }
    }
  }

  const imageOwner = { revision: session.userRevision, editor: turns.current.editor };
  function imageResult(failed: boolean) {
    if (!ownsActor() || sessionState.getSnapshot().userRevision !== imageOwner.revision || turns.current.editor !== imageOwner.editor) return;
    setImageError(failed);
  }

  return {
    user, readError, reading, policy, emailPolicy, activeEditor, nickname, file, preview, passwords, fields, busy, dirty,
    emailCandidate, emailCurrentPassword, emailToken,
    outcome, displayError: displayError || imageError, displayRefreshing, confirmedSave: confirmedSave.current, switchTo, resetOpen,
    suspended: session.phase !== 'authenticated', userRevision: session.userRevision,
    openEditor, confirmSwitch, setSwitchTo, setResetOpen,
    cancel: () => { if (ownsActor() && !writing.current) discardEditor(); },
    setNickname: (value: string) => { if (ownsActor() && !writing.current) { setNickname(value); setFields({}); } },
    setPassword: (name: keyof typeof emptyPasswords, value: string) => { if (ownsActor() && !writing.current) { setPasswords(current => ({ ...current, [name]: value })); setFields({}); } },
    setEmailCandidate: (value: string) => { if (ownsActor() && !writing.current) { setEmailCandidate(value); setFields({}); } },
    setEmailCurrentPassword: (value: string) => { if (ownsActor() && !writing.current) { setEmailCurrentPassword(value); setFields({}); } },
    setEmailToken: (value: string) => { if (ownsActor() && !writing.current) { setEmailToken(value); setFields({}); } },
    chooseFile, readDisplay, readPolicy, readEmailPolicy, submit, resendEmail, confirmEmail, cancelPendingEmail, imageResult,
  };
}
