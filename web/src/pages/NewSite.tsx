import { useRef, useState, type FormEvent } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { api, isApiError, sitesDomain } from '../api';
import { AuthShell, ReadyPanel } from '../components/AuthShell';
import { Button, Field, Notice, Spinner, TextArea } from '../components/ui';
import { draft, imageAccept, pickImages } from '../lib/draft';
import { businessTypes } from '../lib/examples';
import { slugify } from '../lib/format';
import { keys } from '../hooks/queries';
import s from './Auth.module.css';

/** "Sobre o teu negócio": the last step before the questions. */
export default function NewSite() {
  const d = draft.get();
  const nav = useNavigate();
  const qc = useQueryClient();
  const fromSignup = (useLocation().state as { fromSignup?: boolean } | null)?.fromSignup;
  const [name, setName] = useState(d?.businessName ?? '');
  const [type, setType] = useState(d?.businessType ?? '');
  const [prompt, setPrompt] = useState(d?.prompt ?? '');
  const [files, setFiles] = useState<File[]>(draft.files);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [stage, setStage] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const next: Record<string, string> = {};
    if (!name.trim()) next.name = 'Escreve o nome do negócio.';
    if (prompt.trim().length < 10) next.prompt = 'Conta um pouco mais: o que vendes, onde, e como os clientes te contactam.';
    setErrors(next);
    setError(null);
    if (Object.keys(next).length) return;

    setStage('A ler o teu pedido…');
    try {
      const v = await api.createSite({ prompt: prompt.trim(), business_name: name.trim(), business_type: type });
      let failed = 0;
      for (const [i, f] of files.entries()) {
        setStage(`A enviar fotos, ${i + 1} de ${files.length}…`);
        try {
          await api.uploadAsset(v.site.id, f);
        } catch {
          failed++;
        }
      }
      draft.clear();
      qc.setQueryData(keys.site(v.site.id), v);
      qc.invalidateQueries({ queryKey: keys.sites });
      nav(`/sites/${v.site.id}`, { state: failed ? { uploadFailed: failed } : undefined });
    } catch (err) {
      setStage(null);
      if (isApiError(err, 'invalid_prompt')) setErrors({ prompt: err.message });
      else if (isApiError(err, 'invalid_business_name')) setErrors({ name: err.message });
      else setError(isApiError(err) ? err.message : 'Não foi possível começar o site. Tenta outra vez.');
    }
  }

  return (
    <AuthShell aside={<ReadyPanel prompt={prompt.trim()} businessName={name} />}>
      <div className={s.head}>
        {fromSignup && <span className={s.step}>Passo 2 de 2</span>}
        <h1 className={s.h1}>Sobre o teu negócio</h1>
        <p className={s.lead}>A seguir faço-te até 3 perguntas rápidas e construo o site.</p>
      </div>
      <form className={s.form} onSubmit={submit} noValidate>
        {error && <Notice>{error}</Notice>}
        <Field
          label="Nome do negócio"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={errors.name}
          maxLength={80}
          autoComplete="organization"
          autoFocus={!name}
        />
        <p className={s.address}>
          Endereço grátis: <span className="mono">{slugify(name) || 'o-teu-negocio'}.{sitesDomain}</span>
        </p>
        <fieldset className={s.types}>
          <legend>Tipo de negócio</legend>
          {businessTypes.map((t) => (
            <label key={t} className={s.type}>
              <input type="radio" name="type" value={t} checked={type === t} onChange={() => setType(t)} />
              <span>{t}</span>
            </label>
          ))}
        </fieldset>
        <TextArea
          label="O que queres no site"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          error={errors.prompt}
          maxLength={4000}
          rows={4}
          placeholder="Menu com preços em meticais, fotos dos pratos e reservas pelo WhatsApp…"
          autoFocus={!!name && !prompt}
        />
        <div className={s.files}>
          <Button variant="secondary" size="sm" onClick={() => fileInput.current?.click()}>
            Adicionar fotos
          </Button>
          <span>{files.length ? `${files.length} ${files.length === 1 ? 'foto' : 'fotos'}` : 'Opcional: logótipo, espaço, produtos.'}</span>
          {files.length > 0 && (
            <Button variant="ghost" size="sm" onClick={() => setFiles([])}>
              Tirar
            </Button>
          )}
          <input
            ref={fileInput}
            type="file"
            accept={imageAccept}
            multiple
            hidden
            onChange={(e) => {
              if (!e.target.files) return;
              const { ok, rejected } = pickImages(e.target.files);
              setError(rejected);
              setFiles((f) => [...f, ...ok].slice(0, 12));
              e.target.value = '';
            }}
          />
        </div>
        {stage ? (
          <p className={s.waiting} role="status">
            <Spinner /> {stage}
          </p>
        ) : (
          <Button type="submit" size="lg" block>
            Construir o meu site
          </Button>
        )}
      </form>
      {!fromSignup && (
        <p className={s.switch}>
          <Link to="/painel">Voltar ao painel</Link>
        </p>
      )}
    </AuthShell>
  );
}
