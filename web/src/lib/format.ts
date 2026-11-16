// Formatting and validation shared by every screen. The rules mirror the
// backend (internal/payments, internal/product) so the form says no before
// the server has to.

/** 12000 → "12 000", the way prices are written in Mozambique. */
export function fmt(n: number): string {
  return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
}

export function mzn(n: number): string {
  return `${fmt(n)} MZN`;
}

export function credits(n: number): string {
  return n === 1 ? '1 crédito' : `${fmt(n)} créditos`;
}

/** Mirrors payments.VATIncluded: IVA of 16% already inside a price. */
export const VAT_RATE = 0.16;
export function vatIncluded(amount: number): number {
  return Math.round((amount * VAT_RATE) / (1 + VAT_RATE));
}

export type Wallet = 'mpesa' | 'emola';

export const wallets: Record<Wallet, { name: string; network: string; prefixes: string[] }> = {
  mpesa: { name: 'M-Pesa', network: 'Vodacom', prefixes: ['84', '85'] },
  emola: { name: 'e-Mola', network: 'Movitel', prefixes: ['86', '87'] },
};

export type PhoneResult =
  | { ok: true; national: string; msisdn: string; wallet: Wallet }
  | { ok: false; error: string };

/** Mirrors payments.ParseMSISDN: 9 digits starting with 8, optional 258. */
export function parsePhone(input: string): PhoneResult {
  let d = input.replace(/\D/g, '');
  if (d.startsWith('258')) d = d.slice(3);
  if (d.length !== 9 || d[0] !== '8') {
    return { ok: false, error: 'Escreve um número de 9 dígitos, por exemplo 84 123 4567.' };
  }
  const p = d.slice(0, 2);
  const wallet = (Object.keys(wallets) as Wallet[]).find((w) => wallets[w].prefixes.includes(p));
  if (!wallet) {
    return { ok: false, error: 'Usa um número M-Pesa (84, 85) ou e-Mola (86, 87).' };
  }
  return { ok: true, national: d, msisdn: '258' + d, wallet };
}

/** "841234567" → "84 123 4567"; partial input is grouped as typed. */
export function fmtPhone(input: string): string {
  const d = input.replace(/\D/g, '').slice(0, 9);
  return [d.slice(0, 2), d.slice(2, 5), d.slice(5)].filter(Boolean).join(' ');
}

/** A free address from a business name: "Mar Azul, Lda." → "mar-azul-lda". */
export function slugify(v: string, max = 40): string {
  return v
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, max)
    .replace(/-+$/, '');
}

/** Mirrors the backend rule for subdomains, so the form can say why early. */
export function subdomainProblem(name: string): string | null {
  if (!/^[a-z0-9]([a-z0-9-]{1,38}[a-z0-9])$/.test(name)) {
    return 'Usa 3 a 40 letras, números e hífens, sem hífen no início ou no fim.';
  }
  if (name.includes('--')) return 'Não uses dois hífens seguidos.';
  return null;
}

const rtf = new Intl.RelativeTimeFormat('pt', { numeric: 'auto' });

/** "há 5 minutos", "ontem", or a date for anything older than a week. */
export function since(iso: string, now = Date.now()): string {
  const s = Math.round((new Date(iso).getTime() - now) / 1000);
  const a = Math.abs(s);
  if (a < 45) return 'agora mesmo';
  if (a < 3600) return rtf.format(Math.round(s / 60), 'minute');
  if (a < 86400) return rtf.format(Math.round(s / 3600), 'hour');
  if (a < 7 * 86400) return rtf.format(Math.round(s / 86400), 'day');
  return new Date(iso).toLocaleDateString('pt-MZ', { day: 'numeric', month: 'short', year: 'numeric' });
}

export function firstName(name: string): string {
  return name.trim().split(/\s+/)[0] ?? '';
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  const a = parts[0]?.[0] ?? '';
  const b = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : '';
  return (a + b).toUpperCase();
}

/** A stable color for a site card, picked from the prototype's examples. */
const swatches = ['#0F5A5C', '#7A2E4A', '#B4452A', '#3B4A1F', '#2C3E66', '#6B4E16'];
export function swatch(id: string): string {
  let h = 0;
  for (const ch of id) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return swatches[h % swatches.length]!;
}
