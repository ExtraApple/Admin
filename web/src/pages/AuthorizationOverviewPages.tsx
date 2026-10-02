import { useEffect, useState, type ReactNode } from 'react';
import { AlertTriangle, ArrowLeft, ArrowRight, CheckCircle2, ExternalLink, RotateCcw, ShieldCheck } from 'lucide-react';
import { Link, useLocation, useSearchParams } from 'react-router-dom';
import type { AuthorizationOverview, AuthorizationRisk, AuthorizationRiskKind, AuthorizationRiskPage, AuthorizationRiskQuery } from '@/lib/models';
import { api, errorMessage } from '@/lib/api';
import { canAccessAuthorizationOverview, canAccessAuthorizationRisks } from '@/lib/navigation';
import { useSession } from '@/lib/session';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { PageHeading, scopeLabels, useResource } from '@/components/management/shared';
import '@/management.css';

const riskLabels: Record<AuthorizationRiskKind, string> = {
  disabled_assigned_menu: '已分配菜单已停用',
  missing_menu_permission: '菜单权限码缺失',
  used_role_without_permissions: '已使用角色无权限',
  managed_user_without_role: '可管理用户无角色',
};

export function authorizationRiskLabel(kind: AuthorizationRiskKind): string {
  return riskLabels[kind] || kind;
}

function riskPath(risk: AuthorizationRisk): string | null {
  if (risk.resource === 'role') return `/roles/${risk.resource_id}`;
  if (risk.resource === 'user') return `/users/${risk.resource_id}`;
  return null;
}

function displayRiskName(risk: AuthorizationRisk): string {
  return risk.resource_name || `${risk.resource === 'role' ? '角色' : '用户'} ID ${risk.resource_id}`;
}

function ScopeBand({ scope }: { scope: AuthorizationOverview['scope'] }) {
  const label = scopeLabels[scope.data_scope] || scope.data_scope;
  return <Card className="mg-overview-scope" aria-label="当前授权影响范围"><CardContent><div><span className="mg-eyebrow">当前查看范围</span><strong>{label}</strong><p>只读范围事实，不是临时筛选器，也不等同于最终有效权限清单。</p></div><Badge variant={scope.data_scope === 'all' ? 'default' : 'secondary'}>{scope.organization_count} 个可见组织</Badge></CardContent></Card>;
}

function SummaryCard({ title, total, details, icon }: { title: string; total: number; details: string[]; icon: ReactNode }) {
  return <Card className="mg-overview-summary-card"><CardHeader><CardDescription>{title}</CardDescription><CardTitle>{total}</CardTitle></CardHeader><CardContent><div className="mg-overview-summary-details">{details.map(detail => <span key={detail}>{detail}</span>)}</div><span className="mg-overview-summary-icon" aria-hidden="true">{icon}</span></CardContent></Card>;
}

function RiskCard({ risk }: { risk: AuthorizationRisk }) {
  const { can } = useSession();
  const location = useLocation();
  const path = riskPath(risk);
  const canDetail = path && (risk.resource === 'role' ? can('admin.roles.id.get') : can('admin.users.id.get'));
  const name = displayRiskName(risk);
  return <Card className="mg-risk-card"><CardHeader><div className="mg-risk-card-heading"><div><CardDescription>{risk.resource === 'role' ? '角色' : '用户'} · 需要核对</CardDescription><CardTitle>{canDetail ? <Link to={path} state={{ returnTo: `${location.pathname}${location.search}` }}>{name}<ExternalLink size={15} aria-hidden="true" /></Link> : name}</CardTitle></div><Badge variant="destructive">{risk.issue_count} 项</Badge></div></CardHeader><CardContent><div className="mg-risk-kinds" aria-label="风险类型">{risk.issue_kinds.map(kind => <Badge key={kind} variant="outline">{authorizationRiskLabel(kind)}</Badge>)}</div><dl className="mg-risk-impact"><div><dt>潜在影响用户</dt><dd>{risk.potentially_affected_users}</dd></div><div><dt>潜在影响组织</dt><dd>{risk.potentially_affected_organizations}</dd></div></dl><p className="mg-risk-policy">{risk.session_policy.revalidate_on_next_request ? '授权变化将在后续请求重新校验。' : '会话规则由服务端返回。'}</p></CardContent></Card>;
}

function RiskCards({ risks, emptyLabel = '当前范围内无需核对的配置风险。' }: { risks: AuthorizationRisk[]; emptyLabel?: string }) {
  if (!risks.length) return <Card className="mg-overview-empty"><CardContent><CheckCircle2 size={26} aria-hidden="true" /><strong>{emptyLabel}</strong><p>仍可使用上方范围和授权摘要核对当前授权状态。</p></CardContent></Card>;
  return <div className="mg-risk-list">{risks.map(risk => <RiskCard key={`${risk.resource}:${risk.resource_id}`} risk={risk} />)}</div>;
}
function OverviewSkeleton() {
  return <div className="mg-overview-skeleton" role="status" aria-live="polite" aria-label="正在读取授权总览"><span /><span /><span /><span /><span /><span /></div>;
}

