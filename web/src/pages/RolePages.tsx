import { useEffect, useRef, useState, type Dispatch, type ReactNode, type SetStateAction } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { ColumnDef } from '@tanstack/react-table';
import { api } from '@/lib/api';
import type { DataScope, Menu, Organization, Page, Permission, Role, RoleCreateInput, UserInfo } from '@/lib/models';
import { useSession } from '@/lib/session';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { allPages, Choice, Confirm, DataTable, ErrorNotice, flattenTree, Guard, Notice, OrganizationChoices, PageHeading, Pagination, ResourceState, sameIds, scopeLabels, SelectionSummary, Status, toggleId, UnsavedGuard, useListQuery, useObjectId, useResource, useSavedNavigation } from '@/components/management/shared';

const createRoleScopeOptions: { value: RoleCreateInput['data_scope']; label: string }[] = [
  { value: 'self', label: scopeLabels.self },
  { value: 'org', label: scopeLabels.org },
  { value: 'org_and_children', label: scopeLabels.org_and_children },
  { value: 'all', label: scopeLabels.all },
];

type RoleUsersCheck = { status: 'idle' | 'loading' | 'ready' | 'error'; count: number };

function useRoleDelete(onDeleted: (role: Role) => void) {
  const { can } = useSession();
  const [pending, setPending] = useState<Role | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [message, setMessage] = useState('');
  const [affectedUsers, setAffectedUsers] = useState<RoleUsersCheck>({ status: 'idle', count: 0 });
  const lookupSerial = useRef(0);

  const close = () => {
    if (busy) return;
    lookupSerial.current += 1;
    setPending(null);
    setAffectedUsers({ status: 'idle', count: 0 });
  };

  const choose = (role: Role) => {
    if (busy || pending || role.code === 'admin' || !can('admin.roles.id.delete')) return;
    setError(null);
    setMessage('');
    setPending(role);
    const serial = ++lookupSerial.current;
    if (!can('admin.roles.id.users.get')) {
      setAffectedUsers({ status: 'idle', count: 0 });
      return;
    }
    setAffectedUsers({ status: 'loading', count: 0 });
    api.get<UserInfo[]>(`/api/admin/roles/${role.id}/users`).then(
      users => { if (lookupSerial.current === serial) setAffectedUsers({ status: 'ready', count: users.length }); },
      () => { if (lookupSerial.current === serial) setAffectedUsers({ status: 'error', count: 0 }); },
    );
  };

  const execute = async () => {
    if (busy || !pending || pending.code === 'admin' || !can('admin.roles.id.delete')) return;
    setBusy(true);
    setError(null);
    const target = pending;
    try {
      await api.delete(`/api/admin/roles/${target.id}`);
      lookupSerial.current += 1;
      setMessage('角色已删除。');
      setPending(null);
      setAffectedUsers({ status: 'idle', count: 0 });
      onDeleted(target);
    } catch (failure) {
      setError(failure);
      setPending(null);
      setAffectedUsers({ status: 'idle', count: 0 });
    } finally {
      setBusy(false);
    }
  };

  const feedback = <>
    <ErrorNotice error={error} />
    {message && <p className="mg-success" role="status">{message}</p>}
    <Confirm open={!!pending} onOpenChange={open => { if (!open) close(); }} busy={busy} title={`确认删除角色「${pending?.name || ''}」？`} label="确认删除" onConfirm={execute}>
      <p>对象：{pending?.name} · <code>{pending?.code}</code></p>
      {affectedUsers.status === 'loading' && <p role="status">正在核对当前绑定该角色的用户数…</p>}
      {affectedUsers.status === 'ready' && <p>当前绑定用户：<strong>{affectedUsers.count}</strong> 人。</p>}
      {affectedUsers.status === 'error' && <p className="mg-warning-inline">暂时无法核对当前绑定用户数，请谨慎确认后再删除。</p>}
      {affectedUsers.status === 'idle' && <p className="mg-hint">当前账号缺少角色成员读取权限，无法展示绑定用户数。</p>}
      <p>此操作将永久硬删除该角色，并级联解除该角色与用户、权限、菜单及数据范围的关联；原关联用户的旧会话将在后续请求失效，无法撤销。</p>
    </Confirm>
  </>;

  return { choose, feedback, busy };
}

