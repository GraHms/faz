# faz

Two things live here:

- **The pipeline** (`cmd/faz`): turns a client's brief into a verified, single file web page.
- **The product** (`cmd/fazd`, `worker/`): the Faz service around it, with accounts, credits, clarifying questions, chat edits, mobile money top ups and publishing to `nome.faz.co.mz`. See [The product](#the-product-fazd) below.

The pipeline turns a client's brief into a verified, single file web page. Seven model stages, deterministic checks between them, a real browser to measure and photograph the result, and one bounded repair pass.

```
brief.md ─▶ intake ─▶ content ─▶ plan ─▶ [plan checks] ─▶ critique ─▶ build
             Haiku     Sonnet     Opus     contrast,        Opus        Sonnet
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

| Stage | quality | economy |
|---|---|---|
| intake | Haiku 5.5, medium | Haiku 5.5, medium |
| content | Sonnet 5.5, medium | Sonnet 5.5, medium |
| plan | Opus 5.5, high | Sonnet 5.5, high |
| critique | Opus 5.5, high | Sonnet 5.5, high |
| build | Sonnet 5.5, medium | Sonnet 5.5, medium |
| review | Opus 5.5, medium | Sonnet 5.5, medium |
| repair | Sonnet 5.5, medium | Sonnet 5.5, medium |
| classify (product chat) | Haiku 5.5, low | Haiku 5.5, low |
| edit (product chat) | Sonnet 5.5, low | Sonnet 5.5, low |

Run the same briefs through both before deciding what Opus is worth for your clients. Edit `internal/pipeline/profile.go` to add your own.

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
2. **Describe the business** (`POST /v1/sites`): Haiku reads the request and returns up to three clarifying questions. Free.
3. **Answer or skip** (`POST /v1/sites/{id}/answers`): the first page is charged and a build job is queued. The full pipeline runs; every output goes to R2 as version 1.
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
| `POST /v1/sites`, `GET /v1/sites`, `GET /v1/sites/{id}` | bearer | Create (returns questions), list, editor view |
| `POST /v1/sites/{id}/assets` | bearer | Multipart `file`; JPEG, PNG or WebP, scaled to fit inline |
| `POST /v1/sites/{id}/answers` | bearer | `{answers: {id: value}}` or `{skip: true}`; 202 with `job_id` |
| `POST /v1/sites/{id}/messages` | bearer | `{text}`; 200 with a free reply, or 202 with `job_id` when charged |
| `GET /v1/sites/{id}/preview` | bearer | Presigned R2 URL for an iframe, 15 minutes |
| `GET /v1/subdomains/{name}` | bearer | Availability and the reason when not |
| `POST /v1/sites/{id}/publish` | bearer | `{subdomain}`; returns the live URL |
| `GET /v1/jobs/{id}` | bearer | Poll a build or edit; `progress` names the stage it reached |
| `POST /v1/payments`, `GET /v1/payments`, `GET /v1/payments/{id}` | bearer | `{amount_mzn, msisdn, nuit?}` |
| `POST /v1/payments/callback/{provider}` | provider | Settlement webhook |

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

### Security choices

- **Bearer tokens, no cookies.** Customer sites live on subdomains of the same registrable domain as the app, so they count as same site. Without cookies they cannot ride a customer's session. Even so, consider serving sites from a separate domain (as GitHub does with github.io) before adding anything cookie based.
- **Generated HTML never runs on the app's origin.** Previews are presigned R2 URLs; live sites come from the Worker with a CSP that blocks network calls and limits scripts to the same CDNs the build checks allow.
- **Passwords** use PBKDF2 with SHA-256 at 600 000 iterations; sessions store only a SHA-256 of the token.
- **Uploads** accept JPEG, PNG and WebP only. SVG is refused because it can carry scripts.

### Measure margins before launch

Every model call is stored in `model_usage` with its cost in dollars. Compare it with what each operation charges:

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
- **Not built yet:** domain search and registration, Google sign in, password reset, rate limiting.
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

- The HTTP client was tested against a local server that mimics the Messages API stream, not against the live API. Run one real brief before relying on it.
- Prices in `internal/llm/cost.go` are list prices from November 2026; cache read rates for Haiku 5.5 and Fable 5.2 are assumed. Verify against the pricing page.
- Repairs are verified by the checks and new screenshots, but not re-reviewed by a model. That is deliberate (one pass, no loop); look at `screenshots/final` before shipping.
- Headless Chrome cannot open windows narrower than 500px, so phone widths render inside a same origin iframe of the exact width. Media queries and viewport units behave correctly; mobile browser quirks (address bar, text autosizing) are not emulated.
