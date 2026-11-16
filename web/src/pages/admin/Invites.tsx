import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, isApiError, type Invite } from '../../api';
import { Button, Field, Notice, Pill, Spinner } from '../../components/ui';
import { fmt, since } from '../../lib/format';
import { ActionDialog, AdminPage, useAdmin, useRefusal } from './shell';
import s from './admin.module.css';

export function AdminInvites() {
  return (
    <AdminPage>
      <InvitesPage />
    </AdminPage>
  );
}

/** The link to share: sign up with the code already filled in. */
export function inviteLink(code: string): string {
  return `${location.origin}/criar-conta?convite=${encodeURIComponent(code)}`;
}

function state(i: Invite): { label: string; tone: 'green' | 'amber' | 'red' | 'dim' } {
  if (i.disabled_at) return { label: 'Desactivado', tone: 'red' };
  if (i.expires_at && Date.parse(i.expires_at) <= Date.now()) return { label: 'Expirado', tone: 'dim' };
  if (i.uses >= i.max_uses) return { label: 'Esgotado', tone: 'amber' };
  return { label: 'Activo', tone: 'green' };
}

function InvitesPage() {
  const { auth } = useAdmin();
  const qc = useQueryClient();
  const invites = useQuery({ queryKey: ['admin', 'invites'], queryFn: () => api.adminInvites(auth), retry: false });
  const status = useQuery({ queryKey: ['admin', 'status'], queryFn: () => api.adminStatus(auth), retry: false });
  const [creating, setCreating] = useState(false);
  const [disabling, setDisabling] = useState<Invite | null>(null);
  const [code, setCode] = useState('');
  const [note, setNote] = useState('');
  const [uses, setUses] = useState('20');
  const [bonus, setBonus] = useState('0');
  const [expires, setExpires] = useState('');
  const [copied, setCopied] = useState<string | null>(null);
  const [created, setCreated] = useState<Invite | null>(null);
  if (useRefusal(invites.error ?? status.error)) return null;

  return (
    <>
      <div className={s.head}>
        <div>
          <h1 className={s.h1}>Convites</h1>
          <p className="muted">
            {status.data?.beta_required
              ? 'O beta está fechado: só entra quem tiver um código.'
              : 'O beta está aberto: qualquer pessoa cria conta, e um código só junta o bónus.'}{' '}
            Muda isto em Controlo.
          </p>
        </div>
        <Button onClick={() => (setCode(''), setNote(''), setUses('20'), setBonus('0'), setExpires(''), setCreating(true))}>Criar convite</Button>
      </div>
      {created && (
        <Notice tone="info">
          Convite <span className="mono">{created.code}</span> criado. Partilha o link: <span className="mono">{inviteLink(created.code)}</span>
        </Notice>
      )}
      {invites.isPending ? (
        <div className={s.loading}>
          <Spinner label="A carregar" />
        </div>
      ) : invites.isError ? (
        <Notice>{isApiError(invites.error) ? invites.error.message : 'Não consegui carregar os convites.'}</Notice>
      ) : invites.data.length === 0 ? (
        <p className={s.empty}>Ainda sem convites. Cria um para partilhar no WhatsApp.</p>
      ) : (
        <div className={s.tableWrap}>
          <table className={s.table}>
            <thead>
              <tr>
                <th>Código</th>
                <th>Para quem</th>
                <th className={s.num}>Usos</th>
                <th className={s.num}>Bónus</th>
                <th>Válido até</th>
                <th>Estado</th>
                <th>Criado</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {invites.data.map((i) => {
                const st = state(i);
                return (
                  <tr key={i.code}>
                    <td className="mono">{i.code}</td>
                    <td className={s.wrap}>{i.note}</td>
                    <td className={s.num}>
                      {fmt(i.uses)} de {fmt(i.max_uses)}
                    </td>
                    <td className={s.num}>{i.bonus_credits ? `+${fmt(i.bonus_credits)}` : ''}</td>
                    <td className={s.dim}>{i.expires_at ? new Date(i.expires_at).toLocaleDateString('pt-MZ') : 'sem fim'}</td>
                    <td>
                      <Pill tone={st.tone}>{st.label}</Pill>
                    </td>
                    <td className={s.dim}>
                      {since(i.created_at)} por {i.created_by}
                    </td>
                    <td className={s.rowActions}>
                      {st.label === 'Activo' && (
                        <>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              void navigator.clipboard?.writeText(inviteLink(i.code)).then(() => setCopied(i.code));
                            }}
                          >
                            {copied === i.code ? 'Copiado' : 'Copiar link'}
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setDisabling(i)}>
                            Desactivar
                          </Button>
                        </>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <ActionDialog
        open={creating}
        onClose={() => setCreating(false)}
        title="Criar convite"
        description="Um código pode servir a várias pessoas. O bónus junta-se aos créditos de oferta de quem o usar."
        confirm="Criar convite"
        canConfirm={Number(uses) >= 1}
        onConfirm={async (reason) => {
          const inv = await api.adminCreateInvite(
            auth,
            {
              code: code.trim() || undefined,
              note: note.trim(),
              max_uses: Number(uses),
              bonus_credits: Number(bonus) || 0,
              expires_at: expires ? new Date(`${expires}T23:59:59`).toISOString() : undefined,
            },
            reason,
          );
          setCreated(inv);
          await qc.invalidateQueries({ queryKey: ['admin'] });
        }}
      >
        <Field label="Código (opcional)" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} placeholder="Gerado se ficar vazio, como FAZ-7KQ2-M9XA" maxLength={40} />
        <Field label="Para quem" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Grupo de comerciantes do Xipamanine" maxLength={120} />
        <div className={s.fieldRow}>
          <Field label="Usos" inputMode="numeric" value={uses} onChange={(e) => setUses(e.target.value.replace(/\D/g, ''))} />
          <Field label="Bónus em créditos" inputMode="numeric" value={bonus} onChange={(e) => setBonus(e.target.value.replace(/\D/g, ''))} />
        </div>
        <Field label="Válido até (opcional)" type="date" value={expires} onChange={(e) => setExpires(e.target.value)} />
      </ActionDialog>

      <ActionDialog
        open={!!disabling}
        onClose={() => setDisabling(null)}
        title={`Desactivar ${disabling?.code ?? ''}`}
        description="Ninguém mais o pode usar. Quem já criou conta com ele não perde nada."
        confirm="Desactivar"
        danger
        onConfirm={async (reason) => {
          if (!disabling) return;
          await api.adminDisableInvite(auth, disabling.code, reason);
          await qc.invalidateQueries({ queryKey: ['admin'] });
        }}
      />
    </>
  );
}
