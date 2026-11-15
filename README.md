# faz

Turns a client's brief into a verified, single file web page. Seven model stages, deterministic checks between them, a real browser to measure and photograph the result, and one bounded repair pass.

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

Run the same briefs through both before deciding what Opus is worth for your clients. Edit `internal/pipeline/profile.go` to add your own.

## Operating it

- **Prompts** live in `internal/prompts/*.md`. Pass `-prompts dir` to override any file without rebuilding.
- **Generic defaults** are listed once in `internal/prompts/defaults.md` and pulled into the plan, critique and review prompts. The matching deterministic checks are in `internal/check/plan.go`. Review both monthly.
- **History** (`-history`, default in your user config dir) records each final plan's fonts, colors and bold move. The plan stage is told not to repeat the last ten, and the checks flag repeats.
- **Batches**: pass `-cache-1h` so the system prompts stay cached between briefs.
- **Fonts in CI**: if the render machine cannot reach Google Fonts, the report says so as `info` and screenshots use the fallback stack.

## Sample output

`examples/sample-output` holds the dry run above: the page, its plan, the report and screenshots before and after the repair. The sample page's phone layout overflows on purpose, so you can see the review and repair loop catch it. Token counts and costs in that report are estimates made by the fake client, not real usage.

## Tests

```sh
go test ./...
```

The pipeline test runs every stage against `examples/fake`, including a real headless render when Chrome is installed: it confirms the sample page overflows on a phone, that the recorded repair fixes it, and that the final status is `ready`.

## Known limits

- The HTTP client was tested against a local server that mimics the Messages API stream, not against the live API. Run one real brief before relying on it.
- Prices in `internal/llm/cost.go` are list prices from November 2026; cache read rates for Haiku 5.5 and Fable 5.2 are assumed. Verify against the pricing page.
- Repairs are verified by the checks and new screenshots, but not re-reviewed by a model. That is deliberate (one pass, no loop); look at `screenshots/final` before shipping.
- Headless Chrome cannot open windows narrower than 500px, so phone widths render inside a same origin iframe of the exact width. Media queries and viewport units behave correctly; mobile browser quirks (address bar, text autosizing) are not emulated.
