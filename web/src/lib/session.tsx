import { createContext, useContext, useEffect, useMemo, useSyncExternalStore, type ReactNode } from 'react';
import { sessionState, type SessionSnapshot } from './session-state';
import { hasPermission } from './navigation';
import type { LoginInput, UserContext } from './models';
interface Session extends SessionSnapshot { can(permission: string): boolean; login(input: LoginInput): Promise<void>; logout(reason?: 'password-changed'): Promise<void>; retryLogout(): Promise<void>; refresh(): Promise<void>; reloadContext(): Promise<UserContext>; beginLogin(): void }
const SessionContext = createContext<Session | null>(null);
export function SessionProvider({ children }: { children: ReactNode }) {
  const snapshot = useSyncExternalStore(sessionState.subscribe, sessionState.getSnapshot, sessionState.getServerSnapshot);
  useEffect(() => { void sessionState.initialize(); }, []);
  // Retain permission values while suspended so mounted editors survive; the shell and Client block use.
  const value = useMemo<Session>(() => ({ ...snapshot, can: permission => hasPermission(snapshot.context, permission), login: sessionState.login, logout: sessionState.logout, retryLogout: sessionState.retryLogout, refresh: sessionState.refresh, reloadContext: sessionState.reloadContext, beginLogin: sessionState.beginLogin }), [snapshot]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}
export function useSession(): Session { const session = useContext(SessionContext); if (!session) throw new Error('SessionProvider is required'); return session; }
