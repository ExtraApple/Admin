import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, Navigate, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { ChevronDown, UserRound, LogOut } from 'lucide-react';
import { api, errorMessage } from '@/lib/api';
import { canAccessAuthorizationOverview, canAccessModule, type ManagementModule } from '@/lib/navigation';
import { getWorkbenchBreadcrumbs, useWorkbenchNavigation } from '@/lib/workbench-navigation';
import { applyNavigationToTabs, planBatchCloseTabs, planCloseTab, resolveWorkbenchTab, togglePinTab, type PendingTabRemoval, type WorkbenchTab, type WorkbenchTabKey } from '@/lib/workbench-tabs';
import { useSession } from '@/lib/session';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '@/components/ui/breadcrumb';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Sidebar, SidebarContent, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarInset, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarTrigger, useSidebar } from '@/components/ui/sidebar';
import type { UserContext } from '@/lib/models';
import { ThemeSelector } from '@/components/ThemeSelector';
import { SessionStatus } from '@/components/SessionStatus';
import { WorkbenchTabBar } from '@/components/WorkbenchTabBar';
const modules: ManagementModule[] = ['roles', 'users', 'organizations', 'menus', 'apis'];
export function UserAvatar({ id, name, revision, onLoad, onError }: { id: number; name: string; revision?: number; onLoad?: () => void; onError?: () => void }) {
  const [image, setImage] = useState<{ id: number; revision: number | undefined; url: string; loaded(): void; failed(): void } | null>(null);
  const [failed, setFailed] = useState(false);
  const callbacks = useRef({ onLoad, onError });
  callbacks.current = { onLoad, onError };
  useEffect(() => {
    let active = true; let objectUrl = '';
    const controller = new AbortController();
    setImage(null); setFailed(false);
    function fail() {
      if (!active) return;
      if (objectUrl) { URL.revokeObjectURL(objectUrl); objectUrl = ''; }
      setImage(null); setFailed(true); callbacks.current.onError?.();
    }
    void api.image(`/api/avatars/${id}`, { cache: revision ? 'reload' : undefined, signal: controller.signal }).then(blob => {
      if (!active) return;
      objectUrl = URL.createObjectURL(blob);
      setImage({ id, revision, url: objectUrl, loaded: () => { if (active) callbacks.current.onLoad?.(); }, failed: fail });
    }).catch(fail);
    return () => { active = false; controller.abort(); if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [id, revision]);
  const current = image?.id === id && image.revision === revision ? image : null;
  return <span className="avatar" title={failed ? '头像加载失败' : name}>{current ? <img src={current.url} alt={`${name}的头像`} onLoad={current.loaded} onError={current.failed} /> : <span aria-label={failed ? '头像加载失败' : '头像加载中'}>{name.slice(0, 1) || <UserRound size={18} />}</span>}</span>;
}
export function WorkbenchShell() {
  const { context, phase, sessionId } = useSession();
  const location = useLocation();
  if (!context) return phase === 'anonymous'
    ? <Navigate to="/login" state={{ from: `${location.pathname}${location.search}${location.hash}` }} replace />
    : <main className="content"><SessionStatus /></main>;
  const suspended = phase !== 'authenticated';
  return <><div hidden={suspended} inert={suspended}><SidebarProvider key={sessionId} className="app-shell"><WorkbenchLayout context={context} /></SidebarProvider></div>{suspended && <main className="content"><SessionStatus /></main>}</>;
}

function WorkbenchLayout({ context }: { context: UserContext }) {
  const location = useLocation();
  const navigate = useNavigate();
  const main = useRef<HTMLElement>(null);
  const { setOpenMobile } = useSidebar();
  const { notice, refresh, userRevision } = useSession();
  const { groups, openGroups, setGroupOpen } = useWorkbenchNavigation(context, location.pathname);
  const name = context.user.nickname || context.user.username;
  const breadcrumbs = useMemo(() => getWorkbenchBreadcrumbs(location.pathname), [location.pathname]);
  const visitedTab = useMemo(
    () => resolveWorkbenchTab(location.pathname, location.search, context),
    [location.pathname, location.search, context],
  );
  const [tabs, setTabs] = useState<WorkbenchTab[]>(() => (visitedTab ? [visitedTab] : []));
  const pendingRemovalRef = useRef<PendingTabRemoval | null>(null);
  const locationSignature = `${location.key}:${location.pathname}${location.search}`;
  const lastLocationRef = useRef(locationSignature);

  useEffect(() => { main.current?.focus({ preventScroll: true }); }, [location.pathname]);
  // Close only after navigation commits, preserving the Sheet when an unsaved-change guard cancels.
  useEffect(() => { setOpenMobile(false); }, [location.key, setOpenMobile]);
  useEffect(() => {
    const locationChanged = locationSignature !== lastLocationRef.current;
    const pending = locationChanged ? pendingRemovalRef.current : null;
    if (locationChanged) {
      lastLocationRef.current = locationSignature;
      pendingRemovalRef.current = null;
    }
    setTabs(current => applyNavigationToTabs(current, visitedTab, pending, context).tabs);
  }, [locationSignature, visitedTab, context]);

  const clearPendingRemoval = useCallback(() => {
    pendingRemovalRef.current = null;
  }, []);

  const handleCloseTab = useCallback((targetKey: WorkbenchTabKey) => {
    const plan = planCloseTab(tabs, visitedTab?.key ?? null, targetKey);
    if (plan.navigateTo) {
      pendingRemovalRef.current = plan.pendingRemoval;
      navigate(plan.navigateTo.url);
    } else {
      pendingRemovalRef.current = null;
      setTabs(plan.nextTabs);
    }
  }, [tabs, visitedTab, navigate]);

  const handleBatchClose = useCallback((anchorKey: WorkbenchTabKey, mode: 'others' | 'right') => {
    const plan = planBatchCloseTabs(tabs, visitedTab?.key ?? null, anchorKey, mode);
    if (plan.navigateTo) {
      pendingRemovalRef.current = plan.pendingRemoval;
      navigate(plan.navigateTo.url);
    } else {
      pendingRemovalRef.current = null;
      setTabs(plan.nextTabs);
    }
  }, [tabs, visitedTab, navigate]);

  const handleTogglePin = useCallback((targetKey: WorkbenchTabKey) => {
    setTabs(current => togglePinTab(current, targetKey));
  }, []);

  return <>
    <a className="skip-link" href="#main-content">跳转到主要内容</a>
    <Sidebar className="workbench-sidebar">
      <SidebarHeader>
        <Link className="brand" to="/" onClick={clearPendingRemoval}><span className="brand-mark" aria-hidden="true"><span /><span /><span /></span>Admin <small>工作台</small></Link>
      </SidebarHeader>
      <SidebarContent>
        <nav aria-label="工作台导航">
          {groups.map(group => <Collapsible key={group.id} open={openGroups[group.id]} onOpenChange={open => setGroupOpen(group.id, open)} asChild>
            <SidebarGroup>
              <SidebarGroupLabel asChild>
                <CollapsibleTrigger><span>{group.label}</span><ChevronDown className="workbench-group-chevron" size={16} aria-hidden="true" /></CollapsibleTrigger>
              </SidebarGroupLabel>
              <CollapsibleContent asChild>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {group.items.map(item => <SidebarMenuItem key={item.to}>
                      <SidebarMenuButton asChild>
                        <NavLink to={item.to} end={group.id !== 'identity-organization' && group.id !== 'configuration-management'} onClick={clearPendingRemoval}><item.icon size={18} aria-hidden="true" /><span>{item.label}</span></NavLink>
                      </SidebarMenuButton>
                    </SidebarMenuItem>)}
                  </SidebarMenu>
                </SidebarGroupContent>
              </CollapsibleContent>
            </SidebarGroup>
          </Collapsible>)}
        </nav>
      </SidebarContent>
    </Sidebar>
    <SidebarInset className="workspace">
      {notice && <Alert role="status"><AlertDescription>{notice} <Button variant="link" onClick={() => void refresh().catch(() => {})}>手动恢复会话</Button></AlertDescription></Alert>}
      <div className="workbench-header">
        <header className="topbar">
          <div className="topbar-location">
            <SidebarTrigger className="workbench-menu-trigger" />
            <Breadcrumb aria-label="当前位置">
              <BreadcrumbList>
                {breadcrumbs.map((crumb, index) => (
                  <Fragment key={`${crumb.label}-${index}`}>
                    {index > 0 && <BreadcrumbSeparator />}
                    <BreadcrumbItem>
                      {crumb.to && index < breadcrumbs.length - 1
                        ? <BreadcrumbLink asChild><Link to={crumb.to} onClick={clearPendingRemoval}>{crumb.label}</Link></BreadcrumbLink>
                        : <BreadcrumbPage>{crumb.label}</BreadcrumbPage>}
                    </BreadcrumbItem>
                  </Fragment>
                ))}
              </BreadcrumbList>
            </Breadcrumb>
          </div>
          <div className="top-actions">
            <ThemeSelector />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" className="identity" aria-label={`${name}的账号菜单`}>
                  <UserAvatar id={context.user.id} name={name} revision={userRevision} />
                  <span className="identity-name">{name}</span>
                  <ChevronDown className="identity-chevron" size={14} aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="account-menu">
                <DropdownMenuItem asChild><Link to="/profile" onClick={clearPendingRemoval}><UserRound size={17} aria-hidden="true" />个人资料</Link></DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild><Link to="/logout" onClick={clearPendingRemoval}><LogOut size={17} aria-hidden="true" />退出登录</Link></DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>
        <WorkbenchTabBar
          tabs={tabs}
          activeKey={visitedTab?.key ?? null}
          onSelectTab={clearPendingRemoval}
          onCloseTab={handleCloseTab}
          onBatchClose={handleBatchClose}
          onTogglePin={handleTogglePin}
        />
      </div>
      <main id="main-content" ref={main} tabIndex={-1} className="content" onPointerDown={clearPendingRemoval}><Outlet /></main>
    </SidebarInset>
  </>;
}
export function LogoutPage() {
  const { logout } = useSession();
  const navigate = useNavigate();
  useEffect(() => {
    void logout().then(
      () => navigate('/login', { replace: true }),
      () => navigate('/login', { replace: true }),
    );
  }, [logout, navigate]);
  return <main className="content" role="status">正在退出…</main>;
}
export function WorkbenchHome() {
  const { context } = useSession();
  if (canAccessAuthorizationOverview(context)) return <Navigate to="/authorization-overview" replace />;
  const first = modules.find(module => canAccessModule(context, module));
  return first ? <Navigate to={`/${first}`} replace /> : <NoManagementPage />;
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
