import { useEffect, useState, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type SettingsPatch } from '../../api';
import { Button, Field, Notice, Pill, Spinner } from '../../components/ui';
import { usd } from '../../lib/telemetry';
import { ActionDialog, AdminPage, useAdmin, useRefusal } from './shell';
import s from './admin.module.css';

export function AdminControl() {
  return (
    <AdminPage>
      <ControlPage />
    </AdminPage>
  );
}

interface Pending {
  title: string;
  description?: string;
  confirm: string;
  danger?: boolean;
  patch: SettingsPatch;
}

function Card({ title, state, children }: { title: string; state?: ReactNode; children: ReactNode }) {
  return (
    <section className={s.card}>
      <div className={s.cardHead}>
        <h2 className={s.h2}>{title}</h2>
        {state}
      </div>
      {children}
    </section>
  );
}

/** Today's spend against the cap: a meter whose fill turns amber, then red. */
function SpendMeter({ spend, cap }: { spend: number; cap: number }) {
  const p = cap > 0 ? Math.min(100, (100 * spend) / cap) : 0;
  const tone = p >= 100 ? s.meterRed : p >= 80 ? s.meterAmber : s.meterOk;
  return (
    <div className={s.meter} role="meter" aria-valuemin={0} aria-valuemax={cap} aria-valuenow={spend} aria-label="Gasto de hoje face ao limite">
      <div className={s.meterTrack}>
        <span className={`${s.meterFill} ${tone}`} style={{ width: `${p}%` }} />
      </div>
      <span className={s.meterText}>
        {usd(spend)} de {usd(cap)} hoje ({Math.round(p)}%)
      </span>
    </div>
  );
}

