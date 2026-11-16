import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import type { Question } from '../../api';
import { Button } from '../../components/ui';
import { fmtPhone, parsePhone } from '../../lib/format';
import s from './editor.module.css';

export interface QuestionsProps {
  questions: Question[];
  /** What the intake understood; the owner can correct it before paying. */
  understood?: string;
  /** Answers kept from a build that failed, as form values. */
  initial?: Record<string, string>;
  initialCorrection?: string;
  onDone: (answers: Record<string, string>, correction: string) => void;
  onSkipAll: (correction: string) => void;
  busy?: boolean;
  footnote?: string;
}

/** "O que percebi", then the clarifying questions one at a time. */
export function Questions({ questions, understood, initial, initialCorrection = '', onDone, onSkipAll, busy, footnote }: QuestionsProps) {
  const [i, setI] = useState(0);
  const [answers, setAnswers] = useState<Record<string, string>>(() => ({ ...initial }));
  const [correction, setCorrection] = useState(initialCorrection);
  const [error, setError] = useState<string | null>(null);
  const card = useRef<HTMLDivElement>(null);
  const started = useRef(false);
  const q = questions[i];
  const last = i === questions.length - 1;
  const retry = Object.keys(initial ?? {}).length > 0 || !!initialCorrection;

  useEffect(() => {
    setError(null);
    // Focus follows the owner through the questions, not on first render,
    // so the summary above is read first.
    if (started.current) card.current?.querySelector<HTMLElement>('input,textarea,[data-option]')?.focus({ preventScroll: true });
    started.current = true;
  }, [i]);

  const set = (v: string) => {
    if (!q) return;
    setError(null);
    setAnswers((a) => ({ ...a, [q.id]: v }));
  };

  function next(skip = false) {
    if (!q) return;
    const value = answers[q.id] ?? '';
    let a = answers;
    if (skip) {
      a = { ...answers };
      delete a[q.id];
      setAnswers(a);
    } else if (q.kind === 'phone' && value.trim()) {
      const p = parsePhone(value);
      if (!p.ok) {
        setError(p.error);
        return;
      }
      a = { ...answers, [q.id]: p.national };
      setAnswers(a);
    }
    if (last) onDone(a, correction.trim());
    else setI(i + 1);
  }

  function toggle(v: string) {
    if (!q) return;
    const value = answers[q.id] ?? '';
    if (q.kind === 'multi') {
      const cur = value ? value.split(',') : [];
      set(cur.includes(v) ? cur.filter((x) => x !== v).join(',') : [...cur, v].join(','));
    } else {
      set(value === v ? '' : v);
    }
  }

  function onKey(e: KeyboardEvent) {
    if (!q || busy) return;
    if ((q.kind === 'choice' || q.kind === 'multi') && /^[1-9]$/.test(e.key) && !(e.target instanceof HTMLInputElement)) {
      const o = q.options?.[Number(e.key) - 1];
      if (o) {
        e.preventDefault();
        toggle(o.value);
      }
    }
    if (e.key === 'Enter' && !e.shiftKey && !(e.target instanceof HTMLTextAreaElement) && !(e.target instanceof HTMLButtonElement)) {
      e.preventDefault();
      next();
    }
  }

  const intro = retry
    ? 'Guardei as tuas respostas. Podes construir já, ou mudar alguma antes.'
    : questions.length === 0
      ? 'Já tenho o que preciso para começar.'
      : `Antes de começar, tenho ${questions.length === 1 ? 'uma pergunta rápida' : `${questions.length} perguntas rápidas`} para acertar no site.`;

  return (
    <div className={s.aiBlock}>
      <FazLabel />
      {understood && <Understood text={understood} correction={correction} onCorrection={setCorrection} busy={busy} />}
      <p>
        {intro}{' '}
        {retry ? (
          <button className={s.linkBtn} onClick={() => onDone(answers, correction.trim())} disabled={busy}>
            Construir com estas respostas
          </button>
        ) : (
          questions.length > 0 && (
            <button className={s.linkBtn} onClick={() => onSkipAll(correction.trim())} disabled={busy}>
              Prefiro saltar
            </button>
          )
        )}
      </p>
      {q ? (
        <div className={s.qCard} ref={card} onKeyDown={onKey}>
          <div className={s.qHead}>
            <span className={s.qCount}>
              Pergunta {i + 1} de {questions.length}
            </span>
            <div className={s.qSegs} aria-hidden>
              {questions.map((x, k) => (
                <span key={x.id} className={k <= i ? s.segOn : undefined} />
              ))}
            </div>
            <button className={s.linkBtn} onClick={() => next(true)} disabled={busy}>
              Saltar
            </button>
          </div>
          <div className={s.qBody}>
            <div className={s.qText}>
              <span id={`q-${q.id}`}>{q.text}</span>
              {q.hint && <span className={s.qHint}>{q.hint}</span>}
            </div>
            <Answer q={q} value={answers[q.id] ?? ''} set={set} toggle={toggle} invalid={!!error} onEnter={() => next()} />
            {error && (
              <span className={s.qError} role="alert">
                {error}
              </span>
            )}
          </div>
          <div className={s.qFoot}>
            <span className={s.qFree}>Responder não gasta créditos</span>
            {i > 0 && (
              <Button variant="secondary" size="sm" onClick={() => setI(i - 1)} disabled={busy}>
                Voltar
              </Button>
            )}
            <Button size="sm" onClick={() => next()} loading={busy && last}>
              {last ? 'Construir site' : 'Continuar'}
            </Button>
          </div>
        </div>
      ) : (
        <div>
          <Button size="sm" loading={busy} onClick={() => onSkipAll(correction.trim())}>
            Construir site
          </Button>
        </div>
      )}
      {footnote && <p className={s.small}>{footnote}</p>}
    </div>
  );
}

