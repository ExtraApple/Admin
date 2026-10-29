import { FileText, FolderTree, MousePointer2 } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '@/lib/api';
import { encodeApiListQuery, parseApiListQuery, parseMenuSelectionQuery, updateMenuSelectionQuery, type ApiListQueryState } from '@/lib/management-query';
import type { ApiGroupOption, ApiMetadata, ApiPermissionSyncResult, ApiSyncResult, Menu, MenuSyncItem, MenuType, RouteState } from '@/lib/models';
import { managementMenuRoutes } from '@/lib/navigation';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { ErrorNotice, ImpactPreviewDialog, Notice, PageHeading, Pagination, ResourceState, Status, UnsavedGuard, flattenTree, useImpactMutation, useResource } from '@/components/management/shared';

const menuTypeLabels: Record<number, string> = { 1: '目录', 2: '页面', 3: '按钮' };
const routeStateLabels: Record<RouteState, string> = {
  matched: '已匹配',
  metadata_only: '仅有 Metadata',
  catalog_only: '仅有路由',
  access_mismatch: '授权漂移',
};

function menuIcon(type: number) {
  if (type === 1) return <FolderTree size={17} aria-hidden="true" />;
  if (type === 3) return <MousePointer2 size={17} aria-hidden="true" />;
  return <FileText size={17} aria-hidden="true" />;
}

function MenuTree({ nodes, selected, onSelect }: { nodes: Menu[]; selected: number | null; onSelect: (id: number) => void }) {
  return <ul className="mg-org-tree">{nodes.map(menu => <li key={menu.id}>
    <button type="button" className="mg-tree-node" aria-current={selected === menu.id ? 'page' : undefined} onClick={() => onSelect(menu.id)}>
      <span className="mg-tree-node-label">{menuIcon(menu.type)}<span>{menu.name}</span></span>
      <small>{menuTypeLabels[menu.type] || `类型 ${menu.type}`} · {menu.status === 1 ? '启用' : '停用'}</small>
    </button>
    {menu.children?.length ? <MenuTree nodes={menu.children} selected={selected} onSelect={onSelect} /> : null}
  </li>)}</ul>;
}

export interface MenuFormValue {
  id?: number;
  parent_id: number;
  name: string;
  path: string;
  component: string;
  icon: string;
  permission_code: string;
  sort: number;
  type: MenuType;
  status: number;
}

export function menuFormValue(menu?: Menu): MenuFormValue {
  return {
    id: menu?.id,
    parent_id: menu?.parent_id ?? 0,
    name: menu?.name ?? '',
    path: menu?.path ?? '',
    component: menu?.component ?? '',
    icon: menu?.icon ?? '',
    permission_code: menu?.permission_code ?? '',
    sort: menu?.sort ?? 0,
    type: (menu?.type === 2 || menu?.type === 3 ? menu.type : 1),
    status: menu?.status === 0 ? 0 : 1,
  };
}

export function MenuForm({ value, menus, error, validationMessage, busy = false, submitLabel = '保存菜单', onChange, onSubmit }: { value: MenuFormValue; menus: Menu[]; error?: unknown; validationMessage?: string; busy?: boolean; submitLabel?: string; onChange: (value: MenuFormValue) => void; onSubmit: () => void }) {
  const parents = flattenTree(menus).filter(menu => menu.id !== value.id && (value.type !== 3 || menu.type === 1 || menu.type === 2));
  const set = <K extends keyof MenuFormValue>(key: K, next: MenuFormValue[K]) => onChange({ ...value, [key]: next });
  return <form className="mg-menu-form" onSubmit={event => { event.preventDefault(); onSubmit(); }}>
    <fieldset disabled={busy} className="mg-fieldset">
      <legend className="sr-only">{value.id ? '编辑菜单' : '创建菜单'}</legend>
      <div className="mg-form-grid">
        <label>菜单名称<Input value={value.name} required maxLength={100} onChange={event => set('name', event.target.value)} /></label>
        <label>菜单类型<select value={value.type} onChange={event => { const type = Number(event.target.value) as MenuType; onChange({ ...value, type, path: type === 2 ? value.path : '', component: type === 2 ? value.component : '' }); }}><option value={1}>目录</option><option value={2}>页面</option><option value={3}>按钮</option></select></label>
        <label>上级菜单<select value={value.parent_id} onChange={event => set('parent_id', Number(event.target.value))}><option value={0}>根菜单</option>{parents.map(menu => <option key={menu.id} value={menu.id}>{'　'.repeat(menu.parent_id ? 1 : 0)}{menu.name} · {menuTypeLabels[menu.type]}</option>)}</select></label>
        {value.type === 2 && <label>前端路径<Input value={value.path} required maxLength={255} placeholder="/system/example" onChange={event => set('path', event.target.value)} /></label>}
        {value.type === 2 && <label>组件路径<Input value={value.component} maxLength={255} placeholder="system/example/index" onChange={event => set('component', event.target.value)} /></label>}
        <label>图标<Input value={value.icon} maxLength={100} placeholder="可选图标名称" onChange={event => set('icon', event.target.value)} /></label>
        <label>权限码<Input value={value.permission_code} maxLength={100} placeholder="可选，如 admin.example.get" onChange={event => set('permission_code', event.target.value)} /></label>
        <label>排序<Input type="number" value={value.sort} onChange={event => set('sort', Number(event.target.value) || 0)} /></label>
        <label>状态<select value={value.status} onChange={event => set('status', Number(event.target.value))}><option value={1}>启用</option><option value={0}>停用</option></select></label>
      </div>
    </fieldset>
    <ErrorNotice error={error} />{validationMessage && <div className="mg-error" role="alert"><p>{validationMessage}</p></div>}
    <div className="mg-edit-footer"><Button type="submit" disabled={busy}>{busy ? '正在保存…' : submitLabel}</Button></div>
  </form>;
}