function ReadFailure({ error, reload }: { error: unknown; reload: () => void }) {
  return <Alert variant="destructive"><AlertTitle>授权总览读取失败</AlertTitle><AlertDescription><p>{errorMessage(error)}</p><Button variant="outline" onClick={reload}><RotateCcw size={16} aria-hidden="true" />重新读取</Button></AlertDescription></Alert>;
}

export function AuthorizationOverviewPage() {
  const { context } = useSession();
  const resource = useResource('authorization-overview', () => api.authorizationOverview());
  if (!context || !canAccessAuthorizationOverview(context)) return <div className="management"><Alert variant="destructive"><AlertTitle>没有访问权限</AlertTitle><AlertDescription>当前账号不能读取授权总览。</AlertDescription></Alert></div>;
  return <div className="management mg-overview"><PageHeading title="授权总览" eyebrow="AUTHORIZATION · 范围审查" intro="先核对当前范围内的授权配置风险，再进入角色或用户详情。这里不执行任何修复写入。" />{resource.loading ? <OverviewSkeleton /> : resource.error ? <ReadFailure error={resource.error} reload={resource.reload} /> : resource.data ? <OverviewContent overview={resource.data} /> : null}</div>;
}

function OverviewContent({ overview }: { overview: AuthorizationOverview }) {
  return <><ScopeBand scope={overview.scope} /><div className="mg-overview-layout"><section aria-labelledby="authorization-risk-heading"><div className="mg-overview-section-heading"><div><span className="mg-eyebrow">优先核对</span><h2 id="authorization-risk-heading">配置风险</h2><p>服务端已按潜在影响范围排序；风险提示描述配置待核对状态，不代表安全事件。</p></div><Badge variant={overview.risks.total ? 'destructive' : 'default'}>{overview.risks.total} 个风险对象</Badge></div><RiskCards risks={overview.risks.items} />{overview.risks.has_more && <Card className="mg-overview-more"><CardContent><div><strong>还有未展示的风险对象</strong><p>总览只展示前 {overview.risks.limit} 条，请进入完整清单继续核对。</p></div><Button asChild variant="outline"><Link to="/authorization-risks">查看全部风险<ArrowRight size={16} aria-hidden="true" /></Link></Button></CardContent></Card>}</section><aside className="mg-overview-summary" aria-labelledby="authorization-summary-heading"><div className="mg-overview-section-heading"><div><span className="mg-eyebrow">配置摘要</span><h2 id="authorization-summary-heading">授权状态</h2></div></div><div className="mg-summary-cards"><SummaryCard title="角色" total={overview.summary.roles.total} details={[`启用 ${overview.summary.roles.enabled}`, `已被用户使用 ${overview.summary.roles.used ?? 0}`]} icon={<ShieldCheck size={21} />} /><SummaryCard title="用户" total={overview.summary.users.total} details={[`启用 ${overview.summary.users.enabled}`, `无角色 ${overview.summary.users.without_role ?? 0}`]} icon={<AlertTriangle size={21} />} /><SummaryCard title="组织" total={overview.summary.organizations.total} details={[`可管理 ${overview.summary.organizations.manageable ?? 0}`, '按当前范围统计']} icon={<CheckCircle2 size={21} />} /></div><Card className="mg-overview-readonly"><CardHeader><CardTitle>只读核对</CardTitle><CardDescription>风险卡片只提供领域详情入口。</CardDescription></CardHeader><CardContent><p>不会在此页面启用菜单、补充权限、修改角色或调整组织归属。</p></CardContent></Card></aside></div></>;
}

function parsePage(value: string | null): number {
  const page = Number(value || 1);
  return Number.isSafeInteger(page) && page > 0 ? page : 1;
}

function RiskFilters({ kind, resource, keyword, onChange, onKeywordChange, onReset }: { kind: string; resource: string; keyword: string; onChange: (changes: Record<string, string>) => void; onKeywordChange: (value: string) => void; onReset: () => void }) {
  return <form className="mg-risk-filters" onSubmit={event => { event.preventDefault(); onChange({ keyword: keyword.trim(), page: '1' }); }}><label><span>风险类型</span><Select value={kind || 'all'} onValueChange={value => onChange({ kind: value === 'all' ? '' : value, page: '1' })}><SelectTrigger aria-label="风险类型"><SelectValue placeholder="全部风险类型" /></SelectTrigger><SelectContent><SelectItem value="all">全部风险类型</SelectItem>{Object.entries(riskLabels).map(([value, label]) => <SelectItem value={value} key={value}>{label}</SelectItem>)}</SelectContent></Select></label><label><span>目标资源</span><Select value={resource || 'all'} onValueChange={value => onChange({ resource: value === 'all' ? '' : value, page: '1' })}><SelectTrigger aria-label="目标资源"><SelectValue placeholder="全部资源" /></SelectTrigger><SelectContent><SelectItem value="all">全部资源</SelectItem><SelectItem value="role">角色</SelectItem><SelectItem value="user">用户</SelectItem></SelectContent></Select></label><label className="mg-risk-keyword"><span>关键字</span><Input value={keyword} onChange={event => onKeywordChange(event.target.value)} placeholder="目标名称" /></label><Button type="submit" variant="outline">应用筛选</Button><Button type="button" variant="ghost" onClick={onReset}><RotateCcw size={16} aria-hidden="true" />重置</Button></form>;
}

