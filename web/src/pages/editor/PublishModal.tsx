import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, isApiError, sitesDomain, type Published, type SiteView } from '../../api';
import { Modal } from '../../components/Modal';
import { Button, Dot, Notice, Spinner } from '../../components/ui';
import { keys } from '../../hooks/queries';
import { slugify, subdomainProblem } from '../../lib/format';
import s from './editor.module.css';

function useDebounced<T>(v: T, ms: number): T {
  const [d, setD] = useState(v);
  useEffect(() => {
    const t = setTimeout(() => setD(v), ms);
    return () => clearTimeout(t);
  }, [v, ms]);
  return d;
}

export function PublishModal({ open, onClose, view }: { open: boolean; onClose: () => void; view: SiteView }) {
  const qc = useQueryClient();
  const site = view.site;
  const [name, setName] = useState(site.subdomain ?? slugify(site.business_name).slice(0, 40));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<Published | null>(null);
  const [copied, setCopied] = useState(false);
  const debounced = useDebounced(name, 350);
  const local = subdomainProblem(debounced);

  // Reset each time the dialog opens, and only then: publishing changes
  // site.subdomain, which must not wipe the "No ar" state it produced.
  const initial = site.subdomain ?? slugify(site.business_name).slice(0, 40);
  const initialRef = useRef(initial);
  initialRef.current = initial;
  useEffect(() => {
    if (open) {
      setDone(null);
      setError(null);
      setCopied(false);
      setName(initialRef.current);
    }
  }, [open]);

  const check = useQuery({
    queryKey: ['subdomain', debounced, site.id],
    queryFn: () => api.availability(debounced, site.id),
    enabled: open && !done && !local && debounced.length > 0,
    staleTime: 10_000,
  });
  const settled = debounced === name;
  const available = settled && !local && check.data?.available === true;
  const republish = !!site.subdomain && name === site.subdomain;

  async function publish() {
    setBusy(true);
    setError(null);
    try {
      const r = await api.publish(site.id, name);
      setDone(r);
      qc.invalidateQueries({ queryKey: keys.site(site.id) });
      qc.invalidateQueries({ queryKey: keys.sites });
    } catch (e) {
      setError(isApiError(e) ? e.message : 'Não foi possível publicar. Tenta outra vez.');
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return (
      <Modal
        open={open}
        onClose={onClose}
        title={
          <span className={s.liveTitle}>
            <Dot tone="green" /> No ar
          </span>
        }
        description={`O teu site foi publicado. Qualquer pessoa já pode abrir o site de ${site.business_name}.`}
        footer={<Button onClick={onClose}>Fechar</Button>}
      >
        <div className={s.liveRow}>
          <a href={done.url} target="_blank" rel="noopener noreferrer" className="mono">
            {done.url.replace(/^https?:\/\//, '')}
          </a>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              void navigator.clipboard?.writeText(done.url).then(() => setCopied(true));
            }}
          >
            {copied ? 'Copiado' : 'Copiar'}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setDone(null)}>
            Mudar
          </Button>
        </div>
      </Modal>
    );
  }

  let status: ReactNode = null;
  if (!name) status = <span className="dim">Escreve um endereço.</span>;
  else if (!settled || check.isFetching) status = <><Spinner /> A verificar…</>;
  else if (local) status = <span className={s.bad}>{local}</span>;
  else if (check.isError) status = <span className={s.bad}>Não consegui verificar. Tenta outra vez.</span>;
  else if (check.data && !check.data.available) status = <span className={s.bad}>{check.data.reason}</span>;
  else if (available) status = <span className={s.good}><Dot tone="green" /> {republish ? 'É o endereço actual deste site' : 'Disponível'}</span>;

  const alternatives =
    check.data && !check.data.available && settled
      ? [`${name}-mz`, `${name}-maputo`, `o-${name}`].filter((a) => a.length <= 40 && !subdomainProblem(a))
      : [];

  return (
    <Modal
      open={open}
      onClose={onClose}
      locked={busy}
      title={site.subdomain ? 'Publicar alterações' : 'Escolhe o endereço do site'}
      description="É grátis e podes mudar depois. Usa letras, números e hífens."
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            Cancelar
          </Button>
          <Button onClick={publish} loading={busy} disabled={!available}>
            Publicar
          </Button>
        </>
      }
    >
      <div className={s.publish}>
        {error && <Notice>{error}</Notice>}
        <label htmlFor="subdomain" className="sr-only">
          Endereço
        </label>
        <div className={s.subdomain}>
          <input
            id="subdomain"
            data-autofocus
            value={name}
            onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '').slice(0, 40))}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && available && !busy) void publish();
            }}
            spellCheck={false}
            autoCapitalize="off"
            autoComplete="off"
          />
          <span>.{sitesDomain}</span>
        </div>
        <p className={s.subStatus} aria-live="polite">
          {status}
        </p>
        {alternatives.length > 0 && (
          <div className={s.alts}>
            <span className="dim">Experimenta:</span>
            {alternatives.map((a) => (
              <button key={a} className={s.chip} onClick={() => setName(a)}>
                {a}
              </button>
            ))}
          </div>
        )}
        <div className={s.ownDomain}>
          <span>Domínio próprio, como {name || 'onegocio'}.co.mz</span>
          <span className={s.soon}>
            <Dot tone="amber" /> em breve
          </span>
        </div>
      </div>
    </Modal>
  );
}
