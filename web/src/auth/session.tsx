import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Navigate, useLocation } from 'react-router-dom';
import { api, tokenStore, type Account, type Session } from '../api';
import { Spinner } from '../components/ui';

interface SessionValue {
  token: string | null;
  account: Account | undefined;
  loading: boolean;
  signIn: (s: Session) => void;
  signOut: () => Promise<void>;
}

const Ctx = createContext<SessionValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState(tokenStore.get);
  const qc = useQueryClient();
  useEffect(
    () =>
      tokenStore.subscribe((t) => {
        setToken(t);
        if (!t) qc.clear();
      }),
    [qc],
  );

  const me = useQuery({ queryKey: ['me'], queryFn: api.me, enabled: !!token, staleTime: 15_000 });

  const signIn = useCallback(
    (s: Session) => {
      qc.clear();
      qc.setQueryData(['me'], s.account);
      tokenStore.set(s.token);
    },
    [qc],
  );

  const signOut = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      /* the token is dropped either way */
    }
    tokenStore.set(null);
  }, []);

  const value = useMemo<SessionValue>(
    () => ({ token, account: me.data, loading: !!token && me.isPending, signIn, signOut }),
    [token, me.data, me.isPending, signIn, signOut],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession(): SessionValue {
  const v = useContext(Ctx);
  if (!v) throw new Error('useSession outside SessionProvider');
  return v;
}

/** Sends visitors without a session to the login page, then back here. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const { token, account, loading } = useSession();
  const loc = useLocation();
  if (!token) {
    const back = encodeURIComponent(loc.pathname + loc.search);
    return <Navigate to={`/entrar?voltar=${back}`} replace />;
  }
  if (loading || !account) {
    return (
      <div style={{ minHeight: '60vh', display: 'grid', placeItems: 'center' }}>
        <Spinner label="A carregar" />
      </div>
    );
  }
  return <>{children}</>;
}

/** Only follow redirects inside the app, never to another origin. */
export function safeBack(v: string | null, fallback = '/painel'): string {
  return v && v.startsWith('/') && !v.startsWith('//') ? v : fallback;
}