type MenuMutation =
  | { operation: 'create'; payload: Omit<MenuFormValue, 'id'> }
  | { operation: 'update'; payload: Omit<MenuFormValue, 'id'> & { menu_id: number } }
  | { operation: 'delete'; payload: { menu_id: number } };

function menuPayload(value: MenuFormValue): Omit<MenuFormValue, 'id'> {
  return {
    parent_id: value.parent_id,
    name: value.name.trim(),
    path: value.type === 2 ? value.path.trim() : '',
    component: value.type === 2 ? value.component.trim() : '',
    icon: value.icon.trim(),
    permission_code: value.permission_code.trim(),
    sort: value.sort,
    type: value.type,
    status: value.status,
  };
}

type MenuBindingCandidate = { menu_id: number; api_ids: number[]; permission_code: string };

function MenuApiBinding({ menu, onChanged, onDirtyChange }: { menu: Menu; onChanged: () => void; onDirtyChange: (dirty: boolean) => void }) {
  const [keyword, setKeyword] = useState('');
  const [method, setMethod] = useState('');
  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [permissionCode, setPermissionCode] = useState(menu.permission_code);
  const bound = useResource(`menu-apis-${menu.id}`, () => api.menus.listApis(menu.id));
  const candidateQuery = useMemo(() => {
    const params = new URLSearchParams({ page: '1', size: '50', status: '1', need_auth: '1' });
    if (keyword.trim()) params.set('keyword', keyword.trim());
    if (method) params.set('method', method);
    return params.toString();
  }, [keyword, method]);
  const candidates = useResource(`menu-api-candidates-${candidateQuery}`, () => api.apis.list(`/api/admin/apis?${candidateQuery}`));
  const bindMutation = useImpactMutation<MenuBindingCandidate, void>(
    candidate => api.menus.preview({ operation: 'bind-apis', candidate }),
    (candidate, fingerprint) => api.menus.bindApis(candidate.menu_id, { api_ids: candidate.api_ids, permission_code: candidate.permission_code, expected_fingerprint: fingerprint }),
  );
  const clearMutation = useImpactMutation<{ menu_id: number }, void>(
    candidate => api.menus.preview({ operation: 'clear-apis', candidate }),
    (candidate, fingerprint) => api.menus.clearApis(candidate.menu_id, { clear: true, expected_fingerprint: fingerprint }),
  );
  useEffect(() => {
    if (!bound.data) return;
    setSelectedIds([...new Set(bound.data.map(item => item.id))]);
    setPermissionCode(menu.permission_code);
  }, [bound.data, menu.id, menu.permission_code]);
  useEffect(() => {
    if (bindMutation.state.phase !== 'success') return;
    bound.reload();
    onChanged();
    bindMutation.reset();
  }, [bindMutation.state.phase]);
  useEffect(() => {
    if (clearMutation.state.phase !== 'success') return;
    bound.reload();
    onChanged();
    clearMutation.reset();
  }, [clearMutation.state.phase]);
  const dirty = !!bound.data && JSON.stringify(selectedIds) !== JSON.stringify([...new Set(bound.data.map(item => item.id))]) || permissionCode !== menu.permission_code;
  const busy = [bindMutation.state.phase, clearMutation.state.phase].some(phase => phase === 'previewing' || phase === 'submitting');
  useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);
  function toggle(id: number, checked: boolean) {
    setSelectedIds(current => checked ? [...new Set([...current, id])] : current.filter(value => value !== id));
  }
  function prepareBinding() {
    if (selectedIds.length === 0) return;
    void bindMutation.prepare({ menu_id: menu.id, api_ids: [...new Set(selectedIds)], permission_code: permissionCode.trim() });
  }
  function prepareClear() {
    if (bound.data?.length) void clearMutation.prepare({ menu_id: menu.id });
  }
  return <section className="mg-candidates">
    <div className="mg-card-head"><div><h2>按钮 API 绑定</h2><p className="mg-hint">只显示启用且需要认证的接口；接口权限码与按钮自身权限码分开显示。确认绑定后，所选接口的权限码会被统一改为按钮权限码。</p></div><Button disabled={busy || selectedIds.length === 0} onClick={prepareBinding}>预览绑定</Button></div>
    <label htmlFor={`binding-permission-${menu.id}`}>按钮权限码（独立字段）</label><Input id={`binding-permission-${menu.id}`} value={permissionCode} disabled={busy} onChange={event => setPermissionCode(event.target.value)} />
    <h3>已绑定接口（{bound.data?.length ?? 0}）</h3>
    <ResourceState loading={bound.loading} error={bound.error} reload={bound.reload} />
    {!bound.loading && !bound.error && !bound.data?.length && <p className="mg-empty">当前按钮没有绑定接口。</p>}
    {!!bound.data?.length && <><ul className="mg-member-list">{bound.data.map(item => <li key={item.id}><div><strong>{item.name}</strong><code>{item.method} {item.path}</code><small>接口权限码：{item.permission_code || '无'}</small></div><Status value={item.status} /></li>)}</ul><Button variant="outline" disabled={busy} onClick={prepareClear}>预览清空绑定</Button></>}
    <h3>候选接口</h3>
    <div className="mg-filters"><Input value={keyword} disabled={busy} placeholder="按名称、Path 或权限码搜索" onChange={event => setKeyword(event.target.value)} /><select value={method} disabled={busy} onChange={event => setMethod(event.target.value)}><option value="">全部 Method</option>{['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map(value => <option key={value} value={value}>{value}</option>)}</select></div>
    <ResourceState loading={candidates.loading} error={candidates.error} reload={candidates.reload} />
    {!candidates.loading && !candidates.error && !candidates.data?.list.length && <p className="mg-empty">没有符合条件的候选接口。</p>}
    {!!candidates.data?.list.length && <div className="mg-binding-options">{candidates.data.list.map(item => <label className="mg-choice" key={item.id}><input type="checkbox" checked={selectedIds.includes(item.id)} disabled={busy} onChange={event => toggle(item.id, event.target.checked)} /><span><strong>{item.name}</strong><small>{item.method} {item.path} · 接口权限码：{item.permission_code || '无'}</small></span></label>)}</div>}
    <ErrorNotice error={bindMutation.state.error || clearMutation.state.error} />
    <ImpactPreviewDialog open={bindMutation.state.phase === 'awaiting_confirmation' || bindMutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) bindMutation.reset(); }} preview={bindMutation.state.preview} title="确认绑定按钮 API" onConfirm={() => void bindMutation.confirm()} busy={bindMutation.state.phase === 'submitting'} />
    <ImpactPreviewDialog open={clearMutation.state.phase === 'awaiting_confirmation' || clearMutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) clearMutation.reset(); }} preview={clearMutation.state.preview} title="确认清空按钮 API" onConfirm={() => void clearMutation.confirm()} busy={clearMutation.state.phase === 'submitting'} />
  </section>;
}

