import type {
  AdminAuth,
  AdminSiteRow,
  AdminStatus,
  AdminUser,
  AdminUserDetail,
  AdminUserRow,
  AuditEntry,
  CreditAdjustment,
  Invite,
  NewInvite,
  PublicConfig,
  SettingsPatch,
  Account,
  ApiErrorBody,
  Asset,
  Availability,
  Credits,
  Job,
  MessageResult,
  Payment,
  Preview,
  Pricing,
  Published,
  Session,
  Site,
  SiteTelemetry,
  SiteView,
  TelemetryOverview,
} from './types';

/** Everything the app asks of fazd. The HTTP client and the mock both implement it. */
export interface Api {
  pricing(): Promise<Pricing>;
  /** Whether sign up needs an invite code right now. */
  config(): Promise<PublicConfig>;

  signup(input: { name: string; email: string; password: string; invite_code?: string }): Promise<Session>;
  login(input: { email: string; password: string }): Promise<Session>;
  logout(): Promise<void>;
  me(): Promise<Account>;
  credits(): Promise<Credits>;

  /** Photos go with the request so the questions are written with them in view. */
  createSite(input: { prompt: string; business_name: string; business_type: string }, files?: File[]): Promise<SiteView>;
  sites(): Promise<Site[]>;
  site(id: string): Promise<SiteView>;
  uploadAsset(siteId: string, file: File): Promise<Asset>;
  answer(siteId: string, answers: Record<string, string>, skip?: boolean, correction?: string): Promise<{ job_id: number }>;
  sendMessage(siteId: string, text: string): Promise<MessageResult>;
  preview(siteId: string, version?: number): Promise<Preview>;
  availability(name: string, siteId?: string): Promise<Availability>;
  publish(siteId: string, subdomain: string): Promise<Published>;

  job(id: number): Promise<Job>;

  topUp(input: { amount_mzn: number; msisdn: string; nuit?: string }): Promise<Payment>;
  payments(): Promise<Payment[]>;
  payment(id: string): Promise<Payment>;

  // Operators only: the admin key (FAZ_ADMIN_TOKEN), never a session. Writes
  // carry the operator's name and a reason, both kept in the audit log.
  /** Days are inclusive, UTC. */
  adminTelemetry(auth: AdminAuth, period?: { from?: string; to?: string }): Promise<TelemetryOverview>;
  adminSiteTelemetry(auth: AdminAuth, siteId: string): Promise<SiteTelemetry>;
  adminStatus(auth: AdminAuth): Promise<AdminStatus>;
  adminUpdateSettings(auth: AdminAuth, patch: SettingsPatch, reason: string): Promise<AdminStatus>;
  adminInvites(auth: AdminAuth): Promise<Invite[]>;
  adminCreateInvite(auth: AdminAuth, invite: NewInvite, reason: string): Promise<Invite>;
  adminDisableInvite(auth: AdminAuth, code: string, reason: string): Promise<Invite>;
  adminUsers(auth: AdminAuth, query: string): Promise<AdminUserRow[]>;
  adminUser(auth: AdminAuth, userId: string): Promise<AdminUserDetail>;
  adminAdjustCredits(auth: AdminAuth, userId: string, adjustment: CreditAdjustment, reason: string): Promise<{ balance: number }>;
  adminSuspend(auth: AdminAuth, userId: string, suspended: boolean, reason: string): Promise<AdminUser>;
  adminSignOut(auth: AdminAuth, userId: string, reason: string): Promise<{ sessions_ended: number }>;
  adminRefundJob(auth: AdminAuth, jobId: number, reason: string): Promise<{ credits: number }>;
  adminSites(auth: AdminAuth, query: string): Promise<AdminSiteRow[]>;
  adminTakeDown(auth: AdminAuth, siteId: string, reason: string): Promise<Site>;
  adminRestore(auth: AdminAuth, siteId: string, reason: string): Promise<Site>;
  adminAudit(auth: AdminAuth, target?: { type: string; id: string }): Promise<AuditEntry[]>;
}



/** An error the API returned, with its machine readable code. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: Record<string, unknown>;

  constructor(status: number, code: string, message: string, details: Record<string, unknown> = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }

  /** The backend's message is written for owners, in Portuguese; use it as is. */
  static network(): ApiError {
    return new ApiError(0, 'network', 'Sem ligação ao servidor. Verifica a internet e tenta outra vez.');
  }
}

export function isApiError(e: unknown, code?: string): e is ApiError {
  return e instanceof ApiError && (code === undefined || e.code === code);
}

export interface HttpOptions {
  /** API origin, or "" for the app's own origin. */
  baseUrl: string;
  getToken: () => string | null;
  /** Called when the API rejects the token, so the app can sign out. */
  onUnauthorized?: () => void;
  fetch?: typeof fetch;
}

