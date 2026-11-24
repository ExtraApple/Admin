import type { UserContext } from './models';
import { canAccessWorkbenchModule, resolveWorkbenchModule, type WorkbenchModuleKey } from './workbench-navigation';

export type WorkbenchTabKey = WorkbenchModuleKey;

export interface WorkbenchTab {
  key: WorkbenchTabKey;
  label: string;
  rootPath: string;
  url: string;
  pinned: boolean;
}

export interface PendingTabRemoval {
  removeKeys: WorkbenchTabKey[];
  expectedTargetKey: WorkbenchTabKey;
}

export interface TabClosePlan {
  nextTabs: WorkbenchTab[];
  navigateTo: WorkbenchTab | null;
  pendingRemoval: PendingTabRemoval | null;
}

export interface BatchCloseState {
  canCloseOthers: boolean;
  canCloseRight: boolean;
  otherKeys: WorkbenchTabKey[];
  rightKeys: WorkbenchTabKey[];
}

export function resolveWorkbenchTab(pathname: string, search: string, context: UserContext | null): WorkbenchTab | null {
  const meta = resolveWorkbenchModule(pathname, context);
  if (!meta) return null;
  return {
    key: meta.key,
    label: meta.label,
    rootPath: meta.rootPath,
    url: `${pathname}${search}`,
    pinned: false,
  };
}

export function syncVisitedTab(tabs: WorkbenchTab[], visited: WorkbenchTab | null): WorkbenchTab[] {
  if (!visited) return tabs;
  if (visited.key === 'no-management') {
    if (tabs.some(tab => tab.key !== 'no-management')) return tabs;
    const existing = tabs[0];
    if (existing && existing.key === 'no-management' && existing.url === visited.url && !existing.pinned) return tabs;
    return [{ ...visited, pinned: false }];
  }

  const withoutFallback = tabs.filter(tab => tab.key !== 'no-management');
  const index = withoutFallback.findIndex(tab => tab.key === visited.key);
  if (index >= 0) {
    const current = withoutFallback[index];
    if (current.url === visited.url && current.label === visited.label && withoutFallback.length === tabs.length) {
      return tabs;
    }
    const next = withoutFallback.slice();
    next[index] = { ...visited, pinned: current.pinned };
    return next;
  }
  return [...withoutFallback, { ...visited, pinned: false }];
}

export function pruneUnauthorizedTabs(tabs: WorkbenchTab[], context: UserContext | null): WorkbenchTab[] {
  const filtered = tabs.filter(tab => canAccessWorkbenchModule(context, tab.key));
  return filtered.length === tabs.length ? tabs : filtered;
}

export function canPinTab(tab: WorkbenchTab | null | undefined): boolean {
  return Boolean(tab && tab.key !== 'no-management');
}

export function togglePinTab(tabs: WorkbenchTab[], targetKey: WorkbenchTabKey): WorkbenchTab[] {
  const target = tabs.find(tab => tab.key === targetKey);
  if (!target || !canPinTab(target)) return tabs;
  const pinnedTabs = tabs.filter(tab => tab.pinned && tab.key !== targetKey);
  const unpinnedTabs = tabs.filter(tab => !tab.pinned && tab.key !== targetKey);
  if (target.pinned) {
    return [...pinnedTabs, { ...target, pinned: false }, ...unpinnedTabs];
  }
  return [...pinnedTabs, { ...target, pinned: true }, ...unpinnedTabs];
}

export function canCloseTab(tabs: WorkbenchTab[], targetKey?: WorkbenchTabKey): boolean {
  if (tabs.length <= 1) return false;
  if (!targetKey) return tabs.some(tab => !tab.pinned);
  const target = tabs.find(tab => tab.key === targetKey);
  return Boolean(target && !target.pinned);
}

