import { useEffect, useState, type Dispatch, type SetStateAction } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { ColumnDef } from '@tanstack/react-table';
import { api, ApiError } from '@/lib/api';
import type { AdminUser, Organization, Page, Role } from '@/lib/models';
import { useSession } from '@/lib/session';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { allPages, Choice, Confirm, DataTable, ErrorNotice, flattenTree, Guard, ListFilters, Notice, OrganizationChoices, OrganizationsText, PageHeading, Pagination, protectedReason, ResourceState, sameIds, SelectionSummary, Status, toggleId, UnsavedGuard, useListQuery, useObjectId, useResource, useSavedNavigation } from '@/components/management/shared';

type UserAction = 'status' | 'kick' | 'delete';
const actionPermissions: Record<UserAction, string> = { status: 'admin.users.id.status.put', kick: 'admin.users.id.kick.put', delete: 'admin.users.id.delete' };
function useUserActions(onChanged: (user: AdminUser, action: UserAction, status?: number) => void) {
  const { context, can } = useSession();
  const [pending, setPending] = useState<{ user: AdminUser; action: UserAction } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [message, setMessage] = useState('');
  const choose = (user: AdminUser, action: UserAction) => {
    if (busy || pending || protectedReason(user, context?.user.id) || !can(actionPermissions[action])) return;
    setError(null); setMessage(''); setPending({ user, action });
  };
  const execute = async () => {
    if (busy || !pending || protectedReason(pending.user, context?.user.id) || !can(actionPermissions[pending.action])) return;
    setBusy(true); setError(null);
    try {
      const path = `/api/admin/users/${pending.user.id}`;
      let status: number | undefined;
      if (pending.action === 'delete') await api.delete(path);
      else if (pending.action === 'status') status = (await api.put<{ status: number }>(`${path}/status`)).status;
      else await api.put(`${path}/kick`);
      setMessage(pending.action === 'kick' ? '已强制下线；用户资料与状态未改变。' : pending.action === 'delete' ? '用户已删除。' : `用户已${status === 1 ? '启用' : '停用'}。`);
      onChanged(pending.user, pending.action, status); setPending(null);
    } catch (failure) { setError(failure); setPending(null); } finally { setBusy(false); }
  };
  const label = pending ? pending.action === 'status' ? pending.user.status === 1 ? '停用' : '启用' : pending.action === 'kick' ? '强制下线' : '删除' : '';
  const feedback = <><ErrorNotice error={error} />{message && <p className="mg-success" role="status">{message}</p>}<Confirm open={!!pending} onOpenChange={open => { if (!open) setPending(null); }} busy={busy} title={`确认${label}${pending?.user.nickname || pending?.user.username || ''}？`} label={`确认${label}`} onConfirm={execute}><p>对象：{pending?.user.nickname || pending?.user.username} · {pending?.user.username}</p><p>{pending?.action === 'delete' ? '此操作删除用户，不是停用，无法撤销。' : pending?.action === 'kick' ? '使该用户的旧会话失效；资料和状态保持不变。' : '切换该用户启用状态；旧会话将在后续请求失效。'}</p></Confirm></>;
  return { choose, feedback, busy };
}
function UserMenu({ user, open, setOpen, onAction, disabled }: { user: AdminUser; open: number | null; setOpen: Dispatch<SetStateAction<number | null>>; onAction: (user: AdminUser, action: UserAction) => void; disabled: boolean }) {
  const { context, can } = useSession();
  const reason = protectedReason(user, context?.user.id);
  if (reason) return <span className="mg-protected" title={reason}>受保护{user.id === context?.user.id ? ' · 本人' : ' · admin'}</span>;
  const actions = (['status', 'kick', 'delete'] as UserAction[]).filter(action => can(actionPermissions[action]));
  if (!actions.length) return <span className="mg-hint">无可用动作</span>;
  return <DropdownMenu modal={false} open={open === user.id} onOpenChange={value => setOpen(current => value ? user.id : current === user.id ? null : current)}><DropdownMenuTrigger asChild><Button variant="outline" disabled={disabled} aria-label={`${user.nickname || user.username}的操作`}>操作</Button></DropdownMenuTrigger><DropdownMenuContent className="mg-action-menu" side="right" align="start" sideOffset={8} avoidCollisions={false}>{actions.map(action => <div key={action}>{action === 'delete' && actions.length > 1 && <DropdownMenuSeparator />}<DropdownMenuItem disabled={disabled} className={action === 'delete' ? 'mg-danger' : ''} onSelect={() => { setOpen(null); onAction(user, action); }}>{action === 'status' ? user.status === 1 ? '停用用户' : '启用用户' : action === 'kick' ? '强制下线' : '删除用户'}</DropdownMenuItem></div>)}</DropdownMenuContent></DropdownMenu>;
}
export function UsersList() {
  const list = useListQuery();
  const { can } = useSession();
  const resource = useResource(`users:${list.query}`, () => api.get<Page<AdminUser>>(`/api/admin/users?${list.query}`));
  const actions = useUserActions((_user, action) => { if (action !== 'kick') resource.reload(); });
  const [open, setOpen] = useState<number | null>(null);
  const [compact, setCompact] = useState(() => window.matchMedia('(max-width: 1100px)').matches);
  useEffect(() => { const media = window.matchMedia('(max-width: 1100px)'); const change = () => { setCompact(media.matches); setOpen(null); }; media.addEventListener('change', change); return () => media.removeEventListener('change', change); }, []);
  useEffect(() => { const close = () => setOpen(null); window.addEventListener('scroll', close, true); return () => window.removeEventListener('scroll', close, true); }, []);
  useEffect(() => setOpen(null), [list.query, resource.loading, actions.busy]);
  const identity = (user: AdminUser) => <>{can('admin.users.id.get') ? <Link className="mg-list-link" to={`/users/${user.id}`}>{user.nickname || user.username}</Link> : <strong>{user.nickname || user.username}</strong>}<small className="mg-row-sub">{user.username}</small></>;
  const menu = (user: AdminUser) => <div className="mg-action-reserve"><UserMenu user={user} open={open} setOpen={setOpen} disabled={actions.busy} onAction={actions.choose} /></div>;
  const columns: ColumnDef<AdminUser>[] = [
    { id: 'user', header: '用户', cell: ({ row }) => identity(row.original) },
    { id: 'roles', header: '角色', cell: ({ row }) => row.original.roles.map(role => role.name).join('、') || '未分配角色' },
    { id: 'organizations', header: '范围内组织', cell: ({ row }) => <OrganizationsText user={row.original} /> },
    { id: 'status', header: '状态', cell: ({ row }) => <Status value={row.original.status} /> },
    { id: 'actions', header: '操作', cell: ({ row }) => menu(row.original) },
  ];
  return <><PageHeading title="用户管理" eyebrow="PEOPLE · 人员与角色" intro="核对真实角色、状态及当前范围内的组织归属。列表只统计可管理范围内的结果。" />{actions.feedback}<Card className="mg-list mg-user-list"><div className="mg-toolbar"><strong>范围内用户</strong><ListFilters list={list} label="搜索用户名或昵称" /></div><ResourceState {...resource} />{resource.data && <>{resource.data.list.length ? compact ? <div className="mg-user-cards">{resource.data.list.map(user => <article className="mg-person-card" key={user.id}><div className="mg-card-head"><div>{identity(user)}</div><Status value={user.status} /></div><p>角色 · {user.roles.map(role => role.name).join('、') || '未分配角色'}</p><p>可见组织 · <OrganizationsText user={user} /></p>{menu(user)}</article>)}</div> : <DataTable columns={columns} data={resource.data.list} caption="范围内真实用户列表" /> : <Notice title="没有符合条件的用户">换个用户名、昵称或状态试试。</Notice>}<Pagination list={list} total={resource.data.total} loading={resource.loading} /></>}</Card></>;
}
export function UserDetail() {
  const id = useObjectId();
  const resource = useResource(`user:${id}`, () => id ? api.get<AdminUser>(`/api/admin/users/${id}`) : Promise.reject(new Error('用户 ID 无效')));
  const navigate = useNavigate();
  const actions = useUserActions((_user, action) => { if (action === 'delete') navigate('/users', { state: { message: '用户已删除。' } }); else if (action === 'status') resource.reload(); });
  const { can, context } = useSession();
  const user = resource.data;
  const reason = user ? protectedReason(user, context?.user.id) : '';
  return <><PageHeading title="用户详情" eyebrow="USER · 用户资料" back={{ to: '/users', label: '返回用户列表' }} /><ResourceState {...resource} />{actions.feedback}{user && <><Card className="mg-detail-hero"><div><span className="mg-eyebrow">USER · 真实身份</span><h1>{user.nickname || user.username}</h1><p>{user.username} · {user.email || '未设置邮箱'}</p><code>用户 ID {user.id}</code></div><Status value={user.status} /></Card><div className="mg-info-grid"><Card className="mg-panel"><h2>已分配角色</h2>{user.roles.length ? <ul className="mg-detail-list">{user.roles.map(role => <li key={role.id}>{role.name}<code>{role.code}</code></li>)}</ul> : <p>未分配角色</p>}{!reason && can('admin.users.id.roles.put') && can('admin.roles.get') && <Button variant="outline" asChild><Link to={`/users/${user.id}/roles`}>编辑角色</Link></Button>}</Card><Card className="mg-panel"><h2>范围内组织归属</h2><OrganizationsText user={user} /><p className="mg-hint">这里只展示当前操作者可管理的组织，不表示用户的完整归属。</p>{!reason && can('admin.users.id.organizations.put') && can('admin.organizations.tree.get') && <Button variant="outline" asChild><Link to={`/users/${user.id}/organizations`}>编辑组织归属</Link></Button>}</Card></div>{reason ? <p className="mg-warning">{reason}</p> : <div className="mg-detail-actions">{(['status', 'kick', 'delete'] as UserAction[]).filter(action => can(actionPermissions[action])).map(action => <Button key={action} variant="outline" className={action === 'delete' ? 'mg-danger' : ''} disabled={actions.busy} onClick={() => actions.choose(user, action)}>{action === 'status' ? user.status === 1 ? '停用用户' : '启用用户' : action === 'kick' ? '强制下线' : '删除用户'}</Button>)}</div>}</>}</>;
}