function RoleMenu({ role, open, setOpen, onDelete, disabled, align }: { role: Role; open: number | null; setOpen: Dispatch<SetStateAction<number | null>>; onDelete: (role: Role) => void; disabled: boolean; align: 'start' | 'end' }) {
  const { can } = useSession();
  if (role.code === 'admin') return <span className="mg-protected" title="admin 角色受系统保护，不允许删除。">受保护 · admin</span>;
  if (!can('admin.roles.id.delete')) return <span className="mg-hint">无可用动作</span>;
  return <DropdownMenu modal={false} open={open === role.id} onOpenChange={value => setOpen(current => value ? role.id : current === role.id ? null : current)}>
    <DropdownMenuTrigger asChild><Button variant="outline" disabled={disabled} aria-label={`${role.name}的操作`}>操作</Button></DropdownMenuTrigger>
    <DropdownMenuContent className="mg-action-menu" side="bottom" align={align}>
      <DropdownMenuItem disabled={disabled} className="mg-danger" onSelect={() => { setOpen(null); onDelete(role); }}>删除角色</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>;
}

export function RolesList() {
  const list = useListQuery();
  const { can } = useSession();
  const resource = useResource(`roles:${list.page}:${list.size}`, () => api.get<Page<Role>>(`/api/admin/roles?page=${list.page}&size=${list.size}`));
  const deleter = useRoleDelete(() => {
    if (resource.data && resource.data.list.length <= 1 && list.page > 1) list.update({ page: list.page - 1 });
    else resource.reload();
  });
  const [open, setOpen] = useState<number | null>(null);
  const [compact, setCompact] = useState(() => typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 760px)').matches);
  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
    const media = window.matchMedia('(max-width: 760px)');
    const change = () => { setCompact(media.matches); setOpen(null); };
    media.addEventListener('change', change);
    return () => media.removeEventListener('change', change);
  }, []);
  useEffect(() => { const close = () => setOpen(null); window.addEventListener('scroll', close, true); return () => window.removeEventListener('scroll', close, true); }, []);
  useEffect(() => setOpen(null), [list.page, list.size, resource.loading, deleter.busy]);
  const columns: ColumnDef<Role>[] = [
    { id: 'name', header: '角色', cell: ({ row }) => <>{can('admin.roles.id.get') ? <Link className="mg-list-link" to={`${row.original.id}`}>{row.original.name}</Link> : <strong>{row.original.name}</strong>}<small className="mg-row-sub">{row.original.description || '无描述'}</small></> },
    { accessorKey: 'code', header: '角色编码', cell: ({ row }) => <code>{row.original.code}</code> },
    { id: 'status', header: '状态', cell: ({ row }) => <Status value={row.original.status} /> },
    { id: 'scope', header: '数据范围', cell: ({ row }) => scopeLabels[row.original.data_scope] || row.original.data_scope },
    { id: 'actions', header: '操作', cell: ({ row }) => <RoleMenu role={row.original} open={open} setOpen={setOpen} disabled={deleter.busy} onDelete={deleter.choose} align="end" /> },
  ];
  return <>
    <PageHeading title="角色管理" intro="从角色进入详情，分别核对权限、已分配菜单与数据范围。" actions={can('admin.roles.post') && <Button asChild><Link to="/roles/new">新建角色</Link></Button>} />
    {deleter.feedback}
    <Card className="mg-list mg-role-list">
      <div className="mg-toolbar"><strong>角色列表</strong><span className="mg-hint">各授权维度独立保存</span></div>
      <ResourceState {...resource} />
      {resource.data && <>
        {resource.data.list.length ? compact ? <div className="mg-mobile-cards">{resource.data.list.map(role => <article className="mg-person-card" key={role.id}>
          <div className="mg-card-head"><div><h2>{can('admin.roles.id.get') ? <Link to={`${role.id}`}>{role.name}</Link> : role.name}</h2><p>{role.description || '无描述'}</p></div><Status value={role.status} /></div>
          <code>{role.code}</code>
          <p>数据范围 · {scopeLabels[role.data_scope] || role.data_scope}</p>
          <div className="mg-card-actions"><RoleMenu role={role} open={open} setOpen={setOpen} disabled={deleter.busy} onDelete={deleter.choose} align="start" /></div>
        </article>)}</div> : <DataTable columns={columns} data={resource.data.list} caption="真实角色分页列表" /> : <Notice title="暂无角色">当前页没有角色，请调整页码。</Notice>}
        <Pagination list={list} total={resource.data.total} loading={resource.loading} />
      </>}
    </Card>
  </>;
}

