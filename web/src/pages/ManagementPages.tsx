import type { RouteObject } from 'react-router-dom';
import { Guard } from '@/components/management/shared';
import { GuardedRoleEditor, RoleDetail, RolesList } from './RolePages';
import { GuardedUserEditor, UserDetail, UsersList } from './UserPages';
import { GuardedOrganizationMove, OrganizationDetail, OrganizationsList } from './OrganizationPages';
import '@/management.css';

export const managementRoutes: RouteObject[] = [
  { path: 'roles', element: <Guard module="roles" permissions={['admin.roles.get']}><RolesList /></Guard> },
  { path: 'roles/:id', element: <Guard module="roles" permissions={['admin.roles.id.get']}><RoleDetail /></Guard> },
  { path: 'roles/:id/permissions', element: <GuardedRoleEditor kind="permissions" /> },
  { path: 'roles/:id/menus', element: <GuardedRoleEditor kind="menus" /> },
  { path: 'roles/:id/data-scope', element: <GuardedRoleEditor kind="data-scope" /> },
  { path: 'users', element: <Guard module="users" permissions={['admin.users.get']}><UsersList /></Guard> },
  { path: 'users/:id', element: <Guard module="users" permissions={['admin.users.id.get']}><UserDetail /></Guard> },
  { path: 'users/:id/roles', element: <GuardedUserEditor kind="roles" /> },
  { path: 'users/:id/organizations', element: <GuardedUserEditor kind="organizations" /> },
  { path: 'organizations', element: <Guard module="organizations" permissions={['admin.organizations.get', 'admin.organizations.tree.get']}><OrganizationsList /></Guard> },
  { path: 'organizations/:id', element: <Guard module="organizations" permissions={['admin.organizations.tree.get']}><OrganizationDetail /></Guard> },
  { path: 'organizations/:id/move', element: <GuardedOrganizationMove /> },
];
