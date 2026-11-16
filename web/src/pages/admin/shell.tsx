import { createContext, useContext, useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { isApiError, type AdminAuth } from '../../api';
import { Modal } from '../../components/Modal';
import { Button, Field, Logo, Notice, TextArea } from '../../components/ui';
import s from './admin.module.css';

// The admin area: one key (FAZ_ADMIN_TOKEN) and the operator's name, kept
// in sessionStorage for this tab only. The name goes to the audit log with
// every action until named operator accounts exist.

const KEY = 'faz.admin';

interface AdminContextValue {
  auth: AdminAuth;
  signOut: () => void;
  /** Sends the operator back to the key form after the API refused the key. */
  refused: () => void;
}

const Ctx = createContext<AdminContextValue | null>(null);

export function useAdmin(): AdminContextValue {
  const v = useContext(Ctx);
  if (!v) throw new Error('useAdmin outside AdminPage');
  return v;
}

/** Calls refused() once a query or action fails because of the key. */
export function useRefusal(error: unknown) {
  const { refused } = useAdmin();
  const denied = isApiError(error, 'admin_unauthorized');
  useEffect(() => {
    if (denied) refused();
  }, [denied, refused]);
  return denied;
}

function load(): AdminAuth | null {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return null;
    const v = JSON.parse(raw) as AdminAuth;
    return v.key ? v : null;
  } catch {
    return null;
  }
}

function save(v: AdminAuth | null) {
  try {
    if (v) sessionStorage.setItem(KEY, JSON.stringify(v));
    else sessionStorage.removeItem(KEY);
  } catch {
    /* kept in memory for this tab */
  }
}

const nav = [
  { to: '/admin', label: 'Telemetria', end: true },
  { to: '/admin/utilizadores', label: 'Utilizadores' },
  { to: '/admin/sites', label: 'Sites' },
  { to: '/admin/convites', label: 'Convites' },
  { to: '/admin/controlo', label: 'Controlo' },
  { to: '/admin/registo', label: 'Registo' },
];

export function AdminPage({ children }: { children: ReactNode }) {
  const [auth, setAuth] = useState<AdminAuth | null>(load);
  const [error, setError] = useState<string>();
  const set = (v: AdminAuth | null) => {
    save(v);
    setAuth(v);
  };
  if (!auth) return <Gate error={error} onAuth={(v) => (setError(undefined), set(v))} />;
  const value: AdminContextValue = {
    auth,
    signOut: () => set(null),
    refused: () => {
      setError('Chave errada. Confirma o FAZ_ADMIN_TOKEN do fazd.');
      set(null);
    },
  };
  return (
    <Ctx.Provider value={value}>
      <Shell operator={auth.operator} onSignOut={value.signOut}>
        {children}
      </Shell>
    </Ctx.Provider>
  );
}

function Shell({ children, operator, onSignOut }: { children: ReactNode; operator?: string; onSignOut?: () => void }) {
  return (
    <div className={s.page}>
      <header className={s.header}>
        <div className={s.bar}>
          <Logo to="/admin" tld={false} />
          <span className={s.slash} aria-hidden>
            /
          </span>
          <span className={s.title}>Administração</span>
          <span className={s.grow} />
          {operator && <span className={s.operator}>{operator}</span>}
          {onSignOut && (
            <Button variant="ghost" size="sm" onClick={onSignOut}>
              Sair
            </Button>
          )}
        </div>
        {onSignOut && (
          <nav className={s.nav} aria-label="Administração">
            {nav.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => (isActive ? s.navOn : undefined)}>
                {n.label}
              </NavLink>
            ))}
          </nav>
        )}
      </header>
      <main className={s.main}>{children}</main>
    </div>
  );
}

function Gate({ onAuth, error }: { onAuth: (v: AdminAuth) => void; error?: string }) {
  const [key, setKey] = useState('');
  const [operator, setOperator] = useState('');
  const ready = key.trim() && operator.trim().length >= 2;
  return (
    <Shell>
      <form
        className={s.gate}
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          if (ready) onAuth({ key: key.trim(), operator: operator.trim() });
        }}
      >
        <h1 className={s.h1}>Administração</h1>
        <p className="muted">Telemetria, convites, créditos, utilizadores e sites. Só para quem opera a Faz.</p>
        {error && <Notice>{error}</Notice>}
        <Field label="Chave de administração" type="password" value={key} onChange={(e) => setKey(e.target.value)} hint="A variável FAZ_ADMIN_TOKEN do fazd." autoFocus autoComplete="off" />
        <Field label="O teu nome" value={operator} onChange={(e) => setOperator(e.target.value)} hint="Fica no registo de cada acção que fizeres." autoComplete="name" />
        <Button type="submit" disabled={!ready}>
          Entrar
        </Button>
      </form>
    </Shell>
  );
}

/**
 * Confirms an admin action and asks why. The reason goes to the audit log,
 * never to the owner. Extra fields (an amount, a note) come as children.
 */
export function ActionDialog({
  open,
  onClose,
  title,
  description,
  confirm,
  danger,
  children,
  canConfirm = true,
  onConfirm,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: ReactNode;
  confirm: string;
  danger?: boolean;
  children?: ReactNode;
  canConfirm?: boolean;
  onConfirm: (reason: string) => Promise<unknown>;
}) {
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { refused } = useAdmin();
  useEffect(() => {
    if (open) {
      setReason('');
      setError(null);
    }
  }, [open]);
  const ok = reason.trim().length >= 5 && canConfirm;

  async function go() {
    setBusy(true);
    setError(null);
    try {
      await onConfirm(reason.trim());
      onClose();
    } catch (e) {
      if (isApiError(e, 'admin_unauthorized')) refused();
      setError(isApiError(e) ? e.message : 'A acção falhou. Tenta outra vez.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      locked={busy}
      title={title}
      description={description}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            Cancelar
          </Button>
          <Button variant={danger ? 'danger' : 'primary'} onClick={go} loading={busy} disabled={!ok}>
            {confirm}
          </Button>
        </>
      }
    >
      <div className={s.dialog}>
        {error && <Notice>{error}</Notice>}
        {children}
        <TextArea
          label="Motivo"
          hint="Fica no registo, com o teu nome. O cliente não o vê."
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          rows={2}
          maxLength={500}
          data-autofocus
        />
      </div>
    </Modal>
  );
}

/** A random id for a request, so a repeated click is harmless. */
export function requestId(): string {
  return crypto.randomUUID();
}
