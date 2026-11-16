// End to end walk through the app in a real Chrome, against a running fazd
// started with FAZ_FAKE_DIR=examples/fake (see README). It drives the whole
// owner journey and saves a screenshot of every screen to e2e/screenshots.
//
//   FAZ_WEB=http://127.0.0.1:4173 CHROME=/usr/bin/google-chrome npm run e2e

import { chromium } from 'playwright-core';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const BASE = process.env.FAZ_WEB ?? 'http://127.0.0.1:4173';
const CHROME = process.env.CHROME ?? '/usr/bin/google-chrome';
const OUT = join(import.meta.dirname, 'screenshots');
mkdirSync(OUT, { recursive: true });

const problems = [];
// Each run publishes its own address, so runs can share a database.
const run = Date.now().toString(36).slice(-5);
const business = `Mel da Ana ${run}`;
const address = `mel-da-ana-${run}.faz.co.mz`;
const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });

// Only the app is checked here. Requests made by the generated page inside
// the preview iframe (its fonts, the fixture's missing logo) are its own.
let expect401 = false;
let expect503 = false;
function watch(page, label) {
  const app = (url) => url.startsWith(BASE);
  page.on('pageerror', (e) => problems.push(`${label}: page error: ${e.message}`));
  page.on('console', (m) => {
    if (m.type() !== 'error' || !app(m.location().url || BASE)) return;
    // The 402 for the edit without credits and the 401 for a wrong password are part of the journey.
    if (/status of 402/.test(m.text()) || (expect401 && /status of 401/.test(m.text())) || (expect503 && /status of 503/.test(m.text()))) return;
    problems.push(`${label}: console: ${m.text()}`);
  });
  page.on('requestfailed', (r) => {
    if (r.frame() !== page.mainFrame() || /aborted/i.test(r.failure()?.errorText ?? '')) return;
    problems.push(`${label}: request failed: ${r.url()} ${r.failure()?.errorText}`);
  });
}

let n = 0;
async function shot(page, name) {
  n++;
  await page.waitForTimeout(250);
  await page.screenshot({ path: join(OUT, `${String(n).padStart(2, '0')}-${name}.png`), fullPage: false });
  console.log('shot', name);
}

function step(name) {
  console.log('\n#', name);
}

const desktop = await browser.newContext({ viewport: { width: 1360, height: 860 }, deviceScaleFactor: 1, locale: 'pt-PT' });
const page = await desktop.newPage();
watch(page, 'desktop');

// Operator settings live in the database; start every run from an open beta
// with the models running, whatever an earlier run left behind.
if (process.env.FAZ_ADMIN_TOKEN) {
  const r = await fetch(BASE + '/v1/admin/settings', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Faz-Admin': process.env.FAZ_ADMIN_TOKEN, 'X-Faz-Operator': 'e2e' },
    body: JSON.stringify({ beta_required: false, builds_paused: false, daily_spend_cap_usd: 0, reason: 'Preparar o teste' }),
  });
  if (!r.ok) throw new Error(`reset settings: ${r.status}`);
}

step('landing');
await page.goto(BASE + '/');
await page.getByRole('heading', { name: 'O que queres criar?' }).waitFor();
await shot(page, 'landing');
await page.getByRole('button', { name: /Mar Azul/ }).click();
await page.locator('#prompt').fill(
  'Um site para o mel da Ana em Marracuene: frascos de mel e cera, entregas em Maputo e na Matola, encomendas pelo WhatsApp.',
);
await page.locator('#prompt').press('Enter');
await page.waitForURL('**/criar-conta');

step('sign up');
await shot(page, 'signup');
const email = `ana+${run}@example.com`;
await page.getByLabel('Nome', { exact: true }).fill('Ana Machava');
await page.getByLabel('Email').fill(email);
await page.getByLabel('Palavra-passe').fill('mel-de-marracuene');
await page.getByRole('button', { name: 'Criar conta' }).click();
await page.waitForURL('**/novo');