interface UserEditData { user: AdminUser; roles: Role[]; tree: Organization[]; }
type EditKind = 'roles' | 'organizations';
export function UserEditor({ kind }: { kind: EditKind }) {
  const id = useObjectId();
  const { context } = useSession();
  const resource = useResource(`user-editor:${id}:${kind}`, async (): Promise<UserEditData> => {
    if (!id) throw new Error('用户 ID 无效');
    const user = await api.get<AdminUser>(`/api/admin/users/${id}`);
    if (protectedReason(user, context?.user.id)) return { user, roles: [], tree: [] };
    const [roles, tree] = await Promise.all([kind === 'roles' ? allPages<Role>('/api/admin/roles') : Promise.resolve([]), kind === 'organizations' ? api.get<Organization[]>('/api/admin/organizations/tree') : Promise.resolve([])]);
    return { user, roles, tree };
  });
  const reason = resource.data ? protectedReason(resource.data.user, context?.user.id) : '';
  return <><PageHeading title={`编辑用户${kind === 'roles' ? '角色' : '组织归属'}`} eyebrow="USER · 逐人修改" back={{ to: `/users/${id || ''}`, label: '返回用户详情' }} intro="只修改当前用户的这一项归属，其他用户及另一维度不会被覆盖。" /><ResourceState {...resource} />{resource.data && (reason ? <Notice title="用户受保护">{reason}</Notice> : <UserEditForm key={`${id}:${kind}`} data={resource.data} kind={kind} />)}</>;
}
function UserEditForm({ data, kind }: { data: UserEditData; kind: EditKind }) {
  const { context } = useSession();
  const initial = kind === 'roles' ? data.user.roles.map(role => role.id) : data.user.organizations.map(org => org.id);
  const [user, setUser] = useState(data.user);
  const [selected, setSelected] = useState(initial);
  const [roles, setRoles] = useState(data.roles);
  const [tree, setTree] = useState(data.tree);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [conflict, setConflict] = useState(false);
  const [latest, setLatest] = useState<AdminUser | null>(null);
  const [reviewed, setReviewed] = useState(false);
  const [reloading, setReloading] = useState(false);
  const navigation = useSavedNavigation(`/users/${data.user.id}`);
  const baseline = kind === 'roles' ? user.roles.map(role => role.id) : user.organizations.map(org => org.id);
  const options = kind === 'roles' ? [...roles, ...user.roles.filter(role => !roles.some(option => option.id === role.id)), ...selected.filter(id => !roles.some(role => role.id === id) && !user.roles.some(role => role.id === id)).map(id => ({ id, name: `角色 ID ${id}`, code: '已不在最新可选角色中，请移除', status: 0 }))] : flattenTree(tree);
  const dirty = !navigation.saved && (!sameIds(selected, baseline) || conflict);
  const reason = protectedReason(latest || user, context?.user.id);
  const invalid = kind === 'organizations' ? selected.some(id => !flattenTree(tree).some(node => node.id === id && node.manageable)) : selected.some(id => !roles.some(role => role.id === id && role.code !== 'admin'));
  const reload = async () => {
    if (reloading || busy) return;
    setReloading(true); setError(null); setReviewed(false); setLatest(null);
    try {
      const current = await api.get<AdminUser>(`/api/admin/users/${user.id}`);
      if (!protectedReason(current, context?.user.id)) {
        if (kind === 'roles') setRoles(await allPages<Role>('/api/admin/roles'));
        else setTree(await api.get<Organization[]>('/api/admin/organizations/tree'));
      }
      setLatest(current); setUser(current);
    } catch (failure) { setError(failure); } finally { setReloading(false); }
  };
  const save = async () => {
    if (busy || reloading || reason || invalid || (conflict && (!latest || !reviewed))) return;
    setBusy(true); setError(null);
    try {
      const body = kind === 'roles' ? { role_ids: selected, expected_access_version: user.access_version } : { organization_ids: selected, expected_access_version: user.access_version };
      await api.put<{ access_version: number }>(`/api/admin/users/${user.id}/${kind}`, body);
      setConfirm(false); navigation.finish();
    } catch (failure) { setError(failure); setConfirm(false); if (failure instanceof ApiError && failure.status === 409) { setConflict(true); setLatest(null); setReviewed(false); } } finally { setBusy(false); }
  };
  return <><UnsavedGuard dirty={dirty} busy={busy} /><ErrorNotice error={error} />{conflict && <Card className="mg-conflict"><h2>归属已被修改，请重载并核对</h2><p>您的选择已保留，不会自动覆盖其他管理员的更改。</p><Button variant="outline" disabled={reloading || busy} onClick={reload}>{reloading ? '正在重载…' : '重新读取最新资料'}</Button>{latest && <><h3>最新服务器归属 → 您保留的选择</h3><SelectionSummary before={baseline} after={selected} options={options} /><Choice checked={reviewed} disabled={!!reason} onChange={setReviewed} label="已核对最新归属，按当前选择重新提交" description={`最新授权版本 ${latest.access_version}`} /></>}</Card>}{reason && <p className="mg-warning">{reason}</p>}<div className="mg-edit-layout"><Card className="mg-panel"><h2>{user.nickname || user.username}</h2>{user.has_unmanaged_organizations && kind === 'organizations' && <p className="mg-warning">该用户还有不可见的组织归属。本次只编辑可管理集合，范围外关系由服务器保留，不会被移除。</p>}<fieldset disabled={busy || reloading || !!reason} className="mg-fieldset"><legend className="sr-only">选择{kind === 'roles' ? '角色' : '可管理组织'}</legend>{kind === 'roles' ? options.map(option => <Choice key={option.id} checked={selected.includes(option.id)} disabled={'code' in option && option.code === 'admin'} onChange={checked => { setSelected(toggleId(selected, option.id, checked)); setReviewed(false); }} label={option.name} description={'code' in option ? `${option.code}${option.code === 'admin' ? ' · 受保护，不可分配' : 'status' in option && option.status !== 1 ? ' · 已停用' : ''}` : undefined} />) : <OrganizationChoices tree={tree} selected={selected} onChange={ids => { setSelected(ids); setReviewed(false); }} disabled={busy || reloading || !!reason} />}</fieldset>{invalid && <p className="mg-warning" role="alert">选择中包含当前不可用或不可管理的{kind === 'roles' ? '角色' : '组织'}，请移除并重新核对。</p>}<div className="mg-edit-footer"><Button asChild variant="outline" disabled={busy}><Link to={`/users/${user.id}`}>取消</Link></Button><Button disabled={!dirty || busy || reloading || !!reason || invalid || (conflict && (!latest || !reviewed))} onClick={() => setConfirm(true)}>保存{kind === 'roles' ? '角色' : '组织归属'}</Button></div></Card><Card className="mg-panel mg-edit-aside"><h2>变更影响</h2><p>只保存此用户的{kind === 'roles' ? '角色' : '可管理组织集合'}。</p><p className="mg-hint">保存后目标用户的旧会话将在后续请求失效。空选择表示清空当前可编辑集合。</p></Card></div><Confirm open={confirm} onOpenChange={setConfirm} busy={busy} title={`确认更新${user.nickname || user.username}的${kind === 'roles' ? '角色' : '组织归属'}？`} onConfirm={save}><SelectionSummary before={baseline} after={selected} options={options} /><p>仅修改当前维度。目标用户旧会话将在后续请求失效。{kind === 'organizations' && '范围外组织归属不会被移除。'}</p></Confirm></>;
}
export function GuardedUserEditor({ kind }: { kind: EditKind }) {
  return <Guard module="users" permissions={['admin.users.id.get', `admin.users.id.${kind}.put`, kind === 'roles' ? 'admin.roles.get' : 'admin.organizations.tree.get']}><UserEditor kind={kind} /></Guard>;
}
