import {
  forwardRef,
  useId,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type TextareaHTMLAttributes,
} from 'react';
import { Link, type LinkProps } from 'react-router-dom';
import s from './ui.module.css';

const cx = (...c: (string | false | null | undefined)[]) => c.filter(Boolean).join(' ');

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
type Size = 'sm' | 'md' | 'lg';

function buttonClass(variant: Variant, size: Size, block?: boolean, extra?: string) {
  return cx(s.btn, s[variant], size !== 'md' && s[size], block && s.block, extra);
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  block?: boolean;
  loading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'primary', size = 'md', block, loading, className, children, disabled, type = 'button', ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={buttonClass(variant, size, block, className)}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...rest}
    >
      {loading && <Spinner onLight={variant === 'primary'} />}
      {children}
    </button>
  );
});

export function ButtonLink({
  variant = 'primary',
  size = 'md',
  block,
  className,
  ...rest
}: LinkProps & { variant?: Variant; size?: Size; block?: boolean }) {
  return <Link className={buttonClass(variant, size, block, className)} {...rest} />;
}

export function Spinner({ onLight, label }: { onLight?: boolean; label?: string }) {
  return <span className={cx(s.spinner, onLight && s.onLight)} role={label ? 'status' : undefined} aria-label={label} />;
}

export function Logo({ to = '/', tld = true }: { to?: string; tld?: boolean }) {
  return (
    <Link to={to} className={s.logo} aria-label="Faz, página inicial">
      <span className={s.mark} aria-hidden />
      <span className={s.word}>
        faz{tld && <span className={s.tld}>.co.mz</span>}
      </span>
    </Link>
  );
}

interface FieldShell {
  label: ReactNode;
  hint?: ReactNode;
  error?: string | null;
  aside?: ReactNode;
}

function Shell({ id, label, hint, error, aside, children }: FieldShell & { id: string; children: ReactNode }) {
  return (
    <div className={s.field}>
      <div className={s.labelRow}>
        <label className={s.label} htmlFor={id}>
          {label}
        </label>
        {aside}
      </div>
      {children}
      {error ? (
        <span className={s.error} id={`${id}-msg`} role="alert">
          {error}
        </span>
      ) : hint ? (
        <span className={s.hint} id={`${id}-msg`}>
          {hint}
        </span>
      ) : null}
    </div>
  );
}

export type FieldProps = FieldShell &
  InputHTMLAttributes<HTMLInputElement> & {
    prefix?: ReactNode;
    suffix?: ReactNode;
  };

export const Field = forwardRef<HTMLInputElement, FieldProps>(function Field(
  { label, hint, error, aside, prefix, suffix, id, className, ...rest },
  ref,
) {
  const auto = useId();
  const fid = id ?? auto;
  const input = (
    <input
      ref={ref}
      id={fid}
      className={cx(s.input, className)}
      aria-invalid={error ? true : undefined}
      aria-describedby={error || hint ? `${fid}-msg` : undefined}
      {...rest}
    />
  );
  return (
    <Shell id={fid} label={label} hint={hint} error={error} aside={aside}>
      {prefix || suffix ? (
        <div className={s.affix}>
          {prefix && <span className={s.affixText}>{prefix}</span>}
          {input}
          {suffix && (typeof suffix === 'string' ? <span className={s.affixText}>{suffix}</span> : suffix)}
        </div>
      ) : (
        input
      )}
    </Shell>
  );
});

export function PasswordField(props: Omit<FieldProps, 'type' | 'suffix'>) {
  const [shown, setShown] = useState(false);
  return (
    <Field
      {...props}
      type={shown ? 'text' : 'password'}
      suffix={
        <button type="button" className={s.reveal} onClick={() => setShown((v) => !v)} aria-pressed={shown}>
          {shown ? 'Esconder' : 'Mostrar'}
        </button>
      }
    />
  );
}

export const TextArea = forwardRef<HTMLTextAreaElement, FieldShell & TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function TextArea({ label, hint, error, aside, id, className, ...rest }, ref) {
    const auto = useId();
    const fid = id ?? auto;
    return (
      <Shell id={fid} label={label} hint={hint} error={error} aside={aside}>
        <textarea
          ref={ref}
          id={fid}
          className={cx(s.input, className)}
          aria-invalid={error ? true : undefined}
          aria-describedby={error || hint ? `${fid}-msg` : undefined}
          {...rest}
        />
      </Shell>
    );
  },
);

export function Notice({ tone = 'error', children }: { tone?: 'error' | 'info'; children: ReactNode }) {
  return (
    <div className={cx(s.notice, tone === 'error' ? s.noticeError : s.noticeInfo)} role={tone === 'error' ? 'alert' : 'status'}>
      {children}
    </div>
  );
}

export type DotTone = 'green' | 'amber' | 'red' | 'dim';

export function Dot({ tone = 'dim', pulse }: { tone?: DotTone; pulse?: boolean }) {
  const t = { green: s.dotGreen, amber: s.dotAmber, red: s.dotRed, dim: undefined }[tone];
  return <span className={cx(s.dot, t, pulse && s.pulse)} aria-hidden />;
}

export function Pill({ tone, pulse, children }: { tone?: DotTone; pulse?: boolean; children: ReactNode }) {
  return (
    <span className={s.pill}>
      <Dot tone={tone} pulse={pulse} />
      {children}
    </span>
  );
}

export { cx };
