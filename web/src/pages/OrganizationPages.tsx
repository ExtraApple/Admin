import { useState, type ReactNode } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import type { ColumnDef } from '@tanstack/react-table';
import { api, ApiError } from '@/lib/api';
import type { AdminUser, Organization, OrganizationCreateInput, Page, UserInfo } from '@/lib/models';
import { useSession } from '@/lib/session';
import { canAccessModule } from '@/lib/navigation';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { allPages, Choice, Confirm, DataTable, ErrorNotice, flattenTree, Guard, ListFilters, Notice, orgPath, OrganizationsText, PageHeading, Pagination, protectedReason, ResourceState, SelectionSummary, Status, UnsavedGuard, useListQuery, useObjectId, useResource, useSavedNavigation } from '@/components/management/shared';

function OrganizationTree({ tree, selected }: { tree: Organization[]; selected?: number }) {
  const render = (nodes: Organization[]): ReactNode => <ul className="mg-org-tree">{nodes.map(node => <li key={node.id}>{node.manageable ? <Link className="mg-tree-node" aria-current={node.id === selected ? 'page' : undefined} to={`/organizations/${node.id}`}>{node.name}<small>{node.code}</small></Link> : <span className="mg-tree-node mg-tree-ancestor">{node.name}<small>仅供层级定位 · 不可管理</small></span>}{node.children?.length ? render(node.children) : null}</li>)}</ul>;
  return <nav className="mg-tree-panel" aria-label="组织层级"><h2>组织单位</h2>{tree.length ? render(tree) : <p className="mg-hint">当前范围内没有组织。</p>}</nav>;
}
export function OrganizationsList() {
  const list = useListQuery();
  const { can } = useSession();
  const tree = useResource('organizations-tree', () => api.get<Organization[]>('/api/admin/organizations/tree'));
  const resource = useResource(`organizations:${list.query}`, () => api.get<Page<Organization>>(`/api/admin/organizations?${list.query}`));
  const all = flattenTree(tree.data || []);
  const name = (node: Organization) => all.some(item => item.id === node.id && item.manageable) ? <Link className="mg-list-link" to={`/organizations/${node.id}`}>{node.name}</Link> : <span>{node.name}<small className="mg-row-sub">{tree.loading ? '正在核对管理范围' : tree.error ? '无法核对管理范围' : '仅供层级定位'}</small></span>;
  const columns: ColumnDef<Organization>[] = [
    { id: 'name', header: '组织单位', cell: ({ row }) => name(row.original) },
    { accessorKey: 'code', header: '组织编码', cell: ({ row }) => <code>{row.original.code}</code> },
    { id: 'path', header: '完整路径', cell: ({ row }) => orgPath(row.original.id, all) || '正在读取层级' },
    { id: 'status', header: '状态', cell: ({ row }) => <Status value={row.original.status} /> },
  ];
  return <><PageHeading title="组织管理" intro="从层级定位组织。浅色祖先仅补全路径，不能查看成员或修改。" actions={can('admin.organizations.post') && can('admin.organizations.tree.get') && <Button asChild><Link to="/organizations/new">新建组织</Link></Button>} /><div className="mg-org-layout"><Card><ResourceState {...tree} />{tree.data && <OrganizationTree tree={tree.data} />}</Card><Card className="mg-list"><div className="mg-toolbar"><strong>范围内组织</strong><ListFilters list={list} label="搜索组织名称或编码" /></div><ResourceState {...resource} />{resource.data && <>{resource.data.list.length ? <><DataTable columns={columns} data={resource.data.list} caption="服务端组织分页结果" /><div className="mg-mobile-cards">{resource.data.list.map(node => <article className="mg-person-card" key={node.id}><h2>{name(node)}</h2><code>{node.code}</code><p>{orgPath(node.id, all)}</p><Status value={node.status} /></article>)}</div></> : <Notice title="没有符合条件的组织">调整关键词或状态筛选。</Notice>}<Pagination list={list} total={resource.data.total} loading={resource.loading} /></>}</Card></div></>;
}

