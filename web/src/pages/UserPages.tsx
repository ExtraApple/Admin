import { Fragment, useEffect, useState, type Dispatch, type SetStateAction } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { ColumnDef } from '@tanstack/react-table';
import { api, ApiError } from '@/lib/api';
import type { AdminUser, AdminUserCreateInput, DeletedAdminUser, Organization, Page, Role } from '@/lib/models';
import { passwordRequirementsText, validateNewPassword } from '@/lib/password-policy';
import { useSession } from '@/lib/session';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
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
      setMessage(pending.action === 'kick' ? '已强制下线；用户资料与状态未改变。' : pending.action === 'delete' ? '用户已软删除并移入已删除用户回收站。' : `用户已${status === 1 ? '启用' : '停用'}。`);
      onChanged(pending.user, pending.action, status); setPending(null);
    } catch (failure) { setError(failure); setPending(null); } finally { setBusy(false); }
  };
  const label = pending ? pending.action === 'status' ? pending.user.status === 1 ? '停用' : '启用' : pending.action === 'kick' ? '强制下线' : '删除' : '';
  const feedback = <><ErrorNotice error={error} />{message && <p className="mg-success" role="status">{message}</p>}<Confirm open={!!pending} onOpenChange={open => { if (!open) setPending(null); }} busy={busy} title={`确认${label}${pending?.user.nickname || pending?.user.username || ''}？`} label={`确认${label}`} onConfirm={execute}><p>对象：{pending?.user.nickname || pending?.user.username} · {pending?.user.username}</p><p>{pending?.action === 'delete' ? '此操作将软删除用户，不是停用：账号会移入已删除用户回收站，旧会话与待验证邮箱立即失效；在恢复或彻底注销前仍保留用户名与邮箱占用。' : pending?.action === 'kick' ? '使该用户的旧会话失效；资料和状态保持不变。' : '切换该用户启用状态；旧会话将在后续请求失效。'}</p></Confirm></>;
  return { choose, feedback, busy };
}
function UserMenu({ user, open, setOpen, onAction, disabled, align }: { user: AdminUser; open: number | null; setOpen: Dispatch<SetStateAction<number | null>>; onAction: (user: AdminUser, action: UserAction) => void; disabled: boolean; align: 'start' | 'end' }) {
  const { context, can } = useSession();
  const reason = protectedReason(user, context?.user.id);
  if (reason) return <span className="mg-protected" title={reason}>受保护{user.id === context?.user.id ? ' · 本人' : ' · admin'}</span>;
  const actions = (['status', 'kick', 'delete'] as UserAction[]).filter(action => can(actionPermissions[action]));
  if (!actions.length) return <span className="mg-hint">无可用动作</span>;
  return <DropdownMenu modal={false} open={open === user.id} onOpenChange={value => setOpen(current => value ? user.id : current === user.id ? null : current)}><DropdownMenuTrigger asChild><Button variant="outline" disabled={disabled} aria-label={`${user.nickname || user.username}的操作`}>操作</Button></DropdownMenuTrigger><DropdownMenuContent className="mg-action-menu" side="bottom" align={align}>{actions.map(action => <Fragment key={action}>{action === 'delete' && actions.length > 1 && <DropdownMenuSeparator />}<DropdownMenuItem disabled={disabled} className={action === 'delete' ? 'mg-danger' : ''} onSelect={() => { setOpen(null); onAction(user, action); }}>{action === 'status' ? user.status === 1 ? '停用用户' : '启用用户' : action === 'kick' ? '强制下线' : '删除用户'}</DropdownMenuItem></Fragment>)}</DropdownMenuContent></DropdownMenu>;
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
  const columns: ColumnDef<AdminUser>[] = [
    { id: 'user', header: '用户', cell: ({ row }) => identity(row.original) },
    { id: 'roles', header: '角色', cell: ({ row }) => row.original.roles.map(role => role.name).join('、') || '未分配角色' },
    { id: 'organizations', header: '范围内组织', cell: ({ row }) => <OrganizationsText user={row.original} /> },
    { id: 'status', header: '状态', cell: ({ row }) => <Status value={row.original.status} /> },
    { id: 'actions', header: '操作', cell: ({ row }) => <UserMenu user={row.original} open={open} setOpen={setOpen} disabled={actions.busy} onAction={actions.choose} align="end" /> },
  ];
  const headingActions = (can('admin.deleted-users.get') || can('admin.users.post')) ? <>{can('admin.deleted-users.get') && <Button variant="outline" asChild><Link to="/users/deleted">已删除用户</Link></Button>}{can('admin.users.post') && <Button asChild><Link to="/users/new">新建用户</Link></Button>}</> : undefined;
  return <><PageHeading title="用户管理" intro="核对真实角色、状态及当前范围内的组织归属。列表只统计可管理范围内的结果。" actions={headingActions} />{actions.feedback}<Card className="mg-list mg-user-list"><div className="mg-toolbar"><strong>范围内用户</strong><ListFilters list={list} label="搜索用户名或昵称" /></div><ResourceState {...resource} />{resource.data && <>{resource.data.list.length ? compact ? <div className="mg-user-cards">{resource.data.list.map(user => <article className="mg-person-card" key={user.id}><div className="mg-card-head"><div>{identity(user)}</div><Status value={user.status} /></div><p>角色 · {user.roles.map(role => role.name).join('、') || '未分配角色'}</p><p>可见组织 · <OrganizationsText user={user} /></p><div className="mg-card-actions"><UserMenu user={user} open={open} setOpen={setOpen} disabled={actions.busy} onAction={actions.choose} align="start" /></div></article>)}</div> : <DataTable columns={columns} data={resource.data.list} caption="范围内真实用户列表" /> : <Notice title="没有符合条件的用户">换个用户名、昵称或状态试试。</Notice>}<Pagination list={list} total={resource.data.total} loading={resource.loading} /></>}</Card></>;
}

