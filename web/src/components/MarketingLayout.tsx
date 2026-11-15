import { useEffect, useState, type ReactNode } from 'react';
import { Link, NavLink, useLocation } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { apiMode } from '../api';
import { useSession } from '../auth/session';
import { ButtonLink, Dot, Logo } from './ui';
import s from './MarketingLayout.module.css';

const nav = [
  { to: '/#como-funciona', label: 'Produto' },
  { to: '/dominios', label: 'Domínios' },
  { to: '/precos', label: 'Preços' },
];

export function MarketingLayout({ children }: { children: ReactNode }) {
  const { token } = useSession();
  const [open, setOpen] = useState(false);
  const loc = useLocation();
  useEffect(() => setOpen(false), [loc.pathname, loc.hash]);

  return (
    <div className={s.page}>
      <header className={s.header}>
        <div className={s.bar}>
          <Logo />
          <nav className={s.nav} aria-label="Principal">
            {nav.map((n) => (
              <NavLink key={n.to} to={n.to} className={({ isActive }) => (isActive && !n.to.includes('#') ? s.active : undefined)}>
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className={s.actions}>
            {token ? (
              <ButtonLink to="/painel" size="sm">
                Painel
              </ButtonLink>
            ) : (
              <>
                <ButtonLink to="/entrar" size="sm" variant="ghost">
                  Entrar
                </ButtonLink>
                <ButtonLink to="/criar-conta" size="sm">
                  Começar
                </ButtonLink>
              </>
            )}
          </div>
          <button className={s.burger} aria-expanded={open} aria-controls="menu" onClick={() => setOpen((v) => !v)}>
            <span className="sr-only">{open ? 'Fechar menu' : 'Abrir menu'}</span>
            <span aria-hidden className={open ? s.burgerOpen : undefined} />
          </button>
        </div>
        {open && (
          <div id="menu" className={s.menu}>
            {nav.map((n) => (
              <Link key={n.to} to={n.to}>
                {n.label}
              </Link>
            ))}
            {token ? (
              <ButtonLink to="/painel" block>
                Ir para o painel
              </ButtonLink>
            ) : (
              <>
                <Link to="/entrar">Entrar</Link>
                <ButtonLink to="/criar-conta" block>
                  Começar grátis
                </ButtonLink>
              </>
            )}
          </div>
        )}
      </header>
      <main className={s.main}>{children}</main>
      <Footer />
    </div>
  );
}

function SystemStatus() {
  const health = useQuery({
    queryKey: ['healthz'],
    queryFn: async () => {
      const r = await fetch(`${import.meta.env.VITE_API_URL ?? ''}/healthz`);
      if (!r.ok) throw new Error(String(r.status));
      return true;
    },
    enabled: apiMode === 'real',
    retry: false,
    staleTime: 60_000,
  });
  if (apiMode === 'mock') {
    return (
      <span className={s.status}>
        <Dot tone="amber" /> Modo de demonstração
      </span>
    );
  }
  if (health.isPending) return null;
  return (
    <span className={s.status}>
      {health.isSuccess ? (
        <>
          <Dot tone="green" /> Todos os sistemas a funcionar
        </>
      ) : (
        <>
          <Dot tone="amber" /> Sem ligação ao servidor
        </>
      )}
    </span>
  );
}

export function Footer() {
  return (
    <footer className={s.footer}>
      <div className={s.footBar}>
        <span>© {new Date().getFullYear()} Faz · Maputo, Moçambique</span>
        <nav aria-label="Rodapé" className={s.footNav}>
          <Link to="/precos">Preços</Link>
          <Link to="/dominios">Domínios</Link>
          <Link to="/entrar">Entrar</Link>
        </nav>
        <SystemStatus />
      </div>
    </footer>
  );
}
