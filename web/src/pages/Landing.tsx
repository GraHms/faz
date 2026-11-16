import { useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { MarketingLayout } from '../components/MarketingLayout';
import { PromptCard } from '../components/PromptCard';
import { Soon } from '../components/ui';
import { useSession } from '../auth/session';
import { draft } from '../lib/draft';
import { examples, type Example } from '../lib/examples';
import { slugify } from '../lib/format';
import { sitesDomain } from '../api';
import s from './Landing.module.css';

export default function Landing() {
  const nav = useNavigate();
  const { token } = useSession();
  const [prompt, setPrompt] = useState(() => draft.get()?.prompt ?? '');
  const [files, setFiles] = useState<File[]>(() => draft.files());
  const [name, setName] = useState('');
  const top = useRef<HTMLDivElement>(null);

  function start(d: { prompt: string; businessName?: string; businessType?: string }) {
    draft.set(d);
    draft.setFiles(files);
    nav(token ? '/novo' : '/criar-conta');
  }

  function pickExample(e: Example) {
    setPrompt(e.prompt);
    draft.set({ prompt: e.prompt, businessName: e.name, businessType: e.type });
    top.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    top.current?.querySelector('textarea')?.focus({ preventScroll: true });
  }

  return (
    <MarketingLayout>
      <section className={s.hero}>
        <div className={s.grid} aria-hidden />
        <div className={s.glow} aria-hidden />
        <div className={s.heroInner} ref={top}>
          <h1 className={s.h1}>O que queres criar?</h1>
          <PromptCard
            value={prompt}
            onChange={setPrompt}
            files={files}
            onFiles={setFiles}
            onSubmit={() => {
              // An example's name and type only carry over if its prompt was kept.
              const d = draft.get();
              start(d?.prompt === prompt.trim() ? d : { prompt: prompt.trim() });
            }}
          />
          <p className={s.heroNote}>O primeiro site é por nossa conta. O alojamento em .{sitesDomain} é grátis.</p>
        </div>
      </section>

      <div className={s.frame}>
        <span className={s.cornerL} aria-hidden />
        <span className={s.cornerR} aria-hidden />

        <section className={s.head}>
          <div className={s.headTitle}>
            <span className={s.eyebrow}>EXEMPLOS</span>
            <h2 className={s.h2}>Começa por um destes</h2>
          </div>
          <p className={s.headLead}>Toca num exemplo para o usar como ponto de partida.</p>
        </section>
        <section className={s.cells}>
          {examples.map((e, i) => (
            <button key={e.name} type="button" className={s.example} onClick={() => pickExample(e)}>
              <div className={s.browser}>
                <div className={s.browserBar} aria-hidden>
                  <span />
                  <span />
                  <span />
                </div>
                <ExampleArt index={i} e={e} />
              </div>
              <div className={s.exampleMeta}>
                <span className={s.swatch} style={{ background: e.color }} aria-hidden />
                <span>
                  <span className={s.exampleName}>{e.name}</span>
                  <span className={s.examplePrompt}>{e.prompt}</span>
                </span>
              </div>
            </button>
          ))}
        </section>

        <section className={s.head} id="como-funciona">
          <div className={s.headTitle}>
            <span className={s.eyebrow}>COMO FUNCIONA</span>
            <h2 className={s.h2}>Do pedido ao site no ar</h2>
          </div>
        </section>
        <section className={s.cells}>
          <div className={s.step}>
            <div className={s.stepArt}>
              Site para o meu salão na Matola, com preços e marcações pelo WhatsApp.
              <span className={s.caret} aria-hidden />
            </div>
            <span className={s.num}>01</span>
            <h3 className={s.h3}>Descreve</h3>
            <p className={s.p}>
              Escreve como falarias com um cliente. Não precisas de saber nada de sites, e o primeiro site é por nossa conta.
            </p>
          </div>
          <div className={s.step}>
            <div className={`${s.stepArt} ${s.chat}`}>
              <span className={s.bubble}>Muda a cor para verde</span>
              <span className={s.reply}>
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--green)" strokeWidth="2.4" aria-hidden>
                  <path d="M5 12l5 5L20 7" />
                </svg>
                Botões e contactos actualizados
              </span>
            </div>
            <span className={s.num}>02</span>
            <h3 className={s.h3}>Ajusta a conversar</h3>
            <p className={s.p}>Pede mudanças em português e vês o resultado na hora, no computador e no telemóvel.</p>
          </div>
          <div className={s.step}>
            <div className={`${s.stepArt} ${s.domains}`}>
              <div>
                <span className="muted">nyeleti.{sitesDomain}</span>
                <span className="dim">grátis</span>
              </div>
              <hr />
              <div>
                <span>nyeleti.co.mz</span>
                <Soon />
              </div>
            </div>
            <span className={s.num}>03</span>
            <h3 className={s.h3}>Publica</h3>
            <p className={s.p}>Publica grátis em .{sitesDomain}. Domínios próprios .co.mz, pagos com M-Pesa, chegam em breve.</p>
          </div>
        </section>

        <section className={s.cta}>
          <div className={s.ctaGlow} aria-hidden />
          <div className={s.ctaText}>
            <h2 className={s.ctaH2}>O teu site no ar, de graça, em .{sitesDomain}.</h2>
            <p className={s.ctaP}>Escolhe o nome agora. Pagas só pelas alterações que pedires, com M-Pesa ou e-Mola.</p>
          </div>
          <form
            className={s.ctaForm}
            onSubmit={(e) => {
              e.preventDefault();
              start({ prompt: prompt.trim(), businessName: name.trim() || undefined });
            }}
          >
            <div className={s.domainBox}>
              <label htmlFor="cta-name" className="sr-only">
                Nome do negócio
              </label>
              <input id="cta-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Nome do teu negócio" autoComplete="organization" maxLength={80} />
              <button type="submit">Começar</button>
            </div>
            <p className={s.address}>
              Endereço grátis: <span className="mono">{slugify(name) || 'o-teu-negocio'}.{sitesDomain}</span>
            </p>
            <div className={s.payments}>
              <span>
                <span className={s.mpesa} aria-hidden />
                M-Pesa
              </span>
              <span>
                <span className={s.emola} aria-hidden />
                e-Mola
              </span>
              <span>Alojamento grátis</span>
            </div>
          </form>
        </section>
      </div>
    </MarketingLayout>
  );
}