function RiskPager({ page, size, total, loading, onPage }: { page: number; size: number; total: number; loading: boolean; onPage: (page: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / size));
  return <nav className="mg-risk-pager" aria-label="授权风险分页"><span>范围内共 {total} 个风险对象 · 第 {page} / {pages} 页</span><div><Button variant="outline" disabled={loading || page <= 1} onClick={() => onPage(page - 1)}><ArrowLeft size={16} aria-hidden="true" />上一页</Button><Button variant="outline" disabled={loading || page >= pages} onClick={() => onPage(page + 1)}>下一页<ArrowRight size={16} aria-hidden="true" /></Button></div></nav>;
}

export function AuthorizationRisksPage() {
  const { context } = useSession();
  const location = useLocation();
  const [params, setParams] = useSearchParams();
  const page = parsePage(params.get('page'));
  const size = 10;
  const kind = params.get('kind') || '';
  const resource = params.get('resource') || '';
  const keyword = params.get('keyword') || '';
  const [draftKeyword, setDraftKeyword] = useState(keyword);
  useEffect(() => setDraftKeyword(keyword), [keyword]);
  const query: AuthorizationRiskQuery = { page, size, kind: kind ? kind as AuthorizationRiskQuery['kind'] : undefined, resource: resource ? resource as AuthorizationRiskQuery['resource'] : undefined, keyword };
  const queryString = new URLSearchParams({ page: String(page), size: String(size), ...(kind ? { kind } : {}), ...(resource ? { resource } : {}), ...(keyword ? { keyword } : {}) }).toString();
  const result = useResource(`authorization-risks:${queryString}`, () => api.authorizationRisks(query));
  if (!context || !canAccessAuthorizationRisks(context)) return <div className="management"><Alert variant="destructive"><AlertTitle>没有访问权限</AlertTitle><AlertDescription>当前账号不能读取授权风险清单。</AlertDescription></Alert></div>;
  const update = (changes: Record<string, string>) => { const next = new URLSearchParams(params); Object.entries(changes).forEach(([name, value]) => { if (value) next.set(name, value); else next.delete(name); }); setParams(next); };
  const reset = () => { setDraftKeyword(''); setParams(new URLSearchParams({ page: '1', size: String(size) })); };
  const canReturnToOverview = canAccessAuthorizationOverview(context);
  return <div className="management mg-overview"><PageHeading title="授权风险清单" eyebrow="AUTHORIZATION · 完整核对" back={canReturnToOverview ? { to: '/authorization-overview', label: '返回授权总览' } : undefined} intro="按服务端风险规则分页读取当前范围内的完整配置风险。这里不执行任何修复写入。" /><Card className="mg-risk-filter-card"><CardHeader><CardTitle>筛选范围内风险</CardTitle><CardDescription>筛选只改变服务器查询条件，不会扩大当前操作者的数据范围。</CardDescription></CardHeader><CardContent><RiskFilters kind={kind} resource={resource} keyword={draftKeyword} onChange={update} onKeywordChange={setDraftKeyword} onReset={reset} /></CardContent></Card>{result.loading ? <OverviewSkeleton /> : result.error ? <ReadFailure error={result.error} reload={result.reload} /> : result.data ? <RiskListContent page={result.data} returnTo={`${location.pathname}${location.search}`} onPage={value => update({ page: String(value) })} canReturnToOverview={canReturnToOverview} /> : null}</div>;
}

function RiskListContent({ page, returnTo, onPage, canReturnToOverview }: { page: AuthorizationRiskPage; returnTo: string; onPage: (page: number) => void; canReturnToOverview: boolean }) {
  return <><ScopeBand scope={page.scope} /><div className="mg-overview-risk-results"><RiskCards risks={page.list} emptyLabel="当前筛选条件下没有配置风险。" />{page.list.length > 0 || page.total > 0 ? <RiskPager page={page.page} size={page.size} total={page.total} loading={false} onPage={onPage} /> : null}</div><div className="mg-risk-context">{canReturnToOverview && <Link to="/authorization-overview">返回总览</Link>}<span>当前清单地址为：{returnTo}，详情可通过浏览器返回继续核对。</span></div></>;
}
