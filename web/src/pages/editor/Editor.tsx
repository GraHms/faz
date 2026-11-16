import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { api, isApiError, type Job, type Message, type SiteView } from '../../api';
import { CORRECTION_KEY } from '../../api/types';
import { answerSummary, storedToValues } from '../../lib/answers';
import { useSession } from '../../auth/session';
import { TopUpModal, type TopUpReason } from '../../components/TopUpModal';
import { Button, Notice, Pill, Spinner } from '../../components/ui';
import { activeJobId, keys, useJob, usePricing, useSite } from '../../hooks/queries';
import { draft, imageAccept, pickImages } from '../../lib/draft';
import { credits, firstName } from '../../lib/format';
import { siteStatus } from '../../lib/status';
import { Preview, type Device } from './Preview';
import { Progress } from './Progress';
import { PublishModal } from './PublishModal';
import { FazLabel, Questions } from './Questions';
import s from './editor.module.css';

export default function Editor() {
  const { id = '' } = useParams();
  const site = useSite(id);

  if (site.isPending) {
    return (
      <div className={s.center}>
        <Spinner label="A abrir o site" />
      </div>
    );
  }
  if (site.isError || !site.data) {
    const missing = isApiError(site.error) && site.error.status === 404;
    return (
      <div className={s.center}>
        <Notice>{missing ? 'Este site não existe ou não é teu.' : 'Não consegui abrir o site. Actualiza a página.'}</Notice>
        <Link to="/painel">Voltar ao painel</Link>
      </div>
    );
  }
  return <Workspace view={site.data} />;
}

interface TopUpState {
  reason: TopUpReason;
  cost?: number;
}