type MenuRouteSyncCandidate = { routes: MenuSyncItem[] };

function MenuRouteSync({ onChanged }: { onChanged: () => void }) {
  const [lastCreated, setLastCreated] = useState<number | null>(null);
  const mutation = useImpactMutation<MenuRouteSyncCandidate, { created: number }>(
    candidate => api.menus.preview({ operation: 'sync', candidate }),
    (candidate, fingerprint) => api.menus.sync({ routes: candidate.routes, expected_fingerprint: fingerprint }),
  );
  useEffect(() => {
    if (mutation.state.phase !== 'success') return;
    setLastCreated(mutation.state.result?.created ?? 0);
    onChanged();
    mutation.reset();
  }, [mutation.state.phase]);
  const busy = mutation.state.phase === 'previewing' || mutation.state.phase === 'submitting';
  return <Card className="mg-panel">
    <div className="mg-card-head"><div><h2>路由目录同步</h2><p className="mg-hint">目录来自前端真实路由表（<code>managementMenuRoutes</code>），同步只补充缺失菜单，不覆盖已有配置。</p></div><Button variant="outline" disabled={busy} onClick={() => { setLastCreated(null); void mutation.prepare({ routes: managementMenuRoutes }); }}>预览同步</Button></div>
    <ul className="mg-member-list">{managementMenuRoutes.map(item => <li key={item.path}><div><strong>{item.name}</strong><code>{item.path}</code><small>权限码：{item.permission_code}</small></div><Status value={1} /></li>)}</ul>
    {lastCreated !== null && <p className="mg-success" role="status">已提交同步，服务器创建 {lastCreated} 个菜单。</p>}
    <ErrorNotice error={mutation.state.error} />
    <ImpactPreviewDialog open={mutation.state.phase === 'awaiting_confirmation' || mutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) mutation.reset(); }} preview={mutation.state.preview} title="确认同步菜单路由目录" onConfirm={() => void mutation.confirm()} busy={mutation.state.phase === 'submitting'} />
  </Card>;
}

