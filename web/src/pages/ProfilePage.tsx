import { useEffect, useState } from 'react';
import { api, errorMessage } from '@/lib/api';
import { useSession } from '@/lib/session';
import type { UserInfo } from '@/lib/models';
import { UserAvatar } from '@/components/WorkbenchShell';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';
export function ProfilePage() {
  const { context } = useSession();
  const [user, setUser] = useState<UserInfo | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true; setUser(null); setError(null);
    void api.get<UserInfo>('/api/user/info').then(result => { if (active) setUser(result); }).catch(reason => { if (active) setError(reason); });
    return () => { active = false; };
  }, [revision]);
  return <section><span className="eyebrow">当前账号</span><h1>个人资料</h1><p className="intro">资料来自当前会话的真实用户信息。角色与权限以实际授权上下文为准。</p>{error ? <Alert variant="destructive"><AlertDescription>{errorMessage(error)}<Button variant="outline" onClick={() => setRevision(value => value + 1)}>重新加载</Button></AlertDescription></Alert> : !user ? <div role="status" aria-label="正在加载个人资料"><Skeleton className="profile-skeleton" /><span className="hint">正在加载个人资料…</span></div> : <><Card className="detail-hero"><div className="profile-identity"><UserAvatar id={user.id} name={user.nickname || user.username} /><div><h2>{user.nickname || user.username}</h2><code>{user.username}</code></div><Badge variant={user.status === 1 ? 'default' : 'destructive'}>{user.status === 1 ? '已启用' : '已停用'}</Badge></div></Card><div className="profile-grid"><Card><CardHeader><CardTitle>身份资料</CardTitle></CardHeader><CardContent><dl className="profile-fields"><div><dt>用户 ID</dt><dd><code>{user.id}</code></dd></div><div><dt>用户名</dt><dd>{user.username}</dd></div><div><dt>昵称</dt><dd>{user.nickname || '未设置'}</dd></div></dl></CardContent></Card><Card><CardHeader><CardTitle>邮箱</CardTitle></CardHeader><CardContent><p>{user.email || '未设置邮箱'}</p>{user.email && <Badge variant={user.email_verified ? 'default' : 'secondary'}>{user.email_verified ? '已验证' : '未验证'}</Badge>}{user.pending_email && <p className="warning-note">待验证的新邮箱：{user.pending_email}</p>}</CardContent></Card><Card><CardHeader><CardTitle>当前角色</CardTitle></CardHeader><CardContent>{context?.roles.length ? <ul className="profile-role-list">{context.roles.map(role => <li key={role}><code>{role}</code></li>)}</ul> : <p className="hint">尚未分配角色</p>}<p className="hint">角色变更后，旧会话会在后续请求失效。</p></CardContent></Card></div></>}</section>;
}
