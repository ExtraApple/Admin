import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ApiError } from '@/lib/api';
import { api } from '@/lib/api';
import type { AuditLog, AuditLogPage, AuditLogQueryState } from '@/lib/models';
import {
  AUDIT_METHODS,
  AUDIT_TAB_LABELS,
  auditLogPath,
  defaultAuditRange,
  encodeAuditLogQuery,
  fromDateTimeLocal,
  parseAuditLogQuery,
  presetAuditRange,
  toDateTimeLocal,
  validateAuditRange,
} from '@/lib/audit-query';
import { allowedAuditLogTabs, canAccessAuditLogs } from '@/lib/navigation';
import { useSession } from '@/lib/session';
import { Notice, PageHeading, Pagination, ResourceState, useResource } from '@/components/management/shared';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';

const categoryLabels: Record<string, string> = {
  api: 'API 请求',
  login: '登录',
  operation: '操作',
  permission: '权限变更',
  data_access: '数据访问',
  message: '消息',
};

function statusClass(status: number): string {
  if (status >= 500) return 'mg-audit-status-error';
  if (status >= 400) return 'mg-audit-status-warn';
  if (status >= 300) return 'mg-audit-status-info';
  if (status >= 200) return 'mg-audit-status-ok';
  return '';
}

function AuditStatus({ value }: { value: number }) {
  return <span className={`mg-audit-status ${statusClass(value)}`}>{value || '—'}</span>;
}

function AuditTable({ items, onSelect }: { items: AuditLog[]; onSelect: (log: AuditLog) => void }) {
  return <>
    <div className="mg-table-desktop">
      <Table className="mg-audit-table">
        <caption className="sr-only">审计日志列表</caption>
        <TableHeader><TableRow><TableHead>时间</TableHead><TableHead>分类</TableHead><TableHead>用户</TableHead><TableHead>Method / Path</TableHead><TableHead>状态</TableHead><TableHead>耗时</TableHead><TableHead>客户端 IP</TableHead></TableRow></TableHeader>
        <TableBody>{items.map(log => <TableRow key={log.id}>
          <TableCell><button className="mg-list-link" type="button" aria-label={`查看日志 ${log.id} 详情`} onClick={() => onSelect(log)}>{log.created_at}</button><small>#{log.id}</small></TableCell>
          <TableCell>{categoryLabels[log.category] ?? log.category}</TableCell>
          <TableCell><div className="mg-audit-user"><strong>{log.username || '—'}</strong><small>ID {log.user_id || '—'}</small></div></TableCell>
          <TableCell><code>{log.method} {log.path}</code></TableCell>
          <TableCell><AuditStatus value={log.status} /></TableCell>
          <TableCell>{log.duration} ms</TableCell>
          <TableCell><code>{log.client_ip || '—'}</code></TableCell>
        </TableRow>)}</TableBody>
      </Table>
    </div>
    <div className="mg-mobile-cards">{items.map(log => <Card key={log.id} className="mg-mobile-card">
      <div className="mg-card-head"><h2><button className="mg-list-link" type="button" aria-label={`查看日志 ${log.id} 详情`} onClick={() => onSelect(log)}>{log.created_at}</button></h2><AuditStatus value={log.status} /></div>
      <p>{categoryLabels[log.category] ?? log.category} · {log.username || '—'}（ID {log.user_id || '—'}）</p>
      <code>{log.method} {log.path}</code>
      <p className="mg-hint">{log.duration} ms · {log.client_ip || '无客户端 IP'}</p>
    </Card>)}</div>
  </>;
}

interface FilterProps {
  query: AuditLogQueryState;
  onApply: (changes: Partial<AuditLogQueryState>) => void;
  onReset: () => void;
  onPreset: (hours: number) => void;
}

