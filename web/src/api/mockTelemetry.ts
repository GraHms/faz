// Telemetry for the mock API: the same groupings and money rules as
// internal/product/telemetry.go, over invented but plausible token counts,
// so the admin pages can be designed without a backend.

import type { Money, Operation, StageTelemetry, UsageCall, UsageGroup, UsageTotals } from './types';

/** Per million tokens, as in internal/llm/cost.go. */
const prices: Record<string, { in: number; out: number; cacheWrite: number; cacheRead: number }> = {
  'claude-opus-5-5': { in: 4, out: 20, cacheWrite: 5, cacheRead: 0.2 },
  'claude-sonnet-5-5': { in: 2, out: 10, cacheWrite: 2.5, cacheRead: 0.1 },
  'claude-haiku-5-5': { in: 0.1, out: 0.5, cacheWrite: 0.125, cacheRead: 0.01 },
};

export const OPUS = 'claude-opus-5-5';
export const SONNET = 'claude-sonnet-5-5';
export const HAIKU = 'claude-haiku-5-5';

export function cost(model: string, t: { input: number; output: number; cacheRead?: number; cacheWrite?: number }): number {
  const p = prices[model];
  if (!p) return 0;
  return (t.input * p.in + t.output * p.out + (t.cacheRead ?? 0) * p.cacheRead + (t.cacheWrite ?? 0) * p.cacheWrite) / 1e6;
}

export interface CallPlan {
  stage: string;
  model: string;
  effort: string;
  input: number;
  output: number;
  cacheRead?: number;
  latency: number;
  error?: string;
}

/** The model calls of each operation, in pipeline order (internal/pipeline/profile.go, quality). */
export function plannedCalls(op: Operation, photos = 0): CallPlan[] {
  switch (op) {
    case 'questions':
      return [{ stage: 'intake', model: SONNET, effort: 'medium', input: 2400 + 1600 * photos, output: 720, latency: 6200 }];
    case 'build':
      return [
        { stage: 'intake', model: SONNET, effort: 'medium', input: 2700 + 1600 * photos, output: 780, latency: 6400 },
        { stage: 'content', model: SONNET, effort: 'medium', input: 1900, output: 1100, latency: 14000 },
        { stage: 'plan', model: OPUS, effort: 'high', input: 5300, output: 1500, latency: 31000 },
        { stage: 'critique', model: OPUS, effort: 'high', input: 6600, output: 1600, latency: 28000 },
        { stage: 'build', model: SONNET, effort: 'medium', input: 5800, output: 9400, latency: 72000 },
        { stage: 'review', model: OPUS, effort: 'medium', input: 9200, output: 900, latency: 21000 },
      ];
    case 'classify':
      return [{ stage: 'classify', model: HAIKU, effort: 'low', input: 1700, output: 90, latency: 1300 }];
    case 'edit':
      return [{ stage: 'edit', model: SONNET, effort: 'low', input: 1400, output: 1300, cacheRead: 9800, latency: 17000 }];
  }
}

const empty = (): UsageTotals => ({
  calls: 0,
  errors: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  cost_usd: 0,
  latency_ms: 0,
});

function add(t: UsageTotals, c: UsageTotals | UsageCall) {
  const one = 'attempt' in c;
  t.calls += one ? 1 : c.calls;
  t.errors += one ? (c.error ? 1 : 0) : c.errors;
  t.input_tokens += c.input_tokens;
  t.output_tokens += c.output_tokens;
  t.cache_read_tokens += c.cache_read_tokens;
  t.cache_write_tokens += c.cache_write_tokens;
  t.cost_usd += c.cost_usd;
  t.latency_ms += c.latency_ms;
}

const order: Record<string, number> = { questions: 0, build: 1, classify: 2, edit: 3 };
const round = (v: number, places: number) => Math.round(v * 10 ** places) / 10 ** places;

export const VAT_RATE = 0.16;
export const USD_TO_MZN = 64;

export function money(costUSD: number, charged: number, refunded: number, mznPerCredit: number, rate = USD_TO_MZN): Money {
  const net = charged - refunded;
  const creditsMZN = round(net * mznPerCredit, 2);
  const exVat = round(creditsMZN / (1 + VAT_RATE), 2);
  const costMZN = round(costUSD * rate, 2);
  const margin = round(exVat - costMZN, 2);
  return {
    usd_to_mzn: rate,
    cost_usd: round(costUSD, 6),
    cost_mzn: costMZN,
    credits_charged: charged,
    credits_refunded: refunded,
    credits_net: net,
    credits_mzn: creditsMZN,
    credits_mzn_ex_vat: exVat,
    margin_mzn: margin,
    margin_percent: exVat > 0 ? round((100 * margin) / exVat, 1) : 0,
  };
}

/** Groups calls like SiteTelemetry in Go: totals, by operation, by model, by stage. */
export function groupCalls(calls: UsageCall[]) {
  const totals = empty();
  const stages = new Map<string, StageTelemetry>();
  const ops = new Map<string, StageTelemetry>();
  const models = new Map<string, StageTelemetry>();
  const blank = (o: Partial<StageTelemetry>): StageTelemetry => ({ operation: '', stage: '', model: '', effort: '', retries: 0, ...empty(), ...o });
  for (const c of calls) {
    add(totals, c);
    const groups: [Map<string, StageTelemetry>, string, Partial<StageTelemetry>][] = [
      [stages, [c.operation, c.stage, c.model, c.effort].join('|'), { operation: c.operation, stage: c.stage, model: c.model, effort: c.effort }],
      [ops, c.operation, { operation: c.operation }],
      [models, c.model, { model: c.model }],
    ];
    for (const [m, k, init] of groups) {
      const g = m.get(k) ?? blank(init);
      add(g, c);
      if (c.attempt > 1) g.retries++;
      m.set(k, g);
    }
  }
  const byStage = [...stages.values()].sort((a, b) => (order[a.operation] ?? 9) - (order[b.operation] ?? 9));
  const byOperation = [...ops.values()].sort((a, b) => (order[a.operation] ?? 9) - (order[b.operation] ?? 9));
  const byModel = [...models.values()].sort((a, b) => b.cost_usd - a.cost_usd);
  totals.cost_usd = round(totals.cost_usd, 6);
  return { totals, byStage, byOperation, byModel };
}

/** Sums calls by operation, stage and model, costliest first, like UsageByStage. */
export function groupByStage(calls: UsageCall[]): UsageGroup[] {
  const m = new Map<string, UsageGroup>();
  for (const c of calls) {
    const k = [c.operation, c.stage, c.model].join('|');
    const g = m.get(k) ?? { operation: c.operation, stage: c.stage, model: c.model, ...empty() };
    add(g, c);
    m.set(k, g);
  }
  return [...m.values()].sort((a, b) => b.cost_usd - a.cost_usd);
}

export function sumTotals(groups: UsageTotals[]): UsageTotals {
  const t = empty();
  for (const g of groups) add(t, g);
  t.cost_usd = round(t.cost_usd, 6);
  return t;
}
