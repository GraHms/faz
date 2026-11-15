import type { ReactNode } from 'react';
import { Logo } from './ui';
import { usePricing } from '../hooks/queries';
import { slugify } from '../lib/format';
import { sitesDomain } from '../api';
import s from './AuthShell.module.css';

/** The split layout of the sign up and login screens. */
export function AuthShell({ children, aside }: { children: ReactNode; aside?: ReactNode }) {
  return (
    <div className={s.shell}>
      <div className={s.main}>
        <div className={s.top}>
          <Logo />
        </div>
        <div className={s.content}>{children}</div>
      </div>
      {aside && <aside className={s.aside}>{aside}</aside>}
    </div>
  );
}

/** "Pronto a construir": what the visitor asked for, waiting for an account. */
export function ReadyPanel({ prompt, businessName }: { prompt?: string; businessName?: string }) {
  const { data: p } = usePricing();
  const slug = slugify(businessName ?? '') || 'o-teu-negocio';
  return (
    <>
      <div className={s.asideGrid} aria-hidden />
      <div className={s.asideGlow} aria-hidden />
      <div className={s.ready}>
        <div className={s.readyHead}>
          <span className={s.amber} aria-hidden />
          Pronto a construir
        </div>
        <div className={prompt ? s.readyPrompt : `${s.readyPrompt} ${s.placeholder}`}>
          {prompt || 'Descreve o teu negócio e o que o site deve fazer. Fazemos as perguntas que faltarem.'}
        </div>
        <dl className={s.readyRows}>
          <div>
            <dt>Endereço grátis</dt>
            <dd className="mono">
              {slug}
              <span className="dim">.{sitesDomain}</span>
            </dd>
          </div>
          <div>
            <dt>Domínio próprio</dt>
            <dd>Em breve</dd>
          </div>
          <div>
            <dt>Créditos de oferta</dt>
            <dd>{p ? `${p.signup_gift} créditos` : '…'}</dd>
          </div>
          <div>
            <dt>Idioma</dt>
            <dd>Português</dd>
          </div>
        </dl>
      </div>
    </>
  );
}