function AuditFilters({ query, onApply, onReset, onPreset }: FilterProps) {
  const [start, setStart] = useState(toDateTimeLocal(query.start));
  const [end, setEnd] = useState(toDateTimeLocal(query.end));
  const [method, setMethod] = useState(query.method);
  const [status, setStatus] = useState(query.status);
  const [path, setPath] = useState(query.path);
  const [username, setUsername] = useState(query.username);
  const [userId, setUserId] = useState(query.userId);
  const [error, setError] = useState('');
  useEffect(() => {
    setStart(toDateTimeLocal(query.start));
    setEnd(toDateTimeLocal(query.end));
    setMethod(query.method);
    setStatus(query.status);
    setPath(query.path);
    setUsername(query.username);
    setUserId(query.userId);
  }, [query.start, query.end, query.method, query.status, query.path, query.username, query.userId]);
  function submit(event: FormEvent) {
    event.preventDefault();
    const startValue = fromDateTimeLocal(start);
    const endValue = fromDateTimeLocal(end);
    if (!startValue || !endValue) {
      setError('请选择有效的开始和结束时间。');
      return;
    }
    const rangeError = validateAuditRange(startValue, endValue);
    if (rangeError) {
      setError(rangeError);
      return;
    }
    if (status && !/^[1-5]\d\d$/.test(status)) {
      setError('状态码必须是 100–599 的三位数字，或留空。');
      return;
    }
    if (userId && !/^[1-9]\d*$/.test(userId)) {
      setError('用户 ID 必须是正整数，或留空。');
      return;
    }
    setError('');
    onApply({ start: startValue, end: endValue, method, status, path: path.trim(), username: username.trim(), userId, page: 1 });
  }
  return <form className="mg-audit-filters" onSubmit={submit}>
    <label>开始时间（本地）<input type="datetime-local" step="1" value={start} onChange={event => setStart(event.target.value)} required /></label>
    <label>结束时间（本地）<input type="datetime-local" step="1" value={end} onChange={event => setEnd(event.target.value)} required /></label>
    <label>Method<select value={method} onChange={event => setMethod(event.target.value)}><option value="">全部 Method</option>{AUDIT_METHODS.map(value => <option key={value} value={value}>{value}</option>)}</select></label>
    <label>状态码<input type="number" min={100} max={599} inputMode="numeric" placeholder="如 403" value={status} onChange={event => setStatus(event.target.value)} /></label>
    <label>路径包含<input type="search" placeholder="如 /api/admin" value={path} onChange={event => setPath(event.target.value)} /></label>
    <label>用户名包含<input type="search" placeholder="如 alice" value={username} onChange={event => setUsername(event.target.value)} /></label>
    <label>用户 ID<input type="number" min={1} inputMode="numeric" placeholder="精确匹配" value={userId} onChange={event => setUserId(event.target.value)} /></label>
    <div className="mg-audit-presets" aria-label="时间范围预设">
      {[[1, '1 小时'], [24, '24 小时'], [168, '7 天'], [720, '30 天'], [2160, '90 天']].map(([hours, label]) => <Button key={hours} type="button" variant="outline" onClick={() => { setError(''); onPreset(hours as number); }}>{label}</Button>)}
    </div>
    {error && <div className="mg-error mg-audit-range" role="alert"><p>{error}</p></div>}
    <div className="mg-audit-actions"><Button type="button" variant="outline" onClick={onReset}>重置</Button><Button type="submit">查询</Button></div>
  </form>;
}

function metadataText(value: unknown): string {
  if (value === null || value === undefined) return '';
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2);
}

function LogBlock({ title, value }: { title: string; value: string }) {
  const [open, setOpen] = useState(false);
  const [copyState, setCopyState] = useState('');
  if (!value) return <section className="mg-audit-block"><h3>{title}</h3><p className="mg-hint">无</p></section>;
  const long = value.length > 240;
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopyState('已复制');
    } catch {
      setCopyState('复制失败，请手动选择');
    }
  }
  return <section className="mg-audit-block">
    <div className="mg-card-head"><h3>{title}</h3><div className="mg-detail-actions"><Button type="button" variant="outline" onClick={() => void copy()}>{copyState || '复制'}</Button>{long && <Button type="button" variant="outline" onClick={() => setOpen(!open)}>{open ? '收起' : '展开'}</Button>}</div></div>
    <pre className={`mg-audit-pre ${long && !open ? 'mg-audit-pre-collapsed' : ''}`}>{value}</pre>
  </section>;
}

function AuditDetail({ log, onOpenChange }: { log: AuditLog | null; onOpenChange: (open: boolean) => void }) {
  const lastLog = useRef(log);
  if (log) lastLog.current = log;
  const shown = log ?? lastLog.current;
  if (!shown) return null;
  return <Sheet open={!!log} onOpenChange={onOpenChange}>
    <SheetContent side="right" className="mg-audit-sheet" closeLabel="关闭审计日志详情">
      <SheetHeader><SheetTitle>审计日志 #{shown.id}</SheetTitle><SheetDescription>{shown.created_at} · {categoryLabels[shown.category] ?? shown.category}</SheetDescription></SheetHeader>
      <dl className="mg-detail-list">
        <li><strong>用户</strong><span>{shown.username || '—'}（ID {shown.user_id || '—'}）</span></li>
        <li><strong>Method / Path</strong><code>{shown.method} {shown.path}</code></li>
        <li><strong>状态 / 耗时</strong><span><AuditStatus value={shown.status} /> · {shown.duration} ms</span></li>
        <li><strong>客户端 IP</strong><code>{shown.client_ip || '—'}</code></li>
      </dl>
      <LogBlock title="Query" value={shown.query} />
      <LogBlock title="Body" value={shown.body} />
      <LogBlock title="Metadata" value={metadataText(shown.metadata)} />
      <LogBlock title="User-Agent" value={shown.user_agent} />
    </SheetContent>
  </Sheet>;
}

