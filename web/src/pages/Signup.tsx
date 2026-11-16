import { useState, type FormEvent } from 'react';
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { api, isApiError } from '../api';
import { useSession } from '../auth/session';
import { AuthShell, ReadyPanel } from '../components/AuthShell';
import { Button, Field, Notice, PasswordField } from '../components/ui';
import { draft, invite as savedInvite } from '../lib/draft';
import s from './Auth.module.css';

export default function Signup() {
  const { token, signIn } = useSession();
  const nav = useNavigate();
  const d = draft.get();
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [params] = useSearchParams();
  const [code, setCode] = useState(() => (params.get('convite') ?? savedInvite.get()).trim().toUpperCase());
  const config = useQuery({ queryKey: ['config'], queryFn: api.config, staleTime: 60_000 });
  const required = config.data?.beta_required ?? false;
  const [showCode, setShowCode] = useState(!!code);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (token && !busy) return <Navigate to="/novo" replace />;

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    if (!name.trim()) next.name = 'Escreve o teu nome.';
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email.trim())) next.email = 'Escreve um email válido, por exemplo ana@gmail.com.';
    if (password.length < 8) next.password = 'Usa pelo menos 8 caracteres.';
    if (required && !code.trim()) next.invite = 'A Faz está em beta fechado: escreve o teu código de convite.';
    setErrors(next);
    setError(null);
    if (Object.keys(next).length) return;
    setBusy(true);
    try {
      const session = await api.signup({ name: name.trim(), email: email.trim(), password, invite_code: code.trim() || undefined });
      savedInvite.set('');
      signIn(session);
      nav('/novo', { replace: true, state: { fromSignup: true } });
    } catch (err) {
      setBusy(false);
      if (isApiError(err, 'email_taken')) setErrors({ email: err.message });
      else if (isApiError(err, 'weak_password')) setErrors({ password: err.message });
      else if (isApiError(err, 'invite_required') || isApiError(err, 'invite_invalid')) {
        setShowCode(true);
        setErrors({ invite: err.message });
      }
      else setError(isApiError(err) ? err.message : 'Não foi possível criar a conta. Tenta outra vez.');
    }
  }

  return (
    <AuthShell aside={<ReadyPanel prompt={d?.prompt} businessName={d?.businessName} />}>
      <div className={s.head}>
        <span className={s.step}>Passo 1 de 2</span>
        <h1 className={s.h1}>Cria a tua conta</h1>
        <p className={s.lead}>
          {required ? 'A Faz está em beta fechado: precisas de um código de convite.' : 'O primeiro site é por nossa conta. Não precisas de cartão.'}
        </p>
      </div>
      <form className={s.form} onSubmit={submit} noValidate>
        {error && <Notice>{error}</Notice>}
        <Field label="Nome" value={name} onChange={(e) => setName(e.target.value)} error={errors.name} autoComplete="name" autoFocus />
        <Field
          label="Email"
          type="email"
          inputMode="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          error={errors.email}
          autoComplete="email"
        />
        <PasswordField
          label="Palavra-passe"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={errors.password}
          hint="Pelo menos 8 caracteres."
          autoComplete="new-password"
        />
        {required || showCode ? (
          <Field
            label="Código de convite"
            value={code}
            onChange={(e) => setCode(e.target.value.toUpperCase())}
            error={errors.invite}
            hint={required ? 'A Faz está em beta fechado. Pede um código à equipa Faz.' : 'Alguns códigos trazem créditos extra.'}
            autoComplete="off"
            spellCheck={false}
            maxLength={40}
            className="mono"
          />
        ) : (
          <button type="button" className={s.linkBtn} onClick={() => setShowCode(true)}>
            Tenho um código de convite
          </button>
        )}
        <Button type="submit" size="lg" block loading={busy}>
          Criar conta
        </Button>
        <p className={s.terms}>Ao criar a conta aceitas os termos de utilização da Faz.</p>
      </form>
      <p className={s.switch}>
        Já tens conta? <Link to="/entrar">Entrar</Link>
      </p>
    </AuthShell>
  );
}
