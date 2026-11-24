import { useCallback, useMemo, useState } from 'react';
import { FileClock, Menu as MenuIcon, Network, Route, Shield, ShieldCheck, TriangleAlert, Users, type LucideIcon } from 'lucide-react';
import type { UserContext } from './models';
import { canAccessAuditLogs, canAccessAuthorizationOverview, canAccessAuthorizationRisks, canAccessModule, hasPermission, type ManagementModule } from './navigation';

export type WorkbenchGroupId = 'authorization-review' | 'identity-organization' | 'configuration-management' | 'audit';
export type WorkbenchModuleKey =
  | 'authorization-overview'
  | 'authorization-risks'
  | 'roles'
  | 'users'
  | 'organizations'
  | 'menus'
  | 'apis'
  | 'audit-logs'
  | 'profile'
  | 'no-management';
export interface WorkbenchItem { to: string; label: string; icon: LucideIcon }
export interface WorkbenchGroup { id: WorkbenchGroupId; label: string; items: WorkbenchItem[] }
export interface WorkbenchBreadcrumbItem { label: string; to?: string }
export interface WorkbenchModuleMeta {
  key: WorkbenchModuleKey;
  label: string;
  rootPath: string;
}

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
    id: 'configuration-management', label: '配置管理', items: [
      { to: '/menus', label: '菜单管理', icon: MenuIcon, canAccess: context => canAccessModule(context, 'menus'), matchDescendants: true },
      { to: '/apis', label: '接口管理', icon: Route, canAccess: context => canAccessModule(context, 'apis'), matchDescendants: true },
    ],
  },
  {
    id: 'audit', label: '审计', items: [
      { to: '/audit-logs', label: '审计日志', icon: FileClock, canAccess: canAccessAuditLogs, matchDescendants: true },
    ],
  },
];

const managementModules: ManagementModule[] = ['roles', 'users', 'organizations', 'menus', 'apis'];

const moduleCatalog: Record<WorkbenchModuleKey, { label: string; rootPath: string; canAccess: (context: UserContext | null) => boolean }> = {
  'authorization-overview': { label: '授权总览', rootPath: '/authorization-overview', canAccess: canAccessAuthorizationOverview },
  'authorization-risks': { label: '风险清单', rootPath: '/authorization-risks', canAccess: canAccessAuthorizationRisks },
  roles: { label: '角色管理', rootPath: '/roles', canAccess: context => canAccessModule(context, 'roles') },
  users: { label: '用户管理', rootPath: '/users', canAccess: context => canAccessModule(context, 'users') },
  organizations: { label: '组织管理', rootPath: '/organizations', canAccess: context => canAccessModule(context, 'organizations') },
  menus: { label: '菜单管理', rootPath: '/menus', canAccess: context => canAccessModule(context, 'menus') },
  apis: { label: '接口管理', rootPath: '/apis', canAccess: context => canAccessModule(context, 'apis') },
  'audit-logs': { label: '审计日志', rootPath: '/audit-logs', canAccess: canAccessAuditLogs },
  profile: { label: '个人资料', rootPath: '/profile', canAccess: context => !!context },
  'no-management': { label: '访问说明', rootPath: '/no-management', canAccess: context => !!context },
};

const roleEditLabels: Record<string, string> = {
  permissions: '编辑权限',
  menus: '编辑菜单',
  'data-scope': '编辑数据范围',
};
const userEditLabels: Record<string, string> = {
  roles: '编辑角色',
  organizations: '编辑组织',
};

function normalizePathname(pathname: string): string {
  if (!pathname) return '/';
  return pathname.length > 1 && pathname.endsWith('/') ? pathname.slice(0, -1) : pathname;
}