function Workspace({ view }: { view: SiteView }) {
  const site = view.site;
  const qc = useQueryClient();
  const loc = useLocation();
  const { account } = useSession();
  const { data: pricing } = usePricing();
  const [device, setDevice] = useState<Device>('desktop');
  const [tab, setTab] = useState<'chat' | 'site'>('chat');
  const [publishing, setPublishing] = useState(false);
  const [topUp, setTopUp] = useState<TopUpState | null>(null);
  const [notice, setNotice] = useState<string | null>(() => {
    const failed = (loc.state as { uploadFailed?: number } | null)?.uploadFailed;
    return failed ? `${failed === 1 ? 'Uma foto não foi enviada' : `${failed} fotos não foram enviadas`}. Envia-as outra vez pelo clipe.` : null;
  });
  const [answering, setAnswering] = useState(false);

  const jobId = activeJobId(view);
  const job = useJob(jobId);
  const jobKind = job.data?.kind === 'edit' ? 'edit' : view.messages.find((m) => m.job_id === jobId)?.role === 'user' ? 'edit' : 'build';

  // When the job ends, the site, balance and history have changed.
  const finished = job.data?.status === 'succeeded' || job.data?.status === 'failed';
  useEffect(() => {
    if (!finished) return;
    void qc.invalidateQueries({ queryKey: keys.site(site.id) });
    void qc.invalidateQueries({ queryKey: keys.me });
    void qc.invalidateQueries({ queryKey: keys.credits });
    void qc.invalidateQueries({ queryKey: keys.sites });
    if (job.data?.status === 'succeeded') setTab('site');
  }, [finished, job.data?.status, qc, site.id]);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: keys.site(site.id) });
    void qc.invalidateQueries({ queryKey: keys.me });
  };

  async function answer(answers: Record<string, string>, skip: boolean, correction: string) {
    setAnswering(true);
    setNotice(null);
    try {
      await api.answer(site.id, answers, skip, correction);
      refresh();
    } catch (e) {
      if (isApiError(e, 'insufficient_credits')) setTopUp({ reason: 'page', cost: Number(e.details.needed) || undefined });
      else setNotice(isApiError(e) ? e.message : 'Não foi possível começar a construção. Tenta outra vez.');
    } finally {
      setAnswering(false);
    }
  }

  const st = siteStatus(site);
  const balance = account?.balance ?? 0;

  return (
    <div className={s.workspace}>
      <header className={s.header}>
        <Link to="/painel" className={s.homeMark} title="Painel" aria-label="Voltar ao painel">
          <span aria-hidden />
        </Link>
        <span className={s.slash} aria-hidden>
          /
        </span>
        <Link to="/painel" className={s.crumb}>
          {firstName(account?.user.name ?? '')}
        </Link>
        <span className={`${s.slash} ${s.hideSm}`} aria-hidden>
          /
        </span>
        <span className={s.siteName}>{site.business_name}</span>
        <Pill tone={st.tone} pulse={st.pulse}>
          {st.label}
        </Pill>
        <span className={s.grow} />
        <div className={s.devices} role="group" aria-label="Ver como">
          {(['desktop', 'mobile'] as Device[]).map((d) => (
            <button key={d} aria-pressed={device === d} onClick={() => setDevice(d)} title={d === 'desktop' ? 'Computador' : 'Telemóvel'}>
              {d === 'desktop' ? (
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
                  <rect x="3" y="4" width="18" height="12" rx="2" />
                  <path d="M8 20h8M12 16v4" />
                </svg>
              ) : (
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
                  <rect x="7" y="2" width="10" height="20" rx="2" />
                  <path d="M11 18h2" />
                </svg>
              )}
              <span className="sr-only">{d === 'desktop' ? 'Computador' : 'Telemóvel'}</span>
            </button>
          ))}
        </div>
        {view.live_url && (
          <a className={s.liveLink} href={view.live_url} target="_blank" rel="noopener noreferrer">
            Abrir
          </a>
        )}
        <Button size="sm" onClick={() => setPublishing(true)} disabled={site.current_version === 0}>
          Publicar
        </Button>
      </header>

      <div className={s.body} data-tab={tab}>
        <aside className={s.chatPane} aria-label="Conversa">
          <Chat
            view={view}
            jobId={jobId}
            job={job.data}
            jobKind={jobKind}
            answering={answering}
            onAnswer={answer}
            notice={notice}
            setNotice={setNotice}
            onNeedCredits={(cost) => setTopUp({ reason: 'edit', cost })}
            onTopUp={() => setTopUp({ reason: 'default' })}
            onShowSite={() => setTab('site')}
            balance={balance}
            firstPage={pricing?.first_page}
            editCost={pricing?.edit}
          />
        </aside>
        <main className={s.previewPane}>
          <Preview view={view} device={device} />
        </main>
      </div>

      <nav className={s.mobileTabs} aria-label="Vista">
        <button aria-pressed={tab === 'chat'} onClick={() => setTab('chat')}>
          Chat
        </button>
        <button aria-pressed={tab === 'site'} onClick={() => setTab('site')}>
          Site
        </button>
      </nav>

      <PublishModal open={publishing} onClose={() => setPublishing(false)} view={view} />
      <TopUpModal
        open={!!topUp}
        onClose={() => setTopUp(null)}
        pricing={pricing}
        balance={balance}
        reason={topUp?.reason}
        cost={topUp?.cost}
      />
    </div>
  );
}