step('business');
await page.getByLabel('Nome do negócio').fill(business);
await page.locator('label', { hasText: 'Loja' }).click();
// A photo goes with the request, so the questions are written with it in view.
const png = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAEAAAAAwCAIAAAAuKetIAAAAV0lEQVR4Xu3RAQkAIQAEQTXNh7WYreRTLMJMgjt2nv2Nl62Xx/8cqClQU6CmQE2BmgI1BWoK1BSoKVBToKZATYGaAjUFagrUFKgpUFOgpkBNgZoCNQVqF8nLAd3SxgQbAAAAAElFTkSuQmCC',
  'base64',
);
await page.locator('input[type=file]').setInputFiles({ name: 'frascos de mel.png', mimeType: 'image/png', buffer: png });
await page.getByText('1 foto').waitFor();
await shot(page, 'business');
await page.getByRole('button', { name: 'Construir o meu site' }).click();
await page.waitForURL('**/sites/*', { timeout: 30_000 });

step('questions');
await page.getByText('Pergunta 1 de 3').waitFor();
await page.getByText('O que percebi').waitFor();
await page.getByLabel('Fotos enviadas').getByText(/frascos-de-mel/).waitFor();
await shot(page, 'questions');
await page.getByRole('button', { name: 'Corrigir' }).click();
await page.getByLabel(/O que está errado/).fill('Vendemos também a lojas e revendedores, não só a famílias.');
await shot(page, 'questions-correction');
await page.locator('textarea[aria-labelledby="q-entrega"]').fill('50 MT em Maputo e na Matola');
await page.getByRole('button', { name: 'Continuar' }).click();
await page.getByRole('radio', { name: /Sim|sim/ }).first().click();
await page.getByRole('button', { name: 'Continuar' }).click();
await page.getByRole('radio').first().click();
await shot(page, 'questions-last');
await page.getByRole('button', { name: 'Construir site' }).click();

step('build');
await page.getByRole('status').filter({ hasText: /A construir|Na fila/ }).first().waitFor({ timeout: 15_000 });
await shot(page, 'building');
await page.getByText(/^Pronto\./).waitFor({ timeout: 120_000 });
await page.getByText('Correcção: Vendemos também a lojas e revendedores, não só a famílias.').waitFor();
const frame = page.frameLocator('iframe[title^="Pré-visualização"]');
await frame.locator('body').waitFor({ timeout: 30_000 });
await page.waitForTimeout(800);
await shot(page, 'ready');

step('edit without credits');
await page.locator('#chat-input').fill('Põe o custo de entrega: 50 MT em Maputo e na Matola');
await page.locator('#chat-input').press('Enter');
await page.getByRole('dialog', { name: 'Recarrega para continuar a editar' }).waitFor({ timeout: 30_000 });
await shot(page, 'topup-modal');
await page.getByRole('button', { name: /Continuar · 200 MZN/ }).click();
await page.waitForURL('**/recarregar**');

step('checkout');
await page.getByRole('heading', { name: 'Finalizar compra' }).waitFor();
await page.getByLabel('Número').fill('82 123 4567');
await page.getByRole('button', { name: /^Pagar/ }).click();
await page.getByText('M-Pesa (84, 85) ou e-Mola (86, 87)').first().waitFor();
await page.getByLabel('Número').fill('84 123 4567');
await shot(page, 'checkout');
await page.getByRole('button', { name: 'Pagar 200 MZN com M-Pesa' }).click();
await page.getByRole('heading', { name: 'Confirma no teu telemóvel' }).waitFor();
await shot(page, 'payment-pending');
await page.getByRole('heading', { name: 'Pagamento confirmado' }).waitFor({ timeout: 30_000 });
await shot(page, 'payment-confirmed');
await page.getByRole('button', { name: 'Continuar' }).click();
await page.waitForURL('**/sites/*');

