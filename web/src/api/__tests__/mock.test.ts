import { createMockApi } from '../mock';
import { isApiError } from '../client';

function setup() {
  let token: string | null = null;
  let t = Date.parse('2026-11-15T10:00:00Z');
  const api = createMockApi({ persist: false, latency: [0, 0], getToken: () => token, now: () => t });
  return {
    api,
    signIn: (v: string) => (token = v),
    advance: (seconds: number) => (t += seconds * 1000),
  };
}

describe('mock API', () => {
  it('follows the product rules from sign up to a refunded edit', async () => {
    const { api, signIn, advance } = setup();
    const s = await api.signup({ name: 'Ana Machava', email: 'ana@example.com', password: 'segredo123' });
    signIn(s.token);
    expect(s.account.balance).toBe(19);

    const v = await api.createSite({ prompt: 'Site para a minha marisqueira na Costa do Sol', business_name: 'Mar Azul', business_type: 'Restaurante' });
    expect(v.site.status).toBe('questions');
    expect(v.questions?.length).toBe(3);

    await expect(api.answer(v.site.id, { whatsapp: '82 000 0000' })).rejects.toMatchObject({ code: 'invalid_phone' });
    const { job_id } = await api.answer(v.site.id, { whatsapp: '84 123 4567', fotos: 'sim', sentir: 'mar' });
    expect((await api.me()).balance).toBe(0);

    advance(8);
    const running = await api.job(job_id);
    expect(running.status).toBe('running');
    expect(running.progress).toBe('build');
    await expect(api.sendMessage(v.site.id, 'muda a cor para verde')).rejects.toMatchObject({ code: 'site_busy' });

    advance(10);
    expect((await api.job(job_id)).status).toBe('succeeded');
    const built = await api.site(v.site.id);
    expect(built.site).toMatchObject({ status: 'ready', current_version: 1 });
    expect(built.busy).toBe(false);

    const q = await api.sendMessage(v.site.id, 'Quanto custa uma alteração?');
    expect(q.credits_charged).toBe(0);
    expect(q.messages).toHaveLength(2);

    const err = await api.sendMessage(v.site.id, 'Põe o horário das 11h às 23h').catch((e: unknown) => e);
    expect(isApiError(err, 'insufficient_credits') && err.details).toEqual({ needed: 5, balance: 0 });

    const pay = await api.topUp({ amount_mzn: 200, msisdn: '84 123 4567' });
    expect(pay).toMatchObject({ status: 'pending', method: 'mpesa', credits: 200, vat_mzn: 28 });
    advance(6);
    expect((await api.payment(pay.id)).status).toBe('succeeded');
    expect((await api.me()).balance).toBe(200);

    const edit = await api.sendMessage(v.site.id, 'Isto estraga o topo da página');
    expect(edit.credits_charged).toBe(5);
    expect((await api.me()).balance).toBe(195);
    advance(7);
    expect((await api.job(edit.job_id!)).status).toBe('failed');
    expect((await api.me()).balance).toBe(200);
    const after = await api.site(v.site.id);
    expect(after.site.status).toBe('ready');
    expect(after.messages.at(-1)?.text).toMatch(/créditos foram devolvidos/);
  });

  it('checks subdomains like the backend', async () => {
    const { api, signIn } = setup();
    signIn((await api.signup({ name: 'Rui', email: 'rui@example.com', password: 'segredo123' })).token);
    expect(await api.availability('admin')).toMatchObject({ available: false });
    expect(await api.availability('mar--azul')).toMatchObject({ available: false });
    expect(await api.availability('mar-azul')).toMatchObject({ available: true });
  });

  it('fails a payment from a number ending in 0000', async () => {
    const { api, signIn, advance } = setup();
    signIn((await api.signup({ name: 'Rui', email: 'r@example.com', password: 'segredo123' })).token);
    const p = await api.topUp({ amount_mzn: 500, msisdn: '861230000' });
    advance(6);
    expect(await api.payment(p.id)).toMatchObject({ status: 'failed', method: 'emola' });
    expect((await api.me()).balance).toBe(19);
  });
});
