import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { api, isApiError, type Payment, type Pricing } from '../api';
import { safeBack, useSession } from '../auth/session';
import { Button, Field, Logo, Notice, Spinner } from '../components/ui';
import { packs } from '../components/TopUpModal';
import { keys, usePayment, usePricing } from '../hooks/queries';
import { credits, fmt, fmtPhone, mzn, parsePhone, vatIncluded, wallets, type Wallet } from '../lib/format';
import s from './Checkout.module.css';

const WAIT_SECONDS = 120;

export default function Checkout() {
  const [params] = useSearchParams();
  const back = safeBack(params.get('voltar'));
  const { data: pricing } = usePricing();
  const [paymentId, setPaymentId] = useState<string>();
  const payment = usePayment(paymentId);

  return (
    <div className={s.page}>
      <header className={s.header}>
        <Logo to="/painel" />
        <span className={s.secure}>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
            <rect x="5" y="11" width="14" height="10" rx="2" />
            <path d="M8 11V7a4 4 0 018 0v4" />
          </svg>
          Pagamento seguro
        </span>
        <Link to={back} className={s.back}>
          Voltar
        </Link>
      </header>
      {!pricing ? (
        <div className={s.loading}>
          <Spinner label="A carregar" />
        </div>
      ) : paymentId && payment.data ? (
        <Status payment={payment.data} back={back} onRetry={() => setPaymentId(undefined)} />
      ) : (
        <PayForm pricing={pricing} initial={Number(params.get('valor')) || 0} onStarted={setPaymentId} />
      )}
    </div>
  );
}