export function OrganizationCreate() {
  const [params] = useSearchParams();
  const rawParent = params.get('parentId') || '';
  const requestedParentId = /^[1-9]\d*$/.test(rawParent) && Number.isSafeInteger(Number(rawParent)) ? Number(rawParent) : 0;
  const resource = useResource('organization-create-tree', () => api.get<Organization[]>('/api/admin/organizations/tree'));
  const all = flattenTree(resource.data || []);
  const validRequested = requestedParentId && all.some(item => item.id === requestedParentId && item.manageable) ? requestedParentId : 0;
  const back = validRequested ? { to: `/organizations/${validRequested}`, label: '返回组织详情' } : { to: '/organizations', label: '返回组织列表' };
  return <>
    <PageHeading title="新建组织" back={back} intro="填写组织名称、编码及上级位置；仅可挂在当前可管理组织下（全量数据范围可创建根组织）。" />
    <ResourceState {...resource} />
    {resource.data && <OrganizationCreateForm key={`create:${validRequested}`} tree={resource.data} requestedParentId={validRequested} cancelTo={back.to} />}
  </>;
}

function OrganizationCreateForm({ tree, requestedParentId, cancelTo }: { tree: Organization[]; requestedParentId: number; cancelTo: string }) {
  const all = flattenTree(tree);
  const manageable = all.filter(item => item.manageable);
  const hasReadonlyAncestor = all.some(item => !item.manageable);
  const initialParentId = requestedParentId || (hasReadonlyAncestor ? (manageable.length === 1 ? manageable[0].id : 0) : 0);
  const [parentId, setParentId] = useState(initialParentId);
  const [name, setName] = useState('');
  const [code, setCode] = useState('');
  const [remark, setRemark] = useState('');
  const [sort, setSort] = useState(0);
  const [status, setStatus] = useState(1);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [validationError, setValidationError] = useState('');
  const navigation = useSavedNavigation('/organizations');
  const dirty = !navigation.saved && (name !== '' || code !== '' || remark !== '' || sort !== 0 || status !== 1 || parentId !== initialParentId);

  const submit = async () => {
    if (busy) return;
    const trimmedName = name.trim();
    const trimmedCode = code.trim();
    if (!trimmedName) { setValidationError('请填写组织名称。'); return; }
    if (!trimmedCode) { setValidationError('请填写组织编码。'); return; }
    if (hasReadonlyAncestor && !parentId) { setValidationError('当前管理范围仅允许在可管理组织下新建子组织，请选择上级组织。'); return; }
    if (parentId !== 0 && !manageable.some(item => item.id === parentId)) { setValidationError('所选上级组织不在当前可管理范围内。'); return; }
    setValidationError('');
    setError(null);
    setBusy(true);
    try {
      const payload: OrganizationCreateInput = {
        parent_id: parentId,
        name: trimmedName,
        code: trimmedCode,
        remark: remark.trim(),
        sort,
        status,
      };
      const created = await api.post<Organization>('/api/admin/organizations', payload);
      navigation.finish(`/organizations/${created.id}`, '组织已创建。');
    } catch (failure) {
      setError(failure);
    } finally {
      setBusy(false);
    }
  };

  return <>
    <UnsavedGuard dirty={dirty} busy={busy} />
    <Card className="mg-panel mg-create-panel">
      <form className="mg-menu-form" onSubmit={event => { event.preventDefault(); void submit(); }}>
        <fieldset disabled={busy} className="mg-fieldset">
          <legend className="sr-only">新建组织信息</legend>
          <div className="mg-form-grid">
            <label className="mg-form-span">上级组织
              <select value={parentId} onChange={event => setParentId(Number(event.target.value))}>
                {hasReadonlyAncestor ? <option value={0}>请选择可管理的上级组织</option> : <option value={0}>无上级 · 作为根组织</option>}
                {manageable.map(item => <option key={item.id} value={item.id}>{orgPath(item.id, all)}（{item.code}）</option>)}
              </select>
            </label>
            <label>组织名称<Input value={name} required maxLength={100} placeholder="如 华东大区" onChange={event => setName(event.target.value)} /></label>
            <label>组织编码<Input value={code} required maxLength={100} placeholder="如 east_region" onChange={event => setCode(event.target.value)} /></label>
            <label className="mg-form-span">备注<Input value={remark} maxLength={255} placeholder="可选" onChange={event => setRemark(event.target.value)} /></label>
            <label>排序<Input type="number" value={sort} onChange={event => setSort(Number(event.target.value) || 0)} /></label>
            <label>状态<select value={status} onChange={event => setStatus(Number(event.target.value))}><option value={1}>启用</option><option value={0}>停用</option></select></label>
          </div>
          {hasReadonlyAncestor && <p className="mg-hint">当前树包含仅供层级定位的范围外祖先，非全量数据范围不能创建根组织，请选择一个可管理上级。</p>}
        </fieldset>
        <ErrorNotice error={error} />
        {validationError && <div className="mg-error" role="alert"><p>{validationError}</p></div>}
        <div className="mg-edit-footer">
          <Button asChild variant="outline" disabled={busy}><Link to={cancelTo}>取消</Link></Button>
          <Button type="submit" disabled={busy}>{busy ? '正在创建…' : '创建组织'}</Button>
        </div>
      </form>
    </Card>
  </>;
}
export function OrganizationDetail() {
  const id = useObjectId();
  const resource = useResource(`organization-detail-tree:${id}`, () => id ? api.get<Organization[]>('/api/admin/organizations/tree') : Promise.reject(new Error('组织 ID 无效')));
  const all = flattenTree(resource.data || []);
  const node = all.find(item => item.id === id);
  const { can } = useSession();
  return <><PageHeading title="组织详情" back={{ to: '/organizations', label: '返回组织树和列表' }} /><ResourceState {...resource} />{resource.data && <div className="mg-org-layout mg-show-detail"><Card className="mg-org-detail-tree"><OrganizationTree tree={resource.data} selected={id || undefined} /></Card><div>{!node ? <Notice title="未找到可见组织">该组织不存在或不在当前可见层级中。</Notice> : !node.manageable ? <Notice title="仅供层级定位">此祖先不属于可管理范围，不能查看成员、调整上级、新建子组织或删除。</Notice> : <><Card className="mg-panel"><div className="mg-card-head"><div><span className="mg-eyebrow">组织单位</span><h1>{node.name}</h1><code>{node.code}</code></div><Status value={node.status} /></div><dl className="mg-summary"><div><dt>完整路径</dt><dd>{orgPath(node.id, all)}</dd></div><div><dt>备注</dt><dd>{node.remark || '无备注'}</dd></div></dl><div className="mg-inline-actions">{can('admin.organizations.id.put') && <Button asChild variant="outline"><Link to={`/organizations/${node.id}/move`}>修改上级组织</Link></Button>}{can('admin.organizations.post') && <Button asChild variant="outline"><Link to={`/organizations/new?parentId=${node.id}`}>新建子组织</Link></Button>}</div></Card>{can('admin.organizations.id.users.get') ? <OrganizationMembers key={node.id} node={node} tree={resource.data} /> : <Notice title="没有成员读取权限">需要 admin.organizations.id.users.get 才能查看成员。</Notice>}{can('admin.organizations.id.delete') && <OrganizationDeleteSection key={`delete:${node.id}`} node={node} all={all} />}</>}</div></div>}</>;
}