step('edit');
await page.locator('#chat-input').fill('Põe o custo de entrega: 50 MT em Maputo e na Matola');
await page.locator('#chat-input').press('Enter');
await page.getByText('−5 créditos').waitFor({ timeout: 30_000 });
await page.getByRole('status').filter({ hasText: /A alterar|Na fila/ }).first().waitFor({ timeout: 15_000 }).catch(() => undefined);
await shot(page, 'editing');
await page.getByText(/Pus o custo de entrega/).waitFor({ timeout: 120_000 });
await page.waitForTimeout(800);
await shot(page, 'edited');

step('custom domains, coming soon');
await page.getByRole('button', { name: /Ligar domínio/ }).click();
await page.getByRole('dialog', { name: 'Domínio próprio: em breve' }).waitFor();
await shot(page, 'domain-soon');
await page.getByRole('button', { name: 'Fechar' }).click();

step('publish');
await page.getByRole('button', { name: 'Publicar', exact: true }).click();
const dialog = page.getByRole('dialog');
await dialog.getByText('Disponível').waitFor({ timeout: 15_000 });
await shot(page, 'publish');
await dialog.getByRole('button', { name: 'Publicar', exact: true }).click();
await dialog.getByText('No ar').waitFor({ timeout: 15_000 });
await shot(page, 'published');
await dialog.getByRole('button', { name: 'Fechar' }).click();
await page.getByRole('button', { name: 'Telemóvel' }).click();
await page.waitForTimeout(600);
await shot(page, 'device-mobile');

step('dashboard');
await page.goto(BASE + '/painel');
await page.getByRole('heading', { name: /Tudo pronto para criar, Ana\?/ }).waitFor();
await page.getByText(address).waitFor();
await shot(page, 'dashboard');
await page.getByRole('button', { name: 'Domínios' }).click();
await page.getByRole('heading', { name: 'Domínios próprios' }).waitFor();
await page.getByRole('link', { name: address }).waitFor();
await shot(page, 'dashboard-domains');
await page.getByRole('button', { name: 'Pagamentos' }).click();
await page.getByText('Recibos').waitFor();
await page.getByText('Pago').first().waitFor();
await shot(page, 'payments');
const editorUrl = await page.evaluate(async () => {
  const r = await fetch('/v1/sites', { headers: { Authorization: `Bearer ${localStorage.getItem('faz.token')}` } });
  const { sites } = await r.json();
  return `/sites/${sites[0].id}`;
});

step('telemetry');
const adminKey = process.env.FAZ_ADMIN_TOKEN;
if (adminKey) {
  await page.goto(BASE + '/admin');
  expect401 = true; // a wrong key is refused with 401
  await page.getByLabel('Chave de administração').fill('errada');
  await page.getByLabel('O teu nome').fill('Ismael');
  await page.getByRole('button', { name: 'Entrar' }).click();
  await page.getByText('Chave errada').waitFor();
  expect401 = false;
  await page.getByLabel('Chave de administração').fill(adminKey);
  await page.getByLabel('O teu nome').fill('Ismael');
  await page.getByRole('button', { name: 'Entrar' }).click();
  await page.getByRole('heading', { name: 'Por etapa e modelo' }).waitFor();
  await shot(page, 'telemetry');
  await page.getByRole('link', { name: business }).click();
  await page.getByRole('heading', { name: 'Por etapa' }).waitFor();
  // The page's numbers must match the API's for the same site.
  const api = await page.evaluate(async ([key, path]) => {
    const r = await fetch(`/v1/admin/sites/${path.split('/').pop()}/telemetry`, { headers: { 'X-Faz-Admin': key } });
    return r.json();
  }, [adminKey, new URL(page.url()).pathname]);
  if (api.totals.calls < 8 || !api.by_stage.some((x) => x.operation === 'questions' && x.model === 'claude-sonnet-5-5')) {
    problems.push(`telemetry: unexpected site totals ${JSON.stringify(api.totals)}`);
  }
  await page.getByText(String(api.totals.calls), { exact: true }).first().waitFor();
  await shot(page, 'telemetry-site');
  await page.screenshot({ path: join(OUT, `${String(++n).padStart(2, '0')}-telemetry-site-full.png`), fullPage: true });
} else {
  console.log('FAZ_ADMIN_TOKEN not set: telemetry screens skipped');
}

