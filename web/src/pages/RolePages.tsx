import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { ColumnDef } from '@tanstack/react-table';
import { api } from '@/lib/api';
import type { DataScope, Menu, Organization, Page, Permission, Role } from '@/lib/models';
import { useSession } from '@/lib/session';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { allPages, Choice, Confirm, DataTable, ErrorNotice, flattenTree, Guard, Notice, OrganizationChoices, PageHeading, Pagination, ResourceState, sameIds, scopeLabels, SelectionSummary, Status, toggleId, UnsavedGuard, useListQuery, useObjectId, useResource, useSavedNavigation } from '@/components/management/shared';

export function RolesList() {
  const list = useListQuery();
  const { can } = useSession();
  const resource = useResource(`roles:${list.page}:${list.size}`, () => api.get<Page<Role>>(`/api/admin/roles?page=${list.page}&size=${list.size}`));
  const columns: ColumnDef<Role>[] = [
    { id: 'name', header: '角色', cell: ({ row }) => <>{can('admin.roles.id.get') ? <Link className="mg-list-link" to={`${row.original.id}`}>{row.original.name}</Link> : <strong>{row.original.name}</strong>}<small className="mg-row-sub">{row.original.description || '无描述'}</small></> },
    { accessorKey: 'code', header: '角色编码', cell: ({ row }) => <code>{row.original.code}</code> },
    { id: 'status', header: '状态', cell: ({ row }) => <Status value={row.original.status} /> },
    { id: 'scope', header: '数据范围', cell: ({ row }) => scopeLabels[row.original.data_scope] || row.original.data_scope },
  ];
  return <><PageHeading title="角色管理" eyebrow="ACCESS · 授权对象" intro="从角色进入详情，分别核对权限、已分配菜单与数据范围。" /><Card className="mg-list"><div className="mg-toolbar"><strong>角色列表</strong><span className="mg-hint">各授权维度独立保存</span></div><ResourceState {...resource} />{resource.data && <>{resource.data.list.length ? <><DataTable columns={columns} data={resource.data.list} caption="真实角色分页列表" /><div className="mg-mobile-cards">{resource.data.list.map(role => <article className="mg-person-card" key={role.id}><h2>{can('admin.roles.id.get') ? <Link to={`${role.id}`}>{role.name}</Link> : role.name}</h2><p>{role.description || '无描述'}</p><code>{role.code}</code><p>数据范围 · {scopeLabels[role.data_scope] || role.data_scope}</p><Status value={role.status} /></article>)}</div></> : <Notice title="暂无角色">当前页没有角色，请调整页码。</Notice>}<Pagination list={list} total={resource.data.total} loading={resource.loading} /></>}</Card></>;
}

