import { ApiError, createHttpApi } from '../client';

function fakeFetch(status: number, body: unknown) {
  return vi.fn(async (_url: string, _init?: RequestInit) => new Response(body === undefined ? '' : JSON.stringify(body), { status }));
}

describe('createHttpApi', () => {
  it('sends the bearer token and JSON body', async () => {
    const f = fakeFetch(200, { messages: [], credits_charged: 0, kind: 'other' });
    const api = createHttpApi({ baseUrl: 'https://api.test/', getToken: () => 'tok', fetch: f as unknown as typeof fetch });
    await api.sendMessage('site 1', 'olá');
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe('https://api.test/v1/sites/site%201/messages');
    expect(init?.method).toBe('POST');
    expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer tok');
    expect(init?.body).toBe(JSON.stringify({ text: 'olá' }));
  });

  it('turns the error envelope into an ApiError', async () => {
    const f = fakeFetch(402, { error: { code: 'insufficient_credits', message: 'Não tens créditos suficientes.', details: { needed: 5, balance: 0 } } });
    const api = createHttpApi({ baseUrl: '', getToken: () => 'tok', fetch: f as unknown as typeof fetch });
    const err = await api.sendMessage('s', 'muda a cor').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 402, code: 'insufficient_credits', details: { needed: 5, balance: 0 } });
  });

  it('signs out on 401 only when a token was sent', async () => {
    const onUnauthorized = vi.fn();
    const f = fakeFetch(401, { error: { code: 'unauthenticated', message: 'Entra outra vez.' } });
    let token: string | null = 'old';
    const api = createHttpApi({ baseUrl: '', getToken: () => token, onUnauthorized, fetch: f as unknown as typeof fetch });
    await api.me().catch(() => undefined);
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
    token = null;
    await api.login({ email: 'a@b.co', password: 'x' }).catch(() => undefined);
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
  });

  it('reports a network failure in Portuguese', async () => {
    const f = vi.fn(async () => {
      throw new TypeError('Failed to fetch');
    });
    const api = createHttpApi({ baseUrl: '', getToken: () => null, fetch: f as unknown as typeof fetch });
    await expect(api.pricing()).rejects.toMatchObject({ code: 'network', status: 0 });
  });

  it('unwraps list envelopes', async () => {
    const f = fakeFetch(200, { sites: [{ id: 'a' }] });
    const api = createHttpApi({ baseUrl: '', getToken: () => 't', fetch: f as unknown as typeof fetch });
    expect(await api.sites()).toEqual([{ id: 'a' }]);
  });
});
