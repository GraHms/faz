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
    // Telemetry sees every model call, grouped, with credits beside it.
    await expect(api.adminSiteTelemetry({ key: '' }, v.site.id)).rejects.toMatchObject({ code: 'admin_unauthorized' });
    const t = await api.adminSiteTelemetry({ key: 'chave' }, v.site.id);
    expect(t.by_operation.map((o) => o.operation)).toEqual(['questions', 'build', 'classify', 'edit']);
    expect(t.by_stage.filter((x) => x.operation === 'build').map((x) => x.stage)).toEqual(['intake', 'content', 'plan', 'critique', 'build', 'review']);
    expect(t.totals.calls).toBe(t.calls.length);
    expect(t.by_stage.reduce((n, x) => n + x.cost_usd, 0)).toBeCloseTo(t.totals.cost_usd, 6);
    expect(t.money).toMatchObject({ credits_charged: 24, credits_refunded: 5, credits_net: 19 });
    expect(t.jobs).toMatchObject({ builds: 1, edits: 1, failed: 1 });
    const o = await api.adminTelemetry({ key: 'chave' });
    expect(o.sites.map((x) => x.site_id)).toEqual([v.site.id]);
    expect(o.totals.cost_usd).toBeCloseTo(t.totals.cost_usd, 6);
  });

  it('reads photos sent with the site and keeps answers after a failed build', async () => {
    const { api, signIn, advance } = setup();
    signIn((await api.signup({ name: 'Rita', email: 'rita@example.com', password: 'segredo123' })).token);
    const photo = new File([new Uint8Array(10)], 'salao.jpg', { type: 'image/jpeg' });
    const v = await api.createSite({ prompt: 'Site para o meu salão na Matola', business_name: 'Nyeleti', business_type: 'Salão' }, [photo]);
    expect(v.assets.map((a) => a.name)).toEqual(['salao.jpg']);
    expect(v.questions?.map((q) => q.id)).not.toContain('fotos');
    expect(v.understood).toMatch(/^Percebi que/);

    await api.answer(v.site.id, { sentir: 'vivo' }, false, 'Isto estraga tudo');
    advance(16);
    const after = await api.site(v.site.id);
    expect(after.site.status).toBe('questions');
    expect(after.site.answers).toEqual({ sentir: 'Vivo e cheio de cor', _correction: 'Isto estraga tudo' });
    expect(after.understood).toBeTruthy();
    expect((await api.me()).balance).toBe(19);
  });

  it('runs the operator controls like fazd', async () => {
    const { api, signIn } = setup();
    const ops = { key: 'chave', operator: 'Ismael' };
    await expect(api.adminUpdateSettings({ key: 'chave' }, { beta_required: true }, 'Beta fechado')).rejects.toMatchObject({ code: 'operator_required' });
    await expect(api.adminUpdateSettings(ops, { beta_required: true }, 'curto')).resolves.toMatchObject({ beta_required: true });
    expect(await api.config()).toEqual({ beta_required: true });
    await expect(api.signup({ name: 'Sem', email: 'sem@example.com', password: 'segredo123' })).rejects.toMatchObject({ code: 'invite_required' });
    const inv = await api.adminCreateInvite(ops, { code: 'maputo-beta', max_uses: 1, bonus_credits: 100 }, 'Primeiros testes');
    expect(inv.code).toBe('MAPUTO-BETA');
    const s1 = await api.signup({ name: 'Rita', email: 'rita@example.com', password: 'segredo123', invite_code: 'maputo-beta' });
    expect(s1.account.balance).toBe(119);
    await expect(api.signup({ name: 'Outra', email: 'o@example.com', password: 'segredo123', invite_code: 'MAPUTO-BETA' })).rejects.toMatchObject({
      code: 'invite_invalid',
    });

    const uid = s1.account.user.id;
    const grant = { delta: 50, request_id: 'req-1' };
    expect(await api.adminAdjustCredits(ops, uid, grant, 'Compensação')).toEqual({ balance: 169 });
    expect(await api.adminAdjustCredits(ops, uid, grant, 'Compensação')).toEqual({ balance: 169 });
    await expect(api.adminAdjustCredits(ops, uid, { delta: -500, request_id: 'req-2' }, 'Erro de teste')).rejects.toMatchObject({ code: 'would_go_negative' });

    signIn(s1.token);
    await api.adminUpdateSettings(ops, { builds_paused: true }, 'Incidente no fornecedor');
    await expect(api.createSite({ prompt: 'Site para o meu salão na Matola', business_name: 'Nyeleti', business_type: 'Salão' })).rejects.toMatchObject({
      code: 'builds_paused',
    });
    await api.adminUpdateSettings(ops, { builds_paused: false, blocked_words: ['Golpe'] }, 'Fornecedor recuperado');
    expect(await api.availability('golpe-mpesa')).toMatchObject({ available: false });

    const site = await api.createSite({ prompt: 'Site para o meu salão na Matola', business_name: 'Nyeleti', business_type: 'Salão' });
    await api.adminTakeDown(ops, site.site.id, 'Denúncia de burla');
    expect((await api.site(site.site.id)).site.taken_down_at).toBeTruthy();
    expect((await api.adminSites(ops, 'nyel'))[0]?.owner_email).toBe('rita@example.com');

    await api.adminSuspend(ops, uid, true, 'Burla confirmada');
    // Suspension ends every session, as in fazd.
    await expect(api.me()).rejects.toMatchObject({ status: 401 });

    const log = await api.adminAudit(ops);
    expect(log.map((e) => e.action)).toEqual([
      'user.suspend',
      'site.takedown',
      'settings.update',
      'settings.update',
      'credits.adjust',
      'invite.create',
      'settings.update',
    ]);
    expect(log.every((e) => e.operator === 'Ismael')).toBe(true);
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