function Understood({ text, correction, onCorrection, busy }: { text: string; correction: string; onCorrection: (v: string) => void; busy?: boolean }) {
  const [open, setOpen] = useState(!!correction);
  return (
    <div className={s.understood}>
      <div className={s.understoodHead}>
        <span>O que percebi</span>
        {!open && (
          <button className={s.linkBtn} onClick={() => setOpen(true)} disabled={busy}>
            Corrigir
          </button>
        )}
      </div>
      <p className={s.understoodText}>{text}</p>
      {open && (
        <div className={s.correction}>
          <label htmlFor="correction" className={s.qHint}>
            O que está errado ou falta? Vale mais do que o resumo acima.
          </label>
          <textarea
            id="correction"
            className={s.qInput}
            rows={2}
            maxLength={1000}
            value={correction}
            autoFocus={!correction}
            placeholder="Por exemplo: também vendemos a revendedores, não só a famílias."
            onChange={(e) => onCorrection(e.target.value)}
          />
          {!correction && (
            <button className={s.linkBtn} onClick={() => setOpen(false)}>
              Está certo, afinal
            </button>
          )}
        </div>
      )}
    </div>
  );
}

function Answer({
  q,
  value,
  set,
  toggle,
  invalid,
  onEnter,
}: {
  q: Question;
  value: string;
  set: (v: string) => void;
  toggle: (v: string) => void;
  invalid: boolean;
  onEnter: () => void;
}) {
  if ((q.kind === 'choice' || q.kind === 'multi') && q.options) {
    return (
      <div className={s.options} role={q.kind === 'multi' ? 'group' : 'radiogroup'} aria-labelledby={`q-${q.id}`}>
        {q.options.map((o, k) => {
          const on = q.kind === 'multi' ? value.split(',').includes(o.value) : value === o.value;
          return (
            <button
              key={o.value}
              data-option
              role={q.kind === 'multi' ? 'checkbox' : 'radio'}
              aria-checked={on}
              className={on ? `${s.option} ${s.optionOn}` : s.option}
              onClick={() => toggle(o.value)}
            >
              <span className={q.kind === 'multi' ? s.check : s.radio} aria-hidden />
              <span className={s.optionLabel}>{o.label}</span>
              {k < 9 && <kbd className={s.key}>{k + 1}</kbd>}
            </button>
          );
        })}
      </div>
    );
  }
  if (q.kind === 'phone') {
    return (
      <div className={invalid ? `${s.phone} ${s.phoneBad}` : s.phone}>
        <span>+258</span>
        <input
          type="tel"
          inputMode="tel"
          autoComplete="tel-national"
          placeholder="84 123 4567"
          aria-labelledby={`q-${q.id}`}
          aria-invalid={invalid}
          value={fmtPhone(value)}
          onChange={(e) => set(e.target.value.replace(/\D/g, '').slice(0, 9))}
        />
      </div>
    );
  }
  return (
    <textarea
      className={s.qInput}
      rows={2}
      maxLength={500}
      aria-labelledby={`q-${q.id}`}
      value={value}
      onChange={(e) => set(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          onEnter();
        }
      }}
    />
  );
}

export function FazLabel() {
  return (
    <div className={s.faz}>
      <span className={s.fazMark} aria-hidden />
      Faz
    </div>
  );
}
