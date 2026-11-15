// The page the mock API "builds". It stands in for a generated site so the
// editor's preview, device toggle and publish flow can be tried offline.

import { sampleContent } from '../lib/examples';

const accents = [
  { bg: '#F6F1E7', ink: '#1B1A17', accent: '#0F5A5C', soft: '#E2D9C8' },
  { bg: '#FBF6EE', ink: '#2A1B14', accent: '#B4452A', soft: '#EBDDCB' },
  { bg: '#F3F1EC', ink: '#17191C', accent: '#2C3E66', soft: '#DEDAD0' },
  { bg: '#F7F0F2', ink: '#24161C', accent: '#7A2E4A', soft: '#E8D7DC' },
];

const esc = (v: string) =>
  v.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);

export function renderSamplePage(p: { name: string; type: string; prompt: string; accent: number; version: number }): string {
  const c = sampleContent(p.type);
  const a = accents[p.accent % accents.length]!;
  const items = c.items
    .map(
      ([name, note, price]) =>
        `<li><div><strong>${esc(name)}</strong><span>${esc(note)}</span></div><b>${esc(price)}</b></li>`,
    )
    .join('');
  return `<!DOCTYPE html>
<html lang="pt-MZ"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(p.name)}</title>
<style>
:root{--bg:${a.bg};--ink:${a.ink};--accent:${a.accent};--soft:${a.soft}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.55 system-ui,-apple-system,"Segoe UI",sans-serif}
.wrap{max-width:1040px;margin:0 auto;padding:0 24px}
header{display:flex;justify-content:space-between;align-items:center;padding:20px 0;border-bottom:1px solid var(--soft)}
header b{font:600 20px Georgia,serif}header a{color:var(--ink);font-size:14px;text-decoration:none;margin-left:18px}
.hero{display:grid;grid-template-columns:1.1fr .9fr;gap:40px;align-items:center;padding:64px 0}
.tag{font-size:13px;color:var(--accent);letter-spacing:.02em}
h1{font:400 clamp(36px,6vw,60px)/1.05 Georgia,serif;margin:12px 0 18px;letter-spacing:-.01em}
.lead{font-size:18px;max-width:34em;margin:0 0 28px;opacity:.85}
.btn{display:inline-block;background:var(--accent);color:#fff;padding:14px 22px;border-radius:999px;text-decoration:none;font-weight:600}
.photo{aspect-ratio:4/5;border-radius:18px;background:repeating-linear-gradient(135deg,var(--soft) 0 14px,transparent 14px 28px),var(--bg);border:1px solid var(--soft);display:grid;place-items:center;color:var(--accent);font-size:13px}
section{padding:48px 0;border-top:1px solid var(--soft)}h2{font:400 34px Georgia,serif;margin:0 0 24px}
ul{list-style:none;margin:0;padding:0;display:grid;gap:0}li{display:flex;justify-content:space-between;gap:16px;padding:16px 0;border-bottom:1px dashed var(--soft)}
li span{display:block;font-size:14px;opacity:.7}li b{white-space:nowrap}
.contact{display:grid;grid-template-columns:1fr 1fr;gap:24px}.mark{background:#fff3c4;border-radius:4px;padding:0 4px}
footer{padding:32px 0 48px;font-size:13px;opacity:.7}
@media (max-width:720px){.hero,.contact{grid-template-columns:1fr}.hero{padding:40px 0}header a:not(:last-child){display:none}}
</style></head><body>
<div class="wrap">
<header><b>${esc(p.name)}</b><nav><a href="#lista">${esc(c.list)}</a><a href="#contacto">Contacto</a><a href="https://wa.me/258840000000" target="_blank" rel="noopener">WhatsApp</a></nav></header>
<div class="hero"><div><div class="tag">${esc(c.tag)}</div><h1>${esc(c.headline)}</h1><p class="lead">${esc(c.lead)}</p>
<a class="btn" href="https://wa.me/258840000000" target="_blank" rel="noopener">${esc(c.cta)} pelo WhatsApp</a></div>
<div class="photo" aria-label="Espaço para uma foto">espaço para a tua foto</div></div>
<section id="lista"><h2>${esc(c.list)}</h2><ul>${items}</ul></section>
<section id="contacto" class="contact"><div><h2>${esc(c.contact)}</h2><p>${esc(c.hours)}</p></div>
<div><p>${esc(c.address)}</p><p>Entrega: <span class="mark">por confirmar</span></p><p>Email: <span class="mark">por confirmar</span></p></div></section>
<footer>© ${new Date().getFullYear()} ${esc(p.name)} · versão ${p.version} de demonstração</footer>
</div></body></html>`;
}
