import type { ReactNode } from 'react';
import { MarketingLayout } from '../components/MarketingLayout';
import { ButtonLink, Dot, Spinner } from '../components/ui';
import { usePricing } from '../hooks/queries';
import { useSession } from '../auth/session';
import { fmt } from '../lib/format';
import { sitesDomain } from '../api';
import s from './Pricing.module.css';

function Check() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--text)" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden className={s.check}>
      <path d="M5 12l5 5L20 7" />
    </svg>
  );
}

function Cell({ title, lead, price, unit, note, action, items, featured }: {
  title: string;
  lead: string;
  price: string;
  unit: string;
  note: string;
  action: ReactNode;
  items: ReactNode[];
  featured?: boolean;
}) {
  return (
    <div className={featured ? `${s.cell} ${s.featured}` : s.cell}>
      {featured && <span className={s.line} aria-hidden />}
      <div className={s.cellHead}>
        <span className={s.title}>{title}</span>
        <span className={s.lead}>{lead}</span>
      </div>
      <div className={s.price}>
        <span className={s.amount}>{price}</span>
        <span className={s.unit}>{unit}</span>
      </div>
      <span className={s.note}>{note}</span>
      {action}
      <ul className={s.list}>
        {items.map((it, i) => (
          <li key={i}>
            <Check />
            <span>{it}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

export default function Pricing() {
  const { data: p, isError } = usePricing();
  const { token } = useSession();

  return (
    <MarketingLayout>
      <section className={s.hero}>
        <div className={s.grid} aria-hidden />
        <div className={s.heroInner}>
          <h1 className={s.h1}>
            Preços em meticais.
            <br />
            Pagas com o telemóvel.
          </h1>
          <p className={s.heroLead}>
            O alojamento em .{sitesDomain} é grátis. Pagas só os créditos para alterar o site.
          </p>
        </div>
      </section>
      <div className={s.frame}>
        {!p ? (
          <div className={s.loading}>{isError ? 'Não consegui carregar os preços. Tenta outra vez daqui a pouco.' : <Spinner label="A carregar os preços" />}</div>
        ) : (
          <section className={s.cells}>
            <Cell
              title="Alojamento"
              lead="O teu site no ar, sem pagar nada."
              price="0"
              unit="MZN"
              note="Para sempre"
              action={
                <ButtonLink to={token ? '/novo' : '/criar-conta'} variant="secondary">
                  Começar grátis
                </ButtonLink>
              }
              items={[
                <>
                  Endereço <span className={s.mono}>nome.{sitesDomain}</span>
                </>,
                'Alojamento e HTTPS incluídos',
                <>
                  <span className={s.strong}>{p.signup_gift} créditos de oferta</span> para criar o primeiro site
                </>,
              ]}
            />
            <Cell
              featured
              title="Créditos"
              lead="Para alterar o site a conversar, quando precisares."
              price={fmt(p.topup_min_mzn)}
              unit="MZN"
              note={`Recarga mínima · ${fmt(Math.floor(p.topup_min_mzn / p.mzn_per_credit))} créditos`}
              action={<ButtonLink to={token ? '/recarregar' : '/criar-conta'}>Recarregar</ButtonLink>}
              items={[
                `Cada alteração gasta ${p.edit} créditos`,
                `Mudar secções ou o aspecto geral gasta ${p.structural_edit} créditos`,
                `Cada site novo gasta ${p.first_page} créditos`,
                'Perguntas no chat não gastam créditos',
                `${fmt(p.topup_min_mzn)} MZN dão para cerca de ${fmt(Math.floor(p.topup_min_mzn / p.mzn_per_credit / p.edit))} alterações`,
              ]}
            />
            <Cell
              title="Domínio próprio"
              lead="Um endereço só teu, como onegocio.co.mz."
              price="Em breve"
              unit=""
              note="Ainda não está disponível"
              action={
                <ButtonLink to="/dominios" variant="secondary">
                  Saber mais
                </ButtonLink>
              }
              items={['Pago com M-Pesa ou e-Mola', 'Ligado ao site sem configurar nada', 'Até lá, o endereço grátis funciona igual']}
            />
          </section>
        )}
        <p className={s.footLine}>
          <span>
            <Dot tone="green" /> Pagamentos com M-Pesa e e-Mola
          </span>
          <span>IVA incluído</span>
          <span>Sem cartão de crédito</span>
        </p>
      </div>
    </MarketingLayout>
  );
}