export function RoleCreate() {
  const { can } = useSession();
  const [name, setName] = useState('');
  const [code, setCode] = useState('');
  const [description, setDescription] = useState('');
  const [sort, setSort] = useState(0);
  const [status, setStatus] = useState(1);
  const [dataScope, setDataScope] = useState<RoleCreateInput['data_scope']>('self');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [validationError, setValidationError] = useState('');
  const navigation = useSavedNavigation('/roles');
  const dirty = !navigation.saved && (name !== '' || code !== '' || description !== '' || sort !== 0 || status !== 1 || dataScope !== 'self');

  const submit = async () => {
    if (busy) return;
    const trimmedName = name.trim();
    const trimmedCode = code.trim();
    if (!trimmedName) { setValidationError('请填写角色名称。'); return; }
    if (!trimmedCode) { setValidationError('请填写角色编码。'); return; }
    setValidationError('');
    setError(null);
    setBusy(true);
    try {
      const payload: RoleCreateInput = {
        name: trimmedName,
        code: trimmedCode,
        description: description.trim(),
        sort,
        status,
        data_scope: dataScope,
      };
      const created = await api.post<Role>('/api/admin/roles', payload);
      navigation.finish(can('admin.roles.id.get') ? `/roles/${created.id}` : '/roles', '角色已创建。');
    } catch (failure) {
      setError(failure);
    } finally {
      setBusy(false);
    }
  };

  return <>
    <PageHeading title="新建角色" back={{ to: '/roles', label: '返回角色列表' }} intro="创建角色基础身份与初始数据范围；权限、菜单及自定义组织范围在创建后前往角色详情页分别配置。" />
    <UnsavedGuard dirty={dirty} busy={busy} />
    <Card className="mg-panel mg-create-panel">
      <form className="mg-menu-form" onSubmit={event => { event.preventDefault(); void submit(); }}>
        <fieldset disabled={busy} className="mg-fieldset">
          <legend className="sr-only">新建角色信息</legend>
          <div className="mg-form-grid">
            <label>角色名称<Input value={name} required maxLength={50} placeholder="如 运维工程师" onChange={event => setName(event.target.value)} /></label>
            <label>角色编码<Input value={code} required maxLength={50} placeholder="如 ops_engineer" onChange={event => setCode(event.target.value)} /></label>
            <label className="mg-form-span">描述<Input value={description} maxLength={255} placeholder="可选，说明该角色职责" onChange={event => setDescription(event.target.value)} /></label>
            <label>排序<Input type="number" value={sort} onChange={event => setSort(Number(event.target.value) || 0)} /></label>
            <label>状态<select value={status} onChange={event => setStatus(Number(event.target.value))}><option value={1}>启用</option><option value={0}>停用</option></select></label>
            <div className="mg-form-span">
              <p className="mg-hint">初始数据范围（默认仅本人）</p>
              <RadioGroup value={dataScope} onValueChange={value => setDataScope(value as RoleCreateInput['data_scope'])} className="mg-scope-options">
                {createRoleScopeOptions.map(option => <label className="mg-radio-choice" key={option.value}><RadioGroupItem value={option.value} id={`create-role-scope-${option.value}`} /><span>{option.label}</span></label>)}
              </RadioGroup>
              <p className="mg-hint">自定义指定组织单位（custom）需在角色创建后，进入角色详情页的「编辑数据范围」中勾选组织。</p>
            </div>
          </div>
        </fieldset>
        <ErrorNotice error={error} />
        {validationError && <div className="mg-error" role="alert"><p>{validationError}</p></div>}
        <div className="mg-edit-footer">
          <Button asChild variant="outline" disabled={busy}><Link to="/roles">取消</Link></Button>
          <Button type="submit" disabled={busy}>{busy ? '正在创建…' : '创建角色'}</Button>
        </div>
      </form>
    </Card>
  </>;
}