export function RoleDetail() {
  const id = useObjectId();
  const resource = useResource(`role:${id}`, () => id ? api.get<Role>(`/api/admin/roles/${id}`) : Promise.reject(new Error('角色 ID 无效')));
  return <><PageHeading title="角色详情" eyebrow="ROLE · 授权剖面" back={{ to: '/roles', label: '返回角色列表' }} /><ResourceState {...resource} />{resource.data && <RoleProfile role={resource.data} />}</>;
}
function RoleProfile({ role }: { role: Role }) {
  const { can } = useSession();
  const permissions = useResource(`role-permissions:${role.id}`, () => can('admin.roles.id.permissions.get') ? api.get<Permission[]>(`/api/admin/roles/${role.id}/permissions`) : Promise.resolve(null));
  const menus = useResource(`role-menus:${role.id}`, () => can('admin.roles.id.menus.get') ? api.get<Menu[]>(`/api/admin/roles/${role.id}/menus`) : Promise.resolve(null));
  const scope = useResource(`role-scope:${role.id}`, () => can('admin.roles.id.data-scope.get') ? api.get<DataScope>(`/api/admin/roles/${role.id}/data-scope`) : Promise.resolve(null));
  const needsTree = scope.data?.data_scope === 'custom' && can('admin.organizations.tree.get');
  const tree = useResource(`role-tree:${role.id}:${needsTree}`, () => needsTree ? api.get<Organization[]>('/api/admin/organizations/tree') : Promise.resolve([]));
  const protectedRole = role.code === 'admin';
  const permissionCodes = permissions.data ? permissions.data.map(permission => permission.code) : null;
  const section = (title: string, intro: string, kind: string, children: ReactNode, allowed: boolean) => <Card className="mg-profile-card"><h2>{title}</h2><p className="mg-hint">{intro}</p>{children}<div className="mg-card-action">{!protectedRole && allowed && <Button variant="outline" asChild><Link to={`/roles/${role.id}/${kind}`}>编辑{title}</Link></Button>}</div></Card>;
  return <><Card className="mg-detail-hero"><div><span className="mg-eyebrow">ROLE · 角色身份</span><h1>{role.name}</h1><p>{role.description || '无描述'}</p><code>{role.code}</code></div><Status value={role.status} /></Card><div className="mg-profile-grid">{section('权限', '可执行操作与稳定权限码', 'permissions', <>{can('admin.roles.id.permissions.get') ? <><ResourceState {...permissions} />{permissions.data && (permissions.data.length ? <ul className="mg-detail-list">{permissions.data.map(permission => <li key={permission.id}>{permission.name}<code>{permission.code}</code></li>)}</ul> : <p>尚未分配权限</p>)}</> : <p className="mg-hint">没有读取该维度的权限。</p>}</>, can('admin.roles.id.permissions.get') && can('admin.roles.id.permissions.post') && can('admin.permissions.get'))}{section('已分配菜单', '分配不等于每位用户最终可见', 'menus', <>{can('admin.roles.id.menus.get') ? <><ResourceState {...menus} />{menus.data && (menus.data.length ? <ul className="mg-detail-list">{menus.data.map(menu => <li key={menu.id}>{menu.name}<Status value={menu.status} /><code>{menu.path || '目录'} · {menu.permission_code || '无必需权限码'}</code>{menu.status !== 1 && <small className="mg-warning-inline">菜单已停用</small>}{!protectedRole && menu.permission_code && permissionCodes && !permissionCodes.includes(menu.permission_code) && <small className="mg-warning-inline">当前角色缺少该菜单所需权限码</small>}</li>)}</ul> : <p>尚未分配菜单</p>)}</> : <p className="mg-hint">没有读取该维度的权限。</p>}<p className="mg-hint">其他角色仍可能提供所需权限；这里不计算用户的实际可见菜单。</p></>, can('admin.roles.id.menus.get') && can('admin.roles.id.menus.post') && can('admin.menus.get'))}{section('数据范围', '组织数据的可见边界', 'data-scope', <>{can('admin.roles.id.data-scope.get') ? <><ResourceState {...scope} />{scope.data && <><div className="mg-scope-name">{scopeLabels[scope.data.data_scope] || scope.data.data_scope}</div>{scope.data.data_scope === 'custom' && <><ResourceState {...tree} /><p>明确选择：{scope.data.organization_ids.map(id => flattenTree(tree.data || []).find(node => node.id === id)?.name || `组织 ID ${id}`).join('、') || '无'}。选中上级不包含下级。</p></>}{scope.data.data_scope === 'org_and_children' && <p>以用户所属组织为起点，由服务端包含其下级组织。</p>}</>}</> : <p className="mg-hint">没有读取该维度的权限。</p>}</>, can('admin.roles.id.data-scope.get') && can('admin.roles.id.data-scope.post'))}</div>{protectedRole && <p className="mg-warning">admin 角色受系统保护，不允许修改权限、菜单或数据范围。</p>}</>;
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
  return <><PageHeading title={`编辑${titles[kind]}`} eyebrow="ROLE · 独立编辑" back={{ to: `/roles/${id || ''}`, label: '返回角色详情' }} intro="本页只保存当前授权维度，不影响其他两项配置。" /><ResourceState {...resource} />{resource.data && (resource.data.role.code === 'admin' ? <Notice title="角色受保护">admin 角色不可修改授权。</Notice> : <RoleEditForm key={`${id}:${kind}`} data={resource.data} kind={kind} />)}</>;
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
