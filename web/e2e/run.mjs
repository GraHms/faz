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
function watch(page, label) {
  const app = (url) => url.startsWith(BASE);
  page.on('pageerror', (e) => problems.push(`${label}: page error: ${e.message}`));
  page.on('console', (m) => {
    if (m.type() !== 'error' || !app(m.location().url || BASE)) return;
    // The 402 for the edit without credits and the 401 for a wrong password are part of the journey.
    if (/status of 402/.test(m.text()) || (expect401 && /status of 401/.test(m.text()))) return;
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
await shot(page, 'business');
await page.getByRole('button', { name: 'Construir o meu site' }).click();
await page.waitForURL('**/sites/*', { timeout: 30_000 });

step('questions');
await page.getByText('Pergunta 1 de 3').waitFor();
await shot(page, 'questions');
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
await page.getByRole('button', { name: 'Pagamentos' }).click();
await page.getByText('Recibos').waitFor();
await page.getByText('Pago').first().waitFor();
await shot(page, 'payments');
const editorUrl = await page.evaluate(async () => {
  const r = await fetch('/v1/sites', { headers: { Authorization: `Bearer ${localStorage.getItem('faz.token')}` } });
  const { sites } = await r.json();
  return `/sites/${sites[0].id}`;
});

step('pricing and domains');
await page.goto(BASE + '/precos');
await page.getByText('Recarga mínima · 200 créditos').waitFor();
await shot(page, 'pricing');
await page.goto(BASE + '/dominios');
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
