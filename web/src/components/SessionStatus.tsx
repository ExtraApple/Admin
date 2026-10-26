import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useSession } from '@/lib/session';
import { errorMessage } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

export function SessionStatus({ allowLogin = true }: { allowLogin?: boolean }) {
  const session = useSession();
  const location = useLocation();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const starting = session.phase === 'bootstrapping';
  const pending = session.phase === 'logout-pending';
  const unavailable = session.phase === 'unavailable';
  async function run(action: () => Promise<void>) {
    setBusy(true); setError(null);
    try { await action(); } catch (reason) { setError(reason); }
    finally { setBusy(false); }
  }
  function login() {
    // Navigate first: the existing unsaved-change guard must allow departure before clearing edits.
    navigate('/login', { state: { from: location.pathname === '/login' || location.pathname === '/register' ? location.state?.from : `${location.pathname}${location.search}${location.hash}`, relogin: true } });
  }
  return <section className="session-status" aria-live="polite"><Card>
    <CardHeader><CardTitle>{starting ? '正在确认登录状态' : pending ? '退出尚未确认' : unavailable ? '浏览器会话不可用' : '需要恢复会话'}</CardTitle></CardHeader>
    <CardContent><p role={starting ? 'status' : undefined}>{error ? errorMessage(error) : session.notice || (starting ? '正在安全恢复会话，确认身份后才会显示工作台。' : '请手动恢复会话或重新登录。')}</p>
      {session.context && <p>未保存修改仍保留。重新登录或切换身份会清除这些修改。</p>}
      {!starting && <div className="detail-actions">
        <Button disabled={busy} onClick={() => void run(pending ? session.retryLogout : session.refresh)}>{busy ? '正在处理…' : pending ? '重试退出' : unavailable ? '重新检查并恢复' : '重试恢复'}</Button>
        {allowLogin && !unavailable && <Button variant="outline" disabled={busy} onClick={login}>重新登录</Button>}
        {unavailable && <Button variant="outline" disabled={busy} onClick={() => void run(session.logout)}>尝试退出当前浏览器</Button>}
      </div>}
    </CardContent>
  </Card></section>;
}
