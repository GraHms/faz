import { MarketingLayout } from '../components/MarketingLayout';
import { ButtonLink } from '../components/ui';

export default function NotFound() {
  return (
    <MarketingLayout>
      <section style={{ maxWidth: 560, margin: '0 auto', padding: '120px 24px', display: 'grid', gap: 16, justifyItems: 'start' }}>
        <h1 style={{ fontSize: 32, fontWeight: 600, letterSpacing: '-0.04em' }}>Esta página não existe</h1>
        <p className="muted">O endereço pode estar errado, ou a página mudou de sítio.</p>
        <ButtonLink to="/">Ir para o início</ButtonLink>
      </section>
    </MarketingLayout>
  );
}
