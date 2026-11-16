import { sitesDomain, type SiteView } from '../../api';
import { Modal } from '../../components/Modal';
import { Button } from '../../components/ui';

/** "Ligar domínio" from the prototype. Custom domains are not built yet, so it says so. */
export function DomainModal({ open, onClose, view, onPublish }: { open: boolean; onClose: () => void; view: SiteView; onPublish: () => void }) {
  const sub = view.site.subdomain;
  const canPublish = !sub && view.site.current_version > 0;
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Domínio próprio: em breve"
      description={<>Em breve vais poder ligar um domínio .co.mz ou .com a este site e pagá-lo com M-Pesa ou <span className="nowrap">e-Mola</span>, sem configurar nada.</>}
      footer={
        <>
          <Button variant={canPublish ? 'secondary' : 'primary'} onClick={onClose}>
            Fechar
          </Button>
          {canPublish && (
            <Button
              onClick={() => {
                onClose();
                onPublish();
              }}
            >
              Publicar grátis
            </Button>
          )}
        </>
      }
    >
      <p className="muted" style={{ fontSize: 'var(--text-md)', lineHeight: 1.55 }}>
        {sub ? (
          <>
            Até lá, o site está em <span className="mono">{sub}.{sitesDomain}</span>, e esse endereço continua a funcionar depois de ligares um domínio.
          </>
        ) : (
          <>Até lá, publica o site grátis num endereço .{sitesDomain}. Esse endereço continua a funcionar depois de ligares um domínio.</>
        )}
      </p>
    </Modal>
  );
}
