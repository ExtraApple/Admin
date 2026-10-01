import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { Link, useBlocker, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from '@tanstack/react-table';
import { api, errorMessage, fieldMessages } from '@/lib/api';
import type { AdminUser, Organization, Page } from '@/lib/models';
import { useSession } from '@/lib/session';
import { canAccessModule } from '@/lib/navigation';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Card } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

export function Guard({ module, permissions, children }: { module: 'roles' | 'users' | 'organizations'; permissions: string[]; children: ReactNode }) {
  const { context, can } = useSession();
  if (!context || !canAccessModule(context, module) || !permissions.every(can)) return <div className="management"><Notice title="没有访问权限">当前可见菜单或操作权限不允许打开此页面。请联系管理员核对授权。</Notice></div>;
  return <div className="management">{children}</div>;
}

export function useObjectId() {
  const { id } = useParams();
  return id && /^[1-9]\d*$/.test(id) && Number.isSafeInteger(Number(id)) ? Number(id) : null;
}

export function useResource<T>(key: string, load: () => Promise<T>) {
  const loader = useRef(load);
  loader.current = load;
  const [revision, setRevision] = useState(0);
  const [state, setState] = useState<{ key: string; revision: number; data: T | null; error: unknown; loading: boolean }>({ key, revision, data: null, error: null, loading: true });
  useEffect(() => {
    let active = true;
    setState({ key, revision, data: null, error: null, loading: true });
    loader.current().then(data => { if (active) setState({ key, revision, data, error: null, loading: false }); }, error => { if (active) setState({ key, revision, data: null, error, loading: false }); });
    return () => { active = false; };
  }, [key, revision]);
  const current = state.key === key && state.revision === revision;
  return { data: current ? state.data : null, error: current ? state.error : null, loading: !current || state.loading, reload: useCallback(() => setRevision(value => value + 1), []) };
}

export async function allPages<T>(path: string): Promise<T[]> {
  const result: T[] = [];
  let page = 1;
  for (;;) {
    const response = await api.get<Page<T>>(`${path}${path.includes('?') ? '&' : '?'}page=${page}&size=100`);
    result.push(...response.list);
    if (!response.list.length || response.page * response.size >= response.total) return result;
    page = response.page + 1;
  }
}

export function ResourceState({ loading, error, reload }: { loading: boolean; error: unknown; reload: () => void }) {
  if (loading) return <div className="mg-loading" role="status" aria-live="polite"><span className="mg-loading-line" />正在读取服务器数据…</div>;
  if (error) return <Notice title="读取失败"><ErrorNotice error={error} /><Button variant="outline" onClick={reload}>重新读取</Button></Notice>;
  return null;
}

export function ErrorNotice({ error }: { error: unknown }) {
  if (!error) return null;
  const fields = fieldMessages(error);
  return <div className="mg-error" role="alert"><p>{errorMessage(error)}</p>{Object.keys(fields).length > 0 && <ul>{Object.entries(fields).map(([field, messages]) => <li key={field}><strong>{field}</strong>：{messages.join('；')}</li>)}</ul>}</div>;
}

export function Notice({ title, children }: { title: string; children?: ReactNode }) {
  return <Card className="mg-notice"><h2>{title}</h2>{children && <div>{children}</div>}</Card>;
}
export function PageHeading({ title, intro, eyebrow, back }: { title: string; intro?: string; eyebrow: string; back?: { to: string; label: string } }) {
  const location = useLocation();
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { heading.current?.focus(); }, [location.pathname]);
  const message = location.state && typeof location.state.message === 'string' ? location.state.message : '';
  return <header className="mg-heading">{back && <Link className="mg-back" to={back.to}>← {back.label}</Link>}<span className="mg-eyebrow">{eyebrow}</span><h1 ref={heading} tabIndex={-1}>{title}</h1>{intro && <p>{intro}</p>}{message && <p className="mg-success" role="status">{message}</p>}</header>;
}
export function Status({ value }: { value: number }) { return <span className={`mg-status ${value === 1 ? '' : 'mg-off'}`}>{value === 1 ? '已启用' : '已停用'}</span>; }
export const scopeLabels: Record<string, string> = { all: '全部组织', self: '仅本人', org: '本组织', org_and_children: '本组织及下级', custom: '指定组织单位' };
export function sameIds(a: number[], b: number[]) { return a.length === b.length && a.every(id => b.includes(id)); }
export function toggleId(ids: number[], id: number, checked: boolean) { return checked ? [...ids.filter(value => value !== id), id] : ids.filter(value => value !== id); }
export function protectedReason(user: AdminUser, self: number | undefined) { return user.id === self ? '本人受保护，不能通过管理端修改自己的授权、状态或会话。' : user.roles.some(role => role.code === 'admin') ? '具有 admin 角色的用户受保护，不允许管理端修改。' : ''; }
export function OrganizationsText({ user }: { user: AdminUser }) {
  return <><span>{user.organizations.map(org => org.name).join('、') || (user.has_unmanaged_organizations ? '当前范围内无可见归属' : '未分配组织')}</span>{user.has_unmanaged_organizations && <small className="mg-warning-inline">另有不可见的组织归属</small>}</>;
}