export function createHttpApi(opts: HttpOptions): Api {
  const doFetch = opts.fetch ?? globalThis.fetch.bind(globalThis);
  const base = opts.baseUrl.replace(/\/+$/, '');

  async function request<T>(method: string, path: string, body?: unknown, admin?: AdminAuth): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    // The admin key never travels with a user session, and vice versa.
    const token = admin ? null : opts.getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
    if (admin) {
      headers['X-Faz-Admin'] = admin.key;
      if (admin.operator) headers['X-Faz-Operator'] = admin.operator;
    }
    let payload: BodyInit | undefined;
    if (body instanceof FormData) {
      payload = body;
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      payload = JSON.stringify(body);
    }

    let res: Response;
    try {
      res = await doFetch(base + path, { method, headers, body: payload });
    } catch {
      throw ApiError.network();
    }

    const text = await res.text();
    let data: unknown = undefined;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = undefined;
      }
    }
    if (!res.ok) {
      const err = (data as ApiErrorBody | undefined)?.error;
      if (res.status === 401 && token) opts.onUnauthorized?.();
      throw new ApiError(
        res.status,
        err?.code ?? `http_${res.status}`,
        err?.message ?? 'O servidor não respondeu como esperado. Tenta outra vez.',
        err?.details ?? {},
      );
    }
    return data as T;
  }

  const id = encodeURIComponent;

  return {
    pricing: () => request('GET', '/v1/pricing'),
    config: () => request('GET', '/v1/config'),

    signup: (input) => request('POST', '/v1/auth/signup', input),
    login: (input) => request('POST', '/v1/auth/login', input),
    logout: () => request<unknown>('POST', '/v1/auth/logout').then(() => undefined),
    me: () => request('GET', '/v1/me'),
    credits: () => request('GET', '/v1/credits'),

    createSite: (input, files) => {
      if (!files?.length) return request('POST', '/v1/sites', input);
      const form = new FormData();
      form.append('prompt', input.prompt);
      form.append('business_name', input.business_name);
      form.append('business_type', input.business_type);
      for (const f of files) form.append('file', f, f.name);
      return request('POST', '/v1/sites', form);
    },
    sites: () => request<{ sites: Site[] }>('GET', '/v1/sites').then((r) => r.sites),
    site: (siteId) => request('GET', `/v1/sites/${id(siteId)}`),
    uploadAsset: (siteId, file) => {
      const form = new FormData();
      form.append('file', file, file.name);
      return request('POST', `/v1/sites/${id(siteId)}/assets`, form);
    },
    answer: (siteId, answers, skip = false, correction) =>
      request('POST', `/v1/sites/${id(siteId)}/answers`, { answers, skip, correction: correction || undefined }),
    sendMessage: (siteId, text) => request('POST', `/v1/sites/${id(siteId)}/messages`, { text }),
    preview: (siteId, version) =>
      request('GET', `/v1/sites/${id(siteId)}/preview${version ? `?version=${version}` : ''}`),
    availability: (name, siteId) =>
      request('GET', `/v1/subdomains/${id(name)}${siteId ? `?site_id=${id(siteId)}` : ''}`),
    publish: (siteId, subdomain) => request('POST', `/v1/sites/${id(siteId)}/publish`, { subdomain }),

    job: (jobId) => request('GET', `/v1/jobs/${jobId}`),

    topUp: (input) => request('POST', '/v1/payments', input),
    payments: () => request<{ payments: Payment[] }>('GET', '/v1/payments').then((r) => r.payments),
    payment: (paymentId) => request('GET', `/v1/payments/${id(paymentId)}`),

    adminTelemetry: (auth, period = {}) => {
      const q = new URLSearchParams();
      if (period.from) q.set('from', period.from);
      if (period.to) q.set('to', period.to);
      const qs = q.toString();
      return request('GET', `/v1/admin/telemetry${qs ? `?${qs}` : ''}`, undefined, auth);
    },
    adminSiteTelemetry: (auth, siteId) => request('GET', `/v1/admin/sites/${id(siteId)}/telemetry`, undefined, auth),
    adminStatus: (auth) => request('GET', '/v1/admin/status', undefined, auth),
    adminUpdateSettings: (auth, patch, reason) => request('POST', '/v1/admin/settings', { ...patch, reason }, auth),
    adminInvites: (auth) => request<{ invites: Invite[] }>('GET', '/v1/admin/invites', undefined, auth).then((r) => r.invites),
    adminCreateInvite: (auth, invite, reason) => request('POST', '/v1/admin/invites', { ...invite, reason }, auth),
    adminDisableInvite: (auth, code, reason) => request('POST', `/v1/admin/invites/${id(code)}/disable`, { reason }, auth),
    adminUsers: (auth, query) =>
      request<{ users: AdminUserRow[] }>('GET', `/v1/admin/users?q=${id(query)}`, undefined, auth).then((r) => r.users),
    adminUser: (auth, userId) => request('GET', `/v1/admin/users/${id(userId)}`, undefined, auth),
    adminAdjustCredits: (auth, userId, adj, reason) => request('POST', `/v1/admin/users/${id(userId)}/credits`, { ...adj, reason }, auth),
    adminSuspend: (auth, userId, suspended, reason) => request('POST', `/v1/admin/users/${id(userId)}/suspend`, { suspended, reason }, auth),
    adminSignOut: (auth, userId, reason) => request('POST', `/v1/admin/users/${id(userId)}/sign-out`, { reason }, auth),
    adminRefundJob: (auth, jobId, reason) => request('POST', `/v1/admin/jobs/${jobId}/refund`, { reason }, auth),
    adminSites: (auth, query) =>
      request<{ sites: AdminSiteRow[] }>('GET', `/v1/admin/sites?q=${id(query)}`, undefined, auth).then((r) => r.sites),
    adminTakeDown: (auth, siteId, reason) => request('POST', `/v1/admin/sites/${id(siteId)}/takedown`, { reason }, auth),
    adminRestore: (auth, siteId, reason) => request('POST', `/v1/admin/sites/${id(siteId)}/restore`, { reason }, auth),
    adminAudit: (auth, target) =>
      request<{ entries: AuditEntry[] }>(
        'GET',
        `/v1/admin/audit${target ? `?target_type=${id(target.type)}&target_id=${id(target.id)}` : ''}`,
        undefined,
        auth,
      ).then((r) => r.entries),
  };
}