export function MenusPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [creating, setCreating] = useState(false);
  const [draft, setDraft] = useState<MenuFormValue | null>(null);
  const [validationMessage, setValidationMessage] = useState('');
  const [bindingDirty, setBindingDirty] = useState(false);
  const selectedId = parseMenuSelectionQuery(searchParams);
  const menus = useResource('menus', () => api.menus.list());
  const selected = useMemo(() => menus.data ? flattenTree(menus.data).find(menu => menu.id === selectedId) || null : null, [menus.data, selectedId]);
  const mutation = useImpactMutation<MenuMutation, Menu | void>(
    candidate => api.menus.preview({ operation: candidate.operation, candidate: candidate.payload }),
    (candidate, fingerprint) => {
      if (candidate.operation === 'create') return api.menus.create({ ...candidate.payload, expected_fingerprint: fingerprint });
      if (candidate.operation === 'update') {
        const { menu_id, ...fields } = candidate.payload;
        return api.menus.update(menu_id, { ...fields, expected_fingerprint: fingerprint });
      }
      return api.menus.delete(candidate.payload.menu_id, fingerprint);
    },
  );
  const busy = mutation.state.phase === 'previewing' || mutation.state.phase === 'submitting';
  useEffect(() => {
    if (!creating) {
      setDraft(selected ? menuFormValue(selected) : null);
      setValidationMessage('');
    }
  }, [creating, selected]);
  useEffect(() => {
    if (mutation.state.phase !== 'success') return;
    const operation = mutation.state.candidate?.operation;
    const result = mutation.state.result;
    setCreating(false);
    setDraft(null);
    menus.reload();
    if (operation === 'delete') {
      setSearchParams(new URLSearchParams(updateMenuSelectionQuery(searchParams, null)));
    } else if (result && typeof result === 'object' && 'id' in result && typeof result.id === 'number') {
      setSearchParams(new URLSearchParams(updateMenuSelectionQuery(searchParams, result.id)));
    }
    mutation.reset();
  }, [mutation.state.phase]);
  function selectMenu(id: number) {
    setCreating(false);
    setSearchParams(new URLSearchParams(updateMenuSelectionQuery(searchParams, id)));
  }
  function startCreate() {
    setCreating(true);
    setDraft(menuFormValue());
    setValidationMessage('');
    setSearchParams(new URLSearchParams(updateMenuSelectionQuery(searchParams, null)));
  }
  function submitDraft() {
    if (!draft) return;
    if (!draft.name.trim()) return setValidationMessage('菜单名称不能为空。');
    if (draft.type === 2 && !draft.path.trim()) return setValidationMessage('页面菜单必须填写前端路径。');
    if (draft.type !== 2 && (draft.path.trim() || draft.component.trim())) return setValidationMessage('目录和按钮菜单不能填写 path 或 component。');
    if (!creating && !draft.id) return setValidationMessage('未找到要编辑的菜单。');
    setValidationMessage('');
    const payload = menuPayload(draft);
    void mutation.prepare(creating ? { operation: 'create', payload } : { operation: 'update', payload: { ...payload, menu_id: draft.id! } });
  }
  function requestDelete() {
    if (selected && !creating) void mutation.prepare({ operation: 'delete', payload: { menu_id: selected.id } });
  }
  const baseline = selected ? menuFormValue(selected) : menuFormValue();
  const dirty = !!draft && JSON.stringify(menuPayload(draft)) !== JSON.stringify(menuPayload(baseline)) || bindingDirty;
  return <section className="management-page">
    <UnsavedGuard dirty={dirty} busy={busy} />
    <PageHeading eyebrow="配置管理 / 菜单" title="菜单管理" intro="维护菜单树、菜单类型和后续 API 绑定关系。" />
    <ResourceState loading={menus.loading} error={menus.error} reload={menus.reload} />
    {!menus.error && <MenuRouteSync onChanged={menus.reload} />}
    {!menus.loading && !menus.error && !menus.data?.length && !creating && <Notice title="暂无菜单">当前账号可见的菜单数据为空。可以先创建一个根目录。</Notice>}
    {!menus.loading && !menus.error && <div className="mg-org-layout mg-menu-layout">
      <Card className="mg-tree-panel"><div className="mg-card-head"><h2>菜单树</h2><div className="mg-detail-actions"><Button variant="outline" onClick={menus.reload}>刷新</Button><Button onClick={startCreate}>新建菜单</Button></div></div>{menus.data?.length ? <MenuTree nodes={menus.data} selected={selectedId} onSelect={selectMenu} /> : <div className="mg-empty"><p>还没有菜单节点。</p></div>}</Card>
      <Card className="mg-panel">{draft ? <><div className="mg-card-head"><div><span className="mg-eyebrow">{creating ? '新建菜单' : '编辑菜单'}</span><h2>{creating ? '创建菜单' : selected?.name}</h2></div>{!creating && <Button variant="outline" onClick={() => setDraft(selected ? menuFormValue(selected) : null)}>还原草稿</Button>}</div><MenuForm value={draft} menus={menus.data ?? []} error={mutation.state.error} validationMessage={validationMessage} busy={busy} submitLabel={creating ? '创建菜单' : '保存菜单'} onChange={value => { setDraft(value); setValidationMessage(''); }} onSubmit={submitDraft} />{selected && !creating && <div className="mg-detail-actions"><Button variant="destructive" disabled={busy} onClick={requestDelete}>删除菜单</Button></div>}{selected && !creating && selected.type === 3 && <MenuApiBinding menu={selected} onChanged={menus.reload} onDirtyChange={setBindingDirty} />}</> : <div className="mg-empty"><h2>选择一个菜单节点</h2><p>从左侧菜单树选择节点，或创建一个新的菜单。</p></div>}</Card>
    </div>}
    <ImpactPreviewDialog open={mutation.state.phase === 'awaiting_confirmation' || mutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) mutation.reset(); }} preview={mutation.state.preview} title={mutation.state.candidate?.operation === 'delete' ? '确认删除菜单' : mutation.state.candidate?.operation === 'create' ? '确认创建菜单' : '确认保存菜单'} onConfirm={() => void mutation.confirm()} busy={mutation.state.phase === 'submitting'} label="确认提交" />
  </section>;
}

