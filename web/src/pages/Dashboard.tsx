import { useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { sitesDomain, type LedgerEntry, type Payment, type Site } from '../api';
import { useSession } from '../auth/session';
import { AppHeader } from '../components/AppHeader';
import { PromptCard } from '../components/PromptCard';
import { ButtonLink, Notice, Pill, Soon, Spinner } from '../components/ui';
import { useCredits, usePayments, usePricing, useSites } from '../hooks/queries';
import { draft } from '../lib/draft';
import { credits, firstName, fmt, fmtPhone, mzn, since, swatch } from '../lib/format';
import { siteStatus } from '../lib/status';
import s from './Dashboard.module.css';

type Tab = 'projectos' | 'dominios' | 'pagamentos';

export default function Dashboard() {
  const [params, setParams] = useSearchParams();
  const asked = params.get('tab');
  const tab: Tab = asked === 'pagamentos' || asked === 'dominios' ? asked : 'projectos';
  const tabs: [Tab, string][] = [
    ['projectos', 'Projectos'],
    ['dominios', 'Domínios'],
    ['pagamentos', 'Pagamentos'],
  ];

  return (
    <div className={s.page}>
      <AppHeader>
        <nav className={s.tabs} aria-label="Secções do painel">
          {tabs.map(([id, label]) => (
            <button
              key={id}
              className={tab === id ? s.tabOn : s.tab}
              aria-current={tab === id ? 'page' : undefined}
              onClick={() => setParams(id === 'projectos' ? {} : { tab: id })}
            >
              {label}
            </button>
          ))}
        </nav>
      </AppHeader>
      <main className={s.main}>{tab === 'projectos' ? <Projects /> : tab === 'dominios' ? <Domains /> : <Payments />}</main>
    </div>
  );
}

function Projects() {
  const { account } = useSession();
  const sites = useSites();
  const nav = useNavigate();
  const [prompt, setPrompt] = useState('');
  const [files, setFiles] = useState<File[]>([]);
  const [q, setQ] = useState('');
  const list = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (sites.data ?? []).filter((x) => !needle || x.business_name.toLowerCase().includes(needle) || x.subdomain?.includes(needle));
  }, [sites.data, q]);

  return (
    <>
      <section className={s.create}>
        <h1 className={s.h1}>Tudo pronto para criar, {firstName(account?.user.name ?? '')}?</h1>
        <PromptCard
          value={prompt}
          onChange={setPrompt}
          files={files}
          onFiles={setFiles}
          placeholder="Descreve o próximo site…"
          onSubmit={() => {
            draft.set({ prompt: prompt.trim() });
            draft.setFiles(files);
            nav('/novo');
          }}
        />
      </section>

      <section className={s.section}>
        <div className={s.sectionHead}>
          <h2 className={s.h2}>Os teus sites</h2>
          <div className={s.tools}>
            {(sites.data?.length ?? 0) > 3 && (
              <input className={s.search} type="search" placeholder="Procurar" aria-label="Procurar sites" value={q} onChange={(e) => setQ(e.target.value)} />
            )}
            <ButtonLink to="/novo" size="sm">
              Novo site
            </ButtonLink>
          </div>
        </div>
        {sites.isPending ? (
          <div className={s.loading}>
            <Spinner label="A carregar os sites" />
          </div>
        ) : sites.isError ? (
          <Notice>Não consegui carregar os teus sites. Actualiza a página para tentar outra vez.</Notice>
        ) : sites.data.length === 0 ? (
          <div className={s.empty}>
            <p>Ainda não tens sites. Descreve o primeiro acima: é por nossa conta.</p>
          </div>
        ) : list.length === 0 ? (
          <p className={s.muted}>Nenhum site com “{q}”.</p>
        ) : (
          <ul className={s.grid}>
            {list.map((site) => (
              <li key={site.id}>
                <SiteCard site={site} />
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}

function SiteCard({ site }: { site: Site }) {
  const st = siteStatus(site);
  const color = swatch(site.id);
  return (
    <Link to={`/sites/${site.id}`} className={s.card}>
      <div className={s.thumb} aria-hidden>
        <div className={s.thumbBar} />
        <div className={s.thumbBody}>
          <span className={s.thumbBrand} style={{ color }}>
            {site.business_name}
          </span>
          <span className={s.thumbLine} />
          <span className={`${s.thumbLine} ${s.short}`} />
          <span className={s.thumbBtn} style={{ background: color }} />
        </div>
      </div>
      <div className={s.cardBody}>
        <div className={s.cardTitle}>
          <span className={s.dot} style={{ background: color }} aria-hidden />
          <span className={s.cardName}>{site.business_name}</span>
          <Pill tone={st.tone} pulse={st.pulse}>
            {st.label}
          </Pill>
        </div>
        <span className={s.cardAddr}>{site.subdomain ? `${site.subdomain}.${sitesDomain}` : 'Ainda sem endereço'}</span>
        <span className={s.cardTime}>Editado {since(site.updated_at)}</span>
      </div>
    </Link>
  );
}

/** Custom domains are not built yet: say so, and list the free addresses in use. */
function Domains() {
  const sites = useSites();
  const live = (sites.data ?? []).filter((x) => x.subdomain);
  return (
    <div className={s.payments}>
      <section className={s.balanceCard}>
        <div className={s.soonCopy}>
          <Soon />
          <h2 className={s.h2}>Domínios próprios</h2>
          <p className={s.muted}>
            Em breve vais poder ligar um domínio .co.mz ou .com a cada site e pagá-lo com M-Pesa ou <span className="nowrap">e-Mola</span>. Os endereços grátis abaixo
            continuam a funcionar depois.
          </p>
        </div>
      </section>
      <section className={s.section}>
        <h2 className={s.h2}>Endereços grátis</h2>
        {sites.isPending ? (
          <Spinner label="A carregar" />
        ) : live.length === 0 ? (
          <div className={s.empty}>
            <p>Ainda não publicaste nenhum site. Quando publicares, o endereço aparece aqui.</p>
          </div>
        ) : (
          <table className={s.table}>
            <thead>
              <tr>
                <th>Site</th>
                <th>Endereço</th>
                <th>Domínio próprio</th>
              </tr>
            </thead>
            <tbody>
              {live.map((x) => (
                <tr key={x.id}>
                  <td>
                    <Link to={`/sites/${x.id}`}>{x.business_name}</Link>
                  </td>
                  <td>
                    <a className="mono" href={`https://${x.subdomain}.${sitesDomain}`} target="_blank" rel="noopener noreferrer">
                      {x.subdomain}.{sitesDomain}
                    </a>
                  </td>
                  <td>
                    <Soon />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}

const ledgerLabel: Record<LedgerEntry['kind'], string> = {
  signup_gift: 'Oferta',
  topup: 'Recarga',
  charge: 'Gasto',
  refund: 'Devolução',
  adjustment: 'Acerto',
};

function Payments() {
  const c = useCredits();
  const pays = usePayments();
  const { data: p } = usePricing();

  return (
    <div className={s.payments}>
      <section className={s.balanceCard}>
        <div>
          <span className={s.label}>Saldo</span>
          <div className={s.balanceValue}>{c.data ? credits(c.data.balance) : '…'}</div>
          {c.data && p && (
            <span className={s.muted}>
              Cerca de {fmt(Math.floor(c.data.balance / p.edit))} alterações · 1 crédito = {p.mzn_per_credit} MZN
            </span>
          )}
        </div>
        <ButtonLink to="/recarregar?voltar=%2Fpainel%3Ftab%3Dpagamentos">Recarregar</ButtonLink>
      </section>

      <section className={s.section}>
        <h2 className={s.h2}>Recibos</h2>
        {pays.isPending ? (
          <Spinner label="A carregar" />
        ) : !pays.data?.length ? (
          <div className={s.empty}>
            <p>Ainda sem pagamentos. Quando recarregares com M-Pesa ou e-Mola, os recibos aparecem aqui.</p>
          </div>
        ) : (
          <table className={s.table}>
            <thead>
              <tr>
                <th>Data</th>
                <th>Carteira</th>
                <th>Valor</th>
                <th>Créditos</th>
                <th>Estado</th>
              </tr>
            </thead>
            <tbody>
              {pays.data.map((x) => (
                <PaymentRow key={x.id} p={x} />
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className={s.section}>
        <h2 className={s.h2}>Movimentos de créditos</h2>
        {c.data?.ledger.length ? (
          <ul className={s.ledger}>
            {c.data.ledger.map((l) => (
              <li key={l.id}>
                <span className={s.ledgerKind}>{ledgerLabel[l.kind] ?? l.kind}</span>
                <span className={s.ledgerReason}>{l.reason}</span>
                <span className={s.ledgerTime}>{since(l.created_at)}</span>
                <span className={l.delta > 0 ? s.plus : s.minus}>
                  {l.delta > 0 ? '+' : '−'}
                  {fmt(Math.abs(l.delta))}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          !c.isPending && <p className={s.muted}>Sem movimentos.</p>
        )}
      </section>
    </div>
  );
}

function PaymentRow({ p }: { p: Payment }) {
  const status = { pending: ['A aguardar', 'amber'], succeeded: ['Pago', 'green'], failed: ['Falhou', 'red'] } as const;
  const [label, tone] = status[p.status];
  return (
    <tr>
      <td>{new Date(p.created_at).toLocaleDateString('pt-MZ', { day: 'numeric', month: 'short' })}</td>
      <td>
        {p.method === 'mpesa' ? 'M-Pesa' : 'e-Mola'} <span className={s.muted}>{fmtPhone(p.msisdn.replace(/^258/, ''))}</span>
      </td>
      <td>{mzn(p.amount_mzn)}</td>
      <td>{fmt(p.credits)}</td>
      <td>
        <Pill tone={tone}>{label}</Pill>
      </td>
    </tr>
  );
}
