import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useSession } from '../auth/session';
import { usePricing } from '../hooks/queries';
import { apiMode } from '../api';
import { resetMock } from '../api/mock';
import { credits, initials } from '../lib/format';
import { Dot } from './ui';
import s from './AppHeader.module.css';

/** Balance with a green dot while it still covers an edit, amber after. */
export function Balance({ compact }: { compact?: boolean }) {
  const { account } = useSession();
  const { data: p } = usePricing();
  if (!account) return null;
  const enough = !p || account.balance >= p.edit;
  return (
    <span className={s.balance} title="Saldo de créditos">
      <Dot tone={enough ? 'green' : 'amber'} />
      {compact ? account.balance : credits(account.balance)}
    </span>
  );
}

export function UserMenu() {
  const { account, signOut } = useSession();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const nav = useNavigate();

  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', close);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', close);
    };
  }, [open]);

  if (!account) return null;
  return (
    <div className={s.menuWrap} ref={ref}>
      <button className={s.avatar} aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
        <span className="sr-only">Conta de {account.user.name}</span>
        <span aria-hidden>{initials(account.user.name)}</span>
      </button>
      {open && (
        <div className={s.menu} role="menu">
          <div className={s.menuHead}>
            <strong>{account.user.name}</strong>
            <span>{account.user.email}</span>
          </div>
          <Link role="menuitem" to="/painel" onClick={() => setOpen(false)}>
            Os meus sites
          </Link>
          <Link role="menuitem" to="/painel?tab=pagamentos" onClick={() => setOpen(false)}>
            Pagamentos
          </Link>
          {apiMode === 'mock' && (
            <button
              role="menuitem"
              onClick={() => {
                resetMock();
                void signOut().then(() => nav('/'));
              }}
            >
              Apagar dados de demonstração
            </button>
          )}
          <button
            role="menuitem"
            onClick={() => {
              void signOut().then(() => nav('/'));
            }}
          >
            Sair
          </button>
        </div>
      )}
    </div>
  );
}

export function AppHeader({ children }: { children?: ReactNode }) {
  const { account } = useSession();
  return (
    <header className={s.header}>
      <div className={s.bar}>
        <Link to="/painel" className={s.home} aria-label="Painel">
          <span className={s.gradient} aria-hidden />
          <span className={s.name}>{account?.user.name}</span>
        </Link>
        <span className={s.grow} />
        <Balance />
        <Link to="/recarregar?voltar=%2Fpainel" className={s.topup}>
          Recarregar
        </Link>
        <UserMenu />
      </div>
      {children}
    </header>
  );
}
