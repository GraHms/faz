import { useQuery } from '@tanstack/react-query';
import { api, type SiteView } from '../../api';
import { keys } from '../../hooks/queries';
import { Spinner } from '../../components/ui';
import s from './editor.module.css';

export type Device = 'desktop' | 'mobile';

/**
 * The generated page, loaded from its short lived R2 URL on another origin.
 * The sandbox has no allow-same-origin, so even a page served from this
 * origin (the mock's blob URLs) cannot reach the app or its token.
 */
export function Preview({ view, device }: { view: SiteView; device: Device }) {
  const version = view.site.current_version;
  const preview = useQuery({
    queryKey: keys.preview(view.site.id, version),
    queryFn: () => api.preview(view.site.id, version),
    enabled: version > 0,
    staleTime: 10 * 60_000,
    refetchInterval: 12 * 60_000,
  });
  const address = view.live_url?.replace(/^https?:\/\//, '') ?? 'pré-visualização';

  return (
    <div className={s.frame} style={{ width: device === 'mobile' ? 390 : '100%' }}>
      <div className={s.frameBar}>
        <span className={s.lights} aria-hidden>
          <span />
          <span />
          <span />
        </span>
        <span className={s.url}>
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" aria-hidden>
            <rect x="5" y="11" width="14" height="10" rx="2" />
            <path d="M8 11V7a4 4 0 018 0v4" />
          </svg>
          {address}
        </span>
        <span className={s.lightsSpacer} />
      </div>
      <div className={s.frameBody}>
        {version === 0 ? (
          <Skeleton building={view.site.status === 'building'} />
        ) : preview.data ? (
          <iframe
            key={preview.data.url}
            title={`Pré-visualização de ${view.site.business_name}`}
            src={preview.data.url}
            className={s.iframe}
            sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox allow-forms"
            referrerPolicy="no-referrer"
          />
        ) : preview.isError ? (
          <div className={s.frameMsg}>Não consegui abrir a pré-visualização. Actualiza a página.</div>
        ) : (
          <div className={s.frameMsg}>
            <Spinner label="A abrir a pré-visualização" />
          </div>
        )}
      </div>
    </div>
  );
}

/** Paper coloured placeholder blocks while the first version is built. */
function Skeleton({ building }: { building: boolean }) {
  return (
    <div className={building ? `${s.skeleton} ${s.skeletonOn}` : s.skeleton} aria-hidden>
      <div className={s.skRow}>
        <span style={{ width: 110 }} />
        <i />
        <span style={{ width: 'min(220px, 40%)' }} />
      </div>
      <div className={s.skHero}>
        <div>
          <span style={{ width: '40%', height: 12 }} />
          <span style={{ width: '92%', height: 52 }} />
          <span style={{ width: '70%', height: 52 }} />
          <span style={{ width: '80%', height: 14 }} />
          <span style={{ width: '60%', height: 14 }} />
        </div>
        <span className={s.skPhoto} />
      </div>
    </div>
  );
}