function ControlPage() {
  const { auth } = useAdmin();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['admin', 'status'], queryFn: () => api.adminStatus(auth), retry: false, refetchInterval: 30_000 });
  const [pending, setPending] = useState<Pending | null>(null);
  const [pauseReason, setPauseReason] = useState('');
  const [cap, setCap] = useState('');
  const [reserved, setReserved] = useState('');
  const [blocked, setBlocked] = useState('');
  const st = q.data;
  useEffect(() => {
    if (!st) return;
    setCap(st.daily_spend_cap_usd ? String(st.daily_spend_cap_usd) : '');
    setReserved(st.reserved_names.join('\n'));
    setBlocked(st.blocked_words.join('\n'));
  }, [st]);
  if (useRefusal(q.error)) return null;
  const lines = (v: string) => v.split(/[\n,]/).map((x) => x.trim()).filter(Boolean);

  return (
    <>
      <div className={s.head}>
        <div>
          <h1 className={s.h1}>Controlo</h1>
          <p className="muted">Interruptores que mudam a Faz sem publicar código. Cada mudança fica no registo.</p>
        </div>
      </div>
      {!st ? (
        <div className={s.loading}>
          <Spinner label="A carregar" />
        </div>
      ) : (
        <div className={s.cards}>
          {!st.models_open && (
            <Notice>
              Os modelos estão parados{st.cap_reached ? ': o gasto de hoje chegou ao limite diário' : st.pause_reason ? `: ${st.pause_reason}` : ''}. Perguntas,
              construções e o chat respondem “em manutenção”, sem gastar créditos. O que já estava na fila continua.
            </Notice>
          )}

          <Card title="Beta" state={<Pill tone={st.beta_required ? 'amber' : 'green'}>{st.beta_required ? 'Fechado' : 'Aberto'}</Pill>}>
            <p className={s.cardText}>
              {st.beta_required
                ? 'Só cria conta quem tiver um código de convite activo.'
                : 'Qualquer pessoa cria conta. Um código de convite só junta o bónus.'}
            </p>
            <div>
              <Button
                variant={st.beta_required ? 'secondary' : 'primary'}
                size="sm"
                onClick={() =>
                  setPending(
                    st.beta_required
                      ? { title: 'Abrir o beta', description: 'Qualquer pessoa passa a poder criar conta.', confirm: 'Abrir', patch: { beta_required: false } }
                      : { title: 'Fechar o beta', description: 'Só entra quem tiver um código. Quem já tem conta não é afectado.', confirm: 'Fechar', patch: { beta_required: true } },
                  )
                }
              >
                {st.beta_required ? 'Abrir o beta' : 'Fechar o beta'}
              </Button>
            </div>
          </Card>

          <Card title="Modelos" state={<Pill tone={st.models_open ? 'green' : 'red'}>{st.models_open ? 'A funcionar' : 'Parados'}</Pill>}>
            <p className={s.cardText}>Pára todo o trabalho novo dos modelos, por exemplo numa falha do fornecedor ou num gasto fora do normal.</p>
            {st.builds_paused ? (
              <div>
                <Button size="sm" onClick={() => setPending({ title: 'Retomar os modelos', confirm: 'Retomar', patch: { builds_paused: false, pause_reason: '' } })}>
                  Retomar
                </Button>
              </div>
            ) : (
              <div className={s.inline}>
                <Field label="Nota interna" value={pauseReason} onChange={(e) => setPauseReason(e.target.value)} placeholder="Falha no fornecedor dos modelos" maxLength={200} />
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() =>
                    setPending({
                      title: 'Parar os modelos',
                      description: 'Os clientes vêem “em manutenção” e não gastam créditos até retomares.',
                      confirm: 'Parar',
                      danger: true,
                      patch: { builds_paused: true, pause_reason: pauseReason.trim() },
                    })
                  }
                >
                  Parar agora
                </Button>
              </div>
            )}
          </Card>

          <Card
            title="Limite diário de gasto"
            state={st.daily_spend_cap_usd > 0 ? <Pill tone={st.cap_reached ? 'red' : 'green'}>{st.cap_reached ? 'Atingido' : 'Activo'}</Pill> : <Pill>Sem limite</Pill>}
          >
            <p className={s.cardText}>
              Quando o gasto nos modelos de hoje (UTC) chega a este valor, os modelos param sozinhos até à meia noite. Hoje: {usd(st.spend_today_usd)}.
            </p>
            {st.daily_spend_cap_usd > 0 && <SpendMeter spend={st.spend_today_usd} cap={st.daily_spend_cap_usd} />}
            <div className={s.inline}>
              <Field label="Limite em USD (vazio: sem limite)" inputMode="decimal" value={cap} onChange={(e) => setCap(e.target.value.replace(/[^\d.]/g, ''))} />
              <Button
                size="sm"
                variant="secondary"
                onClick={() =>
                  setPending({
                    title: cap ? `Limite de ${usd(Number(cap))} por dia` : 'Tirar o limite diário',
                    confirm: 'Guardar',
                    patch: { daily_spend_cap_usd: Number(cap) || 0 },
                  })
                }
              >
                Guardar limite
              </Button>
            </div>
          </Card>

          <Card title="Endereços">
            <p className={s.cardText}>
              Nomes reservados ficam fora do alcance dos clientes (marcas, nomes oficiais). Palavras bloqueadas recusam qualquer endereço que as contenha. Um por
              linha.
            </p>
            <div className={s.twoCols}>
              <label className={s.listField}>
                <span>Nomes reservados</span>
                <textarea value={reserved} onChange={(e) => setReserved(e.target.value)} rows={6} placeholder={'vodacom\nmovitel\nbci'} />
              </label>
              <label className={s.listField}>
                <span>Palavras bloqueadas</span>
                <textarea value={blocked} onChange={(e) => setBlocked(e.target.value)} rows={6} placeholder={'golpe\nburla'} />
              </label>
            </div>
            <div>
              <Button
                size="sm"
                variant="secondary"
                onClick={() =>
                  setPending({
                    title: 'Guardar as regras dos endereços',
                    description: 'Vale para endereços novos. Os sites já publicados não mudam; retira-os em Sites se for preciso.',
                    confirm: 'Guardar',
                    patch: { reserved_names: lines(reserved), blocked_words: lines(blocked) },
                  })
                }
              >
                Guardar regras
              </Button>
            </div>
          </Card>
        </div>
      )}
      <ActionDialog
        open={!!pending}
        onClose={() => setPending(null)}
        title={pending?.title ?? ''}
        description={pending?.description}
        confirm={pending?.confirm ?? ''}
        danger={pending?.danger}
        onConfirm={async (reason) => {
          if (!pending) return;
          qc.setQueryData(['admin', 'status'], await api.adminUpdateSettings(auth, pending.patch, reason));
          if (pending.patch.builds_paused) setPauseReason('');
        }}
      />
    </>
  );
}
