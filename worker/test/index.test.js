// Run with: npm test (in worker/)
import { test } from 'node:test';
import assert from 'node:assert/strict';
import worker from '../src/index.js';

// fakeR2 imitates the parts of the R2 binding the Worker uses, including
// conditional GETs: a matching If-None-Match returns an object with no body.
function fakeR2(objects) {
  return {
    async get(key, opts = {}) {
      const value = objects[key];
      if (value === undefined) return null;
      const etag = `"${key.length}-${value.length}"`;
      const meta = { httpEtag: etag, httpMetadata: { cacheControl: 'public, max-age=60' }, async json() { return JSON.parse(value); } };
      const inm = opts.onlyIf?.get?.('If-None-Match');
      if (inm && inm === etag) return meta;
      return { ...meta, body: value };
    },
  };
}

const env = {
  SITES_DOMAIN: 'faz.co.mz',
  APP_URL: 'https://app.faz.co.mz',
  SITES: fakeR2({
    'live/mel-niassa/index.html': '<!DOCTYPE html><title>Mel</title>',
    'domains/www.melniassa.co.mz.json': JSON.stringify({ subdomain: 'mel-niassa' }),
  }),
};

const get = (url, headers = {}) => worker.fetch(new Request(url, { headers }), env);

test('serves a published subdomain with security headers', async () => {
  const res = await get('https://mel-niassa.faz.co.mz/');
  assert.equal(res.status, 200);
  assert.equal(await res.text(), '<!DOCTYPE html><title>Mel</title>');
  assert.match(res.headers.get('Content-Type'), /text\/html/);
  assert.match(res.headers.get('Content-Security-Policy'), /connect-src 'none'/);
  assert.ok(res.headers.get('ETag'));
});

test('serves a connected custom domain', async () => {
  const res = await get('https://www.melniassa.co.mz/');
  assert.equal(res.status, 200);
});

test('answers 304 when the ETag matches', async () => {
  const first = await get('https://mel-niassa.faz.co.mz/');
  const res = await get('https://mel-niassa.faz.co.mz/', { 'If-None-Match': first.headers.get('ETag') });
  assert.equal(res.status, 304);
});

test('unknown and invalid names get the not found page', async () => {
  for (const url of ['https://livre.faz.co.mz/', 'https://a--b.faz.co.mz/', 'https://x.y.faz.co.mz/', 'https://outro.co.mz/']) {
    const res = await get(url);
    assert.equal(res.status, 404, url);
    assert.match(await res.text(), /Este site ainda não existe/);
  }
});

test('other paths redirect to the page, robots is allowed', async () => {
  const res = await get('https://mel-niassa.faz.co.mz/contactos');
  assert.equal(res.status, 301);
  assert.equal(res.headers.get('Location'), 'https://mel-niassa.faz.co.mz/');
  const robots = await get('https://mel-niassa.faz.co.mz/robots.txt');
  assert.equal(await robots.text(), 'User-agent: *\nAllow: /\n');
});

test('the bare domain goes to the app, writes are refused', async () => {
  const res = await get('https://faz.co.mz/');
  assert.equal(res.status, 302);
  assert.equal(res.headers.get('Location'), 'https://app.faz.co.mz/');
  const post = await worker.fetch(new Request('https://mel-niassa.faz.co.mz/', { method: 'POST' }), env);
  assert.equal(post.status, 405);
});
