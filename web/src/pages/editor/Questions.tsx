import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import type { Question } from '../../api';
import { Button } from '../../components/ui';
import { fmtPhone, parsePhone } from '../../lib/format';
import s from './editor.module.css';

/** The clarifying questions, one at a time, as in the prototype. */
export function Questions({
  questions,
  onDone,
  onSkipAll,
  busy,
  footnote,
}: {
  questions: Question[];
  onDone: (answers: Record<string, string>) => void;
  onSkipAll: () => void;
  busy?: boolean;
  footnote?: string;
}) {
  const [i, setI] = useState(0);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const card = useRef<HTMLDivElement>(null);
  const q = questions[i];
  const last = i === questions.length - 1;

  useEffect(() => {
    setError(null);
    card.current?.querySelector<HTMLElement>('input,textarea,[data-option]')?.focus({ preventScroll: true });
  }, [i]);

  if (!q) return null;
  const value = answers[q.id] ?? '';
  const set = (v: string) => {
    setError(null);
    setAnswers((a) => ({ ...a, [q.id]: v }));
  };

  function next(skip = false) {
    if (!q) return;
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
    if (last) onDone(a);
    else setI(i + 1);
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

  function toggle(v: string) {
    if (!q) return;
    if (q.kind === 'multi') {
      const cur = value ? value.split(',') : [];
      set(cur.includes(v) ? cur.filter((x) => x !== v).join(',') : [...cur, v].join(','));
    } else {
      set(value === v ? '' : v);
    }
  }

  return (
    <div className={s.aiBlock}>
      <FazLabel />
      <p>
        Antes de começar, tenho {questions.length === 1 ? 'uma pergunta rápida' : `${questions.length} perguntas rápidas`} para acertar no
        site.{' '}
        <button className={s.linkBtn} onClick={onSkipAll} disabled={busy}>
          Prefiro saltar
        </button>
      </p>
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
          {(q.kind === 'choice' || q.kind === 'multi') && q.options && (
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
          )}
          {q.kind === 'phone' && (
            <div className={error ? `${s.phone} ${s.phoneBad}` : s.phone}>
              <span>+258</span>
              <input
                type="tel"
                inputMode="tel"
                autoComplete="tel-national"
                placeholder="84 123 4567"
                aria-labelledby={`q-${q.id}`}
                aria-invalid={!!error}
                value={fmtPhone(value)}
                onChange={(e) => set(e.target.value.replace(/\D/g, '').slice(0, 9))}
              />
            </div>
          )}
          {q.kind === 'text' && (
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
                  next();
                }
              }}
            />
          )}
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
      {footnote && <p className={s.small}>{footnote}</p>}
    </div>
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
