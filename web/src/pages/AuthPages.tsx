import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { ArrowRight, Eye, EyeOff, RefreshCw } from 'lucide-react';
import { api, ApiError, errorMessage, fieldMessages } from '@/lib/api';
import { captchaSource, lockDeadline, lockSeconds } from '@/lib/auth-form';
import { safeDestination } from '@/lib/navigation';
import { useSession } from '@/lib/session';
import type { Captcha, UserInfo } from '@/lib/models';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';

function AuthFrame({ children }: { children: ReactNode }) {
  return <div className="auth-screen"><aside className="auth-aside"><div className="brand"><span className="brand-mark" aria-hidden="true"><span /><span /><span /></span>Admin <small>管理工作台</small></div><div><span className="auth-eyebrow">身份 · 角色 · 组织</span><h1>清晰的归属，<br />明确的授权。</h1><p>从真实身份出发，在各自的数据范围内管理角色、用户与组织。</p></div><footer>安全会话仅保留在当前页面，刷新页面后请重新登录。</footer></aside><main className="auth-main"><div className="auth-content">{children}</div></main></div>;
}
function FieldErrors({ name, fields }: { name: string; fields: Record<string, string[]> }) {
  return fields[name]?.length ? <ul id={`${name}-errors`} className="field-errors" aria-live="polite">{fields[name].map((message, index) => <li key={index}>{message}</li>)}</ul> : null;
}
function useCaptcha() {
  const [captcha, setCaptcha] = useState<Captcha | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const sequence = useRef(0);
  async function reload() {
    const current = ++sequence.current;
    setLoading(true); setCaptcha(null); setError(null);
    try {
      const result = await api.get<Captcha>('/api/captcha');
      if (!result?.captcha_id || !result?.captcha_img) throw new ApiError(200, 'CLIENT_RESPONSE_INVALID', 'Invalid server response.');
      if (sequence.current === current) setCaptcha(result);
    } catch (reason) { if (sequence.current === current) setError(reason); }
    finally { if (sequence.current === current) setLoading(false); }
  }
  useEffect(() => { void reload(); return () => { sequence.current++; }; }, []);
  return { captcha, error, loading, reload };
}
export function LoginPage() { return <AuthenticationForm registration={false} />; }
export function RegisterPage() { return <AuthenticationForm registration />; }
function AuthenticationForm({ registration }: { registration: boolean }) {
  const session = useSession();
  const navigate = useNavigate();
  const location = useLocation();
  const destination = safeDestination(location.state?.from);
  const captchaState = useCaptcha();
  const [values, setValues] = useState({ username: '', password: '', nickname: '', email: '', captcha_code: '' });
  const [showPassword, setShowPassword] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [registered, setRegistered] = useState(false);
  const [deadline, setDeadline] = useState(0);
  const [now, setNow] = useState(Date.now());
  const remaining = lockSeconds(deadline, now);
  useEffect(() => {
    if (!deadline) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [deadline]);
  const fields = fieldMessages(error);
  useEffect(() => {
    if (busy || !error) return;
    const names = Object.keys(fieldMessages(error));
    if (error instanceof ApiError && error.errorCode === 'AUTHN_CAPTCHA_INVALID') names.unshift('captcha_code');
    if (names.length) document.getElementById(names[0])?.focus();
  }, [busy, error]);
  if (error instanceof ApiError && error.errorCode === 'AUTHN_CAPTCHA_INVALID') fields.captcha_code = [errorMessage(error)];
  if (session.context) return <Navigate to={destination} replace />;
  async function changeCaptcha() { setValues(value => ({ ...value, captcha_code: '' })); await captchaState.reload(); }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || remaining || !captchaState.captcha || captchaState.loading) return;
    setBusy(true); setError(null);
    const input = { ...values, captcha_id: captchaState.captcha.captcha_id };
    try {
      if (registration) { await api.post<UserInfo>('/api/register', input); setRegistered(true); }
      else { await session.login({ username: input.username, password: input.password, captcha_id: input.captcha_id, captcha_code: input.captcha_code }); navigate(destination, { replace: true }); }
    } catch (reason) {
      setError(reason); setDeadline(lockDeadline(reason)); setNow(Date.now());
      await changeCaptcha();
    } finally { setBusy(false); }
  }
  function input(name: 'username' | 'password' | 'nickname' | 'email', label: string, required = true) {
    return <div className="field"><Label htmlFor={name}>{label}{required ? ' *' : ''}</Label><div className={name === 'password' ? 'password-field' : undefined}><Input id={name} name={name} value={values[name]} onChange={event => setValues(value => ({ ...value, [name]: event.target.value }))} type={name === 'password' ? showPassword ? 'text' : 'password' : name === 'email' ? 'email' : 'text'} required={required} autoComplete={name === 'password' ? registration ? 'new-password' : 'current-password' : name === 'username' ? 'username' : name === 'email' ? 'email' : 'nickname'} disabled={busy} aria-invalid={!!fields[name]} aria-describedby={fields[name] ? `${name}-errors` : name === 'password' && registration ? 'password-help' : undefined} />{name === 'password' && <Button variant="ghost" size="icon" aria-label={showPassword ? '隐藏密码' : '显示密码'} aria-pressed={showPassword} onClick={() => setShowPassword(value => !value)}>{showPassword ? <EyeOff size={18} /> : <Eye size={18} />}</Button>}</div><FieldErrors name={name} fields={fields} /></div>;
  }
  return <AuthFrame>{registered ? <><span className="eyebrow">公开注册</span><h2>账号已创建</h2><Alert><AlertDescription>你已获得普通用户身份。仍需管理员分配角色，才能使用对应的管理功能。若提供了邮箱，请查收验证邮件。</AlertDescription></Alert><Button asChild className="auth-submit"><Link to="/login" state={{ from: destination }}>返回登录 <ArrowRight size={18} /></Link></Button></> : <><span className="eyebrow">{registration ? '公开注册' : '安全访问'}</span><h2>{registration ? '创建普通用户账号' : '登录工作台'}</h2><p>{registration ? '注册与管理授权是两个独立步骤。' : '使用你的账号和图片验证码继续。'}</p>{location.state?.notice && <Alert><AlertDescription>{location.state.notice}</AlertDescription></Alert>}<form onSubmit={submit} aria-busy={busy}>{input('username', '用户名')}{registration && input('nickname', '昵称', false)}{registration && input('email', '邮箱')}{input('password', '密码')}{registration && <p id="password-help" className="hint">至少 6 位，包含大写、小写、数字、特殊字符中的至少三类。</p>}<div className="field"><Label htmlFor="captcha_code">图片验证码 *</Label><div className="captcha-row"><Input id="captcha_code" name="captcha_code" value={values.captcha_code} onChange={event => setValues(value => ({ ...value, captcha_code: event.target.value }))} required autoComplete="off" disabled={busy} aria-invalid={!!fields.captcha_code} aria-describedby={fields.captcha_code ? 'captcha_code-errors' : undefined} />{captchaState.loading ? <Skeleton className="captcha-image" /> : captchaState.captcha ? <img className="captcha-image" src={captchaSource(captchaState.captcha.captcha_img)} alt="图片验证码" /> : <span className="captcha-image hint">未加载</span>}<Button variant="outline" size="icon" disabled={busy || captchaState.loading} aria-label="更换图片验证码" onClick={() => void changeCaptcha()}><RefreshCw size={17} /></Button></div><FieldErrors name="captcha_code" fields={fields} />{!!captchaState.error && <Alert variant="destructive"><AlertDescription>{errorMessage(captchaState.error)}<Button variant="link" disabled={busy} onClick={() => void changeCaptcha()}>重新加载验证码</Button></AlertDescription></Alert>}</div>{!!error && <Alert variant="destructive"><AlertDescription>{errorMessage(error)}{error instanceof ApiError && error.safeData?.remaining_attempts !== undefined && <p>剩余尝试次数：{error.safeData.remaining_attempts}</p>}{deadline > 0 && <p role="status">{remaining > 0 ? `请等待 ${remaining} 秒后手动重试。` : '等待已结束，请输入当前验证码后手动重试。'}</p>}</AlertDescription></Alert>}<Button type="submit" className="auth-submit" disabled={busy || remaining > 0 || !captchaState.captcha || captchaState.loading}>{busy ? registration ? '正在创建…' : '正在登录…' : remaining ? `等待 ${remaining} 秒` : registration ? '创建账号' : error ? '手动重试登录' : '登录'} <ArrowRight size={17} /></Button></form><p className="auth-hint">{registration ? '已有账号？' : '还没有账号？'} <Link to={registration ? '/login' : '/register'} state={{ from: destination }}>{registration ? '返回登录' : '公开注册'}</Link></p><p className="auth-hint">令牌不会保存到浏览器本地；刷新页面后需要重新登录。</p></>}</AuthFrame>;
}
