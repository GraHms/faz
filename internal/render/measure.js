// Injected at the end of <body>. After the page and its fonts settle, it
// measures what a reviewer would otherwise have to eyeball, then writes the
// result into the DOM as base64 JSON for the headless dump to pick up.
(function () {
  var MAX = 12;

  function sel(el) {
    if (!el || !el.tagName) return '';
    var s = el.tagName.toLowerCase();
    if (el.id) return s + '#' + el.id;
    if (el.classList && el.classList.length) s += '.' + Array.prototype.slice.call(el.classList, 0, 2).join('.');
    var sec = el.closest && el.closest('section[id], header, footer, main > [id]');
    if (sec && sec !== el) s = sel(sec) + ' ' + s;
    return s;
  }

  function ownText(el) {
    var t = '';
    for (var i = 0; i < el.childNodes.length; i++) {
      var n = el.childNodes[i];
      if (n.nodeType === 3) t += n.nodeValue;
    }
    return t.replace(/\s+/g, ' ').trim();
  }

  function parseColor(s) {
    if (!s) return null;
    var m = s.match(/^rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:\s*[,/]\s*([\d.]+%?))?\s*\)$/);
    if (m) {
      var a = m[4] === undefined ? 1 : (m[4].slice(-1) === '%' ? parseFloat(m[4]) / 100 : parseFloat(m[4]));
      return [parseFloat(m[1]), parseFloat(m[2]), parseFloat(m[3]), a];
    }
    m = s.match(/^color\(srgb\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+%?))?\s*\)$/);
    if (m) {
      var a2 = m[4] === undefined ? 1 : (m[4].slice(-1) === '%' ? parseFloat(m[4]) / 100 : parseFloat(m[4]));
      return [m[1] * 255, m[2] * 255, m[3] * 255, a2];
    }
    return null; // oklch, lab and others are skipped, not guessed
  }

  function blend(top, bottom) {
    var a = top[3];
    return [top[0] * a + bottom[0] * (1 - a), top[1] * a + bottom[1] * (1 - a), top[2] * a + bottom[2] * (1 - a), 1];
  }

  function lum(c) {
    function ch(v) { v /= 255; return v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }
    return 0.2126 * ch(c[0]) + 0.7152 * ch(c[1]) + 0.0722 * ch(c[2]);
  }

  function ratio(a, b) {
    var x = lum(a), y = lum(b);
    if (x < y) { var t = x; x = y; y = t; }
    return (x + 0.05) / (y + 0.05);
  }

  // Effective background: stack translucent layers up to the first opaque
  // one. Returns null when an image or gradient sits behind the text.
  function background(el) {
    var layers = [];
    for (var n = el; n && n.nodeType === 1; n = n.parentElement) {
      var cs = getComputedStyle(n);
      if (cs.backgroundImage && cs.backgroundImage !== 'none') return null;
      var c = parseColor(cs.backgroundColor);
      if (c === null && cs.backgroundColor && cs.backgroundColor !== 'transparent') return null;
      if (c && c[3] > 0) {
        layers.push(c);
        if (c[3] >= 1) break;
      }
    }
    var base = [255, 255, 255, 1];
    if (layers.length && layers[layers.length - 1][3] >= 1) base = layers.pop();
    for (var i = layers.length - 1; i >= 0; i--) base = blend(layers[i], base);
    return base;
  }

  function effectiveOpacity(el) {
    var o = 1;
    for (var n = el; n && n.nodeType === 1; n = n.parentElement) {
      var cs = getComputedStyle(n);
      o *= parseFloat(cs.opacity);
      if (cs.visibility === 'hidden') return 0;
    }
    return o;
  }

  function inScrollableBox(el) {
    for (var n = el.parentElement; n && n !== document.body; n = n.parentElement) {
      var ox = getComputedStyle(n).overflowX;
      if (ox === 'auto' || ox === 'scroll' || ox === 'hidden' || ox === 'clip') return true;
    }
    return false;
  }

  function run() {
    var out = {};
    var de = document.documentElement, body = document.body;
    var vw = window.innerWidth;
    out.viewport_width = vw;
    out.scroll_width = Math.max(de.scrollWidth, body ? body.scrollWidth : 0);
    out.doc_height = Math.max(de.scrollHeight, body ? body.scrollHeight : 0);
    out.lang = de.getAttribute('lang') || '';
    out.theme = de.getAttribute('data-theme') || '';
    out.errors = (window.__sgErrors || []).slice(0, MAX);

    out.overflow = [];
    out.hidden_text = [];
    out.low_contrast = [];
    out.contrast_checked = 0;
    out.contrast_skipped = 0;

    var all = body ? body.querySelectorAll('*') : [];
    for (var i = 0; i < all.length; i++) {
      var el = all[i];
      var tag = el.tagName.toLowerCase();
      if (/^(script|style|noscript|template|svg|path|g|defs|title)$/.test(tag)) continue;
      if (el.closest('[aria-hidden="true"]')) continue;
      var cs = getComputedStyle(el);
      if (cs.display === 'none') continue;
      var r = el.getBoundingClientRect();

      if (r.width > 0 && r.right > vw + 1 && cs.position !== 'fixed' && !inScrollableBox(el) && out.overflow.length < MAX) {
        out.overflow.push({ where: sel(el), right: Math.round(r.right), width: Math.round(r.width) });
      }

      var text = ownText(el);
      if (!text) continue;
      var srOnly = r.width <= 1 && r.height <= 1;
      if (srOnly) continue;

      if (effectiveOpacity(el) < 0.05 && out.hidden_text.length < MAX) {
        out.hidden_text.push({ where: sel(el), text: text.slice(0, 60) });
        continue;
      }

      var fg = parseColor(cs.color);
      var bg = background(el);
      if (!fg || !bg) { out.contrast_skipped++; continue; }
      if (fg[3] < 1) fg = blend(fg, bg);
      out.contrast_checked++;
      var size = parseFloat(cs.fontSize), weight = parseInt(cs.fontWeight, 10) || 400;
      var large = size >= 24 || (size >= 18.66 && weight >= 700);
      var need = large ? 3 : 4.5;
      var got = ratio(fg, bg);
      if (got < need && out.low_contrast.length < MAX) {
        out.low_contrast.push({ where: sel(el), text: text.slice(0, 40), ratio: Math.round(got * 100) / 100, need: need, size_px: size });
      }
    }

    out.small_targets = [];
    var targets = document.querySelectorAll('a[href], button, input:not([type=hidden]), select, textarea, [role=button]');
    for (var t = 0; t < targets.length; t++) {
      var tg = targets[t], tcs = getComputedStyle(tg);
      if (tcs.display === 'none' || tcs.display === 'inline') continue; // inline links in running text are exempt
      var tr = tg.getBoundingClientRect();
      if (tr.width === 0 && tr.height === 0) continue;
      if ((tr.height < 44 || tr.width < 24) && out.small_targets.length < MAX) {
        out.small_targets.push({ where: sel(tg), w: Math.round(tr.width), h: Math.round(tr.height) });
      }
    }

    out.images_no_alt = [];
    out.images_broken = [];
    var imgs = document.images;
    for (var k = 0; k < imgs.length; k++) {
      if (!imgs[k].hasAttribute('alt')) out.images_no_alt.push(sel(imgs[k]));
      if (imgs[k].complete && imgs[k].naturalWidth === 0) out.images_broken.push((imgs[k].getAttribute('src') || '').slice(0, 80));
    }

    out.unlabeled_controls = [];
    var controls = document.querySelectorAll('input:not([type=hidden]):not([type=submit]):not([type=button]), select, textarea');
    for (var q = 0; q < controls.length; q++) {
      var ctl = controls[q];
      var named = (ctl.labels && ctl.labels.length) || ctl.getAttribute('aria-label') || ctl.getAttribute('aria-labelledby');
      if (!named) out.unlabeled_controls.push(sel(ctl));
    }

    var hs = document.querySelectorAll('h1, h2, h3, h4, h5, h6');
    out.h1_count = document.querySelectorAll('h1').length;
    out.heading_skips = [];
    var last = 0;
    for (var h = 0; h < hs.length; h++) {
      var lvl = parseInt(hs[h].tagName.slice(1), 10);
      if (last && lvl > last + 1) out.heading_skips.push(hs[h].tagName.toLowerCase() + ' after h' + last + ': ' + ownText(hs[h]).slice(0, 40));
      last = lvl;
    }

    out.fonts = [];
    if (document.fonts && document.fonts.forEach) {
      var seen = {};
      document.fonts.forEach(function (f) {
        var key = f.family.replace(/["']/g, '') + ' ' + f.weight;
        if (!seen[key]) { seen[key] = 1; out.fonts.push({ family: f.family.replace(/["']/g, ''), weight: String(f.weight), status: f.status }); }
      });
    }

    var marks = document.querySelectorAll('mark.ph');
    out.placeholders_shown = 0;
    for (var p = 0; p < marks.length; p++) {
      var mr = marks[p].getBoundingClientRect();
      if (mr.width > 0 && mr.height > 0) out.placeholders_shown++;
    }
    out.placeholders_total = marks.length;

    var pre = document.createElement('pre');
    pre.id = '__faz_metrics';
    pre.textContent = btoa(unescape(encodeURIComponent(JSON.stringify(out))));
    // Phone widths render inside a same origin srcdoc iframe, because
    // headless windows cannot be narrower than 500px. The dump only
    // contains the top document, so write the result there.
    var target = document.body;
    try { if (window.parent !== window && window.parent.document.body) target = window.parent.document.body; } catch (e) {}
    target.appendChild(pre);
  }

  function settle() {
    var fontsReady = document.fonts && document.fonts.ready ? document.fonts.ready : Promise.resolve();
    var timeout = new Promise(function (res) { setTimeout(res, 2500); });
    Promise.race([fontsReady, timeout]).then(function () { setTimeout(run, 400); });
  }

  if (document.readyState === 'complete') settle();
  else window.addEventListener('load', settle);
})();