function Chat({
  view,
  jobId,
  job,
  jobKind,
  answering,
  onAnswer,
  notice,
  setNotice,
  onNeedCredits,
  onTopUp,
  onShowSite,
  balance,
  firstPage,
  editCost,
}: {
  view: SiteView;
  jobId: number | undefined;
  job: Job | undefined;
  jobKind: 'build' | 'edit';
  answering: boolean;
  onAnswer: (a: Record<string, string>, skip: boolean, correction: string) => void;
  notice: string | null;
  setNotice: (v: string | null) => void;
  onNeedCredits: (cost?: number) => void;
  onTopUp: () => void;
  onShowSite: () => void;
  balance: number;
  firstPage?: number;
  editCost?: number;
}) {
  const site = view.site;
  const qc = useQueryClient();
  const nav = useNavigate();
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const [uploading, setUploading] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const ready = site.status === 'ready' && !view.busy;

  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
  }, [view.messages.length, view.busy, site.status]);

  async function send(body: string) {
    const t = body.trim();
    if (!t || sending) return;
    setSending(true);
    setNotice(null);
    try {
      const r = await api.sendMessage(site.id, t);
      setText('');
      qc.setQueryData<SiteView>(keys.site(site.id), (v) =>
        v
          ? {
              ...v,
              messages: [...v.messages, ...r.messages],
              busy: v.busy || !!r.job_id,
              site: r.job_id ? { ...v.site, status: 'editing' } : v.site,
            }
          : v,
      );
      if (r.credits_charged) void qc.invalidateQueries({ queryKey: keys.me });
    } catch (e) {
      if (isApiError(e, 'insufficient_credits')) onNeedCredits(Number(e.details.needed) || undefined);
      else setNotice(isApiError(e) ? e.message : 'A mensagem não foi enviada. Tenta outra vez.');
    } finally {
      setSending(false);
    }
  }

  async function upload(files: FileList) {
    const { ok, rejected } = pickImages(files);
    setNotice(rejected);
    if (!ok.length) return;
    setUploading(true);
    const names: string[] = [];
    for (const f of ok) {
      try {
        names.push((await api.uploadAsset(site.id, f)).name);
      } catch (e) {
        setNotice(isApiError(e) ? e.message : `Não consegui enviar ${f.name}.`);
      }
    }
    setUploading(false);
    void qc.invalidateQueries({ queryKey: keys.site(site.id) });
    if (names.length && site.current_version > 0) {
      setText((t) => (t ? `${t} ` : '') + `Usa ${names.length === 1 ? 'a foto' : 'as fotos'} ${names.join(', ')} `);
    }
  }

  function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      if (ready) void send(text);
    }
  }

  const lastAssistant = [...view.messages].reverse().find((m) => m.role === 'assistant');
  // Quick replies the assistant offered render under its message; these are
  // the defaults when it offered none.
  const chips = ready && !lastAssistant?.options?.length ? ['Muda a cor principal', 'Acrescenta o horário', 'Adiciona uma galeria de fotos'] : [];
  const answers = answerSummary(view.questions, site.answers);
  const correction = site.answers?.[CORRECTION_KEY];
  const enough = editCost === undefined || balance >= editCost;

  return (
    <>
      <div className={s.messages} ref={scroller}>
        <div className={s.userMsg}>{site.prompt}</div>
        {view.assets.length > 0 && (
          <div className={s.assets} aria-label="Fotos enviadas">
            {view.assets.map((a) => (
              <span key={a.name}>{a.name}</span>
            ))}
          </div>
        )}
        {site.status !== 'questions' && answers.length > 0 && <div className={s.userMsg}>{answers.join(' · ')}</div>}
        {site.status !== 'questions' && correction && <div className={s.userMsg}>Correcção: {correction}</div>}

        {view.messages.map((m, k) => (
          <MessageView
            key={m.id}
            m={m}
            prev={view.messages[k - 1]}
            showProgress={m.job_id === jobId && view.busy}
            progress={<Progress key={jobId} job={job} kind={jobKind} />}
            onOption={(o) => void send(o)}
            optionsActive={ready && m === lastAssistant}
          />
        ))}

        {site.status === 'questions' && !view.busy && (
          <Questions
            key={view.messages.length}
            questions={view.questions ?? []}
            understood={view.understood}
            initial={storedToValues(view.questions ?? [], site.answers)}
            initialCorrection={correction ?? ''}
            busy={answering}
            onDone={(a, c) => onAnswer(a, false, c)}
            onSkipAll={(c) => onAnswer({}, true, c)}
            footnote={firstPage ? `Construir gasta ${credits(firstPage)}. Tens ${credits(balance)}.` : undefined}
          />
        )}

        {site.status === 'ready' && site.current_version > 0 && !view.busy && (
          <div className={s.chips}>
            {chips.map((c) => (
              <button key={c} className={s.chip} onClick={() => void send(c)} disabled={sending}>
                {c}
              </button>
            ))}
            <button className={`${s.chip} ${s.chipStrong} ${s.showSm}`} onClick={onShowSite}>
              Ver o site
            </button>
          </div>
        )}
        {sending && (
          <div className={s.typing} aria-live="polite">
            <FazLabel />
            <span className={s.dots} aria-label="A responder">
              <i />
              <i />
              <i />
            </span>
          </div>
        )}
        {notice && <Notice>{notice}</Notice>}
      </div>

      <div className={s.composerWrap}>
        <div className={s.composer}>
          <label htmlFor="chat-input" className="sr-only">
            Pede uma alteração
          </label>
          <textarea
            id="chat-input"
            rows={2}
            value={text}
            maxLength={2000}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKey}
            disabled={!ready}
            placeholder={
              site.status === 'questions'
                ? 'Responde às perguntas acima para começar.'
                : ready
                  ? 'Pede uma alteração…'
                  : 'Espera que o site fique pronto…'
            }
          />
          <div className={s.composerBar}>
            <button className={s.attach} title="Anexar fotos" aria-label="Anexar fotos" onClick={() => fileInput.current?.click()} disabled={uploading}>
              {uploading ? (
                <Spinner />
              ) : (
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
                  <path d="M21 11.5l-8.6 8.6a5 5 0 01-7.1-7.1l8.6-8.6a3.3 3.3 0 014.7 4.7l-8.6 8.6a1.7 1.7 0 01-2.4-2.4l7.9-7.9" />
                </svg>
              )}
            </button>
            <input
              ref={fileInput}
              type="file"
              accept={imageAccept}
              multiple
              hidden
              onChange={(e) => {
                if (e.target.files) void upload(e.target.files);
                e.target.value = '';
              }}
            />
            <span className={s.balanceLine}>
              <span className={enough ? s.dotGreen : s.dotAmber} aria-hidden />
              {credits(balance)}
              {editCost !== undefined && <span className={s.hideSm}> · alteração {editCost}</span>}
              <button className={s.linkBtn} onClick={onTopUp}>
                Recarregar
              </button>
            </span>
            <button className={s.send} onClick={() => void send(text)} disabled={!ready || !text.trim() || sending} aria-label="Enviar">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
                <path d="M12 19V5M6 11l6-6 6 6" />
              </svg>
            </button>
          </div>
        </div>
        {site.status === 'questions' && site.current_version === 0 && view.messages.some((m) => m.role === 'assistant' && m.job_id) && (
          <p className={s.small}>
            <button
              className={s.linkBtn}
              onClick={() => {
                draft.set({ prompt: site.prompt, businessName: site.business_name, businessType: site.business_type });
                nav('/novo');
              }}
            >
              Começar de novo com outro pedido
            </button>
          </p>
        )}
      </div>
    </>
  );
}

function MessageView({
  m,
  prev,
  showProgress,
  progress,
  onOption,
  optionsActive,
}: {
  m: Message;
  prev?: Message;
  showProgress: boolean;
  progress: ReactNode;
  onOption: (o: string) => void;
  optionsActive: boolean;
}) {
  if (m.role === 'user') {
    return (
      <div className={s.userGroup}>
        <div className={s.userMsg}>{m.text}</div>
        {m.credits_charged > 0 && <span className={s.cost}>−{credits(m.credits_charged)}</span>}
        {showProgress && progress}
      </div>
    );
  }
  const free = !m.job_id && prev?.role === 'user' && prev.credits_charged === 0;
  return (
    <div className={s.aiBlock}>
      <FazLabel />
      <p>{m.text}</p>
      {showProgress && progress}
      {m.options && m.options.length > 0 && optionsActive && (
        <div className={s.chips}>
          {m.options.map((o) => (
            <button key={o} className={s.chip} onClick={() => onOption(o)}>
              {o}
            </button>
          ))}
        </div>
      )}
      {free && <span className={s.small}>Esta pergunta não gastou créditos.</span>}
    </div>
  );
}