export function getBatchCloseState(tabs: WorkbenchTab[], anchorKey: WorkbenchTabKey | null): BatchCloseState {
  if (!anchorKey || tabs.length <= 1) {
    return { canCloseOthers: false, canCloseRight: false, otherKeys: [], rightKeys: [] };
  }
  const index = tabs.findIndex(tab => tab.key === anchorKey);
  if (index < 0) {
    return { canCloseOthers: false, canCloseRight: false, otherKeys: [], rightKeys: [] };
  }
  const otherKeys = tabs.filter(tab => tab.key !== anchorKey && !tab.pinned).map(tab => tab.key);
  const rightKeys = tabs.slice(index + 1).filter(tab => !tab.pinned).map(tab => tab.key);
  return {
    canCloseOthers: otherKeys.length > 0,
    canCloseRight: rightKeys.length > 0,
    otherKeys,
    rightKeys,
  };
}

export function planCloseTab(
  tabs: WorkbenchTab[],
  activeKey: WorkbenchTabKey | null,
  targetKey: WorkbenchTabKey,
): TabClosePlan {
  if (!canCloseTab(tabs, targetKey)) {
    return { nextTabs: tabs, navigateTo: null, pendingRemoval: null };
  }
  const index = tabs.findIndex(tab => tab.key === targetKey);
  if (index < 0) {
    return { nextTabs: tabs, navigateTo: null, pendingRemoval: null };
  }
  if (targetKey !== activeKey) {
    return {
      nextTabs: tabs.filter(tab => tab.key !== targetKey),
      navigateTo: null,
      pendingRemoval: null,
    };
  }
  // Closing the active tab navigates to the right neighbor first, otherwise left.
  // Removal is deferred until navigation commits so UnsavedGuard cancellation preserves the tab.
  const neighbor = tabs[index + 1] ?? tabs[index - 1] ?? null;
  if (!neighbor) {
    return { nextTabs: tabs, navigateTo: null, pendingRemoval: null };
  }
  return {
    nextTabs: tabs,
    navigateTo: neighbor,
    pendingRemoval: {
      removeKeys: [targetKey],
      expectedTargetKey: neighbor.key,
    },
  };
}

export function planBatchCloseTabs(
  tabs: WorkbenchTab[],
  activeKey: WorkbenchTabKey | null,
  anchorKey: WorkbenchTabKey,
  mode: 'others' | 'right',
): TabClosePlan {
  const anchor = tabs.find(tab => tab.key === anchorKey);
  if (!anchor) {
    return { nextTabs: tabs, navigateTo: null, pendingRemoval: null };
  }
  const batch = getBatchCloseState(tabs, anchorKey);
  const removeKeys = mode === 'others' ? batch.otherKeys : batch.rightKeys;
  if (removeKeys.length === 0) {
    return { nextTabs: tabs, navigateTo: null, pendingRemoval: null };
  }
  const removeSet = new Set<WorkbenchTabKey>(removeKeys);
  if (!activeKey || !removeSet.has(activeKey)) {
    return {
      nextTabs: tabs.filter(tab => !removeSet.has(tab.key)),
      navigateTo: null,
      pendingRemoval: null,
    };
  }
  return {
    nextTabs: tabs,
    navigateTo: anchor,
    pendingRemoval: {
      removeKeys,
      expectedTargetKey: anchor.key,
    },
  };
}

export function applyNavigationToTabs(
  tabs: WorkbenchTab[],
  visited: WorkbenchTab | null,
  pendingRemoval: PendingTabRemoval | null,
  context: UserContext | null,
): { tabs: WorkbenchTab[]; pendingRemoval: PendingTabRemoval | null } {
  const authorized = pruneUnauthorizedTabs(tabs, context);
  if (pendingRemoval && visited && visited.key === pendingRemoval.expectedTargetKey) {
    const removeSet = new Set<WorkbenchTabKey>(pendingRemoval.removeKeys);
    const remaining = authorized.filter(tab => !removeSet.has(tab.key));
    return {
      tabs: syncVisitedTab(remaining, visited),
      pendingRemoval: null,
    };
  }
  return {
    tabs: syncVisitedTab(authorized, visited),
    pendingRemoval: visited ? null : pendingRemoval,
  };
}