export function UserCreate() {
  const { can } = useSession();
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [nickname, setNickname] = useState('');
  const [status, setStatus] = useState(1);
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [validationError, setValidationError] = useState('');
  const navigation = useSavedNavigation('/users');
  const dirty = !navigation.saved && (username !== '' || email !== '' || nickname !== '' || status !== 1 || password !== '' || confirmPassword !== '');

  const submit = async () => {
    if (busy) return;
    const usernameLength = [...username].length;
    if (!username.trim() || usernameLength < 3 || usernameLength > 100) {
      setValidationError('用户名需要 3–100 个字符，且不能全为空白。');
      return;
    }
    if (!email.trim() || [...email].length > 100) {
      setValidationError('请填写有效的邮箱地址（不超过 100 个字符）。');
      return;
    }
    if (nickname !== '' && (!nickname.trim() || [...nickname].length > 100)) {
      setValidationError('昵称不能全为空白，且不能超过 100 个字符。');
      return;
    }
    const passwordIssue = validateNewPassword(password);
    if (passwordIssue) {
      setValidationError(passwordIssue);
      return;
    }
    if (password !== confirmPassword) {
      setValidationError('两次输入的密码不一致。');
      return;
    }
    setValidationError('');
    setError(null);
    setBusy(true);
    try {
      const payload: AdminUserCreateInput = { username, email, password, nickname, status };
      const created = await api.post<AdminUser>('/api/admin/users', payload);
      navigation.finish(can('admin.users.id.get') ? `/users/${created.id}` : '/users', '用户已创建。');
    } catch (failure) {
      setError(failure);
    } finally {
      setBusy(false);
    }
  };

  return <>
    <PageHeading title="新建用户" back={{ to: '/users', label: '返回用户列表' }} intro="创建纯用户身份；仅全量数据范围管理员可提交，创建后可前往用户详情页分别配置角色与组织归属。" />
    <UnsavedGuard dirty={dirty} busy={busy} />
    <Card className="mg-panel mg-create-panel">
      <form className="mg-menu-form" onSubmit={event => { event.preventDefault(); void submit(); }}>
        <fieldset disabled={busy} className="mg-fieldset">
          <legend className="sr-only">新建用户信息</legend>
          <div className="mg-form-grid">
            <label>用户名<Input value={username} required minLength={3} maxLength={100} autoComplete="off" placeholder="3–100 个字符" onChange={event => setUsername(event.target.value)} /></label>
            <label>邮箱<Input type="email" value={email} required maxLength={100} autoComplete="off" placeholder="name@example.com" onChange={event => setEmail(event.target.value)} /></label>
            <label>昵称<Input value={nickname} maxLength={100} placeholder="可选" onChange={event => setNickname(event.target.value)} /></label>
            <label>状态<select value={status} onChange={event => setStatus(Number(event.target.value))}><option value={1}>启用</option><option value={0}>停用</option></select></label>
            <label>初始密码<Input type="password" value={password} required minLength={6} autoComplete="new-password" onChange={event => setPassword(event.target.value)} /></label>
            <label>确认密码<Input type="password" value={confirmPassword} required minLength={6} autoComplete="new-password" onChange={event => setConfirmPassword(event.target.value)} /></label>
          </div>
          <p className="mg-hint">{passwordRequirementsText}</p>
          <p className="mg-hint">新建账号不会自动发送验证邮件，初始邮箱为未验证状态；角色与组织归属需创建后在详情页按权限分别分配。</p>
        </fieldset>
        <ErrorNotice error={error} />
        {validationError && <div className="mg-error" role="alert"><p>{validationError}</p></div>}
        <div className="mg-edit-footer">
          <Button asChild variant="outline" disabled={busy}><Link to="/users">取消</Link></Button>
          <Button type="submit" disabled={busy}>{busy ? '正在创建…' : '创建用户'}</Button>
        </div>
      </form>
    </Card>
  </>;
}
export function UserDetail() {
  const id = useObjectId();
  const resource = useResource(`user:${id}`, () => id ? api.get<AdminUser>(`/api/admin/users/${id}`) : Promise.reject(new Error('用户 ID 无效')));
  const navigate = useNavigate();
  const actions = useUserActions((_user, action) => { if (action === 'delete') navigate('/users', { state: { message: '用户已软删除并移入已删除用户回收站。' } }); else if (action === 'status') resource.reload(); });
  const { can, context } = useSession();
  const user = resource.data;
  const reason = user ? protectedReason(user, context?.user.id) : '';
  return <><PageHeading title="用户详情" back={{ to: '/users', label: '返回用户列表' }} /><ResourceState {...resource} />{actions.feedback}{user && <><Card className="mg-detail-hero"><div><span className="mg-eyebrow">用户身份</span><h1>{user.nickname || user.username}</h1><p>{user.username} · {user.email || '未设置邮箱'}</p><code>用户 ID {user.id}</code></div><Status value={user.status} /></Card><div className="mg-info-grid"><Card className="mg-panel"><h2>已分配角色</h2>{user.roles.length ? <ul className="mg-detail-list">{user.roles.map(role => <li key={role.id}>{role.name}<code>{role.code}</code></li>)}</ul> : <p>未分配角色</p>}{!reason && can('admin.users.id.roles.put') && can('admin.roles.get') && <Button variant="outline" asChild><Link to={`/users/${user.id}/roles`}>编辑角色</Link></Button>}</Card><Card className="mg-panel"><h2>范围内组织归属</h2><OrganizationsText user={user} /><p className="mg-hint">这里只展示当前操作者可管理的组织，不表示用户的完整归属。</p>{!reason && can('admin.users.id.organizations.put') && can('admin.organizations.tree.get') && <Button variant="outline" asChild><Link to={`/users/${user.id}/organizations`}>编辑组织归属</Link></Button>}</Card></div>{reason ? <p className="mg-warning">{reason}</p> : <div className="mg-detail-actions">{(['status', 'kick', 'delete'] as UserAction[]).filter(action => can(actionPermissions[action])).map(action => <Button key={action} variant="outline" className={action === 'delete' ? 'mg-danger' : ''} disabled={actions.busy} onClick={() => actions.choose(user, action)}>{action === 'status' ? user.status === 1 ? '停用用户' : '启用用户' : action === 'kick' ? '强制下线' : '删除用户'}</Button>)}</div>}</>}</>;
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
  return <><PageHeading title={`编辑用户${kind === 'roles' ? '角色' : '组织归属'}`} back={{ to: `/users/${id || ''}`, label: '返回用户详情' }} intro="只修改当前用户的这一项归属，其他用户及另一维度不会被覆盖。" /><ResourceState {...resource} />{resource.data && (reason ? <Notice title="用户受保护">{reason}</Notice> : <UserEditForm key={`${id}:${kind}`} data={resource.data} kind={kind} />)}</>;
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

function formatDeletedAt(value: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

export function DeletedUsersList() {
  const list = useListQuery();
  const { context, can } = useSession();
  const [draftKeyword, setDraftKeyword] = useState(list.keyword);
  useEffect(() => setDraftKeyword(list.keyword), [list.keyword]);

  const params = new URLSearchParams({ page: String(list.page), size: String(list.size) });
  if (list.keyword) params.set('keyword', list.keyword);
  const query = params.toString();
  const resource = useResource(`deleted-users:${query}`, () => api.get<Page<DeletedAdminUser>>(`/api/admin/deleted-users?${query}`));

  const [compact, setCompact] = useState(() => window.matchMedia('(max-width: 1100px)').matches);
  useEffect(() => {
    const media = window.matchMedia('(max-width: 1100px)');
    const change = () => setCompact(media.matches);
    media.addEventListener('change', change);
    return () => media.removeEventListener('change', change);
  }, []);

  const [restoreTarget, setRestoreTarget] = useState<DeletedAdminUser | null>(null);
  const [anonymizeTarget, setAnonymizeTarget] = useState<DeletedAdminUser | null>(null);
  const [confirmUsername, setConfirmUsername] = useState('');
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [dialogError, setDialogError] = useState<unknown>(null);
  const [feedback, setFeedback] = useState<{ text: string; restoredId?: number } | null>(null);

  const canRestore = can('admin.users.id.restore.post');
  const canAnonymize = can('admin.users.id.anonymize.post');
  const canViewDetail = can('admin.users.id.get');
  const scopeDenied = resource.error instanceof ApiError && resource.error.status === 403 && resource.error.errorCode === 'IDENTITY_PERMISSION_DENIED';

  const refreshAfterMutation = () => {
    if (resource.data && resource.data.list.length <= 1 && list.page > 1) {
      list.update({ page: list.page - 1 });
    } else {
      resource.reload();
    }
  };

  const openRestore = (user: DeletedAdminUser) => {
    if (busy || user.protected || user.id === context?.user.id || !canRestore) return;
    setActionError(null);
    setDialogError(null);
    setFeedback(null);
    setRestoreTarget(user);
  };

  const openAnonymize = (user: DeletedAdminUser) => {
    if (busy || user.protected || user.id === context?.user.id || !canAnonymize) return;
    setActionError(null);
    setDialogError(null);
    setFeedback(null);
    setConfirmUsername('');
    setAnonymizeTarget(user);
  };

  const executeRestore = async () => {
    if (busy || !restoreTarget) return;
    setBusy(true);
    setActionError(null);
    setDialogError(null);
    try {
      const restored = await api.post<AdminUser>(`/api/admin/users/${restoreTarget.id}/restore`, undefined);
      setFeedback({ text: `已恢复用户 ${restored.nickname || restored.username}（${restored.username}）。`, restoredId: restored.id });
      setRestoreTarget(null);
      refreshAfterMutation();
    } catch (failure) {
      setDialogError(failure);
    } finally {
      setBusy(false);
    }
  };

  const executeAnonymize = async () => {
    if (busy || !anonymizeTarget || confirmUsername !== anonymizeTarget.username) return;
    setBusy(true);
    setActionError(null);
    setDialogError(null);
    try {
      await api.post<void>(`/api/admin/users/${anonymizeTarget.id}/anonymize`, { confirm_username: confirmUsername });
      setFeedback({ text: `已彻底注销（匿名化）${anonymizeTarget.username}，原用户名与邮箱已释放。` });
      setAnonymizeTarget(null);
      setConfirmUsername('');
      refreshAfterMutation();
    } catch (failure) {
      setDialogError(failure);
    } finally {
      setBusy(false);
    }
  };

  const renderActions = (user: DeletedAdminUser) => {
    if (user.protected || user.id === context?.user.id) {
      return <span className="mg-protected" title="受保护身份由系统初始化规则托管，不支持在回收站恢复或彻底注销">受保护 · {user.id === context?.user.id ? '本人' : 'admin'}（系统托管）</span>;
    }
    if (!canRestore && !canAnonymize) {
      return <span className="mg-hint">无可用动作</span>;
    }
    return <div className="mg-deleted-actions">
      {canRestore && <Button variant="outline" disabled={busy} onClick={() => openRestore(user)}>恢复用户</Button>}
      {canAnonymize && <Button variant="outline" className="mg-danger" disabled={busy} onClick={() => openAnonymize(user)}>彻底注销</Button>}
    </div>;
  };

  const columns: ColumnDef<DeletedAdminUser>[] = [
    { id: 'user', header: '用户', cell: ({ row }) => <><strong>{row.original.nickname || row.original.username}</strong><small className="mg-row-sub">{row.original.username} · {row.original.email}</small></> },
    { id: 'deleted_at', header: '删除时间', cell: ({ row }) => <span className="mg-num">{formatDeletedAt(row.original.deleted_at)}</span> },
    { id: 'roles', header: '保留角色', cell: ({ row }) => row.original.roles.map(role => role.name).join('、') || '无' },
    { id: 'organizations', header: '保留组织', cell: ({ row }) => row.original.organizations.map(org => org.name).join('、') || '无' },
    { id: 'actions', header: '操作', cell: ({ row }) => renderActions(row.original) },
  ];

  return <>
    <PageHeading
      title="已删除用户"
      back={{ to: '/users', label: '返回用户列表' }}
      intro="展示处于可恢复软删除状态的用户（仅限全量数据范围管理员访问）。恢复将沿用已失效的授权版本；彻底注销（匿名化）会释放原用户名与邮箱并保留墓碑以维持审计、文件与消息引用完整。"
    />
    <ErrorNotice error={actionError} />
    {feedback && <p className="mg-success" role="status">
      {feedback.text}
      {feedback.restoredId && canViewDetail && <> <Link to={`/users/${feedback.restoredId}`}>查看用户详情</Link></>}
    </p>}
    <Card className="mg-list mg-deleted-user-list">
      <div className="mg-toolbar">
        <strong>可恢复软删除用户</strong>
        <form className="mg-filters" onSubmit={event => { event.preventDefault(); list.update({ keyword: draftKeyword.trim(), page: 1 }); }}>
          <label className="sr-only" htmlFor="deleted-users-search">搜索用户名、昵称或邮箱</label>
          <Input id="deleted-users-search" type="search" value={draftKeyword} onChange={event => setDraftKeyword(event.target.value)} placeholder="搜索用户名、昵称或邮箱" />
          <Button type="submit" variant="outline">搜索</Button>
        </form>
      </div>
      {scopeDenied ? (
        <Notice title="需要全量数据范围权限">
          <p>已删除用户回收站仅限具备全量（all）数据范围的管理员查看与操作。当前账号不在全量数据范围内，无法读取或处置已软删除用户。</p>
        </Notice>
      ) : (
        <>
          <ResourceState {...resource} />
          {resource.data && <>
            {resource.data.list.length ? compact ? (
              <div className="mg-user-cards">
                {resource.data.list.map(user => (
                  <article className="mg-person-card" key={user.id}>
                    <div className="mg-card-head">
                      <div>
                        <strong>{user.nickname || user.username}</strong>
                        <small className="mg-row-sub">{user.username} · {user.email}</small>
                      </div>
                      <Status value={user.status} />
                    </div>
                    <p>删除时间 · <span className="mg-num">{formatDeletedAt(user.deleted_at)}</span></p>
                    <p>保留角色 · {user.roles.map(role => role.name).join('、') || '无'}</p>
                    <p>保留组织 · {user.organizations.map(org => org.name).join('、') || '无'}</p>
                    <div className="mg-card-actions">{renderActions(user)}</div>
                  </article>
                ))}
              </div>
            ) : (
              <DataTable columns={columns} data={resource.data.list} caption="已软删除用户列表" />
            ) : (
              <Notice title="没有可恢复的已删除用户">当前没有处于可恢复软删除状态的匹配记录。已完成彻底注销（匿名化）的墓碑记录不会在此显示。</Notice>
            )}
            <Pagination list={list} total={resource.data.total} loading={resource.loading} />
          </>}
        </>
      )}
    </Card>

    <Confirm
      open={!!restoreTarget}
      onOpenChange={open => { if (!open && !busy) { setRestoreTarget(null); setDialogError(null); } }}
      busy={busy}
      title={`确认恢复 ${restoreTarget?.nickname || restoreTarget?.username || ''}？`}
      label="确认恢复"
      onConfirm={executeRestore}
    >
      <p>对象：{restoreTarget?.nickname || restoreTarget?.username} · {restoreTarget?.username}（{restoreTarget?.email}）</p>
      <p>恢复后该用户将回到正常用户列表，保留其当前状态与仍存在的角色/组织关系。</p>
      <p>删除前签发的登录会话与邮箱验证链接仍保持永久失效，用户需重新登录；软删除期间已被清理的角色或组织归属不会自动恢复。</p>
      <ErrorNotice error={dialogError} />
    </Confirm>

    <Confirm
      open={!!anonymizeTarget}
      onOpenChange={open => { if (!open && !busy) { setAnonymizeTarget(null); setConfirmUsername(''); setDialogError(null); } }}
      busy={busy}
      confirmDisabled={!anonymizeTarget || confirmUsername !== anonymizeTarget.username}
      title={`确认彻底注销（匿名化）${anonymizeTarget?.nickname || anonymizeTarget?.username || ''}？`}
      label="确认彻底注销"
      onConfirm={executeAnonymize}
    >
      <p>对象：{anonymizeTarget?.nickname || anonymizeTarget?.username} · <code>{anonymizeTarget?.username}</code>（{anonymizeTarget?.email}）</p>
      <div className="mg-warning">
        <p>此操作不可撤销，执行后无法恢复该用户：</p>
        <ul className="mg-confirm-list">
          <li>改写用户名与邮箱为不可逆墓碑占位，立即释放原用户名与原邮箱供重新使用；</li>
          <li>清除密码、昵称、待验证邮箱、未消费验证凭据，并删除私有存储中的可信自定义头像；</li>
          <li>移除该用户全部角色与组织归属，并提升且保留授权版本记录；</li>
          <li>保留用户 ID 墓碑以维持历史审计日志、文件与消息引用完整（不会改写历史审计日志）。</li>
        </ul>
      </div>
      <label className="mg-confirm-input">
        <span>请输入目标用户名 <code>{anonymizeTarget?.username}</code> 以解锁提交</span>
        <Input
          value={confirmUsername}
          autoComplete="off"
          disabled={busy}
          placeholder={anonymizeTarget?.username || ''}
          onChange={event => setConfirmUsername(event.target.value)}
        />
      </label>
      <ErrorNotice error={dialogError} />
    </Confirm>
  </>;
}
