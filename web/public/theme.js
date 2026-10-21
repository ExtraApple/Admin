(function () {
  const STORAGE_KEY = 'admin-workbench.theme';
  const MODES = ['light', 'dark', 'system'];
  const root = document.documentElement;
  const listeners = new Set();
  let selectedMode = 'system';
  let appearance = 'light';
  let snapshot = Object.freeze({ mode: selectedMode, appearance });

  function isMode(value) {
    return MODES.includes(value);
  }

  function readStoredMode() {
    try {
      const value = window.localStorage.getItem(STORAGE_KEY);
      return isMode(value) ? value : 'system';
    } catch (_error) {
      return 'system';
    }
  }

  function readSystemAppearance() {
    try {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    } catch (_error) {
      return 'light';
    }
  }

  function resolveAppearance(mode) {
    return mode === 'dark' ? 'dark' : mode === 'light' ? 'light' : readSystemAppearance();
  }

  function notify() {
    for (const listener of listeners) listener();
  }

  function applyMode(nextMode, notifyListeners) {
    const nextAppearance = resolveAppearance(nextMode);
    const changed = selectedMode !== nextMode || appearance !== nextAppearance;
    selectedMode = nextMode;
    appearance = nextAppearance;
    root.dataset.themeMode = selectedMode;
    root.dataset.theme = appearance;
    root.style.colorScheme = appearance;
    if (changed) {
      snapshot = Object.freeze({ mode: selectedMode, appearance });
      if (notifyListeners) notify();
    }
  }

  function saveMode(nextMode) {
    try {
      window.localStorage.setItem(STORAGE_KEY, nextMode);
    } catch (_error) {
      // The current page remains usable when browser persistence is unavailable.
    }
  }

  function setMode(nextMode) {
    if (!isMode(nextMode)) return;
    applyMode(nextMode, true);
    saveMode(nextMode);
  }

  function handleStorage(event) {
    if (event.key !== STORAGE_KEY && event.key !== null) return;
    applyMode(readStoredMode(), true);
  }

  function handleMediaChange() {
    if (selectedMode === 'system') applyMode(selectedMode, true);
  }

  function setupMediaListener() {
    try {
      const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
      if (typeof mediaQuery.addEventListener === 'function') {
        mediaQuery.addEventListener('change', handleMediaChange);
      } else if (typeof mediaQuery.addListener === 'function') {
        mediaQuery.addListener(handleMediaChange);
      }
    } catch (_error) {
      return;
    }
  }

  function subscribe(listener) {
    listeners.add(listener);
    return function unsubscribe() {
      listeners.delete(listener);
    };
  }

  function getSnapshot() {
    return snapshot;
  }

  setupMediaListener();
  applyMode(readStoredMode(), false);
  window.addEventListener('storage', handleStorage);
  window.AdminWorkbenchTheme = Object.freeze({
    storageKey: STORAGE_KEY,
    getSnapshot,
    subscribe,
    setMode,
  });
})();