export function RoleDetail() {
  const id = useObjectId();
  const resource = useResource(`role:${id}`, () => id ? api.get<Role>(`/api/admin/roles/${id}`) : Promise.reject(new Error('角色 ID 无效')));
  return <><PageHeading title="角色详情" back={{ to: '/roles', label: '返回角色列表' }} /><ResourceState {...resource} />{resource.data && <RoleProfile role={resource.data} />}</>;
}
function RoleProfile({ role }: { role: Role }) {
  const { can } = useSession();
  const navigate = useNavigate();
  const deleter = useRoleDelete(() => navigate('/roles', { state: { message: '角色已删除。' } }));
  const permissions = useResource(`role-permissions:${role.id}`, () => can('admin.roles.id.permissions.get') ? api.get<Permission[]>(`/api/admin/roles/${role.id}/permissions`) : Promise.resolve(null));
  const menus = useResource(`role-menus:${role.id}`, () => can('admin.roles.id.menus.get') ? api.get<Menu[]>(`/api/admin/roles/${role.id}/menus`) : Promise.resolve(null));
  const scope = useResource(`role-scope:${role.id}`, () => can('admin.roles.id.data-scope.get') ? api.get<DataScope>(`/api/admin/roles/${role.id}/data-scope`) : Promise.resolve(null));
  const needsTree = scope.data?.data_scope === 'custom' && can('admin.organizations.tree.get');
  const tree = useResource(`role-tree:${role.id}:${needsTree}`, () => needsTree ? api.get<Organization[]>('/api/admin/organizations/tree') : Promise.resolve([]));
  const protectedRole = role.code === 'admin';
  const permissionCodes = permissions.data ? permissions.data.map(permission => permission.code) : null;
  const section = (title: string, intro: string, kind: string, children: ReactNode, allowed: boolean) => <Card className="mg-profile-card"><h2>{title}</h2><p className="mg-hint">{intro}</p>{children}<div className="mg-card-action">{!protectedRole && allowed && <Button variant="outline" asChild><Link to={`/roles/${role.id}/${kind}`}>编辑{title}</Link></Button>}</div></Card>;
  return <>{deleter.feedback}<Card className="mg-detail-hero"><div><span className="mg-eyebrow">角色身份</span><h1>{role.name}</h1><p>{role.description || '无描述'}</p><code>{role.code}</code></div><Status value={role.status} /></Card><div className="mg-profile-grid">{section('权限', '可执行操作与稳定权限码', 'permissions', <>{can('admin.roles.id.permissions.get') ? <><ResourceState {...permissions} />{permissions.data && (permissions.data.length ? <ul className="mg-detail-list">{permissions.data.map(permission => <li key={permission.id}>{permission.name}<code>{permission.code}</code></li>)}</ul> : <p>尚未分配权限</p>)}</> : <p className="mg-hint">没有读取该维度的权限。</p>}</>, can('admin.roles.id.permissions.get') && can('admin.roles.id.permissions.post') && can('admin.permissions.get'))}{section('已分配菜单', '分配不等于每位用户最终可见', 'menus', <>{can('admin.roles.id.menus.get') ? <><ResourceState {...menus} />{menus.data && (menus.data.length ? <ul className="mg-detail-list">{menus.data.map(menu => <li key={menu.id}>{menu.name}<Status value={menu.status} /><code>{menu.path || '目录'} · {menu.permission_code || '无必需权限码'}</code>{menu.status !== 1 && <small className="mg-warning-inline">菜单已停用</small>}{!protectedRole && menu.permission_code && permissionCodes && !permissionCodes.includes(menu.permission_code) && <small className="mg-warning-inline">当前角色缺少该菜单所需权限码</small>}</li>)}</ul> : <p>尚未分配菜单</p>)}</> : <p className="mg-hint">没有读取该维度的权限。</p>}<p className="mg-hint">其他角色仍可能提供所需权限；这里不计算用户的实际可见菜单。</p></>, can('admin.roles.id.menus.get') && can('admin.roles.id.menus.post') && can('admin.menus.get'))}{section('数据范围', '组织数据的可见边界', 'data-scope', <>{can('admin.roles.id.data-scope.get') ? <><ResourceState {...scope} />{scope.data && <><div className="mg-scope-name">{scopeLabels[scope.data.data_scope] || scope.data.data_scope}</div>{scope.data.data_scope === 'custom' && <><ResourceState {...tree} /><p>明确选择：{scope.data.organization_ids.map(id => flattenTree(tree.data || []).find(node => node.id === id)?.name || `组织 ID ${id}`).join('、') || '无'}。选中上级不包含下级。</p></>}{scope.data.data_scope === 'org_and_children' && <p>以用户所属组织为起点，由服务端包含其下级组织。</p>}</>}</> : <p className="mg-hint">没有读取该维度的权限。</p>}</>, can('admin.roles.id.data-scope.get') && can('admin.roles.id.data-scope.post'))}</div>{protectedRole ? <p className="mg-warning">admin 角色受系统保护，不允许修改权限、菜单、数据范围或删除。</p> : can('admin.roles.id.delete') && <div className="mg-detail-actions"><Button variant="outline" className="mg-danger" disabled={deleter.busy} onClick={() => deleter.choose(role)}>删除角色</Button></div>}</>;
}