/** The three illustrated thumbnails from the prototype, one per example. */
function ExampleArt({ index, e }: { index: number; e: Example }) {
  if (index === 0) {
    return (
      <div className={s.artA}>
        <div className={s.artCol}>
          <span className={s.artBrand} style={{ color: e.color }}>
            {e.name}
          </span>
          <div>
            <div className={s.artHead}>Marisco fresco, à beira-mar.</div>
            <span className={s.artBtn} style={{ background: e.color }}>
              Reservar mesa
            </span>
          </div>
        </div>
        <div className={s.artPhoto} style={{ backgroundColor: e.color }} />
      </div>
    );
  }
  if (index === 1) {
    return (
      <div className={s.artB}>
        <div className={`${s.artPhoto} ${s.arch}`} style={{ backgroundColor: e.color }} />
        <div className={s.artCol}>
          <span className={s.artBrand} style={{ color: e.color, fontStyle: 'italic' }}>
            {e.name}
          </span>
          <div>
            <div className={s.artHead}>Tranças, cor e cuidado.</div>
            <span className={s.artBtn} style={{ background: e.color }}>
              Marcar horário
            </span>
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className={s.artC}>
      <div className={s.artRow}>
        <span className={s.artBrand}>{e.name}</span>
        <span className={s.artGhost}>Encomendar</span>
      </div>
      <div className={s.artHead}>Capulanas escolhidas a dedo.</div>
      <div className={s.artCloth}>
        <span style={{ backgroundColor: '#B4452A' }} />
        <span style={{ backgroundColor: '#D9A13B' }} />
        <span style={{ backgroundColor: '#2F6B5A' }} />
      </div>
    </div>
  );
}
