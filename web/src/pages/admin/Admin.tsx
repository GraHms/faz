import { type ReactNode } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { api, isApiError, type Money, type StageTelemetry, type UsageCall, type UsageGroup, type UsageTotals } from '../../api';
import { Notice, Pill, Spinner } from '../../components/ui';
import { AdminPage, useAdmin, useRefusal } from './shell';
import { fmt, since } from '../../lib/format';
import { siteStatus } from '../../lib/status';
import { compact, effortLabel, lastDays, modelName, mznExact, operationLabel, percent, seconds, usd } from '../../lib/telemetry';
import s from './admin.module.css';

// Operator telemetry: tokens, cost per stage and model, and what was charged,
// per site and per period. Uses the admin key, never an owner's session.

function Tile({ label, value, note }: { label: string; value: string; note?: ReactNode }) {
  return (
    <div className={s.tile}>
      <span className={s.tileLabel}>{label}</span>
      <span className={s.tileValue}>{value}</span>
      {note && <span className={s.tileNote}>{note}</span>}
    </div>
  );
}

function MoneyTiles({ money, totals }: { money: Money; totals: UsageTotals }) {
  const tokens = totals.input_tokens + totals.output_tokens + totals.cache_read_tokens + totals.cache_write_tokens;
  return (
    <section className={s.tiles} aria-label="Resumo">
      <Tile label="Custo dos modelos" value={mznExact(money.cost_mzn)} note={`${usd(money.cost_usd)} · 1 USD = ${money.usd_to_mzn} MZN`} />
      <Tile label="Créditos cobrados, sem IVA" value={mznExact(money.credits_mzn_ex_vat)} note={`${fmt(money.credits_net)} créditos líquidos · ${fmt(money.credits_refunded)} devolvidos`} />
      <Tile
        label="Margem"
        value={mznExact(money.margin_mzn)}
        note={money.credits_mzn_ex_vat > 0 ? `${String(money.margin_percent).replace('.', ',')}% dos créditos sem IVA` : 'Nada cobrado'}
      />
      <Tile label="Chamadas aos modelos" value={fmt(totals.calls)} note={totals.errors ? `${fmt(totals.errors)} com erro` : 'Nenhuma com erro'} />
      <Tile
        label="Tokens"
        value={compact(tokens)}
        note={`entrada ${compact(totals.input_tokens)} · saída ${compact(totals.output_tokens)} · cache ${compact(totals.cache_read_tokens + totals.cache_write_tokens)}`}
      />
    </section>
  );
}

/** A thin meter for one row's share of the cost: one hue, its figure beside it. */
function Share({ part, whole }: { part: number; whole: number }) {
  const p = percent(part, whole);
  const label = `${p < 1 && p > 0 ? '<1' : Math.round(p)}%`;
  return (
    <span className={s.share} title={`${usd(part)} de ${usd(whole)} (${p.toFixed(1)}%)`}>
      <span className={s.track} aria-hidden>
        <span className={s.fill} style={{ width: `${Math.max(p, part > 0 ? 1 : 0)}%` }} />
      </span>
      <span className={s.shareText}>{label}</span>
    </span>
  );
}

const tokensIn = (t: UsageTotals) => t.input_tokens;
const cacheTokens = (t: UsageTotals) => t.cache_read_tokens + t.cache_write_tokens;
const avgLatency = (t: UsageTotals) => (t.calls ? Math.round(t.latency_ms / t.calls) : 0);