export function Confirm({ open, onOpenChange, title, children, onConfirm, busy = false, label = '确认提交' }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; children: ReactNode; onConfirm: () => void; busy?: boolean; label?: string }) {
  return <AlertDialog open={open} onOpenChange={value => { if (!busy) onOpenChange(value); }}><AlertDialogContent className="mg-dialog"><AlertDialogHeader><AlertDialogTitle>{title}</AlertDialogTitle><AlertDialogDescription asChild><div className="mg-confirm-summary">{children}</div></AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={busy}>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={event => { event.preventDefault(); onConfirm(); }}>{busy ? '正在提交…' : label}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>;
}

export function UnsavedGuard({ dirty, busy = false }: { dirty: boolean; busy?: boolean }) {
  const blocker = useBlocker(dirty || busy);
  useEffect(() => {
    if (!dirty && !busy) return;
    const beforeUnload = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ''; };
    window.addEventListener('beforeunload', beforeUnload);
    return () => window.removeEventListener('beforeunload', beforeUnload);
  }, [dirty, busy]);
  return <Confirm open={blocker.state === 'blocked'} onOpenChange={open => { if (!open && blocker.state === 'blocked') blocker.reset(); }} title={busy ? '提交尚未结束' : '放弃未保存的修改？'} label="放弃修改并离开" busy={busy} onConfirm={() => { if (blocker.state === 'blocked') blocker.proceed(); }}>当前选择尚未保存。留在此页可以继续编辑。</Confirm>;
}

export function useSavedNavigation(to: string) {
  const navigate = useNavigate();
  const [saved, setSaved] = useState(false);
  useEffect(() => { if (saved) navigate(to, { state: { message: '已保存，服务器已确认本次变更。' } }); }, [saved, navigate, to]);
  return { saved, finish: () => setSaved(true) };
}

export function Choice({ checked, onChange, disabled, label, description }: { checked: boolean; onChange: (checked: boolean) => void; disabled?: boolean; label: ReactNode; description?: ReactNode }) {
  const id = useId();
  return <div className="mg-choice" aria-disabled={disabled || undefined}><Checkbox id={id} checked={checked} disabled={disabled} onCheckedChange={value => onChange(value === true)} /><label htmlFor={id}><span>{label}</span>{description && <small>{description}</small>}</label></div>;
}

export function SelectionSummary({ before, after, options }: { before: number[]; after: number[]; options: { id: number; name: string }[] }) {
  const names = (ids: number[]) => ids.map(id => options.find(item => item.id === id)?.name || `ID ${id}`).join('、') || '无';
  return <dl className="mg-summary"><div><dt>新增</dt><dd>{names(after.filter(id => !before.includes(id)))}</dd></div><div><dt>移除</dt><dd>{names(before.filter(id => !after.includes(id)))}</dd></div><div><dt>保存后</dt><dd>{names(after)}</dd></div></dl>;
}

export function flattenTree<T extends { children?: T[] }>(tree: T[]): T[] { return tree.flatMap(node => [node, ...flattenTree(node.children || [])]); }
export function orgPath(id: number, nodes: Organization[]) {
  const names: string[] = [];
  const visited = new Set<number>();
  while (id && !visited.has(id)) {
    visited.add(id);
    const node = nodes.find(item => item.id === id);
    if (!node) break;
    names.unshift(node.name); id = node.parent_id;
  }
  return names.join(' / ');
}
export function OrganizationChoices({ tree, selected, onChange, disabled = false }: { tree: Organization[]; selected: number[]; onChange: (ids: number[]) => void; disabled?: boolean }) {
  const render = (nodes: Organization[]): ReactNode => <ul className="mg-tree-choices">{nodes.map(node => <li key={node.id}><Choice checked={selected.includes(node.id)} disabled={disabled || !node.manageable} onChange={checked => onChange(toggleId(selected, node.id, checked))} label={node.name} description={node.manageable ? node.code : '仅供层级定位，不可选择'} />{node.children?.length ? render(node.children) : null}</li>)}</ul>;
  const all = flattenTree(tree);
  const unavailable = selected.filter(id => !all.some(node => node.id === id && node.manageable));
  return <><p className="mg-hint">独立选择：勾选上级只包含该单位，不自动包含下级。</p>{render(tree)}{unavailable.length > 0 && <div className="mg-warning"><p>以下保留选择不在当前可管理范围中，请明确移除后再保存。用户范围外的现有归属仍由服务器保留。</p>{unavailable.map(id => <Button key={id} variant="outline" disabled={disabled} onClick={() => onChange(selected.filter(value => value !== id))}>移除选择：{all.find(node => node.id === id)?.name || `组织 ID ${id}`}</Button>)}</div>}<div className="mg-selected" aria-live="polite">已明确选择：{selected.map(id => all.find(node => node.id === id)?.name || `ID ${id}`).join('、') || '无'}</div></>;
}

