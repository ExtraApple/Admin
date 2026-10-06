import { useCallback, useMemo, useState } from 'react';
import { Network, Shield, ShieldCheck, TriangleAlert, Users, UserRound, type LucideIcon } from 'lucide-react';
import type { UserContext } from './models';
import { canAccessAuthorizationOverview, canAccessAuthorizationRisks, canAccessModule } from './navigation';

export type WorkbenchGroupId = 'authorization-review' | 'identity-organization' | 'account';
export interface WorkbenchItem { to: string; label: string; icon: LucideIcon }
export interface WorkbenchGroup { id: WorkbenchGroupId; label: string; items: WorkbenchItem[] }

type NavigationItem = WorkbenchItem & {
  canAccess: (context: UserContext | null) => boolean;
  matchDescendants?: boolean;
};
type NavigationGroup = Omit<WorkbenchGroup, 'items'> & { items: NavigationItem[] };

const navigationGroups: NavigationGroup[] = [
  {
    id: 'authorization-review', label: '授权核对', items: [
      { to: '/authorization-overview', label: '授权总览', icon: ShieldCheck, canAccess: canAccessAuthorizationOverview },
      { to: '/authorization-risks', label: '风险清单', icon: TriangleAlert, canAccess: canAccessAuthorizationRisks },
    ],
  },
  {
    id: 'identity-organization', label: '身份与组织', items: [
      { to: '/roles', label: '角色管理', icon: Shield, canAccess: context => canAccessModule(context, 'roles'), matchDescendants: true },
      { to: '/users', label: '用户管理', icon: Users, canAccess: context => canAccessModule(context, 'users'), matchDescendants: true },
      { to: '/organizations', label: '组织管理', icon: Network, canAccess: context => canAccessModule(context, 'organizations'), matchDescendants: true },
    ],
  },
  {
    id: 'account', label: '账户', items: [
      { to: '/profile', label: '个人资料', icon: UserRound, canAccess: context => context !== null },
    ],
  },
];

const descendantPaths: Record<string, boolean> = Object.fromEntries(navigationGroups.flatMap(group => group.items.map(item => [item.to, !!item.matchDescendants])));

function matchesPath(to: string, pathname: string): boolean {
  return pathname === to || (!!descendantPaths[to] && pathname.startsWith(`${to}/`));
}

export function getWorkbenchGroups(context: UserContext | null): WorkbenchGroup[] {
  return navigationGroups.map(group => ({
    id: group.id,
    label: group.label,
    items: group.items.filter(item => item.canAccess(context)).map(({ to, label, icon }) => ({ to, label, icon })),
  })).filter(group => group.items.length > 0);
}

export function getInitialOpenGroups(groups: WorkbenchGroup[], pathname: string): Record<WorkbenchGroupId, boolean> {
  const openGroups: Record<WorkbenchGroupId, boolean> = {
    'authorization-review': false,
    'identity-organization': false,
    account: false,
  };
  for (const group of groups) {
    openGroups[group.id] = group.items.some(item => matchesPath(item.to, pathname));
  }
  return openGroups;
}

export function getWorkbenchLabel(pathname: string): string {
  for (const group of navigationGroups) {
    const item = group.items.find(item => matchesPath(item.to, pathname));
    if (item) return item.label;
  }
  return '访问说明';
}

export function useWorkbenchNavigation(context: UserContext | null, pathname: string) {
  const groups = useMemo(() => getWorkbenchGroups(context), [context]);
  // Only the first visible route initializes state; later routes and permissions leave user choices intact.
  const [openGroups, setOpenGroups] = useState(() => getInitialOpenGroups(groups, pathname));
  const setGroupOpen = useCallback((id: WorkbenchGroupId, open: boolean) => {
    setOpenGroups(current => current[id] === open ? current : { ...current, [id]: open });
  }, []);
  return { groups, openGroups, setGroupOpen };
}
