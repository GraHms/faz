import { useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import type { Pricing } from '../api';
import { Modal } from './Modal';
import { Button } from './ui';
import { credits, fmt } from '../lib/format';
import s from './TopUpModal.module.css';

export type TopUpReason = 'edit' | 'page' | 'default';

/** The usual top ups: the minimum, then two round amounts above it. */
export function packs(p: Pricing): number[] {
  const out = [p.topup_min_mzn, 500, 1000].filter((v) => v >= p.topup_min_mzn && v <= p.topup_max_mzn);
  return [...new Set(out)].sort((a, b) => a - b);
}

export function TopUpModal({
  open,
  onClose,
  pricing,
  balance,
  reason = 'default',
  cost,
}: {
  open: boolean;
  onClose: () => void;
  pricing: Pricing | undefined;
  balance: number;
  reason?: TopUpReason;
  /** What the blocked action costs, from the 402 details. */
  cost?: number;
}) {
  const nav = useNavigate();
  const loc = useLocation();
  const options = useMemo(() => (pricing ? packs(pricing) : []), [pricing]);
  const [pick, setPick] = useState<number | null>(null);
  const amount = pick ?? options[0] ?? 0;

  const copy = {
    edit: {
      title: 'Recarrega para continuar a editar',
      desc: `Cada alteração gasta ${cost ?? pricing?.edit ?? 0} créditos e tens ${balance}.`,
    },
    page: {
      title: 'Recarrega para criar este site',
      desc: `Um site novo gasta ${cost ?? pricing?.first_page ?? 0} créditos e tens ${balance}.`,
    },
    default: {
      title: 'Recarregar créditos',
      desc: 'Os créditos servem para criar sites e fazer alterações. O alojamento continua grátis.',
    },
  }[reason];

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={copy.title}
      description={copy.desc}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Agora não
          </Button>
          <Button
            disabled={!pricing}
            onClick={() => {
              const back = encodeURIComponent(loc.pathname + loc.search);
              nav(`/recarregar?valor=${amount}&voltar=${back}`);
            }}
          >
            Continuar · {fmt(amount)} MZN
          </Button>
        </>
      }
    >
      <p className={s.balance}>
        Saldo: <strong>{credits(balance)}</strong>
      </p>
      {pricing && (
        <fieldset className={s.packs}>
          <legend className="sr-only">Valor da recarga</legend>
          {options.map((v) => {
            const c = Math.floor(v / pricing.mzn_per_credit);
            return (
              <label key={v} className={s.pack}>
                <input type="radio" name="pack" checked={amount === v} onChange={() => setPick(v)} />
                <span className={s.packBody}>
                  <span className={s.packAmount}>{fmt(v)} MZN</span>
                  <span className={s.packNote}>
                    {credits(c)} · cerca de {fmt(Math.floor(c / pricing.edit))} alterações
                  </span>
                </span>
              </label>
            );
          })}
        </fieldset>
      )}
      <p className={s.note}>
        M-Pesa ou e-Mola · mínimo {pricing ? fmt(pricing.topup_min_mzn) : '…'} MZN · IVA incluído
      </p>
    </Modal>
  );
}