function routeStateTag(value: ApiMetadata['route_state']) {
  return value ? <span className={`mg-route-state mg-route-state-${value}`}>{routeStateLabels[value]}</span> : <span className="mg-route-state">未计算</span>;
}

function ApiFilters({ query, groups, update }: { query: ApiListQueryState; groups: ApiGroupOption[]; update: (changes: Partial<ApiListQueryState>) => void }) {
  const [draft, setDraft] = useState(query.keyword);
  useEffect(() => setDraft(query.keyword), [query.keyword]);
  return <form className="mg-filters mg-api-filters" onSubmit={event => { event.preventDefault(); update({ keyword: draft.trim(), page: 1 }); }}>
    <label className="sr-only" htmlFor="api-keyword">搜索接口</label><input id="api-keyword" type="search" value={draft} onChange={event => setDraft(event.target.value)} placeholder="搜索名称、路径或权限码" />
    <label className="sr-only" htmlFor="api-group">接口分组</label><select id="api-group" value={query.group} onChange={event => update({ group: event.target.value, page: 1 })}><option value="">全部分组</option>{groups.map(option => <option value={option.group} key={option.group}>{option.group}（{option.count}）</option>)}</select>
    <label className="sr-only" htmlFor="api-method">HTTP Method</label><select id="api-method" value={query.method} onChange={event => update({ method: event.target.value, page: 1 })}><option value="">全部 Method</option>{['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map(method => <option value={method} key={method}>{method}</option>)}</select>
    <label className="sr-only" htmlFor="api-status">接口状态</label><select id="api-status" value={query.status} onChange={event => update({ status: event.target.value as ApiListQueryState['status'], page: 1 })}><option value="">全部状态</option><option value="1">已启用</option><option value="0">已停用</option></select>
    <label className="sr-only" htmlFor="api-need-auth">认证要求</label><select id="api-need-auth" value={query.needAuth} onChange={event => update({ needAuth: event.target.value as ApiListQueryState['needAuth'], page: 1 })}><option value="">全部认证要求</option><option value="1">需要认证</option><option value="0">无需认证</option></select>
    <label className="sr-only" htmlFor="api-need-audit">审计要求</label><select id="api-need-audit" value={query.needAudit} onChange={event => update({ needAudit: event.target.value as ApiListQueryState['needAudit'], page: 1 })}><option value="">全部审计要求</option><option value="1">需要审计</option><option value="0">无需审计</option></select>
    <label className="sr-only" htmlFor="api-route-state">RouteState</label><select id="api-route-state" value={query.routeState} onChange={event => update({ routeState: event.target.value as ApiListQueryState['routeState'], page: 1 })}><option value="">全部 RouteState</option>{Object.entries(routeStateLabels).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select>
    <Button type="submit" variant="outline">搜索</Button>
  </form>;
}

function ApiTable({ items, onSelect }: { items: ApiMetadata[]; onSelect: (id: number) => void }) {
  return <>
    <div className="mg-table-desktop"><Table><caption className="sr-only">接口列表</caption><TableHeader><TableRow><TableHead>名称</TableHead><TableHead>Method / Path</TableHead><TableHead>分组</TableHead><TableHead>RouteState</TableHead><TableHead>状态</TableHead></TableRow></TableHeader><TableBody>{items.map(apiInfo => <TableRow key={apiInfo.id}><TableCell><button className="mg-list-link" type="button" onClick={() => onSelect(apiInfo.id)}>{apiInfo.name}</button><small>ID {apiInfo.id}</small></TableCell><TableCell><code>{apiInfo.method} {apiInfo.path}</code></TableCell><TableCell>{apiInfo.group || '—'}</TableCell><TableCell>{routeStateTag(apiInfo.route_state)}</TableCell><TableCell><Status value={apiInfo.status} /></TableCell></TableRow>)}</TableBody></Table></div>
    <div className="mg-mobile-cards">{items.map(apiInfo => <Card key={apiInfo.id} className="mg-mobile-card"><div className="mg-card-head"><h2><button className="mg-list-link" type="button" onClick={() => onSelect(apiInfo.id)}>{apiInfo.name}</button></h2>{routeStateTag(apiInfo.route_state)}</div><code>{apiInfo.method} {apiInfo.path}</code><p>{apiInfo.group || '未分组'} · <Status value={apiInfo.status} /></p></Card>)}</div>
  </>;
}

interface ApiFormValue {
  name: string;
  group: string;
  permission_code: string;
  remark: string;
  sort: number;
  status: number;
  need_audit: number;
}

function apiFormValue(source: ApiMetadata): ApiFormValue {
  return {
    name: source.name, group: source.group, permission_code: source.permission_code, remark: source.remark,
    sort: source.sort, status: source.status === 0 ? 0 : 1, need_audit: source.need_audit === 0 ? 0 : 1,
  };
}

const routeStateNotes: Record<RouteState, string> = {
  matched: 'Metadata 与 Route Catalog 的认证要求和权限码一致。',
  metadata_only: 'Route Catalog 中没有对应路由，只保留 Metadata 供人工核对；页面不会自动修复。',
  catalog_only: 'Route Catalog 中存在路由但没有 Metadata；执行路由同步会新增记录。',
  access_mismatch: 'NeedAuth 或权限码与 Route Catalog 期望不一致；只读字段不会被页面修改，请核对后执行权限同步或人工处理。',
};

function ApiDetailPanel({ apiId, onChanged, onDeleted }: { apiId: number; onChanged: () => void; onDeleted: () => void }) {
  const card = useRef<HTMLDivElement>(null);
  const detail = useResource(`api-detail-${apiId}`, () => api.apis.get(apiId));
  const [form, setForm] = useState<ApiFormValue | null>(null);
  const [validationMessage, setValidationMessage] = useState('');
  const [buttonForm, setButtonForm] = useState<{ parent_id: number; name: string; sort: number } | null>(null);
  const updateMutation = useImpactMutation<{ api_id: number; fields: ApiFormValue }, ApiMetadata>(
    candidate => api.apis.preview({ operation: 'update', candidate: { api_id: candidate.api_id, ...candidate.fields } }),
    (candidate, fingerprint) => api.apis.update(candidate.api_id, { ...candidate.fields, expected_fingerprint: fingerprint }),
  );
  const deleteMutation = useImpactMutation<{ api_id: number }, void>(
    candidate => api.apis.preview({ operation: 'delete', candidate }),
    (candidate, fingerprint) => api.apis.delete(candidate.api_id, fingerprint),
  );
  const buttonMutation = useImpactMutation<{ api_id: number; parent_id: number; name: string; sort: number }, Menu>(
    candidate => api.apis.preview({ operation: 'generate-menu-button', candidate }),
    (candidate, fingerprint) => api.apis.generateMenuButton(candidate.api_id, { parent_id: candidate.parent_id, name: candidate.name, sort: candidate.sort, expected_fingerprint: fingerprint }),
  );
  useEffect(() => {
    if (!detail.data) return;
    setForm(apiFormValue(detail.data));
    setButtonForm(null);
    setValidationMessage('');
    const frame = requestAnimationFrame(() => card.current?.scrollIntoView?.({ block: 'start' }));
    return () => cancelAnimationFrame(frame);
  }, [detail.data]);
  useEffect(() => {
    if (updateMutation.state.phase !== 'success') return;
    detail.reload();
    onChanged();
    updateMutation.reset();
  }, [updateMutation.state.phase]);
  useEffect(() => {
    if (deleteMutation.state.phase !== 'success') return;
    deleteMutation.reset();
    onDeleted();
  }, [deleteMutation.state.phase]);
  useEffect(() => {
    if (buttonMutation.state.phase !== 'success') return;
    buttonMutation.reset();
    setButtonForm(null);
    onChanged();
  }, [buttonMutation.state.phase]);
  if (detail.loading) return <Card className="mg-panel"><ResourceState loading error={null} reload={detail.reload} /></Card>;
  if (detail.error || !detail.data || !form) return <Card className="mg-panel"><ResourceState loading={false} error={detail.error} reload={detail.reload} /></Card>;
  const source = detail.data;
  const current = form;
  const busy = [updateMutation.state.phase, deleteMutation.state.phase, buttonMutation.state.phase].some(phase => phase === 'previewing' || phase === 'submitting');
  const dirty = JSON.stringify(current) !== JSON.stringify(apiFormValue(source));
  const set = <K extends keyof ApiFormValue>(key: K, value: ApiFormValue[K]) => setForm({ ...current, [key]: value });
  const submitUpdate = () => {
    if (!current.name.trim()) return setValidationMessage('API 名称不能为空。');
    setValidationMessage('');
    void updateMutation.prepare({ api_id: source.id, fields: { ...current, name: current.name.trim(), group: current.group.trim(), permission_code: current.permission_code.trim(), remark: current.remark.trim() } });
  };
  return <Card ref={card} className="mg-panel">
    <UnsavedGuard dirty={dirty} busy={busy} />
    <div className="mg-card-head"><div><span className="mg-eyebrow">接口详情</span><h2>{source.name}</h2><p><code>{source.method} {source.path}</code></p></div>{routeStateTag(source.route_state)}</div>
    <dl className="mg-detail-list">
      <li><strong>API ID</strong><code>{source.id}</code></li>
      <li><strong>Method / Path</strong><code>{source.method} {source.path}</code><small className="mg-hint">路由事实只读，来自 Route Catalog。</small></li>
      <li><strong>NeedAuth</strong><span>{source.need_auth === 1 ? '需要认证' : '公开接口'}</span><small className="mg-hint">只读；访问策略变更必须经 Route Catalog 与部署。</small></li>
      <li><strong>RouteState</strong>{routeStateTag(source.route_state)}<small className="mg-hint">{source.route_state ? routeStateNotes[source.route_state] : '未计算'}</small></li>
    </dl>
    <fieldset disabled={busy} className="mg-fieldset"><legend className="sr-only">接口元数据</legend><div className="mg-form-grid">
      <label>名称<Input value={form.name} maxLength={100} onChange={event => set('name', event.target.value)} /></label>
      <label>分组<Input value={form.group} maxLength={50} onChange={event => set('group', event.target.value)} /></label>
      <label>权限码<Input value={form.permission_code} maxLength={100} onChange={event => set('permission_code', event.target.value)} /></label>
      <label>备注<Input value={form.remark} maxLength={255} onChange={event => set('remark', event.target.value)} /></label>
      <label>排序<Input type="number" value={form.sort} onChange={event => set('sort', Number(event.target.value) || 0)} /></label>
      <label>状态<select value={form.status} onChange={event => set('status', Number(event.target.value))}><option value={1}>已启用</option><option value={0}>已停用</option></select></label>
      <label>审计<select value={form.need_audit} onChange={event => set('need_audit', Number(event.target.value))}><option value={1}>需要审计</option><option value={0}>无需审计</option></select></label>
    </div></fieldset>
    <ErrorNotice error={updateMutation.state.error} />{validationMessage && <div className="mg-error" role="alert"><p>{validationMessage}</p></div>}
    <div className="mg-edit-footer"><Button variant="outline" disabled={busy} onClick={() => setForm(apiFormValue(source))}>还原草稿</Button><Button disabled={busy || !dirty} onClick={submitUpdate}>预览并保存</Button></div>
    <div className="mg-detail-actions">
      <Button variant="outline" disabled={busy} onClick={() => void deleteMutation.prepare({ api_id: source.id })}>预览删除</Button>
      <Button variant="outline" disabled={busy} onClick={() => setButtonForm(buttonForm ? null : { parent_id: 0, name: source.name, sort: 0 })}>生成按钮菜单</Button>
    </div>
    {buttonForm && <ButtonForm value={buttonForm} busy={busy} error={buttonMutation.state.error} onChange={setButtonForm} onPreview={() => { if (!buttonForm.parent_id) return setValidationMessage('生成按钮菜单必须选择目录或页面父级。'); if (!buttonForm.name.trim()) return setValidationMessage('按钮名称不能为空。'); setValidationMessage(''); void buttonMutation.prepare({ api_id: source.id, parent_id: buttonForm.parent_id, name: buttonForm.name.trim(), sort: buttonForm.sort }); }} />}
    <ImpactPreviewDialog open={updateMutation.state.phase === 'awaiting_confirmation' || updateMutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) updateMutation.reset(); }} preview={updateMutation.state.preview} title="确认保存接口元数据" onConfirm={() => void updateMutation.confirm()} busy={updateMutation.state.phase === 'submitting'} />
    <ImpactPreviewDialog open={deleteMutation.state.phase === 'awaiting_confirmation' || deleteMutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) deleteMutation.reset(); }} preview={deleteMutation.state.preview} title="确认删除接口 Metadata" onConfirm={() => void deleteMutation.confirm()} busy={deleteMutation.state.phase === 'submitting'} />
    <ImpactPreviewDialog open={buttonMutation.state.phase === 'awaiting_confirmation' || buttonMutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) buttonMutation.reset(); }} preview={buttonMutation.state.preview} title="确认生成按钮菜单" onConfirm={() => void buttonMutation.confirm()} busy={buttonMutation.state.phase === 'submitting'} />
  </Card>;
}