export function AuditGuard({ children }: { children: ReactNode }) {
  const { context } = useSession();
  if (!canAccessAuditLogs(context)) return <div className="management"><Notice title="没有访问权限">当前账号没有读取审计日志的权限。请联系管理员核对授权。</Notice></div>;
  return <div className="management">{children}</div>;
}

export function AuditLogsPage() {
  const { context } = useSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const now = useRef(new Date());
  const parsed = parseAuditLogQuery(searchParams, now.current);
  const allowed = allowedAuditLogTabs(context);
  const type = allowed.includes(parsed.type) ? parsed.type : allowed[0] ?? 'all';
  const query: AuditLogQueryState = { ...parsed, type };
  const rangeError = validateAuditRange(query.start, query.end);
  const queryKey = encodeAuditLogQuery(query) + (rangeError ? '|invalid' : '');
  const result = useResource<AuditLogPage | null>(queryKey, () => {
    if (!allowed.length || rangeError) return Promise.resolve(null);
    return api.auditLogs.list(auditLogPath(query));
  });
  const [selected, setSelected] = useState<AuditLog | null>(null);

  useEffect(() => {
    const next = encodeAuditLogQuery({ ...parseAuditLogQuery(searchParams, now.current), type });
    if (next !== searchParams.toString()) setSearchParams(new URLSearchParams(next), { replace: true });
  }, [searchParams, setSearchParams, type]);

  function update(changes: Partial<AuditLogQueryState>) {
    setSearchParams(new URLSearchParams(encodeAuditLogQuery({ ...query, ...changes })));
    setSelected(null);
  }

  function reset() {
    const range = defaultAuditRange();
    update({ ...query, start: range.start, end: range.end, method: '', status: '', path: '', username: '', userId: '', page: 1 });
  }

  const denied = result.error instanceof ApiError && result.error.status === 403;
  const total = result.data?.total ?? 0;
  if (!allowed.length) return <section className="management-page"><Notice title="没有访问权限">当前账号没有读取审计日志的权限。请联系管理员核对授权。</Notice></section>;
  return <section className="management-page">
    <PageHeading eyebrow="审计 / 审计日志" title="审计日志" intro="按类型查询真实审计记录，只读；仅查询热表，不包含已归档数据。" />
    <nav className="mg-audit-tabs" aria-label="审计日志类型">
      {allowed.map(tab => <button key={tab} type="button" aria-current={tab === type ? 'page' : undefined} onClick={() => update({ type: tab, page: 1 })}>{AUDIT_TAB_LABELS[tab]}</button>)}
    </nav>
    <Card className="mg-toolbar mg-audit-toolbar"><AuditFilters query={query} onApply={update} onReset={reset} onPreset={hours => { const range = presetAuditRange(hours); update({ start: range.start, end: range.end, page: 1 }); }} /></Card>
    {rangeError && <Notice title="时间范围无效"><p>{rangeError}</p></Notice>}
    {!rangeError && denied && <Notice title="没有访问权限"><p>当前账号没有读取该类型审计日志的权限。</p><Button variant="outline" onClick={result.reload}>重新读取</Button></Notice>}
    {!rangeError && !denied && <ResourceState loading={result.loading} error={result.error} reload={result.reload} />}
    {!rangeError && !denied && !result.loading && !result.error && !result.data?.list.length && <Notice title="暂无日志">没有符合当前条件的审计日志。</Notice>}
    {!rangeError && !!result.data?.list.length && <Card className="mg-list-card mg-audit-list"><AuditTable items={result.data.list} onSelect={setSelected} /><Pagination list={{ page: query.page, size: query.size, keyword: '', status: '', query: '', update: changes => update({ page: Number(changes.page ?? query.page), size: Number(changes.size ?? query.size) }) }} total={total} loading={result.loading} /></Card>}
    <AuditDetail log={selected} onOpenChange={open => { if (!open) setSelected(null); }} />
  </section>;
}
