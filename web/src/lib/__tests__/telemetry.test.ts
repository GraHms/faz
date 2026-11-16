import { compact, lastDays, modelName, mznExact, seconds, usd } from '../telemetry';
import { money } from '../../api/mockTelemetry';

describe('telemetry formatting', () => {
  it('names models and compacts tokens', () => {
    expect(modelName('claude-sonnet-5-5')).toBe('Sonnet 5.5');
    expect(modelName('claude-opus-5-5')).toBe('Opus 5.5');
    expect(modelName('gpt-x')).toBe('gpt-x');
    expect(compact(1284)).toBe('1 284');
    expect(compact(28440)).toBe('28,4K');
    expect(compact(2_000_000)).toBe('2M');
  });
  it('keeps small costs visible', () => {
    expect(usd(0.000685)).toBe('$0.0007');
    expect(usd(0.130227)).toBe('$0.130');
    expect(usd(12.4)).toBe('$12.40');
    expect(mznExact(8.33)).toBe('8,33 MZN');
    expect(mznExact(-1234.5)).toBe('−1 234,50 MZN');
    expect(seconds(950)).toBe('950 ms');
    expect(seconds(72000)).toBe('1 min 12 s');
  });
  it('gives inclusive UTC day ranges', () => {
    expect(lastDays(7, new Date('2026-11-16T05:00:00Z'))).toEqual({ from: '2026-11-10', to: '2026-11-16' });
  });
});

describe('money', () => {
  // Same numbers the Go service returned for a site with 24 credits charged.
  it('values credits without IVA and subtracts the model cost', () => {
    expect(money(0.130227, 24, 0, 1)).toEqual({
      usd_to_mzn: 64,
      cost_usd: 0.130227,
      cost_mzn: 8.33,
      credits_charged: 24,
      credits_refunded: 0,
      credits_net: 24,
      credits_mzn: 24,
      credits_mzn_ex_vat: 20.69,
      margin_mzn: 12.36,
      margin_percent: 59.7,
    });
    expect(money(0.5, 19, 19, 1).margin_percent).toBe(0);
  });
});
