import type { RouteObject } from 'react-router-dom';
import { AuthorizationGuard } from '@/components/management/shared';
import { Guard } from '@/components/management/shared';
import { AuthorizationOverviewPage, AuthorizationRisksPage } from './AuthorizationOverviewPages';
import { GuardedRoleEditor, RoleCreate, RoleDetail, RolesList } from './RolePages';
import { DeletedUsersList, GuardedUserEditor, UserCreate, UserDetail, UsersList } from './UserPages';
import { GuardedOrganizationMove, OrganizationCreate, OrganizationDetail, OrganizationsList } from './OrganizationPages';
import { MenusPage, ApisPage } from './ConfigurationPages';
import { AuditGuard, AuditLogsPage } from './AuditLogPages';
import '@/management.css';

export const managementRoutes: RouteObject[] = [
  { path: 'authorization-overview', element: <AuthorizationGuard permission="admin.authorization.overview.get"><AuthorizationOverviewPage /></AuthorizationGuard> },
  { path: 'authorization-risks', element: <AuthorizationGuard permission="admin.authorization.risks.get"><AuthorizationRisksPage /></AuthorizationGuard> },
  { path: 'roles', element: <Guard module="roles" permissions={['admin.roles.get']}><RolesList /></Guard> },
  { path: 'roles/new', element: <Guard module="roles" permissions={['admin.roles.post']}><RoleCreate /></Guard> },
  { path: 'roles/:id', element: <Guard module="roles" permissions={['admin.roles.id.get']}><RoleDetail /></Guard> },
  { path: 'roles/:id/permissions', element: <GuardedRoleEditor kind="permissions" /> },
  { path: 'roles/:id/menus', element: <GuardedRoleEditor kind="menus" /> },
  { path: 'roles/:id/data-scope', element: <GuardedRoleEditor kind="data-scope" /> },
  { path: 'users', element: <Guard module="users" permissions={['admin.users.get']}><UsersList /></Guard> },
  { path: 'users/new', element: <Guard module="users" permissions={['admin.users.post']}><UserCreate /></Guard> },
  { path: 'users/deleted', element: <Guard module="users" permissions={['admin.deleted-users.get']}><DeletedUsersList /></Guard> },
  { path: 'users/:id', element: <Guard module="users" permissions={['admin.users.id.get']}><UserDetail /></Guard> },
  { path: 'users/:id/roles', element: <GuardedUserEditor kind="roles" /> },
  { path: 'users/:id/organizations', element: <GuardedUserEditor kind="organizations" /> },
  { path: 'organizations', element: <Guard module="organizations" permissions={['admin.organizations.get', 'admin.organizations.tree.get']}><OrganizationsList /></Guard> },
  { path: 'organizations/new', element: <Guard module="organizations" permissions={['admin.organizations.tree.get', 'admin.organizations.post']}><OrganizationCreate /></Guard> },
  { path: 'organizations/:id', element: <Guard module="organizations" permissions={['admin.organizations.tree.get']}><OrganizationDetail /></Guard> },
  { path: 'organizations/:id/move', element: <GuardedOrganizationMove /> },
  { path: 'menus', element: <Guard module="menus" permissions={['admin.menus.get']}><MenusPage /></Guard> },
  { path: 'apis', element: <Guard module="apis" permissions={['admin.apis.get']}><ApisPage /></Guard> },
  { path: 'audit-logs', element: <AuditGuard><AuditLogsPage /></AuditGuard> },
];
