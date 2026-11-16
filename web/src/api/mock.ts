// An in-browser imitation of fazd for UI work without the backend
// (VITE_API_MODE=mock). It keeps the same rules where the UI depends on
// them: prices, free questions, charged edits, refunds, busy sites,
// subdomain checks and wallet prefixes. It is not a test of the backend.
//
// Testing hooks: a phone number ending in 0000 makes a payment fail, and
// "estraga" in a chat message or in the correction to "O que percebi" makes
// that edit or build fail and refund.

import { ApiError, type Api } from './client';
import { CORRECTION_KEY, MAX_NEW_SITE_FILES } from './types';
import type {
  Account,
  Asset,
  Job,
  LedgerEntry,
  Message,
  MessageKind,
  Payment,
  Pricing,
  Question,
  Session,
  Site,
  SiteView,
  User,
  Version,
} from './types';
import { parsePhone, subdomainProblem, vatIncluded } from '../lib/format';
import { renderSamplePage } from './samplePage';
import { cost, groupByStage, groupCalls, money, plannedCalls, sumTotals } from './mockTelemetry';
import type {
  AdminAuth,
  AdminSettings,
  AdminStatus,
  AdminUser,
  AuditEntry,
  Invite,
  Operation,
  OverviewSite,
  SiteTelemetry,
  TelemetryOverview,
  UsageCall,
} from './types';

const pricing: Pricing = {
  mzn_per_credit: 1,
  signup_gift: 19,
  first_page: 19,
  edit: 5,
  structural_edit: 20,
  topup_min_mzn: 200,
  topup_max_mzn: 50000,
};

/** Stage timeline for a mock build, in seconds from the start. */
const buildStages: [stage: string, at: number][] = [
  ['content', 0],
  ['plan', 3],
  ['critique', 5],
  ['build', 7],
  ['review', 11],
  ['verify', 13],
];
const buildSeconds = 15;
const editStages: [stage: string, at: number][] = [
  ['edit', 0],
  ['verify', 4],
];
const editSeconds = 6;
const paymentSeconds = 5;

const reserved = new Set(['www', 'api', 'app', 'admin', 'faz', 'ajuda', 'entrar', 'conta', 'pagamentos', 'mpesa', 'emola']);

interface MockJob extends Job {
  user_id: string;
  site_id?: string;
  payment_id?: string;
  credits: number;
  request?: string;
  structural?: boolean;
  started: number;
  seconds: number;
  stages: [string, number][];
}

type MockUser = User & { password: string; suspended_at?: string; suspended_reason?: string };

interface State {
  seq: number;
  users: MockUser[];
  tokens: Record<string, string>;
  ledger: (LedgerEntry & { user_id: string })[];
  sites: (Site & { user_id: string; questions: Question[]; understood: string; accent: number })[];
  messages: (Message & { site_id: string })[];
  versions: (Version & { site_id: string })[];
  assets: (Asset & { site_id: string })[];
  jobs: MockJob[];
  payments: (Payment & { user_id: string; settles_at: number })[];
  usage: (UsageCall & { site_id: string; user_id: string })[];
  settings: AdminSettings;
  invites: Invite[];
  audit: AuditEntry[];
}

const defaultSettings = (): AdminSettings => ({
  beta_required: false,
  builds_paused: false,
  pause_reason: '',
  daily_spend_cap_usd: 0,
  reserved_names: [],
  blocked_words: [],
});

const KEY = 'faz.mock.v1';

function empty(): State {
  return {
    seq: 1,
    users: [],
    tokens: {},
    ledger: [],
    sites: [],
    messages: [],
    versions: [],
    assets: [],
    jobs: [],
    payments: [],
    usage: [],
    settings: defaultSettings(),
    invites: [],
    audit: [],
  };
}

export interface MockOptions {
  /** Persist to localStorage so a reload keeps the data. Default true. */
  persist?: boolean;
  /** Network delay range in ms. Default [250, 600]. */
  latency?: [number, number];
  getToken: () => string | null;
  now?: () => number;
}