step('operators');
if (adminKey) {
  const confirmWith = async (reason, button) => {
    const d = page.getByRole('dialog');
    await d.getByLabel('Motivo').fill(reason);
    await d.getByRole('button', { name: button, exact: true }).click();
    await d.waitFor({ state: 'detached' });
  };

  // A code with a bonus, and the beta closed.
  const code = `E2E-${run.toUpperCase()}`;
  await page.getByRole('link', { name: 'Convites' }).click();
  await page.getByRole('button', { name: 'Criar convite' }).click();
  const d = page.getByRole('dialog');
  await d.getByLabel('Código (opcional)').fill(code);
  await d.getByLabel('Para quem').fill('Teste automático');
  await d.getByLabel('Usos').fill('2');
  await d.getByLabel('Bónus em créditos').fill('100');
  await confirmWith('Teste do fluxo de convites', 'Criar convite');
  await page.getByRole('cell', { name: code }).waitFor();
  await shot(page, 'admin-invites');
  await page.getByRole('link', { name: 'Controlo' }).click();
  await page.getByRole('button', { name: 'Fechar o beta' }).click();
  await confirmWith('Beta fechado para o teste', 'Fechar');
  await page.getByText('Fechado', { exact: true }).waitFor();

  // A new owner arrives through the invite link and gets the bonus.
  const guest = await browser.newContext({ viewport: { width: 1360, height: 860 } });
  const g = await guest.newPage();
  watch(g, 'guest');
  await g.goto(`${BASE}/criar-conta?convite=${code}`);
  await g.getByText('beta fechado').first().waitFor();
  if ((await g.getByLabel('Código de convite').inputValue()) !== code) problems.push('invite link did not fill the code');
  const guestEmail = `convidado+${run}@example.com`;
  await g.getByLabel('Nome', { exact: true }).fill('Convidado Teste');
  await g.getByLabel('Email').fill(guestEmail);
  await g.getByLabel('Palavra-passe').fill('palavra-passe');
  await shot(g, 'signup-with-invite');
  await g.getByRole('button', { name: 'Criar conta' }).click();
  await g.waitForURL('**/novo');
  const guestBalance = await g.evaluate(async () => {
    const r = await fetch('/v1/me', { headers: { Authorization: `Bearer ${localStorage.getItem('faz.token')}` } });
    return (await r.json()).balance;
  });
  if (guestBalance !== 119) problems.push(`invite bonus: balance ${guestBalance}, want 119`);

  // Support finds them and gives credits.
  await page.getByRole('link', { name: 'Utilizadores' }).click();
  await page.getByLabel('Procurar utilizadores').fill(guestEmail);
  await page.getByRole('button', { name: 'Procurar' }).click();
  await page.getByRole('link', { name: 'Convidado Teste' }).click();
  await page.getByRole('button', { name: 'Dar créditos' }).click();
  await page.getByRole('dialog').getByLabel('Créditos').fill('50');
  await confirmWith('Compensação pelo teste', 'Dar 50');
  await page.getByText('Saldo agora: 169 créditos').waitFor();
  await shot(page, 'admin-user');

  // Paused models: the owner is told, nothing is charged.
  await page.getByRole('link', { name: 'Controlo' }).click();
  await page.getByLabel('Nota interna').fill('Teste de manutenção');
  await page.getByRole('button', { name: 'Parar agora' }).click();
  await confirmWith('Testar a pausa', 'Parar');
  await page.getByText('Os modelos estão parados').waitFor();
  await shot(page, 'admin-control');
  expect503 = true;
  await g.getByLabel('Nome do negócio').fill('Loja do Convidado');
  await g.getByLabel('O que queres no site').fill('Uma loja de capulanas em Maputo com encomendas pelo WhatsApp.');
  await g.getByRole('button', { name: 'Construir o meu site' }).click();
  await g.getByText('em manutenção').waitFor();
  await shot(g, 'owner-paused');
  expect503 = false;
  await page.getByRole('button', { name: 'Retomar' }).click();
  await confirmWith('Fim do teste da pausa', 'Retomar');
  await page.getByRole('button', { name: 'Abrir o beta' }).click();
  await confirmWith('Reabrir depois do teste', 'Abrir');
  await page.getByText('Aberto', { exact: true }).waitFor();
  await guest.close();

  // A takedown the owner sees, then restored.
  await page.getByRole('link', { name: 'Sites' }).click();
  await page.getByLabel('Procurar sites').fill(business);
  await page.getByRole('button', { name: 'Procurar' }).click();
  await page.getByRole('button', { name: 'Retirar do ar' }).click();
  await confirmWith('Teste de retirada do ar', 'Retirar do ar');
  await page.getByText('foi retirado do ar').waitFor();
  await shot(page, 'admin-sites');
  await page.goto(BASE + editorUrl);
  await page.getByText('A equipa Faz retirou este site do ar').waitFor();
  await shot(page, 'owner-taken-down');
  await page.goto(BASE + '/admin/sites?q=' + encodeURIComponent(business));
  await page.getByRole('button', { name: 'Repor' }).click();
  await confirmWith('Fim do teste de retirada', 'Repor');
  await page.getByText('está de volta').waitFor();

  await page.getByRole('link', { name: 'Registo' }).click();
  await page.getByText('Retirou o site do ar').first().waitFor();
  await shot(page, 'admin-audit');
}