export function DataTable<T>({ columns, data, caption }: { columns: ColumnDef<T>[]; data: T[]; caption: string }) {
  const table = useReactTable({ data, columns, getCoreRowModel: getCoreRowModel(), manualPagination: true });
  return <div className="mg-table-desktop"><Table><caption className="sr-only">{caption}</caption><TableHeader>{table.getHeaderGroups().map(group => <TableRow key={group.id}>{group.headers.map(header => <TableHead scope="col" key={header.id}>{header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}</TableHead>)}</TableRow>)}</TableHeader><TableBody>{table.getRowModel().rows.map(row => <TableRow key={row.id}>{row.getVisibleCells().map(cell => <TableCell key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>)}</TableRow>)}</TableBody></Table></div>;
}

export interface ListQuery {
  page: number;
  size: number;
  keyword: string;
  status: string;
  query: string;
  update: (changes: Record<string, string | number>) => void;
}
export function useListQuery(): ListQuery {
  const [params, setParams] = useSearchParams();
  const rawPage = Number(params.get('page') || 1);
  const rawSize = Number(params.get('size') || 20);
  const page = Number.isSafeInteger(rawPage) && rawPage > 0 ? rawPage : 1;
  const size = [10, 20, 50].includes(rawSize) ? rawSize : 20;
  const keyword = params.get('keyword') || '';
  const status = ['0', '1'].includes(params.get('status') || '') ? params.get('status')! : '';
  const update = (changes: Record<string, string | number>) => { const next = new URLSearchParams(params); Object.entries(changes).forEach(([key, value]) => { if (value === '') next.delete(key); else next.set(key, String(value)); }); setParams(next); };
  const query = new URLSearchParams({ page: String(page), size: String(size) });
  if (keyword) query.set('keyword', keyword);
  if (status) query.set('status', status);
  return { page, size, keyword, status, update, query: query.toString() };
}
export function ListFilters({ list, label }: { list: ListQuery; label: string }) {
  const [draft, setDraft] = useState(list.keyword);
  useEffect(() => setDraft(list.keyword), [list.keyword]);
  const id = useId();
  return <form className="mg-filters" onSubmit={event => { event.preventDefault(); list.update({ keyword: draft.trim(), page: 1 }); }}><label className="sr-only" htmlFor={`${id}-search`}>{label}</label><Input id={`${id}-search`} type="search" value={draft} onChange={event => setDraft(event.target.value)} placeholder={label} /><label className="sr-only" htmlFor={`${id}-status`}>按状态筛选</label><select id={`${id}-status`} value={list.status} onChange={event => list.update({ status: event.target.value, page: 1 })}><option value="">全部状态</option><option value="1">已启用</option><option value="0">已停用</option></select><Button type="submit" variant="outline">搜索</Button></form>;
}
export function Pagination({ list, total, loading }: { list: ListQuery; total: number; loading: boolean }) {
  const pages = Math.max(1, Math.ceil(total / list.size));
  return <nav className="mg-pagination" aria-label="列表分页"><span>范围内共 {total} 项 · 第 {list.page} / {pages} 页</span><div><label>每页 <select value={list.size} onChange={event => list.update({ size: event.target.value, page: 1 })}>{[10, 20, 50].map(size => <option key={size} value={size}>{size}</option>)}</select></label><Button variant="outline" disabled={loading || list.page <= 1} onClick={() => list.update({ page: list.page - 1 })}>上一页</Button><Button variant="outline" disabled={loading || list.page >= pages} onClick={() => list.update({ page: list.page + 1 })}>下一页</Button></div></nav>;
}