export function createMockApi(opts: MockOptions): Api {
  const persist = opts.persist ?? true;
  const [lo, hi] = opts.latency ?? [250, 600];
  const now = opts.now ?? (() => Date.now());
  let s: State = load();
  const pages = new Map<string, string>();

  function load(): State {
    if (!persist) return empty();
    try {
      const raw = localStorage.getItem(KEY);
      // Older saved states lack newer collections; fill them in.
      return raw ? { ...empty(), ...(JSON.parse(raw) as Partial<State>) } : empty();
    } catch {
      return empty();
    }
  }
  function save() {
    if (!persist) return;
    try {
      localStorage.setItem(KEY, JSON.stringify(s));
    } catch {
      /* storage full or blocked: the mock keeps working in memory */
    }
  }
  const iso = (t = now()) => new Date(t).toISOString();
  const nextId = () => s.seq++;
  const wait = () => new Promise<void>((r) => setTimeout(r, lo + Math.random() * (hi - lo)));

  async function run<T>(fn: () => T): Promise<T> {
    await wait();
    advance();
    const out = fn();
    save();
    // Callers get copies, as they would from JSON.
    return structuredClone(out);
  }

  function fail(status: number, code: string, message: string, details?: Record<string, unknown>): never {
    throw new ApiError(status, code, message, details);
  }

  function user(): MockUser {
    const t = opts.getToken();
    const id = t ? s.tokens[t] : undefined;
    const u = id ? s.users.find((x) => x.id === id) : undefined;
    if (!u) fail(401, 'unauthenticated', 'Entra outra vez para continuar.');
    if (u.suspended_at) fail(401, 'account_suspended', 'A tua conta está suspensa. Fala com a equipa Faz para saber porquê.');
    return u;
  }
  const publicUser = (u: User): User => ({ id: u.id, email: u.email, name: u.name, created_at: u.created_at, invite_code: u.invite_code });

  /** Like Service.modelsOpen: paused by hand, or today's spend reached the cap. */
  function status(): AdminStatus {
    const midnight = new Date(now());
    midnight.setUTCHours(0, 0, 0, 0);
    const spend = s.usage.filter((c) => Date.parse(c.created_at) >= midnight.getTime()).reduce((n, c) => n + c.cost_usd, 0);
    const cap = s.settings.daily_spend_cap_usd > 0 && spend >= s.settings.daily_spend_cap_usd;
    return { ...s.settings, spend_today_usd: Math.round(spend * 1e4) / 1e4, cap_reached: cap, models_open: !s.settings.builds_paused && !cap };
  }
  function modelsOpen() {
    if (!status().models_open) {
      fail(503, 'builds_paused', 'A Faz está em manutenção por momentos. Tenta outra vez daqui a pouco; não gastaste créditos.');
    }
  }
  function addressProblem(name: string): string | null {
    if (reserved.has(name) || s.settings.reserved_names.includes(name)) return 'Este endereço está reservado.';
    if (s.settings.blocked_words.some((w) => name.includes(w))) return 'Este endereço não é permitido. Escolhe outro.';
    return null;
  }
  const balance = (userId: string) => s.ledger.filter((l) => l.user_id === userId).reduce((a, l) => a + l.delta, 0);
  const account = (u: User): Account => ({ user: publicUser(u), balance: balance(u.id) });

  function entry(userId: string, delta: number, kind: LedgerEntry['kind'], reason: string, extra: Partial<LedgerEntry> = {}) {
    s.ledger.push({ id: nextId(), user_id: userId, delta, kind, reason, created_at: iso(), ...extra });
  }
  function charge(userId: string, amount: number, reason: string, jobId: number) {
    const b = balance(userId);
    if (b < amount) {
      fail(402, 'insufficient_credits', 'Não tens créditos suficientes.', { needed: amount, balance: b });
    }
    entry(userId, -amount, 'charge', reason, { job_id: jobId });
  }

  function session(u: User): Session {
    const token = `mock_${crypto.randomUUID()}`;
    s.tokens[token] = u.id;
    return { token, expires_at: iso(now() + 30 * 86400_000), account: account(u) };
  }

  function siteOf(id: string, userId: string) {
    const site = s.sites.find((x) => x.id === id && x.user_id === userId);
    if (!site) fail(404, 'not_found', 'Não encontrado.');
    return site;
  }
  const busy = (siteId: string) => s.jobs.some((j) => j.site_id === siteId && (j.status === 'queued' || j.status === 'running'));

  function addMessage(siteId: string, m: Omit<Message, 'id' | 'created_at' | 'credits_charged'> & { credits_charged?: number }) {
    const msg = { id: nextId(), site_id: siteId, created_at: iso(), credits_charged: 0, ...m };
    s.messages.push(msg);
    return msg;
  }
  /** Records the model calls an operation would make, as fazd does in model_usage. */
  function spend(siteId: string, userId: string, op: Operation, jobId?: number, failAt?: string) {
    const photos = s.assets.filter((a) => a.site_id === siteId).length;
    for (const c of plannedCalls(op, photos)) {
      const error = c.stage === failAt ? 'simulated model outage' : undefined;
      s.usage.push({
        id: nextId(),
        site_id: siteId,
        user_id: userId,
        job_id: jobId,
        operation: op,
        stage: c.stage,
        model: c.model,
        effort: c.effort,
        attempt: 1,
        input_tokens: error ? 0 : c.input,
        output_tokens: error ? 0 : c.output,
        cache_read_tokens: error ? 0 : (c.cacheRead ?? 0),
        cache_write_tokens: 0,
        cost_usd: error ? 0 : cost(c.model, { input: c.input, output: c.output, cacheRead: c.cacheRead }),
        latency_ms: c.latency,
        error,
        created_at: iso(),
      });
      if (error) return;
    }
  }

  const stripSite = ({ user_id: _u, questions: _q, understood: _d, accent: _a, ...site }: State['sites'][number]): Site => site;
  const strip = <T extends object>(o: T, ...keys: string[]) => {
    const c = { ...o } as Record<string, unknown>;
    for (const k of keys) delete c[k];
    return c as unknown;
  };

  function view(site: State['sites'][number]): SiteView {
    return {
      site: stripSite(site),
      questions: site.status === 'questions' ? site.questions : undefined,
      understood: site.status === 'questions' ? site.understood : undefined,
      messages: s.messages.filter((m) => m.site_id === site.id).map((m) => strip(m, 'site_id') as Message),
      versions: s.versions.filter((v) => v.site_id === site.id).map((v) => strip(v, 'site_id') as Version),
      assets: s.assets.filter((a) => a.site_id === site.id).map((a) => strip(a, 'site_id') as Asset),
      live_url: site.subdomain ? `https://${site.subdomain}.faz.co.mz` : undefined,
      busy: busy(site.id),
    };
  }

  function questionsFor(type: string, hasPhotos: boolean): Question[] {
    const feel: Record<string, [string, string][]> = {
      Restaurante: [['mar', 'Fresco, como o mar ao fim da tarde'], ['rotulo', 'Clássico, como um rótulo antigo'], ['noite', 'Escuro e elegante, para jantares']],
      Salão: [['suave', 'Suave e calmo'], ['vivo', 'Vivo e cheio de cor'], ['luxo', 'Escuro e sofisticado']],
      Loja: [['tecido', 'Colorido, como os tecidos'], ['limpo', 'Limpo e simples'], ['feira', 'Animado, como uma feira']],
    };
    const qs: Question[] = [
      { id: 'whatsapp', text: 'Para que número de WhatsApp vão as mensagens dos clientes?', hint: 'Fica no botão principal do site.', kind: 'phone', options: null },
      {
        id: 'fotos',
        text: 'Já tens fotos do negócio?',
        hint: 'Sem fotos, o site usa ilustrações e deixa espaço para as tuas.',
        kind: 'choice',
        options: [
          { value: 'sim', label: 'Sim, vou enviar agora' },
          { value: 'depois', label: 'Envio mais tarde' },
          { value: 'nao', label: 'Não tenho fotos' },
        ],
      },
      {
        id: 'sentir',
        text: 'Como queres que o site se sinta?',
        hint: 'Escolhe o que mais se parece com o teu negócio.',
        kind: 'choice',
        options: (feel[type] ?? [['serio', 'Sério e de confiança'], ['proximo', 'Próximo e simpático'], ['ousado', 'Ousado e diferente']]).map(([value, label]) => ({ value, label })),
      },
    ];
    // Like the intake prompt: never ask for photos that came with the request.
    return hasPhotos ? qs.filter((q) => q.id !== 'fotos') : qs;
  }

  function understoodFor(name: string, type: string): string {
    const what = type ? `${name} (${type.toLowerCase()})` : name;
    return `Percebi que é um site para ${what}, pensado para clientes que te procuram pelo telemóvel. O principal é levar quem visita a falar contigo pelo WhatsApp.`;
  }

  /** Moves jobs and payments forward to the current time. */
  function advance() {
    const t = now();
    for (const j of s.jobs) {
      if (j.status === 'succeeded' || j.status === 'failed') continue;
      const elapsed = (t - j.started) / 1000;
      if (elapsed < j.seconds) {
        j.status = 'running';
        j.attempts = 1;
        j.progress = j.stages.filter(([, at]) => elapsed >= at).pop()?.[0];
        j.updated_at = iso();
        continue;
      }
      finish(j);
    }
    for (const p of s.payments) {
      if (p.status !== 'pending' || t < p.settles_at) continue;
      if (p.msisdn.endsWith('0000')) {
        p.status = 'failed';
        p.failure_reason = 'Saldo insuficiente na carteira.';
      } else {
        p.status = 'succeeded';
        p.provider_ref = `MOCK${p.id.slice(0, 8).toUpperCase()}`;
        entry(p.user_id, p.credits, 'topup', `Recarga ${p.method === 'mpesa' ? 'M-Pesa' : 'e-Mola'}`, { payment_id: p.id });
      }
      p.updated_at = iso();
    }
  }

  function finish(j: MockJob) {
    j.updated_at = iso();
    const site = s.sites.find((x) => x.id === j.site_id);
    if (!site) {
      j.status = 'failed';
      return;
    }
    if (j.kind === 'build' && site.answers?.[CORRECTION_KEY]?.toLowerCase().includes('estraga')) {
      spend(site.id, j.user_id, 'build', j.id, 'build');
      j.status = 'failed';
      entry(j.user_id, j.credits, 'refund', 'Construção falhou', { job_id: j.id });
      site.status = 'questions';
      addMessage(site.id, {
        role: 'assistant',
        job_id: j.id,
        text: 'Não consegui construir o site por um erro nosso. Os créditos foram devolvidos e as tuas respostas ficaram guardadas; carrega em Construir para tentar outra vez.',
      });
      return;
    }
    spend(site.id, j.user_id, j.kind === 'edit' ? 'edit' : 'build', j.id);
    if (j.kind === 'edit' && j.request?.toLowerCase().includes('estraga')) {
      j.status = 'failed';
      j.result = { rejected: true, reason: 'a alteração partia a página' };
      entry(j.user_id, j.credits, 'refund', 'Alteração não aplicada', { job_id: j.id });
      site.status = 'ready';
      addMessage(site.id, {
        role: 'assistant',
        job_id: j.id,
        text: 'Não apliquei esta alteração porque estragava a página. Os créditos foram devolvidos. Experimenta pedir de outra forma.',
      });
      return;
    }
    const n = site.current_version + 1;
    s.versions.push({ site_id: site.id, number: n, kind: j.kind === 'edit' ? 'edit' : 'build', status: 'ok', instruction: j.request, job_id: j.id, created_at: iso() });
    site.current_version = n;
    site.status = 'ready';
    site.updated_at = iso();
    if (j.structural) site.accent = (site.accent + 1) % 4;
    j.status = 'succeeded';
    j.result = { version: n };
    addMessage(site.id, {
      role: 'assistant',
      job_id: j.id,
      text:
        j.kind === 'edit'
          ? `Feito: ${j.request}. Vê a pré-visualização e diz-me se queres mais alguma coisa.`
          : 'Pronto. Há 2 informações marcadas no site para completares; diz-me aqui e eu ponho-as. Quando quiseres, carrega em Publicar.',
    });
  }

  function newJob(kind: string, userId: string, extra: Partial<MockJob>): MockJob {
    const j: MockJob = {
      id: nextId(),
      kind,
      status: 'queued',
      attempts: 0,
      user_id: userId,
      credits: 0,
      started: now(),
      seconds: kind === 'edit' ? editSeconds : buildSeconds,
      stages: kind === 'edit' ? editStages : buildStages,
      created_at: iso(),
      updated_at: iso(),
      ...extra,
    };
    s.jobs.push(j);
    return j;
  }

  function classify(text: string): { kind: MessageKind; reply?: string; options?: string[] } {
    const t = text.toLowerCase();
    if (t.length < 12 && !/\d/.test(t)) {
      return { kind: 'vague', reply: 'O que queres mudar exactamente?', options: ['Mudar as cores', 'Mudar o texto do topo', 'Acrescentar o horário'] };
    }
    if (/\?$/.test(t.trim()) || /^(quanto|como|posso|onde|quando|o que|qual)\b/.test(t)) {
      return {
        kind: 'other',
        reply: `Cada alteração simples custa ${pricing.edit} créditos e uma mudança grande custa ${pricing.structural_edit}. Perguntas como esta são grátis. O alojamento em nome.faz.co.mz é grátis.`,
      };
    }
    if (/(cores|estilo|aspecto|secção|secções|layout|tudo|página inteira)/.test(t)) return { kind: 'structural' };
    return { kind: 'edit' };
  }

  return {
    pricing: () => run(() => pricing),

    config: () => run(() => ({ beta_required: s.settings.beta_required })),

    signup: ({ name, email, password, invite_code }) =>
      run(() => {
        name = name.trim();
        email = email.trim().toLowerCase();
        const code = (invite_code ?? '').replace(/\s/g, '').toUpperCase();
        if (!name) fail(400, 'invalid_name', 'Escreve o teu nome.');
        if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) fail(400, 'invalid_email', 'Escreve um email válido.');
        if (password.length < 8) fail(400, 'weak_password', 'A palavra-passe precisa de pelo menos 8 caracteres.');
        if (s.settings.beta_required && !code) {
          fail(403, 'invite_required', 'A Faz está em beta fechado: precisas de um código de convite para criar conta.');
        }
        if (s.users.some((u) => u.email === email)) fail(409, 'email_taken', 'Já existe uma conta com este email.');
        const inv = code ? s.invites.find((i) => i.code === code) : undefined;
        if (code && (!inv || inv.disabled_at || inv.uses >= inv.max_uses || (inv.expires_at && Date.parse(inv.expires_at) <= now()))) {
          fail(403, 'invite_invalid', 'Este código de convite não é válido, expirou ou já foi usado todas as vezes.');
        }
        const u: MockUser = { id: crypto.randomUUID(), email, name, password, created_at: iso(), invite_code: inv?.code };
        s.users.push(u);
        entry(u.id, pricing.signup_gift, 'signup_gift', 'Créditos de oferta para o primeiro site');
        if (inv) {
          inv.uses++;
          if (inv.bonus_credits > 0) entry(u.id, inv.bonus_credits, 'adjustment', `Bónus do convite ${inv.code}`);
        }
        return session(u);
      }),

    login: ({ email, password }) =>
      run(() => {
        const u = s.users.find((x) => x.email === email.trim().toLowerCase() && x.password === password);
        if (!u) fail(401, 'bad_login', 'Email ou palavra-passe errados.');
        if (u.suspended_at) fail(403, 'account_suspended', 'A tua conta está suspensa. Fala com a equipa Faz para saber porquê.');
        return session(u);
      }),

    logout: () =>
      run(() => {
        const t = opts.getToken();
        if (t) delete s.tokens[t];
      }),

    me: () => run(() => account(user())),

    credits: () =>
      run(() => {
        const u = user();
        return {
          balance: balance(u.id),
          ledger: s.ledger.filter((l) => l.user_id === u.id).reverse().map((l) => strip(l, 'user_id') as LedgerEntry),
          pricing,
        };
      }),

    createSite: ({ prompt, business_name, business_type }, files = []) =>
      run(() => {
        const u = user();
        modelsOpen();
        if (files.length > MAX_NEW_SITE_FILES) fail(400, 'too_many_files', `Envia até ${MAX_NEW_SITE_FILES} fotos agora; podes juntar mais depois.`);
        for (const f of files) {
          if (!['image/jpeg', 'image/png', 'image/webp'].includes(f.type)) fail(400, 'unsupported_image', 'Envia uma imagem JPEG, PNG ou WebP.');
          if (f.size > 15 * 1024 * 1024) fail(413, 'file_too_large', 'Envia imagens até 15 MB.');
        }
        prompt = prompt.trim();
        business_name = business_name.trim();
        if (prompt.length < 10 || prompt.length > 4000) fail(400, 'invalid_prompt', 'Descreve o site em 10 a 4000 caracteres.');
        if (!business_name || business_name.length > 80) fail(400, 'invalid_business_name', 'Escreve o nome do negócio.');
        const site = {
          id: crypto.randomUUID(),
          user_id: u.id,
          business_name,
          business_type,
          prompt,
          status: 'questions' as const,
          current_version: 0,
          created_at: iso(),
          updated_at: iso(),
          questions: questionsFor(business_type, files.length > 0),
          understood: understoodFor(business_name, business_type),
          accent: 0,
        };
        s.sites.push(site);
        for (const f of files) {
          s.assets.push({ site_id: site.id, name: f.name, media_type: f.type, size: f.size, created_at: iso() });
        }
        spend(site.id, u.id, 'questions');
        return view(site);
      }),

    sites: () =>
      run(() => {
        const u = user();
        return s.sites
          .filter((x) => x.user_id === u.id)
          .sort((a, b) => b.created_at.localeCompare(a.created_at))
          .map(stripSite);
      }),

    site: (id) => run(() => view(siteOf(id, user().id))),

    uploadAsset: async (siteId, file) => {
      if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type)) {
        await wait();
        fail(400, 'unsupported_image', 'Envia uma imagem JPEG, PNG ou WebP.');
      }
      if (file.size > 15 * 1024 * 1024) {
        await wait();
        fail(413, 'file_too_large', 'Envia imagens até 15 MB.');
      }
      return run(() => {
        siteOf(siteId, user().id);
        const a = { site_id: siteId, name: file.name, media_type: file.type, size: file.size, created_at: iso() };
        s.assets = s.assets.filter((x) => !(x.site_id === siteId && x.name === file.name));
        s.assets.push(a);
        return strip(a, 'site_id') as Asset;
      });
    },

    answer: (siteId, answers, skip = false, correction) =>
      run(() => {
        correction = correction?.trim();
        if (correction && correction.length > 1000) fail(400, 'correction_too_long', 'Escreve a correcção em até 1000 caracteres.');
        const u = user();
        const site = siteOf(siteId, u.id);
        if (site.status !== 'questions') fail(409, 'not_waiting_for_answers', 'Este site já está a ser construído.');
        modelsOpen();
        // Stored like the backend's labelAnswers: labels for choices, +258 numbers.
        const labeled: Record<string, string> = {};
        if (!skip) {
          for (const q of site.questions) {
            const a = answers[q.id]?.trim();
            if (!a) continue;
            if (q.kind === 'phone' && !parsePhone(a).ok) {
              fail(400, 'invalid_phone', 'Escreve um número de telemóvel moçambicano, por exemplo 84 123 4567.');
            }
            if ((q.kind === 'choice' || q.kind === 'multi') && !a.split(',').every((v) => q.options?.some((o) => o.value === v.trim()))) {
              fail(400, 'invalid_option', `Opção desconhecida para "${q.text}".`);
            }
            const phone = parsePhone(a);
            labeled[q.id] =
              q.kind === 'phone' && phone.ok
                ? `+${phone.msisdn}`
                : q.options
                  ? a.split(',').map((v) => q.options!.find((o) => o.value === v.trim())!.label).join(', ')
                  : a;
          }
        }
        const j = newJob('build', u.id, { site_id: site.id, credits: pricing.first_page });
        try {
          charge(u.id, pricing.first_page, `Primeiro site: ${site.business_name}`, j.id);
        } catch (e) {
          s.jobs.pop();
          throw e;
        }
        if (correction) labeled[CORRECTION_KEY] = correction;
        site.answers = labeled;
        site.status = 'building';
        addMessage(site.id, {
          role: 'assistant',
          job_id: j.id,
          credits_charged: pricing.first_page,
          text: 'Obrigado. Estou a construir o teu site; demora um ou dois minutos.',
        });
        return { job_id: j.id };
      }),

    sendMessage: (siteId, text) =>
      run(() => {
        const u = user();
        const site = siteOf(siteId, u.id);
        text = text.trim();
        if (!text || text.length > 2000) fail(400, 'invalid_message', 'Escreve o pedido em até 2000 caracteres.');
        if (site.current_version === 0 || site.status !== 'ready' || busy(site.id)) {
          fail(409, 'site_busy', 'Espera que o site termine a alteração em curso.');
        }
        modelsOpen();
        spend(site.id, u.id, 'classify');
        const c = classify(text);
        if (c.kind !== 'edit' && c.kind !== 'structural') {
          const um = addMessage(site.id, { role: 'user', text });
          const am = addMessage(site.id, { role: 'assistant', text: c.reply ?? '', options: c.options });
          return { messages: [um, am].map((m) => strip(m, 'site_id') as Message), credits_charged: 0, kind: c.kind };
        }
        const price = c.kind === 'structural' ? pricing.structural_edit : pricing.edit;
        const j = newJob('edit', u.id, { site_id: site.id, credits: price, request: text, structural: c.kind === 'structural' });
        try {
          charge(u.id, price, `Alteração: ${text.slice(0, 80)}`, j.id);
        } catch (e) {
          s.jobs.pop();
          throw e;
        }
        site.status = 'editing';
        const um = addMessage(site.id, { role: 'user', text, job_id: j.id, credits_charged: price });
        return { messages: [strip(um, 'site_id') as Message], job_id: j.id, credits_charged: price, kind: c.kind };
      }),

    preview: (siteId, version) =>
      run(() => {
        const site = siteOf(siteId, user().id);
        const v = version || site.current_version;
        if (!v) fail(409, 'not_built', 'O site ainda não foi construído.');
        const key = `${site.id}:${v}:${site.accent}`;
        let url = pages.get(key);
        if (!url) {
          const html = renderSamplePage({ name: site.business_name, type: site.business_type, prompt: site.prompt, accent: site.accent, version: v });
          url = URL.createObjectURL(new Blob([html], { type: 'text/html' }));
          pages.set(key, url);
        }
        return { url, version: v, expires_at: iso(now() + 15 * 60_000) };
      }),

    availability: (name, siteId) =>
      run(() => {
        user();
        name = name.toLowerCase();
        const problem = subdomainProblem(name);
        if (problem) return { name, available: false, reason: problem };
        const rule = addressProblem(name);
        if (rule) return { name, available: false, reason: rule };
        const owner = s.sites.find((x) => x.subdomain === name);
        if (owner && owner.id !== siteId) return { name, available: false, reason: 'Este endereço já tem dono.' };
        return { name, available: true, reason: '' };
      }),

    publish: (siteId, subdomain) =>
      run(() => {
        const site = siteOf(siteId, user().id);
        subdomain = subdomain.toLowerCase();
        const problem = subdomainProblem(subdomain) ?? addressProblem(subdomain);
        if (problem) fail(409, 'subdomain_unavailable', problem);
        if (site.taken_down_at) {
          fail(403, 'site_taken_down', 'Este site foi retirado do ar pela equipa Faz e não pode ser publicado. Fala connosco.');
        }
        if (s.sites.some((x) => x.subdomain === subdomain && x.id !== site.id)) fail(409, 'subdomain_unavailable', 'Este endereço já tem dono.');
        if (!site.current_version) fail(409, 'not_built', 'O site ainda não foi construído.');
        site.subdomain = subdomain;
        site.published_version = site.current_version;
        site.published_at = iso();
        return { url: `https://${subdomain}.faz.co.mz`, subdomain, version: site.current_version };
      }),

    job: (id) =>
      run(() => {
        const u = user();
        const j = s.jobs.find((x) => x.id === id && x.user_id === u.id);
        if (!j) fail(404, 'not_found', 'Não encontrado.');
        return strip(j, 'user_id', 'site_id', 'payment_id', 'credits', 'request', 'structural', 'started', 'seconds', 'stages') as Job;
      }),

    topUp: ({ amount_mzn, msisdn, nuit }) =>
      run(() => {
        const u = user();
        if (!Number.isInteger(amount_mzn) || amount_mzn < pricing.topup_min_mzn || amount_mzn > pricing.topup_max_mzn) {
          fail(400, 'invalid_amount', `Escolhe um valor entre ${pricing.topup_min_mzn} e ${pricing.topup_max_mzn} MZN.`);
        }
        const phone = parsePhone(msisdn);
        if (!phone.ok) fail(400, 'invalid_msisdn', 'Usa um número M-Pesa (84, 85) ou e-Mola (86, 87).');
        if (nuit && !/^\d{9}$/.test(nuit)) fail(400, 'invalid_nuit', 'O NUIT tem 9 dígitos.');
        const p = {
          id: crypto.randomUUID(),
          user_id: u.id,
          method: phone.wallet,
          msisdn: phone.msisdn,
          amount_mzn,
          vat_mzn: vatIncluded(amount_mzn),
          credits: Math.floor(amount_mzn / pricing.mzn_per_credit),
          nuit: nuit || undefined,
          status: 'pending' as const,
          provider: 'mock',
          created_at: iso(),
          updated_at: iso(),
          settles_at: now() + paymentSeconds * 1000,
        };
        s.payments.push(p);
        return strip(p, 'user_id', 'settles_at') as Payment;
      }),

    payments: () =>
      run(() => {
        const u = user();
        return s.payments
          .filter((p) => p.user_id === u.id)
          .reverse()
          .map((p) => strip(p, 'user_id', 'settles_at') as Payment);
      }),

    payment: (id) =>
      run(() => {
        const u = user();
        const p = s.payments.find((x) => x.id === id && x.user_id === u.id);
        if (!p) fail(404, 'not_found', 'Não encontrado.');
        return strip(p, 'user_id', 'settles_at') as Payment;
      }),

    // Any non-empty key opens the admin pages in the mock.
    adminTelemetry: (auth, period = {}) =>
      run((): TelemetryOverview => {
        adminKey(auth);
        const to = period.to ? Date.parse(period.to + 'T00:00:00Z') + 86400_000 : now() + 1;
        const from = period.from ? Date.parse(period.from + 'T00:00:00Z') : to - 30 * 86400_000;
        const inRange = (t: string) => Date.parse(t) >= from && Date.parse(t) < to;
        const calls = s.usage.filter((c) => inRange(c.created_at)).map(publicCall);
        const sites: OverviewSite[] = [];
        for (const site of s.sites) {
          const mine = s.usage.filter((c) => c.site_id === site.id && inRange(c.created_at)).map(publicCall);
          if (!mine.length) continue;
          const t = sumTotals(groupByStage(mine));
          const [charged, refunded] = credits(site.id, inRange);
          sites.push({
            ...t,
            site_id: site.id,
            business_name: site.business_name,
            owner_email: s.users.find((x) => x.id === site.user_id)?.email ?? '',
            status: site.status,
            current_version: site.current_version,
            published_version: site.published_version,
            created_at: site.created_at,
            credits_charged: charged,
            credits_refunded: refunded,
            money: money(t.cost_usd, charged, refunded, pricing.mzn_per_credit),
          });
        }
        sites.sort((a, b) => b.cost_usd - a.cost_usd);
        const byStage = groupByStage(calls);
        const totals = sumTotals(byStage);
        const [charged, refunded] = credits(null, inRange);
        return {
          from: new Date(from).toISOString(),
          to: new Date(to).toISOString(),
          totals,
          money: money(totals.cost_usd, charged, refunded, pricing.mzn_per_credit),
          by_stage: byStage,
          sites,
        };
      }),

    adminSiteTelemetry: (auth, siteId) =>
      run((): SiteTelemetry => {
        adminKey(auth);
        const site = s.sites.find((x) => x.id === siteId);
        if (!site) fail(404, 'not_found', 'Não encontrado.');
        const calls = s.usage.filter((c) => c.site_id === site.id).map(publicCall);
        const g = groupCalls(calls);
        const [charged, refunded] = credits(site.id, () => true);
        const jobs = s.jobs.filter((j) => j.site_id === site.id);
        const builds = jobs.filter((j) => j.kind === 'build' && j.status === 'succeeded');
        return {
          site: stripSite(site),
          owner_email: s.users.find((x) => x.id === site.user_id)?.email ?? '',
          totals: g.totals,
          money: money(g.totals.cost_usd, charged, refunded, pricing.mzn_per_credit),
          jobs: {
            builds: jobs.filter((j) => j.kind === 'build').length,
            edits: jobs.filter((j) => j.kind === 'edit').length,
            failed: jobs.filter((j) => j.status === 'failed').length,
            avg_build_seconds: builds.length ? Math.round(builds.reduce((n, j) => n + j.seconds, 0) / builds.length) : 0,
          },
          by_operation: g.byOperation,
          by_model: g.byModel,
          by_stage: g.byStage,
          calls,
        };
      }),

    adminStatus: (auth) =>
      run(() => {
        adminKey(auth);
        return status();
      }),

    adminUpdateSettings: (auth, patch, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const words = (v: string[] | undefined) =>
          v === undefined ? undefined : [...new Set(v.map((w) => w.trim().toLowerCase()).filter(Boolean))].sort();
        const changes: Partial<AdminSettings> = { ...patch };
        if (patch.reserved_names) changes.reserved_names = words(patch.reserved_names);
        if (patch.blocked_words) changes.blocked_words = words(patch.blocked_words);
        if (Object.keys(changes).length === 0) fail(400, 'nothing_to_change', 'Nada para mudar.');
        s.settings = { ...s.settings, ...changes };
        audit(op, 'settings.update', 'settings', 'global', reason, { changes });
        return status();
      }),

    adminInvites: (auth) =>
      run(() => {
        adminKey(auth);
        return s.invites;
      }),

    adminCreateInvite: (auth, inv, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
        const pick = () => alphabet[Math.floor(Math.random() * alphabet.length)];
        const code = (inv.code ?? '').replace(/\s/g, '').toUpperCase() || `FAZ-${Array.from({ length: 4 }, pick).join('')}-${Array.from({ length: 4 }, pick).join('')}`;
        if (!/^[A-Z0-9][A-Z0-9-]{2,39}$/.test(code)) fail(400, 'invalid_code', 'O código leva 3 a 40 letras maiúsculas, números e hífens.');
        if (inv.max_uses < 1 || inv.max_uses > 10000) fail(400, 'invalid_max_uses', 'Usos: de 1 a 10 000.');
        if (inv.bonus_credits < 0 || inv.bonus_credits > 5000) fail(400, 'invalid_bonus', 'Bónus: de 0 a 5 000 créditos.');
        if (s.invites.some((i) => i.code === code)) fail(409, 'code_taken', 'Esse código já existe.');
        const out: Invite = { code, note: inv.note ?? '', max_uses: inv.max_uses, uses: 0, bonus_credits: inv.bonus_credits, expires_at: inv.expires_at, created_by: op, created_at: iso() };
        s.invites.unshift(out);
        audit(op, 'invite.create', 'invite', code, reason, { ...inv });
        return out;
      }),

    adminDisableInvite: (auth, code, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const inv = s.invites.find((i) => i.code === code);
        if (!inv) fail(404, 'not_found', 'Não encontrado.');
        inv.disabled_at ??= iso();
        audit(op, 'invite.disable', 'invite', code, reason);
        return inv;
      }),

    adminUsers: (auth, query) =>
      run(() => {
        adminKey(auth);
        const q = query.trim().toLowerCase();
        let digits = q.replace(/\D/g, '');
        if (digits.length > 9 && digits.startsWith('258')) digits = digits.slice(3);
        return s.users
          .filter((u) => {
            if (!q) return true;
            if (u.email.includes(q) || u.name.toLowerCase().includes(q) || u.id === q) return true;
            return digits.length >= 6 && s.payments.some((p) => p.user_id === u.id && p.msisdn.includes(digits));
          })
          .slice()
          .reverse()
          .map((u) => ({
            ...adminUser(u),
            balance: balance(u.id),
            sites: s.sites.filter((x) => x.user_id === u.id).length,
            last_msisdn: [...s.payments].reverse().find((p) => p.user_id === u.id)?.msisdn,
          }));
      }),

    adminUser: (auth, userId) =>
      run(() => {
        adminKey(auth);
        const u = s.users.find((x) => x.id === userId);
        if (!u) fail(404, 'not_found', 'Não encontrado.');
        return {
          user: adminUser(u),
          balance: balance(u.id),
          sessions: Object.values(s.tokens).filter((id) => id === u.id).length,
          sites: s.sites.filter((x) => x.user_id === u.id).map(stripSite),
          ledger: s.ledger.filter((l) => l.user_id === u.id).reverse().map((l) => strip(l, 'user_id') as LedgerEntry),
          payments: s.payments.filter((p) => p.user_id === u.id).reverse().map((p) => strip(p, 'user_id', 'settles_at') as Payment),
          audit: s.audit.filter((a) => a.target_type === 'user' && a.target_id === u.id),
        };
      }),

    adminAdjustCredits: (auth, userId, adj, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const u = s.users.find((x) => x.id === userId);
        if (!u) fail(404, 'not_found', 'Não encontrado.');
        if (!adj.delta || Math.abs(adj.delta) > 100000) fail(400, 'invalid_delta', 'Escolhe um valor entre −100 000 e 100 000 créditos, sem ser 0.');
        if (s.audit.some((a) => a.details.request_id === adj.request_id)) return { balance: balance(u.id) };
        const bal = balance(u.id);
        if (bal + adj.delta < 0) fail(400, 'would_go_negative', `O saldo é ${bal} créditos; não dá para tirar ${-adj.delta}.`);
        const note = adj.public_note?.trim() || (adj.delta > 0 ? 'Créditos oferecidos pela equipa Faz' : 'Acerto feito pela equipa Faz');
        entry(u.id, adj.delta, 'adjustment', note);
        audit(op, 'credits.adjust', 'user', u.id, reason, { delta: adj.delta, public_note: note, request_id: adj.request_id, balance_after: bal + adj.delta });
        return { balance: bal + adj.delta };
      }),

    adminSuspend: (auth, userId, suspended, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const u = s.users.find((x) => x.id === userId);
        if (!u) fail(404, 'not_found', 'Não encontrado.');
        u.suspended_at = suspended ? iso() : undefined;
        u.suspended_reason = suspended ? reason.trim() : undefined;
        let ended = 0;
        if (suspended) {
          for (const [t, id] of Object.entries(s.tokens)) if (id === u.id) (delete s.tokens[t], ended++);
        }
        audit(op, suspended ? 'user.suspend' : 'user.unsuspend', 'user', u.id, reason, { sessions_ended: ended });
        return adminUser(u);
      }),

    adminSignOut: (auth, userId, reason) =>
      run(() => {
        const op = operator(auth, reason);
        let ended = 0;
        for (const [t, id] of Object.entries(s.tokens)) if (id === userId) (delete s.tokens[t], ended++);
        audit(op, 'user.sign_out', 'user', userId, reason, { sessions_ended: ended });
        return { sessions_ended: ended };
      }),

    adminRefundJob: (auth, jobId, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const charged = s.ledger.find((l) => l.job_id === jobId && l.kind === 'charge');
        if (!charged) fail(404, 'not_found', 'Não encontrado.');
        if (s.ledger.some((l) => l.job_id === jobId && l.kind === 'refund')) fail(409, 'already_refunded', 'Este trabalho já foi devolvido.');
        entry(charged.user_id, -charged.delta, 'refund', 'Devolução feita pela equipa Faz', { job_id: jobId });
        audit(op, 'credits.refund_job', 'user', charged.user_id, reason, { job_id: jobId, credits: -charged.delta });
        return { credits: -charged.delta };
      }),

    adminSites: (auth, query) =>
      run(() => {
        adminKey(auth);
        const q = query.trim().toLowerCase();
        return s.sites
          .filter((x) => !q || x.business_name.toLowerCase().includes(q) || x.subdomain?.includes(q) || x.id === q)
          .slice()
          .reverse()
          .map((x) => ({ ...stripSite(x), owner_email: s.users.find((u) => u.id === x.user_id)?.email ?? '' }));
      }),

    adminTakeDown: (auth, siteId, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const site = s.sites.find((x) => x.id === siteId);
        if (!site) fail(404, 'not_found', 'Não encontrado.');
        site.taken_down_at = iso();
        site.takedown_reason = reason.trim();
        audit(op, 'site.takedown', 'site', site.id, reason, { subdomain: site.subdomain });
        return stripSite(site);
      }),

    adminRestore: (auth, siteId, reason) =>
      run(() => {
        const op = operator(auth, reason);
        const site = s.sites.find((x) => x.id === siteId);
        if (!site) fail(404, 'not_found', 'Não encontrado.');
        if (!site.taken_down_at) fail(409, 'not_taken_down', 'Este site não está retirado do ar.');
        site.taken_down_at = undefined;
        site.takedown_reason = undefined;
        audit(op, 'site.restore', 'site', site.id, reason, { subdomain: site.subdomain });
        return stripSite(site);
      }),

    adminAudit: (auth, target) =>
      run(() => {
        adminKey(auth);
        return s.audit.filter((a) => !target || (a.target_type === target.type && a.target_id === target.id));
      }),
  };

  function adminKey(auth: AdminAuth) {
    if (!auth.key.trim()) fail(401, 'admin_unauthorized', 'Chave de administração errada.');
  }

  /** Like Operator.check: every write names who acted and why. */
  function operator(auth: AdminAuth, reason: string): string {
    adminKey(auth);
    const name = (auth.operator ?? '').trim();
    if (name.length < 2 || name.length > 60) fail(400, 'operator_required', 'Diz quem és: o nome do operador vai para o registo.');
    if (reason.trim().length < 5) fail(400, 'reason_required', 'Escreve o motivo (pelo menos 5 caracteres); fica no registo.');
    return name;
  }

  function audit(op: string, action: string, targetType: string, targetId: string, reason: string, details: Record<string, unknown> = {}) {
    s.audit.unshift({ id: nextId(), operator: op, action, target_type: targetType, target_id: targetId, reason: reason.trim(), details, created_at: iso() });
  }

  function adminUser(u: MockUser): AdminUser {
    return { ...publicUser(u), suspended_at: u.suspended_at, suspended_reason: u.suspended_reason };
  }

  function publicCall(c: State['usage'][number]): UsageCall {
    return strip(c, 'site_id', 'user_id') as UsageCall;
  }

  /** Credits charged and refunded for a site's jobs (all sites when siteId is null). */
  function credits(siteId: string | null, inRange: (t: string) => boolean): [number, number] {
    let charged = 0;
    let refunded = 0;
    for (const l of s.ledger) {
      if (!l.job_id || !inRange(l.created_at)) continue;
      const job = s.jobs.find((j) => j.id === l.job_id);
      if (!job || (siteId && job.site_id !== siteId)) continue;
      if (l.kind === 'charge') charged -= l.delta;
      if (l.kind === 'refund') refunded += l.delta;
    }
    return [charged, refunded];
  }
}

/** For tests and the "Limpar dados" button in mock mode. */
export function resetMock() {
  try {
    localStorage.removeItem(KEY);
  } catch {
    /* nothing stored */
  }
}
