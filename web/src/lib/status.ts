import type { DotTone } from '../components/ui';
import type { Site } from '../api';

export interface StatusLabel {
  label: string;
  tone: DotTone;
  pulse?: boolean;
}

/** How a site's state reads in a pill, on the dashboard and in the editor. */
export function siteStatus(site: Pick<Site, 'status' | 'current_version' | 'published_version' | 'taken_down_at'>): StatusLabel {
  if (site.taken_down_at) return { label: 'Retirado do ar', tone: 'red' };
  switch (site.status) {
    case 'questions':
      return { label: 'À espera de respostas', tone: 'dim' };
    case 'building':
      return { label: 'A construir', tone: 'amber', pulse: true };
    case 'editing':
      return { label: 'A alterar', tone: 'amber', pulse: true };
    case 'failed':
      return { label: 'Falhou', tone: 'red' };
    default:
      if (!site.published_version) return { label: 'Rascunho', tone: 'dim' };
      if (site.current_version > site.published_version) return { label: 'No ar · por publicar', tone: 'green' };
      return { label: 'No ar', tone: 'green' };
  }
}

/** The steps the editor shows while a build runs, keyed by pipeline stage. */
export const buildSteps: { label: string; stages: string[] }[] = [
  { label: 'Textos e informação do negócio', stages: ['intake', 'content'] },
  { label: 'Estilo, cores e estrutura', stages: ['plan', 'critique'] },
  { label: 'Montar a página', stages: ['build'] },
  { label: 'Verificar no telemóvel e no computador', stages: ['verify', 'review', 'repair'] },
];

export const editSteps: { label: string; stages: string[] }[] = [
  { label: 'Aplicar a alteração', stages: ['edit'] },
  { label: 'Verificar a página', stages: ['verify', 'repair'] },
];

/** Index of the current step, -1 before the first stage is reported. */
export function stepIndex(steps: { stages: string[] }[], progress?: string): number {
  if (!progress) return -1;
  return steps.findIndex((s) => s.stages.includes(progress));
}
