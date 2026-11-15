import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { MarketingLayout } from '../components/MarketingLayout';
import { Dot } from '../components/ui';
import { useSession } from '../auth/session';
import { draft } from '../lib/draft';
import { slugify } from '../lib/format';
import { sitesDomain } from '../api';
import s from './Domains.module.css';

// Custom domains have no backend yet. The page says so plainly and shows the
// free address the owner gets today, instead of a search that cannot buy.
export default function Domains() {
  const [q, setQ] = useState('');
  const { token } = useSession();
  const nav = useNavigate();
  const name = slugify(q) || 'o-teu-negocio';

  return (
    <MarketingLayout>
      <section className={s.hero}>
        <div className={s.inner}>
          <h1 className={s.h1}>Encontra o teu domínio</h1>
          <p className={s.lead}>
            Domínios próprios .co.mz e .com, pagos com M-Pesa, estão a caminho. Até lá, cada site é publicado grátis em
            nome.{sitesDomain}.
          </p>
          <form
            className={s.search}
            onSubmit={(e) => {
              e.preventDefault();
              draft.set({ prompt: draft.get()?.prompt ?? '', businessName: q.trim() || undefined });
              nav(token ? '/novo' : '/criar-conta');
            }}
          >
            <label htmlFor="domain-q" className="sr-only">
              Nome do negócio
            </label>
            <input id="domain-q" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Nome do teu negócio" autoComplete="organization" maxLength={80} />
            <button type="submit">Começar</button>
          </form>
          <ul className={s.rows} aria-live="polite">
            <li>
              <span className={s.name}>
                {name}.{sitesDomain}
              </span>
              <span className={s.free}>
                <Dot tone="green" /> grátis, escolhes ao publicar
              </span>
            </li>
            <li>
              <span className={s.name}>{name}.co.mz</span>
              <span className={s.soon}>
                <Dot tone="amber" /> em breve
              </span>
            </li>
            <li>
              <span className={s.name}>{name}.com</span>
              <span className={s.soon}>
                <Dot tone="amber" /> em breve
              </span>
            </li>
          </ul>
        </div>
      </section>
    </MarketingLayout>
  );
}
