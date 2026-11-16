// Serves published Faz sites from R2.
//
//   nome.faz.co.mz/            -> R2 live/nome/index.html
//   www.cliente.co.mz/         -> R2 domains/www.cliente.co.mz.json -> live/<subdomain>/index.html
//
// Pages are single self contained HTML files, so the Worker only serves "/"
// (and /index.html, /robots.txt). The CSP below limits what a generated page
// can load or send: it matches the hosts the build checks allow.

const CSP = [
  "default-src 'none'",
  "img-src 'self' data:",
  "style-src 'unsafe-inline' https://fonts.googleapis.com",
  "font-src https://fonts.gstatic.com",
  "script-src 'unsafe-inline' https://cdnjs.cloudflare.com https://cdn.jsdelivr.net https://unpkg.com",
  "connect-src 'none'",
  "form-action 'self' mailto:",
  "base-uri 'none'",
  "frame-ancestors 'self'",
].join('; ');

const SECURITY = {
  'Content-Security-Policy': CSP,
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'strict-origin-when-cross-origin',
  'Permissions-Policy': 'camera=(), microphone=(), geolocation=(), payment=()',
  'Strict-Transport-Security': 'max-age=31536000',
};

const NAME = /^[a-z0-9](?:[a-z0-9-]{1,38}[a-z0-9])$/;

function notFound(env) {
  const home = `https://${env.SITES_DOMAIN}`;
  const body = `<!DOCTYPE html><html lang="pt-MZ"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Site não encontrado</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;font:16px/1.5 system-ui,sans-serif;background:#fafaf7;color:#1b1b18}main{max-width:32rem;padding:24px}a{color:inherit}</style></head><body><main><h1>Este site ainda não existe</h1><p>O endereço está livre. Podes criar o teu site em <a href="${home}">${env.SITES_DOMAIN}</a>.</p></main></body></html>`;
  return new Response(body, { status: 404, headers: { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'public, max-age=30', ...SECURITY } });
}

// resolve maps a hostname to a published subdomain name, or null.
async function resolve(host, env) {
  const suffix = '.' + env.SITES_DOMAIN;
  if (host.endsWith(suffix)) {
    const name = host.slice(0, -suffix.length);
    return NAME.test(name) && !name.includes('--') ? name : null;
  }
  // A customer's own domain, written by the API when the domain is connected.
  const mapping = await env.SITES.get(`domains/${host}.json`);
  if (!mapping) return null;
  try {
    const { subdomain } = await mapping.json();
    return typeof subdomain === 'string' && NAME.test(subdomain) ? subdomain : null;
  } catch {
    return null;
  }
}

export default {
  async fetch(request, env) {
    if (request.method !== 'GET' && request.method !== 'HEAD') {
      return new Response('Method not allowed', { status: 405, headers: { Allow: 'GET, HEAD' } });
    }
    const url = new URL(request.url);
    const host = url.hostname.toLowerCase();
    if (host === env.SITES_DOMAIN || host === 'www.' + env.SITES_DOMAIN) {
      return Response.redirect(env.APP_URL || `https://app.${env.SITES_DOMAIN}`, 302);
    }
    const name = await resolve(host, env);
    if (!name) return notFound(env);

    if (url.pathname === '/robots.txt') {
      return new Response('User-agent: *\nAllow: /\n', { headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Cache-Control': 'public, max-age=3600' } });
    }
    if (url.pathname !== '/' && url.pathname !== '/index.html') {
      return Response.redirect(`${url.protocol}//${url.host}/`, 301);
    }

    const obj = await env.SITES.get(`live/${name}/index.html`, { onlyIf: request.headers });
    if (!obj) return notFound(env);

    const headers = new Headers({
      'Content-Type': 'text/html; charset=utf-8',
      'Cache-Control': obj.httpMetadata?.cacheControl || 'public, max-age=60',
      ETag: obj.httpEtag,
      ...SECURITY,
    });
    // R2 returns an object without a body when the conditional matched.
    if (!('body' in obj) || obj.body === undefined) {
      return new Response(null, { status: 304, headers });
    }
    return new Response(request.method === 'HEAD' ? null : obj.body, { status: 200, headers });
  },
};
