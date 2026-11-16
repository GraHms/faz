import { useRef } from 'react';
import type { Job } from '../../api';
import { Spinner } from '../../components/ui';
import { buildSteps, editSteps, stepIndex } from '../../lib/status';
import s from './editor.module.css';

/**
 * Real progress from the job's recorded stage. An edit verifies the page
 * before and after the change, so "verify" only counts once "edit" was seen.
 */
export function Progress({ job, kind }: { job: Job | undefined; kind: 'build' | 'edit' }) {
  const steps = kind === 'edit' ? editSteps : buildSteps;
  const furthest = useRef(-1);
  const sawEdit = useRef(false);
  if (job?.progress === 'edit') sawEdit.current = true;
  let at = stepIndex(steps, job?.progress);
  if (kind === 'edit' && !sawEdit.current) at = job?.status === 'running' ? 0 : -1;
  furthest.current = Math.max(furthest.current, at);
  const done = job?.status === 'succeeded';
  const current = done ? steps.length : furthest.current;
  const label = done ? 'Concluído' : !job || job.status === 'queued' ? 'Na fila' : kind === 'edit' ? 'A alterar' : 'A construir';

  return (
    <div className={s.progress} role="status" aria-live="polite">
      <div className={s.progressHead}>
        <span className={s.progressLabel}>
          <span className={done ? s.dotGreen : s.dotAmber} aria-hidden />
          {label}
        </span>
        <span className={s.progressCount}>
          {Math.max(0, Math.min(current, steps.length))}/{steps.length}
        </span>
      </div>
      <ol className={s.steps}>
        {steps.map((st, k) => {
          const state = k < current ? 'done' : k === current ? 'active' : 'pending';
          return (
            <li key={st.label} className={state === 'pending' ? s.stepPending : undefined} aria-current={state === 'active' ? 'step' : undefined}>
              {state === 'done' ? (
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--green)" strokeWidth="2.6" aria-hidden>
                  <path d="M5 12l5 5L20 7" />
                </svg>
              ) : state === 'active' ? (
                <Spinner />
              ) : (
                <span className={s.pendingDot} aria-hidden>
                  <span />
                </span>
              )}
              <span>{st.label}</span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
