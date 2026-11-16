import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { api, type AuditEntry } from '../../api';
import { Spinner } from '../../components/ui';
import { since } from '../../lib/format';
import { AdminPage, useAdmin, useRefusal } from './shell';
import s from './admin.module.css';

export const actionLabel: Record<string, string> = {
  'settings.update': 'Mudou o controlo',
  'invite.create': 'Criou convite',
  'invite.disable': 'Desactivou convite',
  'credits.adjust': 'Ajustou créditos',
  'credits.refund_job': 'Devolveu um trabalho',
  'user.suspend': 'Suspendeu a conta',
  'user.unsuspend': 'Reactivou a conta',
  'user.sign_out': 'Terminou as sessões',
  'site.takedown': 'Retirou o site do ar',
  'site.restore': 'Repôs o site',
};

function target(e: AuditEntry) {
  switch (e.target_type) {
    case 'user':
      return <Link to={`/admin/utilizadores/${e.target_id}`}>conta</Link>;
    case 'site':
      return <Link to={`/admin/sites/${e.target_id}`}>site</Link>;
    case 'invite':
      return <span className="mono">{e.target_id}</span>;
    default:
      return <span className={s.dim}>{e.target_type}</span>;
  }
}

const yesNo = (v: unknown) => (v ? 'sim' : 'não');

/** Settings changes in words: "beta fechado: sim, limite diário: $5.00". */
function settingsChanges(c: Record<string, unknown>): string {
  const out: string[] = [];
  if ('beta_required' in c) out.push(`beta fechado: ${yesNo(c.beta_required)}`);
  if ('builds_paused' in c) out.push(`modelos parados: ${yesNo(c.builds_paused)}`);
  if (typeof c.pause_reason === 'string' && c.pause_reason) out.push(`nota: ${c.pause_reason}`);
  if ('daily_spend_cap_usd' in c) out.push(Number(c.daily_spend_cap_usd) > 0 ? `limite diário: $${Number(c.daily_spend_cap_usd).toFixed(2)}` : 'sem limite diário');
  if (Array.isArray(c.reserved_names)) out.push(`${c.reserved_names.length} nomes reservados`);
  if (Array.isArray(c.blocked_words)) out.push(`${c.blocked_words.length} palavras bloqueadas`);
  return out.join(', ');
}

/** What changed, in a line: the amount, the code, the settings. */
function details(e: AuditEntry): string {
  const d = e.details;
  if (e.action === 'credits.adjust') return `${Number(d.delta) > 0 ? '+' : ''}${d.delta} créditos`;
  if (e.action === 'credits.refund_job') return `${d.credits} créditos do trabalho ${d.job_id}`;
  if (e.action === 'settings.update') return settingsChanges((d.changes ?? {}) as Record<string, unknown>);
  if (e.action === 'invite.create') return `${d.max_uses ?? ''} usos, bónus ${d.bonus_credits ?? 0}`;
  if (typeof d.subdomain === 'string') return d.subdomain;
  if (typeof d.sessions_ended === 'number' && d.sessions_ended > 0) return `${d.sessions_ended} sessões terminadas`;
  return '';
}

export function AuditList({ entries }: { entries: AuditEntry[] }) {
  if (entries.length === 0) return <p className={s.dim}>Nenhuma acção registada.</p>;
  return (
    <div className={s.tableWrap}>
      <table className={s.table}>
        <thead>
          <tr>
            <th>Quando</th>
            <th>Quem</th>
            <th>O quê</th>
            <th>Em</th>
            <th>Detalhe</th>
            <th>Motivo</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => (
            <tr key={e.id}>
              <td className={s.dim} title={new Date(e.created_at).toLocaleString('pt-MZ')}>
                {since(e.created_at)}
              </td>
              <td>{e.operator}</td>
              <td>{actionLabel[e.action] ?? e.action}</td>
              <td>{target(e)}</td>
              <td className={s.dim}>{details(e)}</td>
              <td className={s.wrap}>{e.reason}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function AdminAudit() {
  return (
    <AdminPage>
      <AuditPage />
    </AdminPage>
  );
}

function AuditPage() {
  const { auth } = useAdmin();
  const q = useQuery({ queryKey: ['admin', 'audit'], queryFn: () => api.adminAudit(auth), retry: false });
  if (useRefusal(q.error)) return null;
  return (
    <>
      <div className={s.head}>
        <div>
          <h1 className={s.h1}>Registo</h1>
          <p className="muted">Cada acção de administração, com quem a fez e porquê. Não se apaga.</p>
        </div>
      </div>
      {q.isPending ? (
        <div className={s.loading}>
          <Spinner label="A carregar" />
        </div>
      ) : (
        <AuditList entries={q.data ?? []} />
      )}
    </>
  );
}