type EditKind = 'permissions' | 'menus' | 'data-scope';
interface RoleEditData { role: Role; assigned: number[]; options: { id: number; name: string; description: string; group?: string; depth?: number }[]; tree: Organization[]; scope: string; }
export function RoleEditor({ kind }: { kind: EditKind }) {
  const id = useObjectId();
  const { can } = useSession();
  const resource = useResource(`role-editor:${id}:${kind}`, async (): Promise<RoleEditData> => {
    if (!id) throw new Error('角色 ID 无效');
    const role = await api.get<Role>(`/api/admin/roles/${id}`);
    if (role.code === 'admin') return { role, assigned: [], options: [], tree: [], scope: role.data_scope };
    if (kind === 'permissions') {
      const [assigned, options] = await Promise.all([api.get<Permission[]>(`/api/admin/roles/${id}/permissions`), allPages<Permission>('/api/admin/permissions')]);
      const combined = [...options, ...assigned.filter(item => !options.some(option => option.id === item.id))];
      return { role, assigned: assigned.map(item => item.id), options: combined.map(item => ({ id: item.id, name: item.name, description: item.code, group: item.group })), tree: [], scope: role.data_scope };
    }
    if (kind === 'menus') {
      const [assigned, menuTree] = await Promise.all([api.get<Menu[]>(`/api/admin/roles/${id}/menus`), api.get<Menu[]>('/api/admin/menus')]);
      const options: RoleEditData['options'] = [];
      const collect = (nodes: Menu[], depth: number) => nodes.forEach(menu => { options.push({ id: menu.id, name: menu.name, description: `${menu.status === 1 ? '已启用' : '已停用'} · ${menu.permission_code || '无必需权限码'} · ${menu.path || '目录'}`, depth }); collect(menu.children || [], depth + 1); });
      collect(menuTree, 0);
      assigned.filter(menu => !options.some(option => option.id === menu.id)).forEach(menu => options.push({ id: menu.id, name: menu.name, description: `已分配 · ${menu.status === 1 ? '已启用' : '已停用'} · ${menu.permission_code || '无必需权限码'}` }));
      return { role, assigned: assigned.map(item => item.id), options, tree: [], scope: role.data_scope };
    }
    const [scope, tree] = await Promise.all([api.get<DataScope>(`/api/admin/roles/${id}/data-scope`), can('admin.organizations.tree.get') ? api.get<Organization[]>('/api/admin/organizations/tree') : Promise.resolve([])]);
    return { role, assigned: scope.organization_ids, options: flattenTree(tree).map(node => ({ id: node.id, name: node.name, description: node.code })), tree, scope: scope.data_scope };
  });
  const titles = { permissions: '权限', menus: '已分配菜单', 'data-scope': '数据范围' };
  return <><PageHeading title={`编辑${titles[kind]}`} back={{ to: `/roles/${id || ''}`, label: '返回角色详情' }} intro="本页只保存当前授权维度，不影响其他两项配置。" /><ResourceState {...resource} />{resource.data && (resource.data.role.code === 'admin' ? <Notice title="角色受保护">admin 角色不可修改授权。</Notice> : <RoleEditForm key={`${id}:${kind}`} data={resource.data} kind={kind} />)}</>;
}
function RoleEditForm({ data, kind }: { data: RoleEditData; kind: EditKind }) {
  const { can } = useSession();
  const [selected, setSelected] = useState(data.assigned);
  const [scope, setScope] = useState(data.scope);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const navigation = useSavedNavigation(`/roles/${data.role.id}`);
  const effective = kind === 'data-scope' && scope !== 'custom' ? [] : selected;
  const original = kind === 'data-scope' && data.scope !== 'custom' ? [] : data.assigned;
  const dirty = !navigation.saved && (scope !== data.scope || !sameIds(effective, original));
  const invalid = kind === 'data-scope' && scope === 'custom' && (!effective.length || !can('admin.organizations.tree.get') || effective.some(id => !flattenTree(data.tree).some(node => node.id === id && node.manageable)));
  const save = async () => {
    setBusy(true); setError(null);
    try {
      const body = kind === 'permissions' ? { permission_ids: selected } : kind === 'menus' ? { menu_ids: selected } : { data_scope: scope, organization_ids: effective };
      await api.post(`/api/admin/roles/${data.role.id}/${kind}`, body);
      setConfirm(false); navigation.finish();
    } catch (failure) { setError(failure); setConfirm(false); } finally { setBusy(false); }
  };
  const groups = [...new Set(data.options.map(option => option.group || '操作权限'))];
  return <><UnsavedGuard dirty={dirty} busy={busy} /><ErrorNotice error={error} /><div className="mg-edit-layout"><Card className="mg-panel"><h2>{data.role.name}</h2><fieldset disabled={busy} className="mg-fieldset"><legend className="sr-only">授权选择</legend>{kind === 'data-scope' ? <><RadioGroup value={scope} onValueChange={setScope} className="mg-scope-options">{Object.entries(scopeLabels).map(([value, label]) => <label className="mg-radio-choice" key={value}><RadioGroupItem value={value} id={`scope-${value}`} /><span>{label}</span></label>)}</RadioGroup>{scope === 'org_and_children' && <p className="mg-hint">由服务端以用户所属组织为起点扩展下级。</p>}{scope === 'custom' && (can('admin.organizations.tree.get') ? <OrganizationChoices tree={data.tree} selected={selected} onChange={setSelected} disabled={busy} /> : <p className="mg-warning">没有组织树读取权限，不能编辑自定义范围。</p>)}{invalid && <p className="mg-warning" role="alert">请选择至少一个实际可管理的组织；仅供定位的祖先不能作为授权目标。</p>}</> : kind === 'permissions' ? groups.map(group => <section className="mg-option-group" key={group}><h3>{group}</h3>{data.options.filter(option => (option.group || '操作权限') === group).map(option => <Choice key={option.id} checked={selected.includes(option.id)} onChange={checked => setSelected(toggleId(selected, option.id, checked))} label={option.name} description={option.description} />)}</section>) : <><p className="mg-hint">勾选菜单不自动授予权限，也不自动选中父级或子级。停用菜单仍是有效分配配置。</p>{data.options.map(option => <div className="mg-menu-option" key={option.id} style={{ paddingInlineStart: `${Math.min(option.depth || 0, 5) * 12}px` }}><Choice checked={selected.includes(option.id)} onChange={checked => setSelected(toggleId(selected, option.id, checked))} label={option.name} description={option.description} /></div>)}</>}{kind !== 'data-scope' && !data.options.length && <p className="mg-hint">没有可分配的选项。</p>}</fieldset><div className="mg-edit-footer"><Button asChild variant="outline" disabled={busy}><Link to={`/roles/${data.role.id}`}>取消</Link></Button><Button disabled={!dirty || busy || invalid} onClick={() => setConfirm(true)}>保存当前维度</Button></div></Card><Card className="mg-panel mg-edit-aside"><h2>保存范围</h2><p>本页只修改{kind === 'permissions' ? '权限' : kind === 'menus' ? '菜单分配' : '数据范围'}。</p><p className="mg-hint">关联用户的旧会话将在后续请求失效。只有服务器确认后才显示保存成功。</p></Card></div><Confirm open={confirm} onOpenChange={setConfirm} title={`确认保存${data.role.name}的授权？`} busy={busy} onConfirm={save}>{kind === 'data-scope' && <p>范围规则：{scopeLabels[data.scope]} → {scopeLabels[scope]}</p>}<SelectionSummary before={original} after={effective} options={data.options} /><p>仅保存当前维度；关联用户的旧会话将在后续请求失效。</p></Confirm></>;
}

export function GuardedRoleEditor({ kind }: { kind: EditKind }) {
  const permissions = ['admin.roles.id.get', `admin.roles.id.${kind}.get`, `admin.roles.id.${kind}.post`];
  if (kind === 'permissions') permissions.push('admin.permissions.get');
  if (kind === 'menus') permissions.push('admin.menus.get');
  return <Guard module="roles" permissions={permissions}><RoleEditor kind={kind} /></Guard>;
}
