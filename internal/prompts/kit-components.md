# The Faz kit

Shadcn-style components as Tailwind v4 classes. Tailwind utilities work everywhere; the classes below are the kit's components. They read the theme tokens, so they follow the page's identity and both themes.

## Tokens (as Tailwind colors)

`background`, `foreground`, `card`, `card-foreground`, `muted`, `muted-foreground`, `border`, `input`, `primary`, `primary-foreground`, `accent`, `accent-foreground`, `inverse`, `inverse-foreground`, `ring`. Use them as `bg-*`, `text-*`, `border-*`, with opacity where useful (`bg-accent/10`). Fonts: `font-sans`, `font-display`. Radius: the components use `--radius-kit`.

## Components

- Layout: `page` (centered container with gutters), `section` (vertical rhythm), `section-muted` (tinted section background).
- Buttons: `btn` plus one of `btn-primary`, `btn-accent`, `btn-outline`, `btn-ghost`, `btn-inverse` (on dark bands); add `btn-lg` for main actions, `btn-icon` for square icon buttons, `w-full` on phones when it helps.
- Badge: `badge`, `badge-accent`, with an optional `<span class="badge-dot"></span>`.
- Surfaces: `card`, `card-raised` (the one card that should float, such as a form), `card-muted` (a quiet inset), `band` (dark full-width block, use `bg-inverse` tokens; add `<div class="glow pointer-events-none absolute inset-0"></div>` inside for an accent glow).
- `icon-tile` (accent-tinted square holding an icon), `icon-tile-lg`.
- `photo` for client images, always with an aspect ratio class.
- Facts: `stats` (a `<dl>` of two to four short facts in a row), `price-row` with `price`, `hours` (a `<dl>` of days and times).
- Forms: `field` (a `<label>` wrapping its text and control), `input` (inputs, selects, textareas).
- Accordion: `<details class="acc">` with a `<summary>` and the answer after it.
- `nav` (sticky top bar), `action-bar` (fixed phone bar), `dots` (fading dot grid behind a hero), `glow`.
- Placeholders: `<mark class="ph">` is already styled.

## Icons

Write `<i data-icon="name" class="size-4"></i>`; it becomes an inline Lucide SVG in the current text color. Sizes: `size-4` in buttons and lists, `size-5` in an `icon-tile`, `size-6` in an `icon-tile-lg`. Use only these names:

{{ICONS}}

## Section patterns

Pick per section; adapt freely with utilities. Keep the structure, change the content.

Sticky nav:
```html
<header class="nav"><div class="page flex h-16 items-center justify-between gap-4">
  <a href="#" class="flex items-center gap-2.5 font-semibold">Name</a>
  <a class="btn btn-primary" href="https://wa.me/...">Primary action</a>
</div></header>
```

Split hero, text with a raised card (a form, a price list or the bold move):
```html
<section class="relative overflow-hidden">
  <div class="dots pointer-events-none absolute inset-0"></div>
  <div class="page relative grid gap-12 pt-12 pb-16 md:grid-cols-[1.1fr_.9fr] md:items-center md:pt-20 md:pb-24">
    <div>
      <span class="badge"><span class="badge-dot"></span>A fact from the copy</span>
      <h1 class="mt-5 text-4xl leading-[1.05] font-semibold tracking-tight md:text-6xl">Headline</h1>
      <p class="mt-5 max-w-xl text-lg text-muted-foreground">Lead.</p>
      <div class="mt-7 flex flex-wrap gap-3"><a class="btn btn-primary btn-lg" href="#"><i data-icon="whatsapp" class="size-[18px]"></i>Action</a><a class="btn btn-outline btn-lg" href="tel:+258...">Ligar</a></div>
      <dl class="stats mt-10 max-w-xl"><div><dt>Label</dt><dd>Value</dd></div>...</dl>
    </div>
    <div class="card-raised">...</div>
  </div>
</section>
```

Photo hero: the same grid with `<img class="photo aspect-[4/5] md:aspect-square" src="assets/..." alt="...">` in place of the card.

Bento grid for services or features (one card leads, the rest follow):
```html
<div class="grid grid-cols-2 gap-3 md:grid-cols-3 md:gap-4">
  <article class="card col-span-2 flex flex-col md:col-span-1 md:row-span-2"><span class="icon-tile"><i data-icon="..." class="size-5"></i></span><h3 class="mt-5 text-xl font-semibold tracking-tight">Lead item</h3><p class="mt-2 text-muted-foreground">...</p><div class="card-muted mt-auto">a price or key fact</div></article>
  <article class="card p-4 md:p-6"><span class="icon-tile"><i data-icon="..." class="size-5"></i></span><h3 class="mt-4 font-semibold">Item</h3><p class="mt-1.5 text-sm text-muted-foreground">...</p></article>
</div>
```

Price list:
```html
<div class="card divide-y divide-border py-2">
  <div class="price-row"><div><h3 class="font-medium">Service</h3><p class="text-sm text-muted-foreground">detail</p></div><p class="price"><small class="mr-1 text-xs font-normal text-muted-foreground">a partir de</small>1500 MT</p></div>
</div>
```

WhatsApp composer, for quotes, orders or bookings (the visitor picks options, the script writes the message):
```html
<form id="pedido" class="card-raised grid gap-4" novalidate>
  <label class="field" for="f-servico">Serviço<select id="f-servico" name="servico" class="input"><option>...</option></select></label>
  <button class="btn btn-primary btn-lg w-full"><i data-icon="whatsapp" class="size-[18px]"></i>Enviar pelo WhatsApp</button>
</form>
<script>
document.getElementById('pedido').addEventListener('submit', function (e) {
  e.preventDefault();
  var f = new FormData(this);
  location.href = 'https://wa.me/258XXXXXXXXX?text=' + encodeURIComponent('Olá, ... ' + f.get('servico'));
});
</script>
```

Contact and hours:
```html
<div class="card"><ul class="grid gap-5">
  <li class="flex gap-3"><span class="icon-tile size-9"><i data-icon="map-pin" class="size-4"></i></span><div><p class="font-medium">Address</p><p class="text-sm text-muted-foreground">landmark</p></div></li>
</ul>
<dl class="hours mt-6"><div><dt>Terça a sábado</dt><dd>8h às 18h</dd></div></dl></div>
```

Questions (only when the copy has them): `<details class="acc" open><summary>Question<i data-icon="chevron-down" class="size-4"></i></summary><p>Answer.</p></details>`.

Dark band: `<div class="band"><div class="glow pointer-events-none absolute inset-0"></div><div class="relative grid gap-8 md:grid-cols-[1.2fr_.8fr] md:items-end">...<a class="btn btn-inverse btn-lg">Action</a></div></div>`.

Gallery: `<div class="grid grid-cols-2 gap-3 md:grid-cols-3"><img class="photo aspect-square" ...></div>`.

Phone action bar, last element of `body`:
```html
<div class="action-bar"><div class="grid grid-cols-[auto_1fr] gap-2">
  <a class="btn btn-outline btn-lg btn-icon" href="tel:+258..." aria-label="Ligar"><i data-icon="phone" class="size-[18px]"></i></a>
  <a class="btn btn-primary btn-lg" href="https://wa.me/..."><i data-icon="whatsapp" class="size-[18px]"></i>Primary action</a>
</div></div>
```

Footer: `<footer class="border-t border-border"><div class="page flex flex-col gap-2 py-8 text-sm text-muted-foreground md:flex-row md:justify-between">...</div></footer>`.