function ButtonForm({ value, busy, error, onChange, onPreview }: { value: { parent_id: number; name: string; sort: number }; busy: boolean; error: unknown; onChange: (value: { parent_id: number; name: string; sort: number }) => void; onPreview: () => void }) {
  const menus = useResource('menus-for-button', async () => flattenTree(await api.menus.list()));
  return <section className="mg-candidates">
    <h3>生成按钮菜单</h3>
    <p className="mg-hint">父级只能是目录或页面；按钮本身不带 path/component。</p>
    <label htmlFor="button-parent">父级菜单</label>
    <select id="button-parent" value={value.parent_id} disabled={busy || menus.loading} onChange={event => onChange({ ...value, parent_id: Number(event.target.value) })}><option value={0}>请选择父级</option>{(menus.data ?? []).filter(menu => menu.type === 1 || menu.type === 2).map(menu => <option key={menu.id} value={menu.id}>{menu.name} · {menuTypeLabels[menu.type]}</option>)}</select>
    <label htmlFor="button-name">按钮名称</label><Input id="button-name" value={value.name} disabled={busy} maxLength={100} onChange={event => onChange({ ...value, name: event.target.value })} />
    <label htmlFor="button-sort">排序</label><Input id="button-sort" type="number" value={value.sort} disabled={busy} onChange={event => onChange({ ...value, sort: Number(event.target.value) || 0 })} />
    <ResourceState loading={menus.loading} error={menus.error} reload={menus.reload} />
    <ErrorNotice error={error} />
    <div className="mg-edit-footer"><Button variant="outline" disabled={busy} onClick={onPreview}>预览生成</Button></div>
  </section>;
}