function PayForm({ pricing, initial, onStarted }: { pricing: Pricing; initial: number; onStarted: (id: string) => void }) {
  const options = packs(pricing);
  const preset = options.includes(initial) ? initial : null;
  const [pack, setPack] = useState<number | 'other'>(preset ?? (initial ? 'other' : options[0]!));
  const [other, setOther] = useState(initial && !preset ? String(initial) : '');
  const [method, setMethod] = useState<Wallet>('mpesa');
  const [phone, setPhone] = useState('');
  const [nuit, setNuit] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const { account } = useSession();

  const clear = (k: string) => setErrors((e) => (e[k] ? { ...e, [k]: '' } : e));
  const amount = pack === 'other' ? Math.floor(Number(other.replace(/\s/g, '')) || 0) : pack;
  const creditCount = Math.floor(amount / pricing.mzn_per_credit);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    if (amount < pricing.topup_min_mzn || amount > pricing.topup_max_mzn) {
      next.amount = `Escolhe um valor entre ${fmt(pricing.topup_min_mzn)} e ${fmt(pricing.topup_max_mzn)} MZN.`;
    }
    const p = parsePhone(phone);
    if (!p.ok) next.phone = p.error;
    else if (p.wallet !== method) {
      next.phone = `Este número é ${wallets[p.wallet].name}. Escolhe ${wallets[p.wallet].name} ou usa um número ${wallets[method].prefixes.join(' ou ')}.`;
    }
    if (nuit && !/^\d{9}$/.test(nuit)) next.nuit = 'O NUIT tem 9 dígitos.';
    setErrors(next);
    setError(null);
    if (Object.keys(next).length || !p.ok) return;
    setBusy(true);
    try {
      const pay = await api.topUp({ amount_mzn: amount, msisdn: p.msisdn, nuit: nuit || undefined });
      onStarted(pay.id);
    } catch (err) {
      setBusy(false);
      if (isApiError(err, 'invalid_msisdn')) setErrors({ phone: err.message });
      else if (isApiError(err, 'invalid_amount')) setErrors({ amount: err.message });
      else if (isApiError(err, 'invalid_nuit')) setErrors({ nuit: err.message });
      else setError(isApiError(err) ? err.message : 'Não foi possível iniciar o pagamento. Tenta outra vez.');
    }
  }

  return (
    <div className={s.layout}>
      <form className={s.form} onSubmit={submit} noValidate>
        <div className={s.head}>
          <h1 className={s.h1}>Finalizar compra</h1>
          <p className={s.lead}>Sem cartão. Confirmas o pagamento no telemóvel com o teu PIN.</p>
        </div>
        {error && <Notice>{error}</Notice>}

        <fieldset className={s.group}>
          <legend>Créditos</legend>
          <div className={s.packs}>
            {options.map((v) => {
              const c = Math.floor(v / pricing.mzn_per_credit);
              return (
                <label key={v} className={s.pack}>
                  <input type="radio" name="pack" checked={pack === v} onChange={() => setPack(v)} />
                  <span className={s.packAmount}>{fmt(v)} MZN</span>
                  <span className={s.packCredits}>{credits(c)}</span>
                  <span className={s.packNote}>cerca de {fmt(Math.floor(c / pricing.edit))} alterações</span>
                </label>
              );
            })}
          </div>
          <label className={s.otherToggle}>
            <input type="radio" name="pack" checked={pack === 'other'} onChange={() => setPack('other')} />
            Outro valor · mínimo {fmt(pricing.topup_min_mzn)} MZN
          </label>
          {pack === 'other' && (
            <Field
              label="Valor"
              inputMode="numeric"
              value={other}
              onChange={(e) => {
                clear('amount');
                setOther(e.target.value.replace(/[^\d\s]/g, ''));
              }}
              suffix="MZN"
              error={errors.amount}
              autoFocus
            />
          )}
          {pack !== 'other' && errors.amount && <span className={s.error}>{errors.amount}</span>}
        </fieldset>

        <fieldset className={s.group}>
          <legend>Pagar com</legend>
          <div className={s.methods}>
            {(Object.keys(wallets) as Wallet[]).map((w) => (
              <label key={w} className={s.method}>
                <input type="radio" name="method" checked={method === w} onChange={() => setMethod(w)} />
                <span className={w === 'mpesa' ? s.mpesa : s.emola} aria-hidden>
                  {w === 'mpesa' ? 'M' : 'e'}
                </span>
                <span>
                  <span className={s.methodName}>{wallets[w].name}</span>
                  <span className={s.methodNote}>
                    {wallets[w].network} · {wallets[w].prefixes.join(', ')}
                  </span>
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        <Field
          label="Número"
          type="tel"
          inputMode="tel"
          autoComplete="tel-national"
          prefix="+258"
          placeholder="84 123 4567"
          value={phone}
          onChange={(e) => {
            const v = fmtPhone(e.target.value);
            clear('phone');
            setPhone(v);
            const p = parsePhone(v);
            if (p.ok) setMethod(p.wallet);
          }}
          error={errors.phone}
          hint={`Recebes um pedido de confirmação ${method === 'mpesa' ? 'do M-Pesa' : 'do e-Mola'} neste número.`}
        />
        <Field
          label="NUIT para a factura · opcional"
          inputMode="numeric"
          maxLength={9}
          value={nuit}
          onChange={(e) => {
            clear('nuit');
            setNuit(e.target.value.replace(/\D/g, ''));
          }}
          error={errors.nuit}
        />
        <Button type="submit" size="lg" block loading={busy}>
          Pagar {fmt(amount)} MZN com {wallets[method].name}
        </Button>
        {account && <p className={s.hint}>Saldo actual: {credits(account.balance)}.</p>}
      </form>

      <aside className={s.summary} aria-label="Resumo">
        <h2 className={s.summaryTitle}>Resumo</h2>
        <dl className={s.rows}>
          <div>
            <dt>Créditos</dt>
            <dd>{credits(creditCount)}</dd>
          </div>
          <div>
            <dt>Dá para</dt>
            <dd>cerca de {fmt(Math.floor(creditCount / pricing.edit))} alterações</dd>
          </div>
          <div className={s.total}>
            <dt>Total</dt>
            <dd>{mzn(amount)}</dd>
          </div>
        </dl>
        <p className={s.vat}>Inclui IVA de 16% ({mzn(vatIncluded(amount))}).</p>
      </aside>
    </div>
  );
}

function Status({ payment, back, onRetry }: { payment: Payment; back: string; onRetry: () => void }) {
  const qc = useQueryClient();
  const nav = useNavigate();
  const [left, setLeft] = useState(WAIT_SECONDS);
  const wallet = wallets[payment.method];
  const national = payment.msisdn.replace(/^258/, '');

  useEffect(() => {
    if (payment.status !== 'pending') return;
    const t = setInterval(() => setLeft((v) => Math.max(0, v - 1)), 1000);
    return () => clearInterval(t);
  }, [payment.status]);

  useEffect(() => {
    if (payment.status === 'succeeded') {
      qc.invalidateQueries({ queryKey: keys.me });
      qc.invalidateQueries({ queryKey: keys.credits });
      qc.invalidateQueries({ queryKey: keys.payments });
    }
  }, [payment.status, qc]);

  if (payment.status === 'pending') {
    return (
      <div className={s.state} role="status" aria-live="polite">
        <div className={s.ring} aria-hidden>
          <span />
          <span />
          <b className={payment.method === 'mpesa' ? s.mpesa : s.emola}>{payment.method === 'mpesa' ? 'M' : 'e'}</b>
        </div>
        <h1 className={s.h1}>Confirma no teu telemóvel</h1>
        <p className={s.lead}>
          Enviámos um pedido de {mzn(payment.amount_mzn)} para o {fmtPhone(national)}. Abre a mensagem do {wallet.name} e
          escreve o teu PIN.
        </p>
        {left > 0 ? (
          <p className={s.countdown}>
            <Spinner /> A aguardar confirmação · {Math.floor(left / 60)}:{String(left % 60).padStart(2, '0')}
          </p>
        ) : (
          <>
            <p className={s.countdown}>Ainda sem resposta. Se cancelaste no telemóvel, tenta outra vez.</p>
            <Button variant="secondary" onClick={onRetry}>
              Tentar outra vez
            </Button>
          </>
        )}
      </div>
    );
  }

  if (payment.status === 'failed') {
    return (
      <div className={s.state}>
        <Notice>
          O pagamento não foi confirmado{payment.failure_reason ? `: ${payment.failure_reason.replace(/\.$/, '')}` : ''}. Verifica o
          saldo e tenta outra vez.
        </Notice>
        <div className={s.actions}>
          <Button onClick={onRetry}>Tentar outra vez</Button>
          <Button variant="secondary" onClick={() => nav(back)}>
            Voltar
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className={s.state}>
      <div className={s.okMark} aria-hidden>
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4">
          <path d="M5 12l5 5L20 7" />
        </svg>
      </div>
      <h1 className={s.h1}>Pagamento confirmado</h1>
      <p className={s.lead}>Juntámos {credits(payment.credits)} ao teu saldo.</p>
      <dl className={`${s.rows} ${s.receipt}`}>
        <div>
          <dt>Valor</dt>
          <dd>{mzn(payment.amount_mzn)}</dd>
        </div>
        <div>
          <dt>IVA incluído</dt>
          <dd>{mzn(payment.vat_mzn)}</dd>
        </div>
        <div>
          <dt>Créditos</dt>
          <dd>{credits(payment.credits)}</dd>
        </div>
        <div>
          <dt>{wallet.name}</dt>
          <dd>+258 {fmtPhone(national)}</dd>
        </div>
        {payment.nuit && (
          <div>
            <dt>NUIT</dt>
            <dd>{payment.nuit}</dd>
          </div>
        )}
        {payment.provider_ref && (
          <div>
            <dt>Referência</dt>
            <dd className="mono">{payment.provider_ref}</dd>
          </div>
        )}
        <div>
          <dt>Data</dt>
          <dd>{new Date(payment.updated_at).toLocaleString('pt-MZ', { dateStyle: 'medium', timeStyle: 'short' })}</dd>
        </div>
      </dl>
      <Button size="lg" onClick={() => nav(back)}>
        Continuar
      </Button>
    </div>
  );
}
