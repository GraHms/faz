import { createHttpApi, type Api } from './client';
import { createMockApi } from './mock';

export * from './client';
export type * from './types';

/** "real" talks to fazd; "mock" runs an imitation of it in the browser. */
export const apiMode: 'real' | 'mock' = import.meta.env.VITE_API_MODE === 'mock' ? 'mock' : 'real';
export const sitesDomain: string = import.meta.env.VITE_SITES_DOMAIN || 'faz.co.mz';

// The token lives in localStorage rather than a cookie: the API takes bearer
// tokens only, so a cross-site request cannot ride on the owner's session.
// Published sites are served from another origin and cannot read it.
const TOKEN_KEY = 'faz.token';
type Listener = (token: string | null) => void;
const listeners = new Set<Listener>();

export const tokenStore = {
  get(): string | null {
    try {
      return localStorage.getItem(TOKEN_KEY);
    } catch {
      return null;
    }
  },
  set(token: string | null) {
    try {
      if (token) localStorage.setItem(TOKEN_KEY, token);
      else localStorage.removeItem(TOKEN_KEY);
    } catch {
      /* private mode: the session lasts until the tab closes */
    }
    for (const l of listeners) l(token);
  },
  subscribe(l: Listener): () => void {
    listeners.add(l);
    return () => listeners.delete(l);
  },
};

export const api: Api =
  apiMode === 'mock'
    ? createMockApi({ getToken: tokenStore.get })
    : createHttpApi({
        baseUrl: import.meta.env.VITE_API_URL ?? '',
        getToken: tokenStore.get,
        onUnauthorized: () => tokenStore.set(null),
      });