function ApiSyncPanel({ onChanged }: { onChanged: () => void }) {
  const [message, setMessage] = useState('');
  const syncMutation = useImpactMutation<Record<string, never>, ApiSyncResult>(
    candidate => api.apis.preview({ operation: 'sync', candidate }),
    (_candidate, fingerprint) => api.apis.sync({ expected_fingerprint: fingerprint }),
  );
  const permissionMutation = useImpactMutation<Record<string, never>, ApiPermissionSyncResult>(
    candidate => api.apis.preview({ operation: 'sync-permissions', candidate }),
    (_candidate, fingerprint) => api.apis.syncPermissions({ expected_fingerprint: fingerprint }),
  );
  useEffect(() => {
    if (syncMutation.state.phase !== 'success') return;
    setMessage(`路由同步完成：服务器创建 ${syncMutation.state.result?.count ?? 0} 条 API Metadata。`);
    syncMutation.reset();
    onChanged();
  }, [syncMutation.state.phase]);
  useEffect(() => {
    if (permissionMutation.state.phase !== 'success') return;
    setMessage(`权限同步完成：创建 ${permissionMutation.state.result?.created_count ?? 0} 个权限，更新 ${permissionMutation.state.result?.updated_api ?? 0} 条 API。`);
    permissionMutation.reset();
    onChanged();
  }, [permissionMutation.state.phase]);
  const busy = [syncMutation.state.phase, permissionMutation.state.phase].some(phase => phase === 'previewing' || phase === 'submitting');
  return <Card className="mg-panel">
    <div className="mg-card-head"><div><h2>Route Catalog 同步</h2><p className="mg-hint">预览与提交都由服务器读取同一 Route Catalog；页面不提交路由事实。</p></div><div className="mg-detail-actions"><Button variant="outline" disabled={busy} onClick={() => { setMessage(''); void syncMutation.prepare({}); }}>预览路由同步</Button><Button variant="outline" disabled={busy} onClick={() => { setMessage(''); void permissionMutation.prepare({}); }}>预览权限同步</Button></div></div>
    {message && <p className="mg-success" role="status">{message}</p>}
    <ErrorNotice error={syncMutation.state.error || permissionMutation.state.error} />
    <ImpactPreviewDialog open={syncMutation.state.phase === 'awaiting_confirmation' || syncMutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) syncMutation.reset(); }} preview={syncMutation.state.preview} title="确认同步 API 路由" onConfirm={() => void syncMutation.confirm()} busy={syncMutation.state.phase === 'submitting'} />
    <ImpactPreviewDialog open={permissionMutation.state.phase === 'awaiting_confirmation' || permissionMutation.state.phase === 'submitting'} onOpenChange={open => { if (!open) permissionMutation.reset(); }} preview={permissionMutation.state.preview} title="确认同步 API 权限" onConfirm={() => void permissionMutation.confirm()} busy={permissionMutation.state.phase === 'submitting'} />
  </Card>;
}

