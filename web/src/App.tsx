import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { SessionProvider } from '@/lib/session';
import { LoginPage, RegisterPage } from '@/pages/AuthPages';
import { ProfilePage } from '@/pages/ProfilePage';
import { managementRoutes } from '@/pages/ManagementPages';
import { WorkbenchShell, WorkbenchHome, NoManagementPage, NotFoundPage, LogoutPage } from '@/components/WorkbenchShell';
const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  { path: '/register', element: <RegisterPage /> },
  { path: '/logout', element: <LogoutPage /> },
  { path: '/', element: <WorkbenchShell />, children: [
    { index: true, element: <WorkbenchHome /> },
    { path: 'profile', element: <ProfilePage /> },
    { path: 'no-management', element: <NoManagementPage /> },
    ...managementRoutes,
    { path: '*', element: <NotFoundPage /> },
  ] },
]);
export default function App() { return <SessionProvider><RouterProvider router={router} /></SessionProvider>; }
