import { createContext, useContext, useMemo, useSyncExternalStore, type ReactNode } from 'react';
import { sessionState } from './session-state';
import { hasPermission } from './navigation';
import type { LoginInput, UserContext } from './models';
interface Session { context: UserContext | null; can(permission: string): boolean; login(input: LoginInput): Promise<void>; logout(): Promise<void>; refresh(): Promise<void>; reloadContext(): Promise<void> }
const SessionContext = createContext<Session | null>(null);
export function SessionProvider({ children }: { children: ReactNode }) {
  const context = useSyncExternalStore(sessionState.subscribe, sessionState.getSnapshot, () => null);
  const value = useMemo<Session>(() => ({ context, can: permission => hasPermission(context, permission), login: sessionState.login, logout: sessionState.logout, refresh: sessionState.refresh, reloadContext: sessionState.reloadContext }), [context]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}
export function useSession(): Session { const session = useContext(SessionContext); if (!session) throw new Error('SessionProvider is required'); return session; }
