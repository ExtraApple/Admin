import { useEffect, useRef, useState } from 'react';
import { Link, Navigate, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Network, Shield, Users, UserRound, LogOut } from 'lucide-react';
import { api, errorMessage } from '@/lib/api';
import { canAccessModule, type ManagementModule } from '@/lib/navigation';
import { useSession } from '@/lib/session';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

const modules: { key: ManagementModule; name: string; icon: typeof Shield }[] = [
  { key: 'roles', name: '角色管理', icon: Shield },
  { key: 'users', name: '用户管理', icon: Users },
  { key: 'organizations', name: '组织管理', icon: Network },
];
export function UserAvatar({ id, name }: { id: number; name: string }) {
  const [url, setUrl] = useState('');
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true; let objectUrl = '';
    setUrl(''); setFailed(false);
    void api.image(`/api/avatars/${id}`).then(blob => {
      if (!active) return;
      objectUrl = URL.createObjectURL(blob); setUrl(objectUrl);
    }).catch(() => { if (active) setFailed(true); });
    return () => { active = false; if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [id]);
  return <span className="avatar" title={failed ? '头像加载失败' : name}>{url ? <img src={url} alt={`${name}的头像`} /> : <span aria-label={failed ? '头像加载失败' : '头像加载中'}>{name.slice(0, 1) || <UserRound size={18} />}</span>}</span>;
}
export function WorkbenchShell() {
  const session = useSession();
  const location = useLocation();
  const main = useRef<HTMLElement>(null);
  useEffect(() => { main.current?.focus(); }, [location.pathname]);
  if (!session.context) return <Navigate to="/login" state={{ from: `${location.pathname}${location.search}${location.hash}` }} replace />;
  const { context } = session;
  const currentModule = modules.find(module => location.pathname.startsWith(`/${module.key}`));
  return <div className="app-shell"><a className="skip-link" href="#main-content">跳转到主要内容</a><aside className="sidebar"><Link className="brand" to="/"><span className="brand-mark" aria-hidden="true"><span /><span /><span /></span>Admin <small>工作台</small></Link><p className="nav-group">授权与归属</p><nav aria-label="工作台导航">{modules.filter(module => canAccessModule(context, module.key)).map(module => <NavLink key={module.key} className="nav-button" to={`/${module.key}`}><module.icon size={19} />{module.name}</NavLink>)}<NavLink className="nav-button" to="/profile"><UserRound size={19} />个人资料</NavLink></nav><div className="sidebar-foot"><strong>{context.user.nickname || context.user.username}</strong><span>仅显示你的可用功能</span></div></aside><div className="workspace"><header className="topbar"><nav className="breadcrumb" aria-label="当前位置"><Link to="/">工作台</Link><span aria-hidden="true">/</span><strong>{currentModule?.name ?? (location.pathname === '/profile' ? '个人资料' : '访问说明')}</strong></nav><div className="top-actions"><Link className="identity" to="/profile"><UserAvatar id={context.user.id} name={context.user.nickname || context.user.username} /><span>{context.user.nickname || context.user.username}</span></Link><Button variant="ghost" asChild><Link to="/logout"><LogOut size={17} />退出</Link></Button></div></header><main id="main-content" ref={main} tabIndex={-1} className="content"><Outlet /></main></div></div>;
}
export function LogoutPage() {
  const { logout } = useSession();
  const navigate = useNavigate();
  useEffect(() => {
    void logout().then(
      () => navigate('/login', { replace: true }),
      error => navigate('/login', { replace: true, state: { notice: `本地会话已清除。服务端退出未确认：${errorMessage(error)}` } }),
    );
  }, [logout, navigate]);
  return <main className="content" role="status">正在退出…</main>;
}
export function WorkbenchHome() {
  const { context } = useSession();
  const first = modules.find(module => canAccessModule(context, module.key));
  return first ? <Navigate to={`/${first.key}`} replace /> : <NoManagementPage />;
}
export function NoManagementPage() {
  const { reloadContext } = useSession();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  async function reload() { setBusy(true); setError(null); try { await reloadContext(); } catch (reason) { setError(reason); } finally { setBusy(false); } }
  return <section className="no-management"><span className="eyebrow">访问说明</span><h1>当前没有可用的管理功能</h1><p className="intro">你的账号已登录，但当前菜单与权限尚未提供本工作台支持的角色、用户或组织管理入口。管理功能需要管理员分配对应角色、菜单与权限。</p><Card><CardHeader><CardTitle>仍可使用个人资料</CardTitle></CardHeader><CardContent><p>这里没有待审核流程。你可以查看真实个人资料、退出登录；授权变更后可重新读取访问上下文。</p><div className="detail-actions"><Button asChild><Link to="/profile">查看个人资料</Link></Button><Button variant="outline" disabled={busy} onClick={() => void reload()}>{busy ? '正在读取…' : '重新读取授权'}</Button></div>{!!error && <Alert variant="destructive"><AlertDescription>{errorMessage(error)}</AlertDescription></Alert>}</CardContent></Card></section>;
}
export function NotFoundPage() {
  return <section><span className="eyebrow">页面不存在</span><h1>找不到这个地址</h1><p className="intro">请从工作台导航选择可用页面。</p><Button asChild><Link to="/">返回工作台</Link></Button></section>;
}