export function ApisPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const query = parseApiListQuery(searchParams);
  const queryString = encodeApiListQuery(query);
  const result = useResource(queryString, () => api.apis.list(`/api/admin/apis?${queryString}`));
  const groups = useResource('api-groups', () => api.apis.groups());
  function update(changes: Partial<ApiListQueryState>) {
    setSearchParams(new URLSearchParams(encodeApiListQuery({ ...query, ...changes })));
    setSelectedId(null);
  }
  const total = result.data?.total ?? 0;
  return <section className="management-page">
    <PageHeading eyebrow="配置管理 / 接口" title="接口管理" intro="以 Route Catalog 为事实来源，维护 API Metadata 与授权诊断。" />
    <Card className="mg-toolbar"><ApiFilters query={query} groups={groups.data ?? []} update={update} /></Card>
    <ApiSyncPanel onChanged={result.reload} />
    <ResourceState loading={result.loading} error={result.error} reload={result.reload} />
    {!result.loading && !result.error && !result.data?.list.length && <Notice title="暂无接口">没有符合当前筛选条件的 API Metadata。</Notice>}
    {!!result.data?.list.length && <Card className="mg-list-card"><ApiTable items={result.data.list} onSelect={setSelectedId} /><Pagination list={{ page: query.page, size: query.size, keyword: query.keyword, status: query.status, query: queryString, update: changes => update(changes as Partial<ApiListQueryState>) }} total={total} loading={result.loading} /></Card>}
    {selectedId !== null && <ApiDetailPanel apiId={selectedId} onChanged={result.reload} onDeleted={() => setSelectedId(null)} />}
  </section>;
}
