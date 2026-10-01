import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { ColumnDef } from '@tanstack/react-table';
import { api, ApiError } from '@/lib/api';
import type { AdminUser, Organization, Page, UserInfo } from '@/lib/models';
import { useSession } from '@/lib/session';
import { canAccessModule } from '@/lib/navigation';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { allPages, Choice, Confirm, DataTable, ErrorNotice, flattenTree, Guard, ListFilters, Notice, orgPath, OrganizationsText, PageHeading, Pagination, protectedReason, ResourceState, SelectionSummary, Status, UnsavedGuard, useListQuery, useObjectId, useResource, useSavedNavigation } from '@/components/management/shared';

function OrganizationTree({ tree, selected }: { tree: Organization[]; selected?: number }) {
  const render = (nodes: Organization[]): ReactNode => <ul className="mg-org-tree">{nodes.map(node => <li key={node.id}>{node.manageable ? <Link className="mg-tree-node" aria-current={node.id === selected ? 'page' : undefined} to={`/organizations/${node.id}`}>{node.name}<small>{node.code}</small></Link> : <span className="mg-tree-node mg-tree-ancestor">{node.name}<small>仅供层级定位 · 不可管理</small></span>}{node.children?.length ? render(node.children) : null}</li>)}</ul>;
  return <nav className="mg-tree-panel" aria-label="组织层级"><h2>组织单位</h2>{tree.length ? render(tree) : <p className="mg-hint">当前范围内没有组织。</p>}</nav>;
}
export function OrganizationsList() {
  const list = useListQuery();
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
  return <><PageHeading title="组织管理" eyebrow="ORGANIZATION · 单位与成员" intro="从层级定位组织。浅色祖先仅补全路径，不能查看成员或修改。" /><div className="mg-org-layout"><Card><ResourceState {...tree} />{tree.data && <OrganizationTree tree={tree.data} />}</Card><Card className="mg-list"><div className="mg-toolbar"><strong>范围内组织</strong><ListFilters list={list} label="搜索组织名称或编码" /></div><ResourceState {...resource} />{resource.data && <>{resource.data.list.length ? <><DataTable columns={columns} data={resource.data.list} caption="服务端组织分页结果" /><div className="mg-mobile-cards">{resource.data.list.map(node => <article className="mg-person-card" key={node.id}><h2>{name(node)}</h2><code>{node.code}</code><p>{orgPath(node.id, all)}</p><Status value={node.status} /></article>)}</div></> : <Notice title="没有符合条件的组织">调整关键词或状态筛选。</Notice>}<Pagination list={list} total={resource.data.total} loading={resource.loading} /></>}</Card></div></>;
}
export function OrganizationDetail() {
  const id = useObjectId();
  const resource = useResource(`organization-detail-tree:${id}`, () => id ? api.get<Organization[]>('/api/admin/organizations/tree') : Promise.reject(new Error('组织 ID 无效')));
  const all = flattenTree(resource.data || []);
  const node = all.find(item => item.id === id);
  const { can } = useSession();
  return <><PageHeading title="组织详情" eyebrow="ORGANIZATION · 单位与成员" back={{ to: '/organizations', label: '返回组织树和列表' }} /><ResourceState {...resource} />{resource.data && <div className="mg-org-layout mg-show-detail"><Card className="mg-org-detail-tree"><OrganizationTree tree={resource.data} selected={id || undefined} /></Card><div>{!node ? <Notice title="未找到可见组织">该组织不存在或不在当前可见层级中。</Notice> : !node.manageable ? <Notice title="仅供层级定位">此祖先不属于可管理范围，不能查看成员、调整上级或修改归属。</Notice> : <><Card className="mg-panel"><div className="mg-card-head"><div><span className="mg-eyebrow">ORGANIZATION · 组织单位</span><h1>{node.name}</h1><code>{node.code}</code></div><Status value={node.status} /></div><dl className="mg-summary"><div><dt>完整路径</dt><dd>{orgPath(node.id, all)}</dd></div><div><dt>备注</dt><dd>{node.remark || '无备注'}</dd></div></dl>{can('admin.organizations.id.put') && <Button asChild variant="outline"><Link to={`/organizations/${node.id}/move`}>修改上级组织</Link></Button>}</Card>{can('admin.organizations.id.users.get') ? <OrganizationMembers key={node.id} node={node} tree={resource.data} /> : <Notice title="没有成员读取权限">需要 admin.organizations.id.users.get 才能查看成员。</Notice>}</>}</div></div>}</>;
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
  return <><PageHeading title="修改上级组织单位" eyebrow="ORGANIZATION · 层级调整" back={{ to: `/organizations/${id || ''}`, label: '返回组织详情' }} intro="提交前核对旧路径与新路径，不能移到自身、自己的下级或范围外祖先。" /><ResourceState {...resource} />{resource.data && (!node ? <Notice title="未找到可见组织" /> : !node.manageable ? <Notice title="仅供层级定位">范围外祖先不可调整上级。</Notice> : <MoveForm key={node.id} node={node} tree={resource.data} />)}</>;
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
