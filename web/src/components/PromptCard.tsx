import { useRef, useState, type KeyboardEvent } from 'react';
import { imageAccept, pickImages } from '../lib/draft';
import { Dot, Spinner } from './ui';
import s from './PromptCard.module.css';

export interface PromptCardProps {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  placeholder?: string;
  files?: File[];
  onFiles?: (f: File[]) => void;
  busy?: boolean;
  label?: string;
  autoFocus?: boolean;
}

export const promptPlaceholder =
  'Um site para a minha marisqueira na Costa do Sol, com menu em meticais, fotos dos pratos e reservas pelo WhatsApp…';

/** The big "describe your site" box from the prototype's hero. */
export function PromptCard({
  value,
  onChange,
  onSubmit,
  placeholder = promptPlaceholder,
  files,
  onFiles,
  busy,
  label = 'Descreve o site que queres',
  autoFocus,
}: PromptCardProps) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const ready = value.trim().length >= 10;

  function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      if (ready && !busy) onSubmit();
    }
  }

  return (
    <form
      className={s.card}
      onSubmit={(e) => {
        e.preventDefault();
        if (ready && !busy) onSubmit();
      }}
    >
      <label className="sr-only" htmlFor="prompt">
        {label}
      </label>
      <textarea
        id="prompt"
        className={s.text}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKey}
        placeholder={placeholder}
        maxLength={4000}
        autoFocus={autoFocus}
      />
      {files && files.length > 0 && (
        <ul className={s.files} aria-label="Fotos anexadas">
          {files.map((f) => (
            <li key={f.name + f.size}>
              {f.name}
              <button type="button" aria-label={`Tirar ${f.name}`} onClick={() => onFiles?.(files.filter((x) => x !== f))}>
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      {fileError && (
        <p className={s.fileError} role="alert">
          {fileError}
        </p>
      )}
      <div className={s.bar}>
        {onFiles && (
          <>
            <button type="button" className={s.icon} title="Anexar fotos" aria-label="Anexar fotos" onClick={() => fileInput.current?.click()}>
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
                <path d="M21 11.5l-8.6 8.6a5 5 0 01-7.1-7.1l8.6-8.6a3.3 3.3 0 014.7 4.7l-8.6 8.6a1.7 1.7 0 01-2.4-2.4l7.9-7.9" />
              </svg>
            </button>
            <input
              ref={fileInput}
              type="file"
              accept={imageAccept}
              multiple
              hidden
              onChange={(e) => {
                if (!e.target.files) return;
                const { ok, rejected } = pickImages(e.target.files);
                setFileError(rejected);
                onFiles([...(files ?? []), ...ok].slice(0, 12));
                e.target.value = '';
              }}
            />
          </>
        )}
        <span className={s.chip}>
          <Dot tone="green" />
          Português
        </span>
        <span className={s.grow} />
        <span className={s.kbd}>
          <kbd>↵</kbd>para construir
        </span>
        <button type="submit" className={s.send} disabled={!ready || busy} aria-label="Construir">
          {busy ? (
            <Spinner onLight />
          ) : (
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="M12 19V5M6 11l6-6 6 6" />
            </svg>
          )}
        </button>
      </div>
    </form>
  );
}
