import { useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, isApiError, sitesDomain, type AdminSiteRow } from '../../api';
import { Button, Notice, Pill, Spinner } from '../../components/ui';
import { since } from '../../lib/format';
import { siteStatus } from '../../lib/status';
import { ActionDialog, AdminPage, useAdmin, useRefusal } from './shell';
import s from './admin.module.css';

export function AdminSites() {
  return (
    <AdminPage>
      <SitesPage />
    </AdminPage>
  );
}

function SitesPage() {
  const { auth } = useAdmin();
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const q = params.get('q') ?? '';
  const [text, setText] = useState(q);
  const [target, setTarget] = useState<{ site: AdminSiteRow; restore: boolean } | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const sites = useQuery({ queryKey: ['admin', 'sites', q], queryFn: () => api.adminSites(auth, q), retry: false });
  if (useRefusal(sites.error)) return null;

  return (
    <>
      <div className={s.head}>
        <div>
          <h1 className={s.h1}>Sites</h1>
          <p className="muted">Retira do ar o que for burla, cópia de marca ou conteúdo proibido. O endereço fica preso ao site.</p>
        </div>
      </div>
      <form
        className={s.search}
        onSubmit={(e) => {
          e.preventDefault();
          setParams(text.trim() ? { q: text.trim() } : {});
        }}
      >
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder={`Nome do negócio ou endereço (nome.${sitesDomain})`} aria-label="Procurar sites" />
        <Button type="submit" size="sm">
          Procurar
        </Button>
      </form>
      {done && <Notice tone="info">{done}</Notice>}
      {sites.isPending ? (
        <div className={s.loading}>
          <Spinner label="A procurar" />
        </div>
      ) : sites.isError ? (
        <Notice>{isApiError(sites.error) ? sites.error.message : 'A procura falhou.'}</Notice>
      ) : sites.data.length === 0 ? (
        <p className={s.empty}>Nenhum site encontrado{q ? ` para “${q}”` : ''}.</p>
      ) : (
        <div className={s.tableWrap}>
          <table className={s.table}>
            <thead>
              <tr>
                <th>Site</th>
                <th>Endereço</th>
                <th>Dono</th>
                <th>Estado</th>
                <th>Editado</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {sites.data.map((x) => {
                const st = siteStatus(x);
                return (
                  <tr key={x.id}>
                    <td>
                      <Link to={`/admin/sites/${x.id}`}>{x.business_name}</Link>
                    </td>
                    <td className="mono">
                      {x.subdomain ? (
                        x.taken_down_at ? (
                          `${x.subdomain}.${sitesDomain}`
                        ) : (
                          <a href={`https://${x.subdomain}.${sitesDomain}`} target="_blank" rel="noopener noreferrer">
                            {x.subdomain}.{sitesDomain}
                          </a>
                        )
                      ) : (
                        ''
                      )}
                    </td>
                    <td className={s.dim}>{x.owner_email}</td>
                    <td>
                      <Pill tone={st.tone}>{st.label}</Pill>
                    </td>
                    <td className={s.dim}>{since(x.updated_at)}</td>
                    <td>
                      {x.taken_down_at ? (
                        <Button size="sm" variant="secondary" onClick={() => setTarget({ site: x, restore: true })}>
                          Repor
                        </Button>
                      ) : (
                        <Button size="sm" variant="danger" onClick={() => setTarget({ site: x, restore: false })}>
                          Retirar do ar
                        </Button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <ActionDialog
        open={!!target && !target.restore}
        onClose={() => setTarget(null)}
        title={`Retirar ${target?.site.business_name ?? ''} do ar`}
        description="O site deixa de abrir já, o dono vê o motivo no editor e não pode voltar a publicar até o repores. Escreve um motivo que possas explicar ao dono."
        confirm="Retirar do ar"
        danger
        onConfirm={async (reason) => {
          if (!target) return;
          await api.adminTakeDown(auth, target.site.id, reason);
          setDone(`${target.site.business_name} foi retirado do ar.`);
          await qc.invalidateQueries({ queryKey: ['admin'] });
        }}
      />
      <ActionDialog
        open={!!target && target.restore}
        onClose={() => setTarget(null)}
        title={`Repor ${target?.site.business_name ?? ''}`}
        description="A última versão publicada volta ao ar no mesmo endereço, e o dono pode voltar a publicar."
        confirm="Repor"
        onConfirm={async (reason) => {
          if (!target) return;
          await api.adminRestore(auth, target.site.id, reason);
          setDone(`${target.site.business_name} está de volta.`);
          await qc.invalidateQueries({ queryKey: ['admin'] });
        }}
      />
    </>
  );
}
