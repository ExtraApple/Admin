import { useSyncExternalStore } from 'react';

export const THEME_MODES = ['light', 'dark', 'system'] as const;
export type ThemeMode = (typeof THEME_MODES)[number];
export type ThemeAppearance = Exclude<ThemeMode, 'system'>;

export interface ThemeSnapshot {
  mode: ThemeMode;
  appearance: ThemeAppearance;
}

interface ThemeOwner {
  getSnapshot: () => ThemeSnapshot;
  subscribe: (listener: () => void) => () => void;
  setMode: (mode: ThemeMode) => void;
}

declare global {
  interface Window {
    AdminWorkbenchTheme?: ThemeOwner;
  }
}

export function isThemeMode(value: string): value is ThemeMode {
  return (THEME_MODES as readonly string[]).includes(value);
}

function getThemeOwner(): ThemeOwner {
  if (typeof window === 'undefined' || !window.AdminWorkbenchTheme) {
    throw new Error('AdminWorkbenchTheme must be loaded before the React application.');
  }
  return window.AdminWorkbenchTheme;
}

export function useTheme() {
  const owner = getThemeOwner();
  const snapshot = useSyncExternalStore(owner.subscribe, owner.getSnapshot, owner.getSnapshot);
  return { ...snapshot, setMode: owner.setMode };
}