function OrganizationDeleteSection({ node, all }: { node: Organization; all: Organization[] }) {
  const navigate = useNavigate();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const childCount = node.children?.length ?? 0;
  const path = orgPath(node.id, all);

  const remove = async () => {
    if (busy || childCount > 0) return;
    setBusy(true);
    setError(null);
    try {
      await api.delete(`/api/admin/organizations/${node.id}`);
      setConfirmOpen(false);
      navigate('/organizations', { state: { message: '组织已删除。' } });
    } catch (failure) {
      setError(failure);
      setConfirmOpen(false);
    } finally {
      setBusy(false);
    }
  };

  return <>
    <ErrorNotice error={error} />
    {childCount > 0 ? <p className="mg-warning">当前组织下仍有 {childCount} 个可见下级组织，无法直接删除。请先调整下级组织的上级或删除下级组织。</p> : <div className="mg-detail-actions"><Button variant="outline" className="mg-danger" disabled={busy} onClick={() => setConfirmOpen(true)}>删除组织</Button></div>}
    <Confirm open={confirmOpen} onOpenChange={open => { if (!busy) setConfirmOpen(open); }} busy={busy} title={`确认删除组织「${node.name}」？`} label="确认删除" onConfirm={remove}>
      <p>对象：{path} · <code>{node.code}</code></p>
      <p>删除将解除该组织现有成员绑定，并使受影响成员的旧会话在后续请求失效；若存在角色自定义数据范围引用、受保护或范围外成员、或并发新增下级组织，服务端将拒绝删除。此操作无法在界面撤销。</p>
    </Confirm>
  </>;
}

