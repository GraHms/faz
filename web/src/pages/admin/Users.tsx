import { useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, isApiError, sitesDomain, type LedgerEntry } from '../../api';
import { Button, Field, Notice, Pill, Spinner } from '../../components/ui';
import { credits, fmt, fmtPhone, mzn, since } from '../../lib/format';
import { siteStatus } from '../../lib/status';
import { ActionDialog, AdminPage, requestId, useAdmin, useRefusal } from './shell';
import { AuditList } from './Audit';
import s from './admin.module.css';

const phone = (msisdn?: string) => (msisdn ? `+258 ${fmtPhone(msisdn.replace(/^258/, ''))}` : '');

export function AdminUsers() {
  return (
    <AdminPage>
      <UserSearch />
    </AdminPage>
  );
}

function UserSearch() {
  const { auth } = useAdmin();
  const [params, setParams] = useSearchParams();
  const q = params.get('q') ?? '';
  const [text, setText] = useState(q);
  const users = useQuery({ queryKey: ['admin', 'users', q], queryFn: () => api.adminUsers(auth, q), retry: false });
  if (useRefusal(users.error)) return null;
  return (
    <>
      <div className={s.head}>
        <h1 className={s.h1}>Utilizadores</h1>
      </div>
      <form
        className={s.search}
        onSubmit={(e) => {
          e.preventDefault();
          setParams(text.trim() ? { q: text.trim() } : {});
        }}
      >
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Email, nome ou número M-Pesa (84 123 4567)" aria-label="Procurar utilizadores" />
        <Button type="submit" size="sm">
          Procurar
        </Button>
      </form>
      {users.isPending ? (
        <div className={s.loading}>
          <Spinner label="A procurar" />
        </div>
      ) : users.isError ? (
        <Notice>{isApiError(users.error) ? users.error.message : 'A procura falhou.'}</Notice>
      ) : users.data.length === 0 ? (
        <p className={s.empty}>Ninguém encontrado{q ? ` para “${q}”` : ''}.</p>
      ) : (
        <div className={s.tableWrap}>
          <table className={s.table}>
            <thead>
              <tr>
                <th>Nome</th>
                <th>Email</th>
                <th>Telefone</th>
                <th className={s.num}>Saldo</th>
                <th className={s.num}>Sites</th>
                <th>Convite</th>
                <th>Conta</th>
                <th>Desde</th>
              </tr>
            </thead>
            <tbody>
              {users.data.map((u) => (
                <tr key={u.id}>
                  <td>
                    <Link to={`/admin/utilizadores/${u.id}`}>{u.name}</Link>
                  </td>
                  <td className={s.dim}>{u.email}</td>
                  <td className="mono">{phone(u.last_msisdn)}</td>
                  <td className={s.num}>{fmt(u.balance)}</td>
                  <td className={s.num}>{u.sites}</td>
                  <td className="mono">{u.invite_code ?? ''}</td>
                  <td>{u.suspended_at ? <Pill tone="red">Suspensa</Pill> : <Pill tone="green">Activa</Pill>}</td>
                  <td className={s.dim}>{since(u.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

export function AdminUser() {
  const { id = '' } = useParams();
  return (
    <AdminPage>
      <UserDetail id={id} />
    </AdminPage>
  );
}

type Dialog = { kind: 'give' | 'take' } | { kind: 'suspend' | 'unsuspend' | 'signout' } | { kind: 'refund'; entry: LedgerEntry } | null;

function UserDetail({ id }: { id: string }) {
  const { auth } = useAdmin();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['admin', 'user', id], queryFn: () => api.adminUser(auth, id), retry: false });
  const [dialog, setDialog] = useState<Dialog>(null);
  const [amount, setAmount] = useState('');
  const [note, setNote] = useState('');
  const [rid, setRid] = useState(requestId);
  const [done, setDone] = useState<string | null>(null);
  if (useRefusal(q.error)) return null;
  const d = q.data;
  const refresh = () => qc.invalidateQueries({ queryKey: ['admin'] });
  const refunded = new Set((d?.ledger ?? []).filter((l) => l.kind === 'refund' && l.job_id).map((l) => l.job_id));
  const n = Math.floor(Number(amount));

  return (
    <>
      <p className={s.crumbs}>
        <Link to="/admin/utilizadores">Utilizadores</Link> / conta
      </p>
      {q.isPending ? (
        <div className={s.loading}>
          <Spinner label="A carregar" />
        </div>
      ) : !d ? (
        <Notice>{isApiError(q.error) && q.error.status === 404 ? 'Esta conta não existe.' : 'Não consegui carregar a conta.'}</Notice>
      ) : (
        <>
          <div className={s.head}>
            <div>
              <h1 className={s.h1}>{d.user.name}</h1>
              <p className="muted">
                {d.user.email} · conta criada {since(d.user.created_at)}
                {d.user.invite_code ? ` · convite ${d.user.invite_code}` : ''} · {d.sessions} {d.sessions === 1 ? 'sessão activa' : 'sessões activas'}
              </p>
            </div>
            {d.user.suspended_at ? <Pill tone="red">Suspensa</Pill> : <Pill tone="green">Activa</Pill>}
          </div>
          {d.user.suspended_at && <Notice>Suspensa {since(d.user.suspended_at)}: {d.user.suspended_reason}</Notice>}
          {done && <Notice tone="info">{done}</Notice>}

          <section className={s.tiles} aria-label="Saldo">
            <div className={s.tile}>
              <span className={s.tileLabel}>Saldo</span>
              <span className={s.tileValue}>{credits(d.balance)}</span>
              <span className={s.tileNote}>{d.payments.filter((p) => p.status === 'succeeded').length} pagamentos confirmados</span>
            </div>
            <div className={`${s.tile} ${s.actions}`}>
              <span className={s.tileLabel}>Acções</span>
              <div className={s.buttons}>
                <Button size="sm" onClick={() => (setAmount(''), setNote(''), setRid(requestId()), setDialog({ kind: 'give' }))}>
                  Dar créditos
                </Button>
                <Button size="sm" variant="secondary" onClick={() => (setAmount(''), setNote(''), setRid(requestId()), setDialog({ kind: 'take' }))}>
                  Tirar créditos
                </Button>
                <Button size="sm" variant="secondary" onClick={() => setDialog({ kind: 'signout' })} disabled={d.sessions === 0}>
                  Terminar sessões
                </Button>
                {d.user.suspended_at ? (
                  <Button size="sm" variant="secondary" onClick={() => setDialog({ kind: 'unsuspend' })}>
                    Reactivar conta
                  </Button>
                ) : (
                  <Button size="sm" variant="danger" onClick={() => setDialog({ kind: 'suspend' })}>
                    Suspender conta
                  </Button>
                )}
              </div>
            </div>
          </section>

          <section className={s.section}>
            <h2 className={s.h2}>Sites</h2>
            {d.sites.length === 0 ? (
              <p className={s.dim}>Ainda sem sites.</p>
            ) : (
              <div className={s.tableWrap}>
                <table className={s.table}>
                  <thead>
                    <tr>
                      <th>Site</th>
                      <th>Endereço</th>
                      <th>Estado</th>
                      <th className={s.num}>Versão</th>
                      <th>Editado</th>
                    </tr>
                  </thead>
                  <tbody>
                    {d.sites.map((x) => {
                      const st = siteStatus(x);
                      return (
                        <tr key={x.id}>
                          <td>
                            <Link to={`/admin/sites/${x.id}`}>{x.business_name}</Link>
                          </td>
                          <td className="mono">{x.subdomain ? `${x.subdomain}.${sitesDomain}` : ''}</td>
                          <td>
                            <Pill tone={st.tone}>{st.label}</Pill>
                          </td>
                          <td className={s.num}>{x.current_version}</td>
                          <td className={s.dim}>{since(x.updated_at)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <section className={s.section}>
            <h2 className={s.h2}>Movimentos de créditos</h2>
            <div className={s.tableWrap}>
              <table className={s.table}>
                <thead>
                  <tr>
                    <th>Quando</th>
                    <th>Tipo</th>
                    <th>O que o cliente lê</th>
                    <th className={s.num}>Créditos</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {d.ledger.map((l) => (
                    <tr key={l.id}>
                      <td className={s.dim}>{since(l.created_at)}</td>
                      <td>{ledgerKind[l.kind] ?? l.kind}</td>
                      <td>{l.reason}</td>
                      <td className={`${s.num} ${l.delta > 0 ? s.good : ''}`}>
                        {l.delta > 0 ? '+' : '−'}
                        {fmt(Math.abs(l.delta))}
                      </td>
                      <td>
                        {l.kind === 'charge' && l.job_id && !refunded.has(l.job_id) && (
                          <Button size="sm" variant="ghost" onClick={() => setDialog({ kind: 'refund', entry: l })}>
                            Devolver
                          </Button>
                        )}
                        {l.kind === 'charge' && l.job_id && refunded.has(l.job_id) && <span className={s.dim}>devolvido</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          <section className={s.section}>
            <h2 className={s.h2}>Pagamentos</h2>
            {d.payments.length === 0 ? (
              <p className={s.dim}>Sem pagamentos.</p>
            ) : (
              <div className={s.tableWrap}>
                <table className={s.table}>
                  <thead>
                    <tr>
                      <th>Quando</th>
                      <th>Carteira</th>
                      <th>Número</th>
                      <th className={s.num}>Valor</th>
                      <th>Estado</th>
                      <th>Referência</th>
                    </tr>
                  </thead>
                  <tbody>
                    {d.payments.map((p) => (
                      <tr key={p.id}>
                        <td className={s.dim}>{since(p.created_at)}</td>
                        <td>{p.method === 'mpesa' ? 'M-Pesa' : 'e-Mola'}</td>
                        <td className="mono">{phone(p.msisdn)}</td>
                        <td className={s.num}>{mzn(p.amount_mzn)}</td>
                        <td>
                          <Pill tone={p.status === 'succeeded' ? 'green' : p.status === 'failed' ? 'red' : 'amber'}>
                            {p.status === 'succeeded' ? 'Pago' : p.status === 'failed' ? `Falhou${p.failure_reason ? `: ${p.failure_reason}` : ''}` : 'A aguardar'}
                          </Pill>
                        </td>
                        <td className="mono">{p.provider_ref ?? ''}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <section className={s.section}>
            <h2 className={s.h2}>Registo desta conta</h2>
            <AuditList entries={d.audit} />
          </section>

          <ActionDialog
            open={dialog?.kind === 'give' || dialog?.kind === 'take'}
            onClose={() => setDialog(null)}
            title={dialog?.kind === 'take' ? 'Tirar créditos' : 'Dar créditos'}
            description={`Saldo actual: ${credits(d.balance)}.`}
            confirm={dialog?.kind === 'take' ? `Tirar ${n > 0 ? fmt(n) : ''}` : `Dar ${n > 0 ? fmt(n) : ''}`}
            canConfirm={n > 0}
            onConfirm={async (reason) => {
              const delta = dialog?.kind === 'take' ? -n : n;
              const r = await api.adminAdjustCredits(auth, id, { delta, public_note: note.trim() || undefined, request_id: rid }, reason);
              setDone(`Feito. Saldo agora: ${credits(r.balance)}.`);
              await refresh();
            }}
          >
            <Field label="Créditos" inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value.replace(/\D/g, ''))} />
            <Field
              label="O que o cliente lê nos movimentos (opcional)"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              maxLength={120}
              placeholder={dialog?.kind === 'take' ? 'Acerto feito pela equipa Faz' : 'Créditos oferecidos pela equipa Faz'}
            />
          </ActionDialog>

          <ActionDialog
            open={dialog?.kind === 'refund'}
            onClose={() => setDialog(null)}
            title="Devolver créditos deste trabalho"
            description={dialog?.kind === 'refund' ? `${dialog.entry.reason}: ${credits(-dialog.entry.delta)}. Só se devolve uma vez.` : undefined}
            confirm="Devolver"
            onConfirm={async (reason) => {
              if (dialog?.kind !== 'refund' || !dialog.entry.job_id) return;
              const r = await api.adminRefundJob(auth, dialog.entry.job_id, reason);
              setDone(`Devolvidos ${credits(r.credits)}.`);
              await refresh();
            }}
          />

          <ActionDialog
            open={dialog?.kind === 'suspend'}
            onClose={() => setDialog(null)}
            title="Suspender esta conta"
            description="A pessoa sai de todas as sessões e não consegue entrar. Os sites publicados continuam no ar; retira-os em Sites se for preciso."
            confirm="Suspender"
            danger
            onConfirm={async (reason) => {
              await api.adminSuspend(auth, id, true, reason);
              setDone('Conta suspensa.');
              await refresh();
            }}
          />
          <ActionDialog
            open={dialog?.kind === 'unsuspend'}
            onClose={() => setDialog(null)}
            title="Reactivar esta conta"
            confirm="Reactivar"
            onConfirm={async (reason) => {
              await api.adminSuspend(auth, id, false, reason);
              setDone('Conta reactivada.');
              await refresh();
            }}
          />
          <ActionDialog
            open={dialog?.kind === 'signout'}
            onClose={() => setDialog(null)}
            title="Terminar todas as sessões"
            description="Útil quando alguém perde o telemóvel. A pessoa volta a entrar com a palavra-passe."
            confirm="Terminar sessões"
            onConfirm={async (reason) => {
              const r = await api.adminSignOut(auth, id, reason);
              setDone(`${r.sessions_ended} ${r.sessions_ended === 1 ? 'sessão terminada' : 'sessões terminadas'}.`);
              await refresh();
            }}
          />
        </>
      )}
    </>
  );
}

const ledgerKind: Record<string, string> = {
  signup_gift: 'Oferta',
  topup: 'Recarga',
  charge: 'Gasto',
  refund: 'Devolução',
  adjustment: 'Ajuste',
};
