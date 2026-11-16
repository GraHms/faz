import { fmt, fmtPhone, parsePhone, since, slugify, subdomainProblem, vatIncluded } from '../format';

describe('fmt', () => {
  it('groups thousands with spaces', () => {
    expect(fmt(200)).toBe('200');
    expect(fmt(1200)).toBe('1 200');
    expect(fmt(50000)).toBe('50 000');
    expect(fmt(1234567)).toBe('1 234 567');
  });
});

describe('vatIncluded', () => {
  // Same numbers as payments.VATIncluded in Go.
  it('takes 16% IVA out of a gross price', () => {
    expect(vatIncluded(200)).toBe(28);
    expect(vatIncluded(500)).toBe(69);
    expect(vatIncluded(1000)).toBe(138);
  });
});

describe('parsePhone', () => {
  it('accepts M-Pesa and e-Mola numbers in any spacing', () => {
    expect(parsePhone('84 123 4567')).toEqual({ ok: true, national: '841234567', msisdn: '258841234567', wallet: 'mpesa' });
    expect(parsePhone('+258 85 123 4567')).toMatchObject({ ok: true, wallet: 'mpesa' });
    expect(parsePhone('258861234567')).toMatchObject({ ok: true, wallet: 'emola' });
    expect(parsePhone('87-123-4567')).toMatchObject({ ok: true, wallet: 'emola' });
  });
  it('refuses other networks and lengths', () => {
    expect(parsePhone('82 123 4567').ok).toBe(false);
    expect(parsePhone('84 123 456').ok).toBe(false);
    expect(parsePhone('').ok).toBe(false);
  });
});

describe('fmtPhone', () => {
  it('groups as typed', () => {
    expect(fmtPhone('8')).toBe('8');
    expect(fmtPhone('8412')).toBe('84 12');
    expect(fmtPhone('841234567')).toBe('84 123 4567');
    expect(fmtPhone('8412345678999')).toBe('84 123 4567');
  });
});

describe('slugify and subdomainProblem', () => {
  it('builds a valid free address from a business name', () => {
    expect(slugify('Mar Azul')).toBe('mar-azul');
    expect(slugify('Capulana & Co.')).toBe('capulana-co');
    expect(slugify('Salão Nyeleti, Lda')).toBe('salao-nyeleti-lda');
    expect(slugify('  --Óptica--  ')).toBe('optica');
    expect(subdomainProblem(slugify('Mar Azul'))).toBeNull();
  });
  it('mirrors the backend rule', () => {
    expect(subdomainProblem('ab')).not.toBeNull();
    expect(subdomainProblem('-mar')).not.toBeNull();
    expect(subdomainProblem('mar-')).not.toBeNull();
    expect(subdomainProblem('mar--azul')).toBe('Não uses dois hífens seguidos.');
    expect(subdomainProblem('a'.repeat(41))).not.toBeNull();
    expect(subdomainProblem('a'.repeat(40))).toBeNull();
  });
});

describe('since', () => {
  it('speaks Portuguese', () => {
    const now = Date.parse('2026-11-15T12:00:00Z');
    expect(since('2026-11-15T11:59:50Z', now)).toBe('agora mesmo');
    expect(since('2026-11-15T11:55:00Z', now)).toBe('há 5 minutos');
    expect(since('2026-11-14T12:00:00Z', now)).toBe('ontem');
  });
});