function StageTable({ rows, total, showEffort }: { rows: (UsageGroup | StageTelemetry)[]; total: number; showEffort?: boolean }) {
  return (
    <div className={s.tableWrap}>
      <table className={s.table}>
        <thead>
          <tr>
            <th>Operação</th>
            <th>Etapa</th>
            <th>Modelo</th>
            {showEffort && <th>Esforço</th>}
            <th className={s.num}>Chamadas</th>
            <th className={s.num}>Entrada</th>
            <th className={s.num}>Saída</th>
            <th className={s.num}>Cache</th>
            <th className={s.num}>Custo</th>
            <th>Parte do custo</th>
            <th className={s.num}>Tempo médio</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={`${r.operation}|${r.stage}|${r.model}|${'effort' in r ? r.effort : ''}`}>
              <td>{operationLabel[r.operation] ?? r.operation}</td>
              <td className="mono">{r.stage}</td>
              <td>{modelName(r.model)}</td>
              {showEffort && <td>{effortLabel['effort' in r ? r.effort : ''] ?? ('effort' in r ? r.effort : '')}</td>}
              <td className={s.num}>
                {fmt(r.calls)}
                {'retries' in r && r.retries > 0 && <span className={s.flag}> {r.retries} repetidas</span>}
                {r.errors > 0 && <span className={s.bad}> {r.errors} com erro</span>}
              </td>
              <td className={s.num}>{fmt(tokensIn(r))}</td>
              <td className={s.num}>{fmt(r.output_tokens)}</td>
              <td className={s.num}>{fmt(cacheTokens(r))}</td>
              <td className={s.num}>{usd(r.cost_usd)}</td>
              <td>
                <Share part={r.cost_usd} whole={total} />
              </td>
              <td className={s.num}>{seconds(avgLatency(r))}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function GroupTable({ title, rows, total, name }: { title: string; rows: StageTelemetry[]; total: number; name: (r: StageTelemetry) => string }) {
  return (
    <section className={s.section}>
      <h2 className={s.h2}>{title}</h2>
      <div className={s.tableWrap}>
        <table className={s.table}>
          <thead>
            <tr>
              <th>{title.replace('Por ', '').replace(/^./, (c) => c.toUpperCase())}</th>
              <th className={s.num}>Chamadas</th>
              <th className={s.num}>Tokens</th>
              <th className={s.num}>Custo</th>
              <th>Parte do custo</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={name(r)}>
                <td>{name(r)}</td>
                <td className={s.num}>{fmt(r.calls)}</td>
                <td className={s.num}>{fmt(r.input_tokens + r.output_tokens + cacheTokens(r))}</td>
                <td className={s.num}>{usd(r.cost_usd)}</td>
                <td>
                  <Share part={r.cost_usd} whole={total} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

const periods = [7, 30, 90] as const;

export function AdminOverview() {
  const [params, setParams] = useSearchParams();
  const days = periods.find((d) => String(d) === params.get('dias')) ?? 30;
  return (
    <AdminPage>
      <Overview days={days} range={lastDays(days)} setDays={(d) => setParams({ dias: String(d) })} />
    </AdminPage>
  );
}

function Overview({ days, range, setDays }: { days: number; range: { from: string; to: string }; setDays: (d: number) => void }) {
  const { auth } = useAdmin();
  const q = useQuery({
    queryKey: ['admin', 'overview', auth.key, range.from, range.to],
    queryFn: () => api.adminTelemetry(auth, range),
    retry: false,
  });
  if (useRefusal(q.error)) return null;
  const o = q.data;
  return (
    <>
      <div className={s.head}>
        <div>
          <h1 className={s.h1}>Telemetria</h1>
          <p className="muted">
            {range.from} a {range.to}, UTC
          </p>
        </div>
        <div className={s.periods} role="group" aria-label="Período">
          {periods.map((d) => (
            <button key={d} aria-pressed={d === days} onClick={() => setDays(d)}>
              {d} dias
            </button>
          ))}
        </div>
      </div>
      {q.isPending ? (
        <div className={s.loading}>
          <Spinner label="A carregar" />
        </div>
      ) : q.isError ? (
        <Notice>
          {isApiError(q.error) && q.error.status === 404
            ? 'A telemetria está desligada: define FAZ_ADMIN_TOKEN no fazd.'
            : isApiError(q.error)
              ? q.error.message
              : 'Não consegui carregar a telemetria.'}
        </Notice>
      ) : o && o.totals.calls === 0 ? (
        <>
          <MoneyTiles money={o.money} totals={o.totals} />
          <p className={s.empty}>Nenhuma chamada aos modelos neste período.</p>
        </>
      ) : (
        o && (
          <>
            <MoneyTiles money={o.money} totals={o.totals} />
            <section className={s.section}>
              <h2 className={s.h2}>Por etapa e modelo</h2>
              <StageTable rows={o.by_stage} total={o.totals.cost_usd} />
            </section>
            <section className={s.section}>
              <h2 className={s.h2}>Sites</h2>
              <div className={s.tableWrap}>
                <table className={s.table}>
                  <thead>
                    <tr>
                      <th>Site</th>
                      <th>Dono</th>
                      <th>Estado</th>
                      <th className={s.num}>Versões</th>
                      <th className={s.num}>Chamadas</th>
                      <th className={s.num}>Tokens</th>
                      <th className={s.num}>Custo</th>
                      <th className={s.num}>Créditos sem IVA</th>
                      <th className={s.num}>Margem</th>
                    </tr>
                  </thead>
                  <tbody>
                    {o.sites.map((x) => {
                      const st = siteStatus(x);
                      return (
                        <tr key={x.site_id}>
                          <td>
                            <Link to={`/admin/sites/${x.site_id}`}>{x.business_name}</Link>
                          </td>
                          <td className={s.dim}>{x.owner_email}</td>
                          <td>
                            <Pill tone={st.tone}>{st.label}</Pill>
                          </td>
                          <td className={s.num}>{x.current_version}</td>
                          <td className={s.num}>{fmt(x.calls)}</td>
                          <td className={s.num}>{compact(x.input_tokens + x.output_tokens + cacheTokens(x))}</td>
                          <td className={s.num}>{mznExact(x.money.cost_mzn)}</td>
                          <td className={s.num}>{mznExact(x.money.credits_mzn_ex_vat)}</td>
                          <td className={`${s.num} ${x.money.margin_mzn < 0 ? s.bad : ''}`}>{mznExact(x.money.margin_mzn)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </section>
            <Footnote />
          </>
        )
      )}
    </>
  );
}

export function AdminSite() {
  const { id = '' } = useParams();
  return (
    <AdminPage>
      <SiteView id={id} />
    </AdminPage>
  );
}

function SiteView({ id }: { id: string }) {
  const { auth } = useAdmin();
  const q = useQuery({
    queryKey: ['admin', 'site', auth.key, id],
    queryFn: () => api.adminSiteTelemetry(auth, id),
    retry: false,
  });
  if (useRefusal(q.error)) return null;
  const t = q.data;
  return (
    <>
      <p className={s.crumbs}>
        <Link to="/admin/sites">Sites</Link> / telemetria
      </p>
      {q.isPending ? (
        <div className={s.loading}>
          <Spinner label="A carregar" />
        </div>
      ) : q.isError || !t ? (
        <Notice>{isApiError(q.error) && q.error.status === 404 ? 'Este site não existe.' : 'Não consegui carregar a telemetria do site.'}</Notice>
      ) : (
        <>
          <div className={s.head}>
            <div>
              <h1 className={s.h1}>{t.site.business_name}</h1>
              <p className="muted">
                {t.owner_email} · criado {since(t.site.created_at)} · versão {t.site.current_version}
                {t.site.subdomain ? ` · ${t.site.subdomain}` : ''}
              </p>
            </div>
            <Pill tone={siteStatus(t.site).tone}>{siteStatus(t.site).label}</Pill>
          </div>
          <MoneyTiles money={t.money} totals={t.totals} />
          <p className={s.jobs}>
            {fmt(t.jobs.builds)} {t.jobs.builds === 1 ? 'construção' : 'construções'} · {fmt(t.jobs.edits)} {t.jobs.edits === 1 ? 'alteração' : 'alterações'}
            {t.jobs.failed > 0 && <span className={s.bad}> · {t.jobs.failed} falharam</span>}
            {t.jobs.avg_build_seconds > 0 && <> · construção em {seconds(t.jobs.avg_build_seconds * 1000)}, do pedido ao fim</>}
          </p>
          <div className={s.twoCols}>
            <GroupTable title="Por operação" rows={t.by_operation} total={t.totals.cost_usd} name={(r) => operationLabel[r.operation] ?? r.operation} />
            <GroupTable title="Por modelo" rows={t.by_model} total={t.totals.cost_usd} name={(r) => modelName(r.model)} />
          </div>
          <section className={s.section}>
            <h2 className={s.h2}>Por etapa</h2>
            <StageTable rows={t.by_stage} total={t.totals.cost_usd} showEffort />
          </section>
          <section className={s.section}>
            <h2 className={s.h2}>Chamadas</h2>
            <Calls calls={t.calls} />
          </section>
          <Footnote />
        </>
      )}
    </>
  );
}

function Calls({ calls }: { calls: UsageCall[] }) {
  return (
    <div className={s.tableWrap}>
      <table className={`${s.table} ${s.dense}`}>
        <thead>
          <tr>
            <th>Hora</th>
            <th>Operação</th>
            <th>Etapa</th>
            <th>Modelo</th>
            <th>Esforço</th>
            <th className={s.num}>Tentativa</th>
            <th className={s.num}>Entrada</th>
            <th className={s.num}>Saída</th>
            <th className={s.num}>Cache</th>
            <th className={s.num}>Custo</th>
            <th className={s.num}>Tempo</th>
            <th>Erro</th>
          </tr>
        </thead>
        <tbody>
          {calls.map((c) => (
            <tr key={c.id}>
              <td className={s.dim}>{new Date(c.created_at).toLocaleString('pt-MZ', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })}</td>
              <td>{operationLabel[c.operation] ?? c.operation}</td>
              <td className="mono">{c.stage}</td>
              <td>{modelName(c.model)}</td>
              <td>{effortLabel[c.effort] ?? c.effort}</td>
              <td className={s.num}>{c.attempt}</td>
              <td className={s.num}>{fmt(c.input_tokens)}</td>
              <td className={s.num}>{fmt(c.output_tokens)}</td>
              <td className={s.num}>{fmt(c.cache_read_tokens + c.cache_write_tokens)}</td>
              <td className={s.num}>{usd(c.cost_usd)}</td>
              <td className={s.num}>{seconds(c.latency_ms)}</td>
              <td className={c.error ? s.bad : s.dim}>{c.error ?? ''}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Footnote() {
  return (
    <p className={s.footnote}>
      Custos estimados no momento de cada chamada com a tabela de preços em internal/llm/cost.go, que ainda deve ser confirmada. Créditos valorizados a
      preço de tabela sem IVA, incluindo os de oferta: um site feito só com créditos de oferta mostra o valor oferecido, não dinheiro recebido.
    </p>
  );
}
