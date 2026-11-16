import { useState, type FormEvent } from 'react';
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { api, isApiError } from '../api';
import { safeBack, useSession } from '../auth/session';
import { AuthShell } from '../components/AuthShell';
import { Button, Field, Notice, PasswordField } from '../components/ui';
import { draft } from '../lib/draft';
import s from './Auth.module.css';

export default function Login() {
  const { token, signIn } = useSession();
  const nav = useNavigate();
  const [params] = useSearchParams();
  const fallback = draft.get()?.prompt ? '/novo' : '/painel';
  const back = safeBack(params.get('voltar'), fallback);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (token && !busy) return <Navigate to={back} replace />;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!email.trim() || !password) {
      setError('Escreve o email e a palavra-passe.');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      signIn(await api.login({ email: email.trim(), password }));
      nav(back, { replace: true });
    } catch (err) {
      setBusy(false);
      setError(isApiError(err) ? err.message : 'Não foi possível entrar. Tenta outra vez.');
    }
  }

  return (
    <AuthShell>
      <div className={s.head}>
        <h1 className={s.h1}>Entrar na Faz</h1>
        <p className={s.lead}>Continua onde ficaste nos teus sites.</p>
      </div>
      <form className={s.form} onSubmit={submit} noValidate>
        {error && <Notice>{error}</Notice>}
        <Field
          label="Email"
          type="email"
          inputMode="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoComplete="email"
          autoFocus
        />
        <PasswordField label="Palavra-passe" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        <Button type="submit" size="lg" block loading={busy}>
          Entrar
        </Button>
      </form>
      <p className={s.switch}>
        Ainda não tens conta? <Link to="/criar-conta">Criar conta</Link>
      </p>
    </AuthShell>
  );
}