step('pricing and domains');
await page.goto(BASE + '/precos');
await page.getByText('Recarga mínima · 200 créditos').waitFor();
await shot(page, 'pricing');
await page.goto(BASE + '/dominios');
await page.getByRole('heading', { name: 'Domínios próprios' }).waitFor();
await shot(page, 'domains');

step('phone');
const state = await desktop.storageState();
const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true, storageState: state });
const m = await phone.newPage();
watch(m, 'phone');
await m.goto(BASE + '/');
await m.getByRole('heading', { name: 'O que queres criar?' }).waitFor();
await shot(m, 'phone-landing');
await m.goto(BASE + editorUrl);
await m.getByText(/Pus o custo de entrega/).waitFor();
await shot(m, 'phone-editor-chat');
await m.getByRole('button', { name: 'Site', exact: true }).click();
await m.frameLocator('iframe[title^="Pré-visualização"]').locator('body').waitFor();
await m.waitForTimeout(800);
await shot(m, 'phone-editor-site');
await m.goto(BASE + '/painel');
await m.getByText(address).waitFor();
await shot(m, 'phone-dashboard');
await m.goto(BASE + '/recarregar?valor=500');
await m.getByRole('heading', { name: 'Finalizar compra' }).waitFor();
await shot(m, 'phone-checkout');

step('sign out and in');
await page.goto(BASE + '/painel');
await page.getByRole('button', { name: /Conta de/ }).click();
await page.getByRole('menuitem', { name: 'Sair' }).click();
await page.waitForURL(BASE + '/');
await page.goto(BASE + '/painel');
await page.waitForURL('**/entrar?voltar=*');
await page.getByLabel('Email').fill(email);
expect401 = true;
await page.getByLabel('Palavra-passe').fill('errada-de-todo');
await page.getByRole('button', { name: 'Entrar' }).click();
await page.getByRole('alert').waitFor();
await shot(page, 'login-error');
await page.getByLabel('Palavra-passe').fill('mel-de-marracuene');
await page.getByRole('button', { name: 'Entrar' }).click();
await page.waitForURL('**/painel');

await browser.close();
if (problems.length) {
  console.error('\nProblems:\n' + problems.join('\n'));
  process.exit(1);
}
console.log(`\nOK: ${n} screenshots in ${OUT}`);
