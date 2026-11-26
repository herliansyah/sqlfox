/**
 * Safe typed wrapper over window.localStorage that degrades gracefully
 * in SSR/test environments, private browsing, or quota-exhausted states.
 */
export const storage = {
  get(key: string): string | null {
    try {
      return typeof localStorage !== 'undefined' ? localStorage.getItem(key) : null;
    } catch {
      return null;
    }
  },

  set(key: string, value: string): void {
    try {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem(key, value);
      }
    } catch {
      // Ignore storage quota or restricted context failures
    }
  },

  remove(key: string): void {
    try {
      if (typeof localStorage !== 'undefined') {
        localStorage.removeItem(key);
      }
    } catch {
      // Ignore restricted context failures
    }
  },

  getJSON<T>(key: string, fallback: T): T {
    const raw = this.get(key);
    if (!raw) return fallback;
    try {
      return JSON.parse(raw) as T;
    } catch {
      return fallback;
    }
  },

  setJSON(key: string, value: unknown): void {
    try {
      this.set(key, JSON.stringify(value));
    } catch {
      // Ignore serialization or quota failures
    }
  },
};
