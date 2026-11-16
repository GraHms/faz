// Formatting for the telemetry pages.

import { fmt } from './format';

export const operationLabel: Record<string, string> = {
  questions: 'Perguntas',
  build: 'Construção',
  classify: 'Classificação do chat',
  edit: 'Alterações',
};

/** "claude-sonnet-5-5" → "Sonnet 5.5"; unknown ids pass through. */
export function modelName(id: string): string {
  const m = /^claude-([a-z]+)-(\d+)-(\d+)$/.exec(id);
  if (!m) return id;
  return `${m[1]![0]!.toUpperCase()}${m[1]!.slice(1)} ${m[2]}.${m[3]}`;
}

/** 28440 → "28,4K", 1284 → "1 284": compact for tiles, grouped for tables. */
export function compact(n: number): string {
  const short = (v: number, unit: string) => `${v.toFixed(1).replace(/\.0$/, '').replace('.', ',')}${unit}`;
  if (Math.abs(n) >= 1e6) return short(n / 1e6, 'M');
  if (Math.abs(n) >= 1e4) return short(n / 1e3, 'K');
  return fmt(n);
}

export const effortLabel: Record<string, string> = { low: 'baixo', medium: 'médio', high: 'alto', '': '·' };

/** Dollars with enough places to see a Haiku call: $0.0007, $0.13, $12.40. */
export function usd(v: number): string {
  const places = v === 0 ? 2 : Math.abs(v) < 0.01 ? 4 : Math.abs(v) < 1 ? 3 : 2;
  return `$${v.toFixed(places)}`;
}

/** Meticais with two decimals and space grouping: "1 234,56 MZN". */
export function mznExact(v: number): string {
  const sign = v < 0 ? '−' : '';
  const [i, d] = Math.abs(v).toFixed(2).split('.');
  return `${sign}${fmt(Number(i))},${d} MZN`;
}

export function seconds(ms: number): string {
  if (ms < 1000) return `${ms} ms`;
  const s = ms / 1000;
  return s < 60 ? `${s.toFixed(1)} s` : `${Math.floor(s / 60)} min ${Math.round(s % 60)} s`;
}

export function percent(part: number, whole: number): number {
  return whole > 0 ? (100 * part) / whole : 0;
}

/** Inclusive UTC day range ending today, for the period picker. */
export function lastDays(days: number, today = new Date()): { from: string; to: string } {
  const to = today.toISOString().slice(0, 10);
  const f = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() - (days - 1)));
  return { from: f.toISOString().slice(0, 10), to };
}
