import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it } from 'vitest';

const themeScript = readFileSync(new URL('../../public/theme.js', import.meta.url), 'utf8');
const themeKey = 'admin-workbench.theme';
type ThemeMode = 'light' | 'dark' | 'system';
type Appearance = 'light' | 'dark';
type ThemeSnapshot = { mode: ThemeMode; appearance: Appearance };
type ThemeOwner = {
  getSnapshot: () => ThemeSnapshot;
  setMode: (mode: ThemeMode) => void;
  subscribe: (listener: () => void) => () => void;
};

type StorageOptions = { readError?: boolean; writeError?: boolean };

class TestStorage {
  private readonly values = new Map<string, string>();
  readonly writes: string[] = [];
  constructor(private readonly options: StorageOptions = {}) {}
  getItem(key: string) {
    if (this.options.readError) throw new Error('storage read blocked');
    return this.values.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    if (this.options.writeError) throw new Error('storage write blocked');
    this.writes.push(`${key}=${value}`);
    this.values.set(key, value);
  }
  removeItem(key: string) { this.values.delete(key); }
  clear() { this.values.clear(); }
  seed(value: string) { this.values.set(themeKey, value); }
}

class TestMediaQuery {
  readonly media = '(prefers-color-scheme: dark)';
  private readonly listeners = new Set<(event: { matches: boolean }) => void>();
  constructor(public matches: boolean) {}
  addEventListener(_type: string, listener: (event: { matches: boolean }) => void) { this.listeners.add(listener); }
  removeEventListener(_type: string, listener: (event: { matches: boolean }) => void) { this.listeners.delete(listener); }
  dispatch(matches: boolean) {
    this.matches = matches;
    for (const listener of this.listeners) listener({ matches });
  }
}

class TestWindow {
  readonly document = { documentElement: { dataset: {} as Record<string, string>, style: { colorScheme: '' } } };
  private readonly listeners = new Map<string, Set<(event: unknown) => void>>();
  constructor(readonly localStorage: TestStorage, readonly media: TestMediaQuery) {}
  matchMedia() { return this.media; }
  addEventListener(type: string, listener: (event: unknown) => void) {
    const listeners = this.listeners.get(type) ?? new Set<(event: unknown) => void>();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }
  dispatchStorage(key: string | null) {
    for (const listener of this.listeners.get('storage') ?? []) listener({ key });
  }
}

function createOwner(saved: string | null, matchesDark: boolean, options: StorageOptions = {}, storage = new TestStorage(options)) {
  if (saved !== null) storage.seed(saved);
  const media = new TestMediaQuery(matchesDark);
  const window = new TestWindow(storage, media);
  const context = { window, document: window.document };
  runInNewContext(themeScript, context);
  return { owner: (window as TestWindow & { AdminWorkbenchTheme: ThemeOwner }).AdminWorkbenchTheme, storage, media, window };
}

describe('theme owner consumer boundary', () => {
  it('uses a saved mode for the first visible state and updates the current page immediately', () => {
    const { owner, window } = createOwner('dark', false);
    expect(owner.getSnapshot()).toEqual({ mode: 'dark', appearance: 'dark' });
    expect(window.document.documentElement.dataset).toMatchObject({ themeMode: 'dark', theme: 'dark' });

    owner.setMode('light');
    expect(owner.getSnapshot()).toEqual({ mode: 'light', appearance: 'light' });
    expect(window.document.documentElement.dataset).toMatchObject({ themeMode: 'light', theme: 'light' });
  });

  it('keeps fixed modes above system changes and resumes system behavior when selected', () => {
    const { owner, media } = createOwner('system', false);
    const seen: ThemeSnapshot[] = [];
    const unsubscribe = owner.subscribe(() => seen.push(owner.getSnapshot()));

    media.dispatch(true);
    expect(owner.getSnapshot()).toEqual({ mode: 'system', appearance: 'dark' });
    owner.setMode('light');
    media.dispatch(true);
    expect(owner.getSnapshot()).toEqual({ mode: 'light', appearance: 'light' });
    owner.setMode('system');
    expect(owner.getSnapshot()).toEqual({ mode: 'system', appearance: 'dark' });
    expect(seen).toEqual([
      { mode: 'system', appearance: 'dark' },
      { mode: 'light', appearance: 'light' },
      { mode: 'system', appearance: 'dark' },
    ]);
    unsubscribe();
  });

  it('applies another tab mode without writing it back, including same-color mode changes', () => {
    const storage = new TestStorage();
    const first = createOwner(null, true, {}, storage);
    const second = createOwner(null, true, {}, storage);
    const firstOwner = first.owner;
    const secondOwner = second.owner;

    firstOwner.setMode('dark');
    const writesBeforeEvent = second.storage.writes.length;
    second.window.dispatchStorage(themeKey);
    expect(secondOwner.getSnapshot()).toEqual({ mode: 'dark', appearance: 'dark' });
    expect(second.storage.writes.length).toBe(writesBeforeEvent);
    expect(storage.writes).toEqual([`${themeKey}=dark`]);
  });

  it('treats deletion, invalid values, and unavailable browser storage as system-safe behavior', () => {
    const { owner, storage, window } = createOwner('invalid', true);
    expect(owner.getSnapshot()).toEqual({ mode: 'system', appearance: 'dark' });

    storage.seed('dark');
    window.dispatchStorage(themeKey);
    expect(owner.getSnapshot()).toEqual({ mode: 'dark', appearance: 'dark' });
    storage.removeItem(themeKey);
    window.dispatchStorage(null);
    expect(owner.getSnapshot()).toEqual({ mode: 'system', appearance: 'dark' });

    const unavailable = createOwner(null, false, { readError: true, writeError: true });
    unavailable.owner.setMode('dark');
    expect(unavailable.owner.getSnapshot()).toEqual({ mode: 'dark', appearance: 'dark' });
    expect(unavailable.storage.writes).toHaveLength(0);
  });
});
