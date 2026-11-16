# faz

Two things live here:

- **The pipeline** (`cmd/faz`): turns a client's brief into a verified, single file web page.
- **The product** (`cmd/fazd`, `worker/`): the Faz service around it, with accounts, credits, clarifying questions, chat edits, mobile money top ups and publishing to `nome.faz.co.mz`. See [The product](#the-product-fazd) below.

The pipeline turns a client's brief into a verified, single file web page. Seven model stages, deterministic checks between them, a real browser to measure and photograph the result, and one bounded repair pass.

```
brief.md ─▶ intake ─▶ content ─▶ plan ─▶ [plan checks] ─▶ critique ─▶ build
             Sonnet    Sonnet     Opus     contrast,        Opus        Sonnet
                                           defaults,
                                           repeats
build ─▶ [static checks + headless Chrome] ─▶ review ─▶ repair ─▶ [checks again] ─▶ index.html
          desktop and phone, light and dark     Opus      Sonnet                     report.json
```

## Quick start

```sh
go install github.com/GraHms/faz/cmd/faz@latest   # or, from a clone:
go build -o faz ./cmd/faz

# Dry run with recorded model output: no API key, no spend.
./faz -brief examples/briefs/mel-niassa.md -assets examples/assets -out out -fake examples/fake

# Real run.
export ANTHROPIC_API_KEY=...
./faz -brief path/to/brief.md -assets path/to/client-files -out out
```

Needs Go 1.24 and Chrome or Chromium on the machine for the render and review stages (`-no-render` skips both). No third party Go modules: the Messages API client and the Chrome driver use only the standard library.

## Outputs

| File | What it is |
|---|---|
| `index.html` | The deliverable. Client assets are inlined as data URIs. |
| `source.html` | The same page with `assets/` references, as the model wrote it. |
| `brief.json`, `copy.json` | Intake and content stage outputs. |
| `plan.draft.json`, `critique.json`, `plan.json` | The design plan before and after critique. |
| `review.json` | Visual review verdict and issues. |
| `screenshots/pre-repair`, `screenshots/final` | Tiles at 1280px and 390px, light and dark. |
| `report.json` | Status, every finding, every stage's tokens and estimated cost. |

`status` is `ready` (no blockers after repair), `needs_review` (blockers remain or repair edits failed), `unverified` (rendering skipped) or `failed`.

## Profiles

| Stage | quality | balanced | economy |
|---|---|---|---|
| intake | Sonnet 5.5, medium | Sonnet 5.5, medium | Sonnet 5.5, medium |
| content | Sonnet 5.5, medium | Sonnet 5.5, medium | Sonnet 5.5, medium |
| plan | Opus 5.5, high | Opus 5.5, medium | Sonnet 5.5, high |
| critique | Opus 5.5, high | Sonnet 5.5, high | Sonnet 5.5, high |
| build | Sonnet 5.5, medium | Sonnet 5.5, medium | Sonnet 5.5, medium |
| review | Opus 5.5, medium | Sonnet 5.5, medium | Sonnet 5.5, medium |
| repair | Sonnet 5.5, medium | Sonnet 5.5, medium | Sonnet 5.5, medium |
| classify (product chat) | Haiku 5.5, low | Haiku 5.5, low | Haiku 5.5, low |
| edit (product chat) | Sonnet 5.5, low | Sonnet 5.5, low | Sonnet 5.5, low |

Intake runs on Sonnet in every profile, and twice per site in the product (the questions, then the build). It is not plain extraction: it reads the photos, picks the questions and writes the summary the owner confirms, so its mistakes reach every later stage. Classify stays on Haiku because every chat message waits on it and it only routes; check its charge decisions on real messages before launch.

### Measured on the live API (16 November 2026)

Nine first pages from four briefs (honey cooperative, car workshop, restaurant, hair salon), rendering on, list prices:

| Profile | Runs | Cost per first page | Time per first page |
|---|---|---|---|
| quality | 4 | $0.80 to $1.39 (51 to 89 MZN) | 6.4 to 9.3 min |
| balanced | 2 | $0.55 | 4.2 min |
| economy | 3 | $0.42 to $0.48 | 3.8 to 4.8 min |

- Plan and critique on Opus at high effort are about two thirds of the time and cost of a quality build, mostly thinking tokens (10k to 22k output tokens each).
- The questions cost about $0.01 and take 7 seconds. A small edit (classify plus edit) cost $0.03 and finished in 6 seconds.
- On these briefs the profiles differed more in taste than in correctness: all three handled the restaurant's tourists, quality by adding English lines, balanced and economy by explaining local dishes in Portuguese. Four briefs is too few to settle it; judge on your own.
- At the default prices the first page loses money on every profile: it charges 19 credits (16.38 MZN before IVA) against 27 to 89 MZN of model spend. Edits are profitable (5 credits against about 2 MZN).

Edit `internal/pipeline/profile.go` to add your own profile.

## Operating it

- **Prompts** live in `internal/prompts/*.md`. Pass `-prompts dir` to override any file without rebuilding.
- **Generic defaults** are listed once in `internal/prompts/defaults.md` and pulled into the plan, critique and review prompts. The matching deterministic checks are in `internal/check/plan.go`. Review both monthly.
- **History** (`-history`, default in your user config dir) records each final plan's fonts, colors and bold move. The plan stage is told not to repeat the last ten, and the checks flag repeats.
- **Batches**: pass `-cache-1h` so the system prompts stay cached between briefs.
- **Fonts in CI**: if the render machine cannot reach Google Fonts, the report says so as `info` and screenshots use the fallback stack.

## The product (fazd)

```
 web app ──HTTPS + bearer token──▶ fazd (API + job workers, Go) ──▶ Postgres (users, sites, jobs, credit ledger, usage)
                                     │  runs the pipeline, headless Chrome
                                     ▼
                                Cloudflare R2  ◀── Worker serves nome.faz.co.mz from live/<nome>/index.html
```

The web app lives in [`web/`](web/README.md): React, Vite and TypeScript, with a typed client for this API and an in-browser mock for UI work without the backend.

One Go binary serves the API and runs the job workers; run several copies behind a load balancer and they share the queue. Builds and edits need Chrome, so fazd runs on a VM or container, not in Workers. Published sites are served by a Cloudflare Worker straight from R2.

### The customer flow

1. **Sign up**: the account gets the signup gift (19 credits by default).
2. **Describe the business** (`POST /v1/sites`, JSON, or multipart with up to 8 photos): Sonnet reads the request and the photos and returns up to three clarifying questions, plus `understood`: two sentences saying what it understood (the business, who the page is for, what it must get visitors to do), which the owner can correct. It never asks for photos it was sent. Free.
3. **Answer or skip** (`POST /v1/sites/{id}/answers`, with an optional `correction`): the first page is charged and a build job is queued. The build reads the answers and the correction next to what it corrects. The full pipeline runs; every output goes to R2 as version 1. If the build fails, the charge comes back and the answers stay stored, so a retry is one tap.
4. **Chat** (`POST /v1/sites/{id}/messages`): Haiku classifies each message. Questions and vague requests get a free reply with quick options; clear edits are charged (5 credits, or 20 for structural changes) and queued. An edit that breaks the page gets one repair; if it is still broken, the old page stays and the credits come back.
5. **Top up** (`POST /v1/payments`): M-Pesa (84, 85) or e-Mola (86, 87), at least 200 MZN, IVA included. The payment stays pending until the provider settles it, through `POST /v1/payments/callback/{provider}`.
6. **Publish** (`POST /v1/sites/{id}/publish`): the current version is copied to `live/<subdomain>/index.html`, and the Worker serves it.

A build or edit that fails for good refunds its charge once, whatever retries happened (ledger entries carry idempotency keys).

### API

| Method and path | Auth | What it does |
|---|---|---|
| `POST /v1/auth/signup`, `POST /v1/auth/login` | none | Returns `{token, expires_at, account}` |
| `POST /v1/auth/logout`, `GET /v1/me`, `GET /v1/credits` | bearer | Session, balance, ledger |
| `GET /v1/pricing` | none | Current prices |
| `POST /v1/sites`, `GET /v1/sites`, `GET /v1/sites/{id}` | bearer | Create (JSON or multipart with `file` photos; returns questions and `understood`), list, editor view |
| `POST /v1/sites/{id}/assets` | bearer | Multipart `file`; JPEG, PNG or WebP, scaled to fit inline |
| `POST /v1/sites/{id}/answers` | bearer | `{answers: {id: value}}` or `{skip: true}`, plus an optional `correction`; 202 with `job_id` |
| `POST /v1/sites/{id}/messages` | bearer | `{text}`; 200 with a free reply, or 202 with `job_id` when charged |
| `GET /v1/sites/{id}/preview` | bearer | Presigned R2 URL for an iframe, 15 minutes |
| `GET /v1/subdomains/{name}` | bearer | Availability and the reason when not |
| `POST /v1/sites/{id}/publish` | bearer | `{subdomain}`; returns the live URL |
| `GET /v1/jobs/{id}` | bearer | Poll a build or edit; `progress` names the stage it reached |
| `POST /v1/payments`, `GET /v1/payments`, `GET /v1/payments/{id}` | bearer | `{amount_mzn, msisdn, nuit?}` |
| `POST /v1/payments/callback/{provider}` | provider | Settlement webhook |
| `GET /v1/config` | none | `{beta_required}`: whether sign up needs an invite code |
| `GET /v1/admin/status`, `POST /v1/admin/settings` | admin key | Beta gate, model pause, daily spend cap, reserved names and blocked words |
| `GET`/`POST /v1/admin/invites`, `POST /v1/admin/invites/{code}/disable` | admin key | Invite codes with uses, bonus credits and an end date |
| `GET /v1/admin/users?q=`, `GET /v1/admin/users/{id}` | admin key | Find accounts by email, name or M-Pesa number; sites, ledger, payments and audit |
| `POST /v1/admin/users/{id}/credits`, `/suspend`, `/sign-out` | admin key | Give or remove credits (`request_id` makes a repeat harmless), suspend, end sessions |
| `POST /v1/admin/jobs/{id}/refund` | admin key | Return a build or edit's charge, once |
| `GET /v1/admin/sites?q=`, `POST /v1/admin/sites/{id}/takedown`, `/restore` | admin key | Find sites; take one off the air and back |
| `GET /v1/admin/audit` | admin key | Every admin action with operator and reason |
| `GET /v1/admin/telemetry?from=&to=` | admin key | Spend in a period (inclusive UTC days, default the last 30): totals, by operation, stage and model, and per site |
| `GET /v1/admin/sites/{id}/telemetry` | admin key | One site: every model call, grouped by operation, model and stage, with credits charged and margin |

Errors are `{"error": {"code", "message", "details"}}` with messages in Portuguese, ready to show. `402 insufficient_credits` carries `needed` and `balance`, so the app can open the top up screen.

### R2 layout

```
sites/<site id>/assets/<name>                 client photos and logos
sites/<site id>/v<n>/index.html               the page, assets inlined
sites/<site id>/v<n>/source.html              the page as the model wrote it
sites/<site id>/v<n>/{brief,copy,plan,report}.json, screenshots/...
live/<subdomain>/index.html                   what the Worker serves
live/<subdomain>/site.json                    which site and version is live
domains/<hostname>.json                       {"subdomain": ...} for a customer's own domain
```

### Run it locally

```sh
# Postgres, and any S3 compatible server (MinIO, or moto: pip install "moto[server]"; moto_server -p 9000)
export DATABASE_URL="postgres://faz:faz@127.0.0.1:5432/faz?sslmode=disable"
export R2_ENDPOINT=http://127.0.0.1:9000 R2_BUCKET=faz-sites R2_ACCESS_KEY_ID=test R2_SECRET_ACCESS_KEY=test
export FAZ_FAKE_DIR=examples/fake          # recorded model output; drop it and set ANTHROPIC_API_KEY for real runs
export FAZ_MOCK_PAYMENT_SECRET=dev-secret
go run ./cmd/fazd
```

Migrations run on start. The web app: `cd web && npm install && npm run dev` (it proxies `/v1` to fazd). The Worker: `cd worker && npm test`, then `npx wrangler deploy` once the R2 bucket and the `*.faz.co.mz` route exist.

### Configuration

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required | Postgres connection string |
| `R2_ACCOUNT_ID`, `R2_BUCKET`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` | required | R2 bucket and an API token with read and write on it |
| `R2_ENDPOINT`, `R2_REGION` | unset | Use an S3 compatible server instead of R2 (local development) |
| `ANTHROPIC_API_KEY` | required | Unless `FAZ_FAKE_DIR` is set |
| `FAZ_PROFILE` | `quality` | Model routing, see Profiles |
| `FAZ_CACHE_TTL` | five minutes | `1h` keeps prompts cached longer |
| `FAZ_SITES_DOMAIN` | `faz.co.mz` | Live sites are `<name>.<domain>` |
| `FAZ_APP_ORIGINS` | none | Space separated origins allowed by CORS |
| `FAZ_WORKERS` | `2` | Job workers in this process |
| `FAZ_PAYMENTS` | `mock` | Only `mock` exists today |
| `FAZ_MOCK_PAYMENT_OUTCOME`, `FAZ_MOCK_PAYMENT_DELAY`, `FAZ_MOCK_PAYMENT_SECRET` | `success`, `3s`, unset | Mock wallet behaviour and webhook secret |
| `FAZ_SIGNUP_GIFT`, `FAZ_PRICE_FIRST_PAGE`, `FAZ_PRICE_EDIT`, `FAZ_PRICE_STRUCTURAL` | 19, 19, 5, 20 | Credits |
| `FAZ_TOPUP_MIN_MZN`, `FAZ_MZN_PER_CREDIT` | 200, 1 | Top up rules |
| `FAZ_CHROME`, `FAZ_CHROME_FLAGS`, `FAZ_NO_RENDER` | found on PATH | Render checks |
| `FAZ_PASSWORD_ITERATIONS` | 600000 | PBKDF2 iterations |
| `FAZ_DATA_DIR` | temp dir | Where the design history file lives |
| `FAZ_ADMIN_TOKEN` | unset | Unlocks `/v1/admin` (header `X-Faz-Admin`) and the web app's `/admin` pages. At least 24 random characters; unset turns the routes off |
| `FAZ_USD_MZN` | `64` | Rate used to show model spend in meticais |

### Security choices

- **Bearer tokens, no cookies.** Customer sites live on subdomains of the same registrable domain as the app, so they count as same site. Without cookies they cannot ride a customer's session. Even so, consider serving sites from a separate domain (as GitHub does with github.io) before adding anything cookie based.
- **Generated HTML never runs on the app's origin.** Previews are presigned R2 URLs; live sites come from the Worker with a CSP that blocks network calls and limits scripts to the same CDNs the build checks allow.
- **Passwords** use PBKDF2 with SHA-256 at 600 000 iterations; sessions store only a SHA-256 of the token.
- **Uploads** accept JPEG, PNG and WebP only. SVG is refused because it can carry scripts.

### Operating Faz

The web app's `/admin` pages and the `/v1/admin` routes are for whoever runs Faz. They need the admin key (`FAZ_ADMIN_TOKEN`, sent as `X-Faz-Admin`). Every write also names its operator (`X-Faz-Operator`) and gives a reason, and lands in an audit log that nothing deletes. That is the stand in for named operator accounts with 2FA, which come before a second operator does.

- **Beta by invite code.** Close the beta in Controlo and sign up needs a live code. A code has a number of uses, optional bonus credits and an optional end date. Share it as a link, `/criar-conta?convite=CODE`, which fills the field. Codes avoid a waitlist because Faz cannot send email or SMS yet. A code typed by mistake is refused even when the beta is open, so nobody believes they got a bonus they did not.
- **Credits.** Give or remove credits with a reason. The owner reads a neutral line ("Créditos oferecidos pela equipa Faz") or a note you write; the reason stays internal. A `request_id` makes a double click harmless, and a balance never goes below zero. Any build or edit can be refunded once, even if a failure already refunded it.
- **Support.** Find an account by email, name or the M-Pesa or e-Mola number of any payment. Suspend it (it is signed out and cannot sign in; its sites stay up) or end its sessions (a lost phone).
- **Sites.** Take a site off the air: the live page goes, the owner sees the reason in the editor, and publishing is refused until restored. The address stays with the site, so nobody can take it over. Restore puts the last published version back. Reserved names and blocked words extend the built in address rules for new addresses.
- **Kill switches.** Pause the models by hand (a provider outage), or set a daily spend cap in dollars that pauses them on its own until midnight UTC. While paused, questions, builds and chat answer "em manutenção" and charge nothing; jobs already queued still run.

Settings are cached for 5 seconds, so several fazd copies agree within that time.

### Telemetry

Every model call is stored in `model_usage`, failed calls and retries included, because they are paid for: the site, the job, the operation (`questions`, `build`, `classify`, `edit`), the stage, the model, the effort, the attempt, input, output and cache tokens, the cost in dollars and the latency.

The admin API and the web app's `/admin` pages read it back:

- **Per period:** model cost in dollars and meticais, credits charged without IVA, margin, calls, errors and tokens. Then a table by operation, stage and model with each row's share of the cost, and the sites, costliest first.
- **Per site:** the same totals, with builds, edits, failures and build time. Then the spend by operation, by model and by stage (effort, retries, errors, tokens, cost, average latency), and every call in order.

Read these numbers with two caveats:

- **Costs are estimates.** They are made at call time from the price table in `internal/llm/cost.go`, which was checked against the pricing page on 16 November 2026. A later price change does not rewrite history.
- **Gift credits count at list price.** Credits are valued without IVA, signup gift included, so a site built only with gift credits shows the value given away, not money received.

The admin key is separate from accounts on purpose. Emails are not verified, so no account can be trusted to be the operator.

### Measure margins before launch

The telemetry pages show this per site. In SQL, compare spend with what each operation charges:

```sql
SELECT operation,
       COUNT(DISTINCT COALESCE(job_id::text, site_id::text)) AS runs,
       ROUND(SUM(cost_usd) / NULLIF(COUNT(DISTINCT COALESCE(job_id::text, site_id::text)), 0), 4) AS usd_per_run,
       ROUND(SUM(cost_usd) / NULLIF(COUNT(DISTINCT COALESCE(job_id::text, site_id::text)), 0) * 64, 2) AS mzn_per_run
FROM model_usage GROUP BY operation ORDER BY operation;
```

The default prices come from the prototype. By the estimates in the design doc, a first page costs 50 to 76 MZN in model spend against the 19 credits gifted, so measure this with real runs before launch.

### Product known limits

- **Job queue:** a small queue on Postgres (`FOR UPDATE SKIP LOCKED`) instead of River. River needs Go 1.26 and this was written with Go 1.24; the `jobs` package mirrors River's shape so the swap stays local.
- **Payments:** only the mock exists. M-Pesa and e-Mola adapters implement `payments.Provider`.
- **Custom domains: coming soon.** The app shows them as "Em breve" (pricing, domains page, publish dialog, "Ligar domínio" in the editor, the Domínios tab). The Worker can already serve a hostname mapped in `domains/<host>.json`, but nothing creates that mapping, registers domains or issues certificates for them yet; the plan is Cloudflare for SaaS custom hostnames.
- **Not built yet:** Google sign in, password reset, owners unpublishing their own site, rate limiting, email or SMS to owners, named operator accounts with 2FA.
- **Dockerfile untested:** it was not built in the authoring sandbox. Chromium's sandbox inside containers may need `--cap-add=SYS_ADMIN` or a seccomp profile. Prefer that to passing `--no-sandbox`, since the rendered HTML is model generated.

## Sample output

`examples/sample-output` holds the dry run above: the page, its plan, the report and screenshots before and after the repair. The sample page's phone layout overflows on purpose, so you can see the review and repair loop catch it. Token counts and costs in that report are estimates made by the fake client, not real usage.

## Tests

```sh
go test ./...                     # pipeline, R2 signing, checks
cd worker && npm test             # site Worker
cd web && npm test                # web app; npm run e2e drives it in Chrome, see web/README.md

# The product end to end: real Postgres and an S3 server
FAZ_TEST_DATABASE_URL="postgres://faz:faz@127.0.0.1:5432/faz_test?sslmode=disable" \
FAZ_TEST_S3=http://127.0.0.1:9000 go test ./internal/api/
```

The pipeline test runs every stage against `examples/fake`, including a real headless render when Chrome is installed: it confirms the sample page overflows on a phone, that the recorded repair fixes it, and that the final status is `ready`.

## Known limits

- The HTTP client has run real briefs against the live API (November 2026); the figures are under Profiles.
- Prices in `internal/llm/cost.go` match the published input, output and cache read rates for Opus, Sonnet and Haiku 5.5 (checked 16 November 2026). Cache write is assumed at 1.25 times input, and Fable 5.2 was not checked.
- Repairs are verified by the checks and new screenshots, but not re-reviewed by a model. That is deliberate (one pass, no loop); look at `screenshots/final` before shipping.
- Headless Chrome cannot open windows narrower than 500px, so phone widths render inside a same origin iframe of the exact width. Media queries and viewport units behave correctly; mobile browser quirks (address bar, text autosizing) are not emulated.
