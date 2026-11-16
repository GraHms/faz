import type {
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
  SiteView,
} from './types';

/** Everything the app asks of fazd. The HTTP client and the mock both implement it. */
export interface Api {
  pricing(): Promise<Pricing>;

  signup(input: { name: string; email: string; password: string }): Promise<Session>;
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

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' };
    const token = opts.getToken();
    if (token) headers.Authorization = `Bearer ${token}`;
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
  };
}