interface PendingMember { user: AdminUser; operation: 'add' | 'remove'; ids: number[]; }
function OrganizationMembers({ node, tree }: { node: Organization; tree: Organization[] }) {
  const { context, can } = useSession();
  const members = useResource(`organization-members:${node.id}`, () => api.get<UserInfo[]>(`/api/admin/organizations/${node.id}/users`));
  const [showCandidates, setShowCandidates] = useState(false);
  const [pending, setPending] = useState<PendingMember | null>(null);
  const [preparing, setPreparing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [localError, setLocalError] = useState('');
  const [message, setMessage] = useState('');
  const [conflict, setConflict] = useState(false);
  const [reloaded, setReloaded] = useState(false);
  const [reviewed, setReviewed] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const canManage = can('admin.users.id.get') && can('admin.users.id.organizations.put');
  const manageableIds = flattenTree(tree).filter(item => item.manageable).map(item => item.id);
  const desired = (user: AdminUser, operation: 'add' | 'remove') => {
    const ids = user.organizations.map(org => org.id);
    if (ids.some(id => !manageableIds.includes(id))) return null;
    return operation === 'add' ? [...ids.filter(id => id !== node.id), node.id] : ids.filter(id => id !== node.id);
  };
  const prepare = async (id: number, operation: 'add' | 'remove') => {
    if (!canManage || preparing || busy) return;
    setPreparing(true); setError(null); setLocalError(''); setMessage(''); setConflict(false); setPending(null); setReviewed(false); setReloaded(false);
    try {
      const user = await api.get<AdminUser>(`/api/admin/users/${id}`);
      const reason = protectedReason(user, context?.user.id);
      if (reason) { setLocalError(reason); return; }
      const already = user.organizations.some(org => org.id === node.id);
      if ((operation === 'add' && already) || (operation === 'remove' && !already)) {
        members.reload();
        setLocalError(operation === 'add' ? '该用户已经属于当前组织，请核对最新成员列表。' : '该用户已不属于当前组织，请核对最新成员列表。');
        return;
      }
      const ids = desired(user, operation);
      if (!ids) { setLocalError('用户归属与当前管理范围不一致，请重新打开组织详情核对。'); return; }
      setPending({ user, operation, ids }); setConfirmOpen(true);
    } catch (failure) { setError(failure); } finally { setPreparing(false); }
  };
  const reload = async () => {
    if (!pending) return;
    setPreparing(true); setError(null); setLocalError(''); setReviewed(false); setReloaded(false);
    try {
      const latest = await api.get<AdminUser>(`/api/admin/users/${pending.user.id}`);
      const ids = desired(latest, pending.operation);
      if (!ids) { setLocalError('用户归属与当前管理范围不一致，请重新打开组织详情核对。'); return; }
      setPending({ user: latest, operation: pending.operation, ids }); setReloaded(true);
    } catch (failure) { setError(failure); } finally { setPreparing(false); }
  };
  const save = async () => {
    if (!pending || !canManage || protectedReason(pending.user, context?.user.id) || (conflict && (!reloaded || !reviewed))) return;
    setBusy(true); setError(null);
    try {
      await api.put<{ access_version: number }>(`/api/admin/users/${pending.user.id}/organizations`, { organization_ids: pending.ids, expected_access_version: pending.user.access_version });
      setMessage(pending.operation === 'add' ? '已加入成员；只修改了该用户的组织归属。' : '已移除成员；保留了该用户的其他组织归属。');
      setConfirmOpen(false); setPending(null); setConflict(false); setShowCandidates(false); members.reload();
    } catch (failure) {
      setError(failure); setConfirmOpen(false); setConflict(true); setReloaded(false); setReviewed(false);
      if (!(failure instanceof ApiError && failure.status === 409)) {
        await reload();
        setError(failure);
      }
    } finally { setBusy(false); }
  };
  const pendingReason = pending ? protectedReason(pending.user, context?.user.id) : '';
  return <Card className="mg-panel mg-members"><div className="mg-card-head"><h2>组织成员</h2>{canManage && can('admin.users.get') && <Button variant="outline" disabled={preparing || busy} onClick={() => setShowCandidates(value => !value)}>{showCandidates ? '收起候选用户' : '加入成员'}</Button>}</div><ErrorNotice error={error} />{<>{localError && <p className="mg-warning" role="alert">{localError}</p>}{message && <p className="mg-success" role="status">{message}</p>}</>}{!canManage && <p className="mg-warning">可以查看成员，但逐人维护需要用户详情读取与用户组织写入权限。旧 role 字段不用于判断受保护身份。</p>}{canManage && !can('admin.users.get') && <p className="mg-hint">缺少用户列表权限，不能选择新增成员；已可核对的非受保护成员仍可移除。</p>}{preparing && <p role="status">正在核对最新用户归属…</p>}{conflict && pending && <div className="mg-conflict"><h3>操作未保存，请核对最新归属</h3><p>加入／移除意图已保留，未将成员列表当作修改成功。</p><Button variant="outline" disabled={preparing || busy} onClick={reload}>重新读取最新用户资料</Button>{reloaded && <><SelectionSummary before={pending.user.organizations.map(org => org.id)} after={pending.ids} options={flattenTree(tree)} /><OrganizationsText user={pending.user} />{pendingReason ? <p className="mg-warning">{pendingReason}</p> : <Choice checked={reviewed} onChange={setReviewed} label="已核对最新归属，确认只调整当前组织" />}<Button disabled={!reviewed || !!pendingReason || busy} onClick={() => setConfirmOpen(true)}>重新确认操作</Button></>}</div>}{showCandidates && <MemberCandidates node={node} onChoose={id => prepare(id, 'add')} disabled={busy || preparing} />}<ResourceState {...members} />{members.data && (members.data.length ? <ul className="mg-member-list">{members.data.map(member => <MemberRow key={member.id} member={member} canManage={canManage} disabled={busy || preparing} onRemove={() => prepare(member.id, 'remove')} />)}</ul> : <p className="mg-empty">当前组织没有成员。{canManage && can('admin.users.get') && '可选择一位用户加入。'}</p>)}<Confirm open={confirmOpen} onOpenChange={open => { setConfirmOpen(open); if (!open && !conflict) setPending(null); }} busy={busy} title={`${pending?.operation === 'add' ? '加入' : '移除'}${pending?.user.nickname || pending?.user.username || ''}？`} onConfirm={save}><p>组织：{node.name}。每次只修改这一位用户。</p>{pending && <SelectionSummary before={pending.user.organizations.map(org => org.id)} after={pending.ids} options={flattenTree(tree)} />}<p>保留其他组织归属及不可见的范围外关系；目标用户旧会话将在后续请求失效。</p></Confirm></Card>;
}
function MemberRow({ member, canManage, disabled, onRemove }: { member: UserInfo; canManage: boolean; disabled: boolean; onRemove: () => void }) {
  const { context, can } = useSession();
  const detail = useResource(`member-authority:${member.id}:${canManage}`, () => canManage ? api.get<AdminUser>(`/api/admin/users/${member.id}`) : Promise.resolve(null));
  const reason = detail.data ? protectedReason(detail.data, context?.user.id) : '';
  return <li><div>{canAccessModule(context, 'users') && can('admin.users.id.get') ? <Link className="mg-list-link" to={`/users/${member.id}`}>{member.nickname || member.username}</Link> : <strong>{member.nickname || member.username}</strong>}<small className="mg-row-sub">{member.username}</small>{reason && <small className="mg-warning-inline">{reason}</small>}{canManage && detail.loading && <small className="mg-hint">正在核对角色和管理范围</small>}{detail.error ? <><ErrorNotice error={detail.error} /><Button variant="outline" onClick={detail.reload}>重新核对</Button></> : null}</div><div className="mg-member-actions"><Status value={member.status} />{canManage && !reason && <Button variant="outline" disabled={disabled || detail.loading || !detail.data} onClick={onRemove}>移除成员</Button>}</div></li>;
}
function MemberCandidates({ node, onChoose, disabled }: { node: Organization; onChoose: (id: number) => void; disabled: boolean }) {
  const { context } = useSession();
  const candidates = useResource(`member-candidates:${node.id}`, () => allPages<AdminUser>('/api/admin/users'));
  const [selected, setSelected] = useState('');
  const options = (candidates.data || []).filter(user => !protectedReason(user, context?.user.id) && !user.organizations.some(org => org.id === node.id));
  return <div className="mg-candidates"><h3>选择加入的用户</h3><ResourceState {...candidates} />{candidates.data && (options.length ? <><label htmlFor={`member-candidate-${node.id}`}>当前可管理用户</label><select id={`member-candidate-${node.id}`} value={selected} disabled={disabled} onChange={event => setSelected(event.target.value)}><option value="">请选择用户</option>{options.map(user => <option value={user.id} key={user.id}>{user.nickname || user.username} · {user.username}</option>)}</select><Button disabled={!selected || disabled} onClick={() => onChoose(Number(selected))}>核对最新归属并加入</Button><p className="mg-hint">先重新读取用户详情，再展示本次单人变更确认。</p></> : <p>没有可加入的非受保护用户。</p>)}</div>;
}

export function OrganizationMove() {
  const id = useObjectId();
  const resource = useResource(`organization-move:${id}`, () => id ? api.get<Organization[]>('/api/admin/organizations/tree') : Promise.reject(new Error('组织 ID 无效')));
  const all = flattenTree(resource.data || []);
  const node = all.find(item => item.id === id);
  return <><PageHeading title="修改上级组织单位" back={{ to: `/organizations/${id || ''}`, label: '返回组织详情' }} intro="提交前核对旧路径与新路径，不能移到自身、自己的下级或范围外祖先。" /><ResourceState {...resource} />{resource.data && (!node ? <Notice title="未找到可见组织" /> : !node.manageable ? <Notice title="仅供层级定位">范围外祖先不可调整上级。</Notice> : <MoveForm key={node.id} node={node} tree={resource.data} />)}</>;
}
function MoveForm({ node, tree }: { node: Organization; tree: Organization[] }) {
  const [parent, setParent] = useState(node.parent_id);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const navigation = useSavedNavigation(`/organizations/${node.id}`);
  const all = flattenTree(tree);
  const excluded = flattenTree([node]).map(item => item.id);
  const targets = all.filter(item => item.manageable && !excluded.includes(item.id));
  const currentParent = all.find(item => item.id === node.parent_id);
  const oldPath = orgPath(node.id, all);
  const newPath = parent === 0 ? node.name : `${orgPath(parent, all)} / ${node.name}`;
  const dirty = !navigation.saved && parent !== node.parent_id;
  const valid = parent === 0 || targets.some(item => item.id === parent);
  const save = async () => {
    if (!dirty || !valid) return;
    setBusy(true); setError(null);
    try { await api.put<Organization>(`/api/admin/organizations/${node.id}`, { parent_id: parent }); setConfirm(false); navigation.finish(); }
    catch (failure) { setError(failure); setConfirm(false); } finally { setBusy(false); }
  };
  return <><UnsavedGuard dirty={dirty} busy={busy} /><ErrorNotice error={error} /><Card className="mg-panel mg-move-form"><h2>{node.name}</h2><dl className="mg-summary"><div><dt>当前完整路径</dt><dd>{oldPath}</dd></div></dl><label htmlFor="organization-parent">新的上级组织单位</label><select id="organization-parent" value={parent} disabled={busy} onChange={event => setParent(Number(event.target.value))}><option value={0}>无上级 · 根组织</option>{currentParent && !targets.some(item => item.id === currentParent.id) && <option value={currentParent.id} disabled>{orgPath(currentParent.id, all)} · 当前上级，仅供定位</option>}{targets.map(item => <option value={item.id} key={item.id}>{orgPath(item.id, all)}</option>)}</select><p className="mg-selected" aria-live="polite">调整后路径：{newPath}</p><div className="mg-edit-footer"><Button asChild variant="outline" disabled={busy}><Link to={`/organizations/${node.id}`}>取消</Link></Button><Button disabled={!dirty || !valid || busy} onClick={() => setConfirm(true)}>确认调整</Button></div></Card><Confirm open={confirm} onOpenChange={setConfirm} busy={busy} title={`确认调整${node.name}的上级？`} onConfirm={save}><dl className="mg-summary"><div><dt>旧路径</dt><dd>{oldPath}</dd></div><div><dt>新路径</dt><dd>{newPath}</dd></div></dl><p>服务器确认后重新读取组织树与详情。组织成员关系不在此次修改中覆盖。</p></Confirm></>;
}
export function GuardedOrganizationMove() { return <Guard module="organizations" permissions={['admin.organizations.tree.get', 'admin.organizations.id.put']}><OrganizationMove /></Guard>; }