function decodeSegment(segment: string): string {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

interface MatchedRoute {
  key: WorkbenchModuleKey | null;
  breadcrumbs: WorkbenchBreadcrumbItem[];
  canAccess: (context: UserContext | null) => boolean;
}

function hasPermissions(context: UserContext | null, permissions: string[]): boolean {
  return permissions.every(permission => hasPermission(context, permission));
}

function hasHomeRedirect(context: UserContext | null): boolean {
  return canAccessAuthorizationOverview(context) || managementModules.some(module => canAccessModule(context, module));
}

function matchWorkbenchRoute(pathname: string): MatchedRoute {
  const normalized = normalizePathname(pathname);
  const root: WorkbenchBreadcrumbItem = { label: '工作台', to: '/' };

  if (normalized === '/') {
    return {
      key: 'no-management',
      breadcrumbs: [root, { label: '访问说明' }],
      canAccess: context => !!context && !hasHomeRedirect(context),
    };
  }
  if (normalized === '/authorization-overview') {
    return { key: 'authorization-overview', breadcrumbs: [root, { label: '授权总览' }], canAccess: canAccessAuthorizationOverview };
  }
  if (normalized === '/authorization-risks') {
    return { key: 'authorization-risks', breadcrumbs: [root, { label: '风险清单' }], canAccess: canAccessAuthorizationRisks };
  }
  if (normalized === '/menus') {
    return { key: 'menus', breadcrumbs: [root, { label: '菜单管理' }], canAccess: context => canAccessModule(context, 'menus') };
  }
  if (normalized === '/apis') {
    return { key: 'apis', breadcrumbs: [root, { label: '接口管理' }], canAccess: context => canAccessModule(context, 'apis') };
  }
  if (normalized === '/audit-logs') {
    return { key: 'audit-logs', breadcrumbs: [root, { label: '审计日志' }], canAccess: canAccessAuditLogs };
  }
  if (normalized === '/profile') {
    return { key: 'profile', breadcrumbs: [root, { label: '个人资料' }], canAccess: context => !!context };
  }
  if (normalized === '/no-management') {
    return { key: 'no-management', breadcrumbs: [root, { label: '访问说明' }], canAccess: context => !!context };
  }

  const segments = normalized.slice(1).split('/');
  if (segments[0] === 'roles') {
    if (segments.length === 1) {
      return { key: 'roles', breadcrumbs: [root, { label: '角色管理' }], canAccess: context => canAccessModule(context, 'roles') };
    }
    if (segments.length === 2 && segments[1] === 'new') {
      return {
        key: 'roles',
        breadcrumbs: [root, { label: '角色管理', to: '/roles' }, { label: '新建角色' }],
        canAccess: context => canAccessModule(context, 'roles') && hasPermission(context, 'admin.roles.post'),
      };
    }
    const id = decodeSegment(segments[1]);
    const detailPath = `/roles/${segments[1]}`;
    if (segments.length === 2) {
      return {
        key: 'roles',
        breadcrumbs: [root, { label: '角色管理', to: '/roles' }, { label: `角色详情 #${id}` }],
        canAccess: context => canAccessModule(context, 'roles') && hasPermission(context, 'admin.roles.id.get'),
      };
    }
    if (segments.length === 3 && segments[2] in roleEditLabels) {
      const kind = segments[2];
      const required = ['admin.roles.id.get', `admin.roles.id.${kind}.get`, `admin.roles.id.${kind}.post`];
      if (kind === 'permissions') required.push('admin.permissions.get');
      if (kind === 'menus') required.push('admin.menus.get');
      return {
        key: 'roles',
        breadcrumbs: [root, { label: '角色管理', to: '/roles' }, { label: `角色详情 #${id}`, to: detailPath }, { label: roleEditLabels[kind] }],
        canAccess: context => canAccessModule(context, 'roles') && hasPermissions(context, required),
      };
    }
  }

  if (segments[0] === 'users') {
    if (segments.length === 1) {
      return { key: 'users', breadcrumbs: [root, { label: '用户管理' }], canAccess: context => canAccessModule(context, 'users') };
    }
    if (segments.length === 2 && segments[1] === 'new') {
      return {
        key: 'users',
        breadcrumbs: [root, { label: '用户管理', to: '/users' }, { label: '新建用户' }],
        canAccess: context => canAccessModule(context, 'users') && hasPermission(context, 'admin.users.post'),
      };
    }
    const id = decodeSegment(segments[1]);
    const detailPath = `/users/${segments[1]}`;
    if (segments.length === 2) {
      return {
        key: 'users',
        breadcrumbs: [root, { label: '用户管理', to: '/users' }, { label: `用户详情 #${id}` }],
        canAccess: context => canAccessModule(context, 'users') && hasPermission(context, 'admin.users.id.get'),
      };
    }
    if (segments.length === 3 && segments[2] in userEditLabels) {
      const kind = segments[2];
      const required = ['admin.users.id.get', `admin.users.id.${kind}.put`, kind === 'roles' ? 'admin.roles.get' : 'admin.organizations.tree.get'];
      return {
        key: 'users',
        breadcrumbs: [root, { label: '用户管理', to: '/users' }, { label: `用户详情 #${id}`, to: detailPath }, { label: userEditLabels[kind] }],
        canAccess: context => canAccessModule(context, 'users') && hasPermissions(context, required),
      };
    }
  }

  if (segments[0] === 'organizations') {
    if (segments.length === 1) {
      return {
        key: 'organizations',
        breadcrumbs: [root, { label: '组织管理' }],
        canAccess: context => canAccessModule(context, 'organizations') && hasPermission(context, 'admin.organizations.tree.get'),
      };
    }
    if (segments.length === 2 && segments[1] === 'new') {
      return {
        key: 'organizations',
        breadcrumbs: [root, { label: '组织管理', to: '/organizations' }, { label: '新建组织' }],
        canAccess: context => canAccessModule(context, 'organizations') && hasPermissions(context, ['admin.organizations.tree.get', 'admin.organizations.post']),
      };
    }
    const id = decodeSegment(segments[1]);
    const detailPath = `/organizations/${segments[1]}`;
    if (segments.length === 2) {
      return {
        key: 'organizations',
        breadcrumbs: [root, { label: '组织管理', to: '/organizations' }, { label: `组织详情 #${id}` }],
        canAccess: context => canAccessModule(context, 'organizations') && hasPermission(context, 'admin.organizations.tree.get'),
      };
    }
    if (segments.length === 3 && segments[2] === 'move') {
      return {
        key: 'organizations',
        breadcrumbs: [root, { label: '组织管理', to: '/organizations' }, { label: `组织详情 #${id}`, to: detailPath }, { label: '调整上级' }],
        canAccess: context => canAccessModule(context, 'organizations') && hasPermissions(context, ['admin.organizations.tree.get', 'admin.organizations.id.put']),
      };
    }
  }

  return { key: null, breadcrumbs: [root, { label: '页面不存在' }], canAccess: () => false };
}

export function getWorkbenchGroups(context: UserContext | null): WorkbenchGroup[] {
  return navigationGroups.map(group => ({
    id: group.id,
    label: group.label,
    items: group.items.filter(item => item.canAccess(context)).map(({ to, label, icon }) => ({ to, label, icon })),
  })).filter(group => group.items.length > 0);
}

export function getInitialOpenGroups(groups: WorkbenchGroup[], _pathname?: string): Record<WorkbenchGroupId, boolean> {
  const openGroups: Record<WorkbenchGroupId, boolean> = {
    'authorization-review': false,
    'identity-organization': false,
    'configuration-management': false,
    'audit': false,
  };
  for (const group of groups) {
    openGroups[group.id] = true;
  }
  return openGroups;
}

export function getWorkbenchBreadcrumbs(pathname: string): WorkbenchBreadcrumbItem[] {
  return matchWorkbenchRoute(pathname).breadcrumbs;
}

export function getWorkbenchLabel(pathname: string): string {
  const crumbs = getWorkbenchBreadcrumbs(pathname);
  return crumbs[crumbs.length - 1]?.label ?? '访问说明';
}

export function getWorkbenchModuleMeta(key: WorkbenchModuleKey): WorkbenchModuleMeta {
  const { label, rootPath } = moduleCatalog[key];
  return { key, label, rootPath };
}

export function canAccessWorkbenchModule(context: UserContext | null, key: WorkbenchModuleKey): boolean {
  return moduleCatalog[key].canAccess(context);
}

export function resolveWorkbenchModule(pathname: string, context: UserContext | null): WorkbenchModuleMeta | null {
  const matched = matchWorkbenchRoute(pathname);
  if (!matched.key || !matched.canAccess(context)) return null;
  return getWorkbenchModuleMeta(matched.key);
}

export function useWorkbenchNavigation(context: UserContext | null, _pathname?: string) {
  const groups = useMemo(() => getWorkbenchGroups(context), [context]);
  // Visible groups open by default; manual user toggles persist across routes and permission updates within this mount.
  const [overrides, setOverrides] = useState<Partial<Record<WorkbenchGroupId, boolean>>>({});
  const openGroups = useMemo<Record<WorkbenchGroupId, boolean>>(() => {
    const base = getInitialOpenGroups(groups);
    for (const group of groups) {
      if (overrides[group.id] !== undefined) base[group.id] = overrides[group.id]!;
    }
    return base;
  }, [groups, overrides]);
  const setGroupOpen = useCallback((id: WorkbenchGroupId, open: boolean) => {
    setOverrides(current => current[id] === open ? current : { ...current, [id]: open });
  }, []);
  return { groups, openGroups, setGroupOpen };
}
