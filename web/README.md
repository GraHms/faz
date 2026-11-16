# Faz web app

The owner facing app for Faz: landing, pricing, sign up, the dashboard, the editor (questions, build progress, chat, preview, publish) and the M-Pesa and e-Mola top up. React 19, Vite, TypeScript, React Router and TanStack Query. The look is a port of the Claude Design prototype (`design/faz-prototype.dc.html` in the original upload) into tokens and components.

```
web/
  src/styles/tokens.css      colours, type, radii and layout from the prototype
  src/components/            Button, Field, Modal, Pill, Logo, PromptCard, TopUpModal, layouts
  src/api/                   typed client for fazd, its types, and the in-browser mock
  src/auth/session.tsx       token storage and the signed-in guard
  src/hooks/queries.ts       queries, and polling for jobs and payments
  src/pages/                 one folder or file per screen; editor/ holds the workspace
  e2e/run.mjs                the whole owner journey in real Chrome, with screenshots
```

## Run it

```sh
npm install

# Against fazd (see the root README): the dev server proxies /v1 to it.
FAZ_API=http://127.0.0.1:8080 npm run dev

# Without a backend: an imitation of the API in the browser, kept in localStorage.
npm run dev:mock
```

| Variable | Default | What it does |
|---|---|---|
| `VITE_API_MODE` | `real` | `mock` runs the in-browser API instead of calling fazd |
| `VITE_API_URL` | empty | The API origin in production. Empty means the app's own origin |
| `VITE_SITES_DOMAIN` | `faz.co.mz` | Where published sites live |
| `FAZ_API` | `http://127.0.0.1:8080` | Dev and preview proxy target for `/v1` and `/healthz` |

When the app and the API are on different origins, list the app's origin in fazd's `FAZ_APP_ORIGINS`.

### The mock

It keeps the rules the screens depend on: the 19 credit gift, free questions, charged edits and the 402 that opens the top up, busy sites, subdomain checks and wallet prefixes. Builds take about 15 seconds and report the same stages as the real pipeline. Two hooks make failures easy to see: a phone number ending in `0000` makes a payment fail, and a chat message containing "estraga" makes an edit fail and refund. "Apagar dados de demonstração" in the account menu resets it. It is a design aid, not a test of the backend.

## How it talks to fazd

- **Auth:** bearer token in `localStorage` (`faz.token`). The API takes no cookies, so other sites cannot ride the session; a 401 signs the owner out.
- **Errors:** the API's messages are written for owners, in Portuguese, so the app shows them as they come. Codes drive behaviour: `insufficient_credits` opens the top up modal with `needed`, `site_busy` explains the wait, field codes such as `email_taken` land on their field.
- **Before the build:** photos go with `POST /v1/sites`, so the questions never ask for photos already sent. The questions card opens with "O que percebi", the intake's summary, and a "Corrigir" link; the correction travels with the answers. After a failed build the stored answers are turned back into form values, and "Construir com estas respostas" retries in one tap.
- **Jobs:** the editor finds the running job from the newest message that carries a `job_id`, polls `GET /v1/jobs/{id}` every 1.5 seconds and shows its `progress`, which the backend records as each pipeline stage starts. The steps on screen are real stages, not a timer.
- **Preview:** a presigned R2 URL in an iframe with `sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox allow-forms"` and no `allow-same-origin`, so a generated page never gets the app's origin or its token.
- **Payments:** `POST /v1/payments`, then `GET /v1/payments/{id}` every 2 seconds until the wallet confirms or refuses.

## Deploy

`npm run build` writes a static site to `dist/`. Serve it from any static host that falls back to `index.html` for unknown paths (Cloudflare Pages does by default when there is no `404.html`), with `VITE_API_URL` set at build time to fazd's origin. Fonts are bundled, so the app makes no third party requests.

## What is not there yet

These have no backend, so the app says so instead of pretending. Custom domains are shown wherever the prototype had them, marked with the shared `<Soon />` badge ("Em breve"): the pricing cell, the domains page, the publish dialog, "Ligar domínio" in the editor and the dashboard's Domínios tab, which lists the free addresses in use. Google sign in and password reset are not offered. The landing examples are labelled as starting points, not as customer sites.

## Admin pages

`/admin` holds the operator tools, behind the admin key and the operator's name (both kept in `sessionStorage` for the tab, sent as `X-Faz-Admin` and `X-Faz-Operator`, never with an owner's session):

- **Telemetria** (`/admin`, `/admin/sites/:id`): cost per site, stage and model.
- **Utilizadores** (`/admin/utilizadores`): search by email, name or M-Pesa number; give or remove credits, refund a job, suspend, end sessions.
- **Sites** (`/admin/sites`): search, take off the air, restore.
- **Convites** (`/admin/convites`): create codes with uses, bonus and end date; copy the link; disable.
- **Controlo** (`/admin/controlo`): close the beta, pause the models, set the daily spend cap with today's spend on a meter, edit reserved names and blocked words.
- **Registo** (`/admin/registo`): the audit log.

Every action asks for a reason in a confirmation dialog; the reason never reaches the owner. Owners see the effects: the invite field on sign up (filled from `?convite=`), "em manutenção" while models are paused, and the takedown notice in the editor.

## Telemetry pages

`/admin` and `/admin/sites/:id` show, for operators, what each site cost: tokens, cost per stage and model, credits charged and margin (see Telemetry in the root README). They ask for the admin key (fazd's `FAZ_ADMIN_TOKEN`), keep it in `sessionStorage` for the tab, and send it as `X-Faz-Admin`, never together with an owner's session. Nothing links to them from the owner screens. In mock mode any key works, and the mock records invented but plausible token counts for every operation.

## Checks

```sh
npm run typecheck     # app and config
npm test              # formatting, the HTTP client, the mock's rules, questions and progress
npm run build

# Real Chrome against a running fazd with FAZ_FAKE_DIR=examples/fake and FAZ_NO_RENDER=1:
npm run build && npx vite preview --port 4173 &
FAZ_WEB=http://127.0.0.1:4173 CHROME=/usr/bin/google-chrome npm run e2e
```

The e2e script signs up, answers the questions, waits for the build, hits the 402, pays with the mock wallet, edits, publishes, checks the dashboard and payments, repeats key screens at phone size and signs out and in. It fails on any page error, console error or failed request from the app itself (requests made by the generated page inside the preview are its own). Screenshots land in `e2e/screenshots/`. With `FAZ_ADMIN_TOKEN` set, it also opens the telemetry pages, tries a wrong key, checks the site page against the API, and walks the operator tools: it creates an invite, closes the beta, signs a new owner up through the invite link and checks the bonus, gives credits, pauses the models and sees the owner told so, takes a site down and restores it, and reads the audit log. It resets the settings to an open beta first, so runs can share a database.
