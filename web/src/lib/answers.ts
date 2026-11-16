import { CORRECTION_KEY, type Question } from '../api/types';

/**
 * Turns stored answers back into form values, so a failed build can be
 * retried without answering again. The backend stores labels for choices
 * and +258 numbers for phones (product.labelAnswers).
 */
export function storedToValues(qs: Question[], stored: Record<string, string> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  if (!stored) return out;
  for (const q of qs) {
    const a = stored[q.id];
    if (!a) continue;
    if (q.kind === 'phone') {
      out[q.id] = a.replace(/\D/g, '').replace(/^258/, '');
    } else if (q.kind === 'choice') {
      const o = q.options?.find((x) => x.label === a);
      if (o) out[q.id] = o.value;
    } else if (q.kind === 'multi') {
      const parts = a.split(', ');
      const vals = (q.options ?? []).filter((o) => parts.includes(o.label) || (o.label.includes(', ') && a.includes(o.label))).map((o) => o.value);
      if (vals.length) out[q.id] = vals.join(',');
    } else {
      out[q.id] = a;
    }
  }
  return out;
}

/** The answers worth showing back in the chat, in question order, without the correction. */
export function answerSummary(qs: Question[] | undefined, stored: Record<string, string> | undefined): string[] {
  if (!stored) return [];
  const order = qs?.map((q) => q.id) ?? Object.keys(stored);
  const rest = Object.keys(stored).filter((k) => !order.includes(k));
  return [...order, ...rest].filter((k) => k !== CORRECTION_KEY && stored[k]).map((k) => stored[k]!);
}
