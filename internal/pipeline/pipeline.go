// Package pipeline runs a brief through every stage, from intake to a
// verified page, and writes each stage's output plus a report.
//
//	intake    Haiku   raw request  -> brief.json
//	content   Sonnet  brief        -> copy.json
//	plan      Opus    brief, copy  -> plan.draft.json
//	(checks)  Go      contrast, generic defaults, repeats of recent work
//	critique  Opus    draft + findings -> plan.json
//	build     Sonnet  brief, copy, plan -> source.html
//	(verify)  Go      static checks, headless Chrome metrics, screenshots
//	review    Opus    screenshots + metrics -> review.json
//	repair    Sonnet  issues -> edits, applied once, then verified again
//
// Every loop is bounded: one retry per stage for invalid output, one repair
// pass, one final verification. Anything still wrong is reported, not looped.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GraHms/faz/internal/assets"
	"github.com/GraHms/faz/internal/check"
	"github.com/GraHms/faz/internal/edit"
	"github.com/GraHms/faz/internal/history"
	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/prompts"
	"github.com/GraHms/faz/internal/render"
	"github.com/GraHms/faz/internal/spec"
)

// Options configures one run.
type Options struct {
	BriefText    string
	Assets       []assets.Asset
	OutDir       string
	Profile      Profile
	ProfileName  string
	PromptDir    string         // optional prompt overrides
	HistoryPath  string         // optional run history for repeat detection
	HistorySize  int            // recent entries considered; default 10
	Chrome       *render.Chrome // nil skips rendering and visual review
	InlineMax    int64          // largest asset inlined as a data URI; default 1 MB
	StageTimeout time.Duration  // default 15 minutes
	Log          io.Writer
	// OnStage, when set, is told each time a stage starts: a model stage
	// name, or "verify" for the rendered checks. It must return quickly.
	OnStage func(stage string)
}

// Pipeline holds a model client and options for one run.
type Pipeline struct {
	LLM llm.Client
	Opt Options
	rep *Report
}

// StageRun records one model call.
type StageRun struct {
	Stage        string  `json:"stage"`
	Model        string  `json:"model"`
	Effort       string  `json:"effort"`
	Attempt      int     `json:"attempt"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheRead    int     `json:"cache_read_tokens"`
	CacheWrite   int     `json:"cache_write_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Seconds      float64 `json:"seconds"`
	Error        string  `json:"error,omitempty"`
}

// RepairReport describes the repair pass.
type RepairReport struct {
	Mode    string         `json:"mode"` // edits, rewrite or none
	Applied int            `json:"applied"`
	Failed  []edit.Failure `json:"failed,omitempty"`
	Skipped []string       `json:"skipped,omitempty"`
}

// Report is written to report.json at the end of every run, failed or not.
type Report struct {
	Status               string             `json:"status"` // ready, needs_review, unverified or failed
	StatusReasons        []string           `json:"status_reasons,omitempty"`
	Error                string             `json:"error,omitempty"`
	Profile              string             `json:"profile"`
	StartedAt            time.Time          `json:"started_at"`
	Seconds              float64            `json:"seconds"`
	TotalCostUSD         float64            `json:"total_cost_usd"`
	Stages               []StageRun         `json:"stages"`
	Gaps                 []string           `json:"gaps,omitempty"`
	Assumptions          []string           `json:"assumptions,omitempty"`
	Placeholders         []spec.Placeholder `json:"placeholders,omitempty"`
	PlanFindingsDraft    []check.Finding    `json:"plan_findings_draft,omitempty"`
	CritiqueChanges      []string           `json:"critique_changes,omitempty"`
	ContrastAutoFixes    []string           `json:"contrast_auto_fixes,omitempty"`
	PlanFindingsFinal    []check.Finding    `json:"plan_findings_final,omitempty"`
	FindingsBeforeRepair []check.Finding    `json:"findings_before_repair,omitempty"`
	Review               *spec.Review       `json:"review,omitempty"`
	Repair               *RepairReport      `json:"repair,omitempty"`
	Findings             []check.Finding    `json:"findings"`
	AssetsNotInlined     []string           `json:"assets_not_inlined,omitempty"`
	Outputs              map[string]string  `json:"outputs"`
}

// Run executes the pipeline and always writes report.json.
func (p *Pipeline) Run(ctx context.Context) (rep *Report, err error) {
	if p.Opt.Log == nil {
		p.Opt.Log = io.Discard
	}
	if err := p.Opt.Profile.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(p.Opt.OutDir, 0o755); err != nil {
		return nil, err
	}
	start := time.Now()
	p.rep = &Report{Profile: p.Opt.ProfileName, StartedAt: start, Outputs: map[string]string{}}
	defer func() {
		p.rep.Seconds = time.Since(start).Seconds()
		if err != nil {
			p.rep.Status, p.rep.Error = "failed", err.Error()
		}
		if werr := p.writeJSON("report.json", p.rep); werr != nil && err == nil {
			err = werr
		}
		fmt.Fprintf(p.Opt.Log, "\nstatus %s, %.0fs, estimated cost $%.3f, outputs in %s\n",
			p.rep.Status, p.rep.Seconds, p.rep.TotalCostUSD, p.Opt.OutDir)
		rep = p.rep
	}()

	brief, err := p.intake(ctx)
	if err != nil {
		return nil, err
	}
	copyDoc, err := p.content(ctx, brief)
	if err != nil {
		return nil, err
	}
	plan, err := p.design(ctx, brief, copyDoc)
	if err != nil {
		return nil, err
	}
	source, err := p.build(ctx, brief, copyDoc, plan)
	if err != nil {
		return nil, err
	}
	return nil, p.verifyReviewRepair(ctx, brief, copyDoc, plan, source)
}

// ---- stages ----

func (p *Pipeline) intake(ctx context.Context) (*spec.Brief, error) {
	parts := []llm.Part{{Text: "# Client request\n\n" + strings.TrimSpace(p.Opt.BriefText) +
		"\n\n# Attached files\n\n" + assets.Listing(p.Opt.Assets)}}
	parts = append(parts, p.visionParts(4)...)
	b, err := callJSON(ctx, p, prompts.Intake, parts, (*spec.Brief).Validate)
	if err != nil {
		return nil, err
	}
	p.rep.Gaps, p.rep.Assumptions = b.Gaps, b.Assumptions
	return b, p.writeJSON("brief.json", b)
}

func (p *Pipeline) content(ctx context.Context, b *spec.Brief) (*spec.Copy, error) {
	c, err := callJSON(ctx, p, prompts.Content, []llm.Part{{Text: block("Brief", b)}}, (*spec.Copy).Validate)
	if err != nil {
		return nil, err
	}
	p.rep.Placeholders = c.Placeholders
	return c, p.writeJSON("copy.json", c)
}

func (p *Pipeline) design(ctx context.Context, b *spec.Brief, c *spec.Copy) (*spec.Plan, error) {
	n := p.Opt.HistorySize
	if n == 0 {
		n = 10
	}
	recent, err := history.Load(p.Opt.HistoryPath, n)
	if err != nil {
		fmt.Fprintf(p.Opt.Log, "warning: history not read: %v\n", err)
	}

	parts := []llm.Part{{Text: block("Brief", b) + "\n\n" + block("Copy", c) + "\n\n" + recentWork(recent)}}
	if imgs := p.visionParts(4); len(imgs) > 0 {
		parts = append(parts, llm.Part{Text: "# Client images"})
		parts = append(parts, imgs...)
	}
	draft, err := callJSON(ctx, p, prompts.Plan, parts, (*spec.Plan).Validate)
	if err != nil {
		return nil, err
	}
	if err := p.writeJSON("plan.draft.json", draft); err != nil {
		return nil, err
	}
	findings := check.Plan(draft, c, b, recent)
	p.rep.PlanFindingsDraft = findings

	critParts := []llm.Part{{Text: block("Brief", b) + "\n\n" + block("Copy", c) + "\n\n" +
		block("Draft design plan", draft) + "\n\n" + block("Automated findings", nonNil(findings))}}
	crit, err := callJSON(ctx, p, prompts.Critique, critParts, (*spec.Critique).Validate)
	if err != nil {
		return nil, err
	}
	if err := p.writeJSON("critique.json", crit); err != nil {
		return nil, err
	}
	for _, ch := range crit.Changes {
		p.rep.CritiqueChanges = append(p.rep.CritiqueChanges, ch.What+": "+ch.Why)
	}
	plan := crit.RevisedPlan
	// Safety net: contrast is arithmetic, so fix what the critique missed.
	p.rep.ContrastAutoFixes = check.FixPlanContrast(&plan)
	p.rep.PlanFindingsFinal = check.Plan(&plan, c, b, recent)

	entry := check.Entry(&plan, b)
	entry.Time = time.Now().UTC()
	if err := history.Append(p.Opt.HistoryPath, entry); err != nil {
		fmt.Fprintf(p.Opt.Log, "warning: history not written: %v\n", err)
	}
	return &plan, p.writeJSON("plan.json", plan)
}

func (p *Pipeline) build(ctx context.Context, b *spec.Brief, c *spec.Copy, plan *spec.Plan) (string, error) {
	parts := []llm.Part{{Text: block("Brief", b) + "\n\n" + block("Copy", c) + "\n\n" +
		block("Approved design plan", plan) + "\n\n# Files available under assets/\n\n" + assets.Listing(p.Opt.Assets)}}
	resp, err := p.call(ctx, prompts.Build, parts, 1)
	if err != nil {
		return "", err
	}
	src, err := llm.ExtractHTML(resp.Text)
	if err != nil {
		return "", fmt.Errorf("build: %w", err)
	}
	return src, p.writeFile("source.html", src)
}

// verifyReviewRepair checks the page, has it reviewed, repairs it once and
// checks it again. The source keeps assets/ references so repair prompts stay
// small; the shipped page has them inlined.
func (p *Pipeline) verifyReviewRepair(ctx context.Context, b *spec.Brief, c *spec.Copy, plan *spec.Plan, source string) error {
	page := p.inline(source)
	if err := p.writeFile("index.html", page); err != nil {
		return err
	}
	findings, shots, err := p.verify(ctx, b, c, plan, page, "pre-repair", true)
	if err != nil {
		return err
	}
	p.rep.FindingsBeforeRepair = findings

	var review *spec.Review
	if p.Opt.Chrome != nil {
		review, err = callJSON(ctx, p, prompts.Review, p.reviewParts(b, plan, findings, shots), (*spec.Review).Validate)
		if err != nil {
			return err
		}
		p.rep.Review = review
		if err := p.writeJSON("review.json", review); err != nil {
			return err
		}
	}

	needsRepair := len(check.Filter(findings, check.Major)) > 0 || (review != nil && review.Verdict == "fix")
	p.rep.Repair = &RepairReport{Mode: "none"}
	if needsRepair {
		repaired, rr, err := p.repair(ctx, source, review, findings)
		if err != nil {
			return err
		}
		p.rep.Repair = rr
		if repaired != source {
			source = repaired
			page = p.inline(source)
			if err := p.writeFile("source.html", source); err != nil {
				return err
			}
			if err := p.writeFile("index.html", page); err != nil {
				return err
			}
			if findings, _, err = p.verify(ctx, b, c, plan, page, "final", true); err != nil {
				return err
			}
		}
	}
	p.rep.Findings = nonNil(findings)
	p.decideStatus(review)
	return nil
}

func (p *Pipeline) repair(ctx context.Context, source string, review *spec.Review, findings []check.Finding) (string, *RepairReport, error) {
	var issues []spec.Issue
	if review != nil {
		issues = review.Issues
	}
	text := "# Current document\n\n```html\n" + source + "\n```\n\n" +
		block("QA issues", nonNil(issues)) + "\n\n" +
		block("Automated failures", nonNil(check.Filter(findings, check.Major)))
	resp, err := p.call(ctx, prompts.Repair, []llm.Part{{Text: text}}, 1)
	if err != nil {
		return source, nil, err
	}
	out, applied, err := applyResponse(resp.Text, source)
	if err != nil {
		return source, nil, fmt.Errorf("repair: %w", err)
	}
	return out, applied.RepairReport, nil
}

// applied describes what an edit or repair response did to the document.
type applied struct {
	*RepairReport
	Summary string
}

// applyResponse turns an edit or repair response into a new document: either
// find and replace edits applied to source, or a whole replacement document.
func applyResponse(text, source string) (string, applied, error) {
	rr := &RepairReport{}
	if _, ok := llm.Fenced(text, "json"); ok {
		var e spec.Edits
		if err := llm.DecodeJSON(text, &e); err != nil {
			return source, applied{}, err
		}
		if err := e.Validate(); err != nil {
			return source, applied{}, err
		}
		edits := make([]edit.Edit, len(e.Edits))
		for i, x := range e.Edits {
			edits[i] = edit.Edit{Find: x.Find, Replace: x.Replace}
		}
		out, res := edit.Apply(source, edits)
		rr.Mode, rr.Applied, rr.Failed = "edits", res.Applied, res.Failed
		for _, sk := range e.Skipped {
			rr.Skipped = append(rr.Skipped, sk.Issue+": "+sk.Reason)
		}
		return out, applied{RepairReport: rr, Summary: strings.TrimSpace(e.Summary)}, nil
	}
	doc, err := llm.ExtractHTML(text)
	if err != nil {
		return source, applied{}, fmt.Errorf("neither edits nor a document: %w", err)
	}
	summary := ""
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "Summary:") {
			summary = strings.TrimSpace(strings.TrimPrefix(t, "Summary:"))
			break
		}
	}
	rr.Mode = "rewrite"
	return doc, applied{RepairReport: rr, Summary: summary}, nil
}

// ---- verification ----

type shot struct {
	Label string
	Tile  int
	Of    int
	JPEG  []byte
}

type pass struct {
	label  string
	width  int
	theme  string
	mobile bool
	dpr    float64
	tiles  int
	tileH  int
	maxCSS int
}

func (p *Pipeline) passes(plan *spec.Plan) []pass {
	light := ""
	if plan.ThemeMode == "dual" {
		light = "light"
	}
	ps := []pass{
		{"desktop-1280-light", 1280, light, false, 1, 6, 1400, 6400},
		{"phone-390-light", 390, light, true, 2, 8, 1600, 6400},
	}
	if plan.ThemeMode == "dual" {
		ps = append(ps,
			pass{"desktop-1280-dark", 1280, "dark", false, 1, 2, 1400, 2800},
			pass{"phone-390-dark", 390, "dark", true, 2, 2, 1600, 1600},
		)
	}
	return ps
}

func (p *Pipeline) verify(ctx context.Context, b *spec.Brief, c *spec.Copy, plan *spec.Plan, page, phase string, shots bool) ([]check.Finding, []shot, error) {
	p.started("verify")
	findings := check.Static(page, plan, c)
	if p.Opt.Chrome == nil {
		return findings, nil, nil
	}
	work, err := os.MkdirTemp("", "faz-render-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(work)
	shotDir := filepath.Join(p.Opt.OutDir, "screenshots", phase)
	if shots {
		if err := os.MkdirAll(shotDir, 0o755); err != nil {
			return nil, nil, err
		}
	}
	var taken []shot
	seen := map[string]bool{}
	for _, ps := range p.passes(plan) {
		m, err := p.Opt.Chrome.Measure(ctx, page, work, ps.label, ps.width, ps.theme)
		if err != nil {
			return nil, nil, fmt.Errorf("render %s: %w", ps.label, err)
		}
		for _, f := range check.Rendered(m, ps.mobile, b.Language) {
			key := f.Check + "|" + f.Message
			if !seen[key] {
				seen[key] = true
				findings = append(findings, f)
			}
		}
		if !shots {
			continue
		}
		height := m.DocHeight
		if height > ps.maxCSS {
			height = ps.maxCSS
		}
		if height < 600 {
			height = 600
		}
		png, err := p.Opt.Chrome.Screenshot(ctx, page, work, ps.label, ps.width, height, ps.dpr, ps.theme)
		if err != nil {
			return nil, nil, fmt.Errorf("screenshot %s: %w", ps.label, err)
		}
		tiles, total, err := render.Tiles(png, ps.tileH, ps.tiles, 82)
		if err != nil {
			return nil, nil, err
		}
		for i, t := range tiles {
			name := fmt.Sprintf("%s-%d.jpg", ps.label, i+1)
			if err := os.WriteFile(filepath.Join(shotDir, name), t, 0o644); err != nil {
				return nil, nil, err
			}
			taken = append(taken, shot{Label: ps.label, Tile: i + 1, Of: total, JPEG: t})
		}
		p.rep.Outputs["screenshots/"+phase] = filepath.Join(p.Opt.OutDir, "screenshots", phase)
	}
	return findings, taken, nil
}

func (p *Pipeline) reviewParts(b *spec.Brief, plan *spec.Plan, findings []check.Finding, shots []shot) []llm.Part {
	summary := fmt.Sprintf("# Brief summary\n\nSubject: %s\nAudience: %s\nPage job: %s\nLanguage: %s\n\n"+
		"# Approved plan summary\n\nConcept: %s\nBold move: %s\nSubject detail: %s\nTheme mode: %s\nAlignment: %s\nMotion: %s\n\n",
		b.Subject, b.Audience, b.PageJob, b.Language,
		plan.Concept, plan.BoldMove, plan.SubjectDetail, plan.ThemeMode, plan.Layout.Alignment, plan.Motion)
	parts := []llm.Part{{Text: summary + block("Automated check report", nonNil(findings))}}
	for _, s := range shots {
		parts = append(parts,
			llm.Part{Text: fmt.Sprintf("Screenshot: %s, tile %d of %d (tiles beyond %d were not captured)", s.Label, s.Tile, s.Of, len(shotsFor(shots, s.Label)))},
			llm.Part{Image: s.JPEG, MediaType: "image/jpeg"},
		)
	}
	return parts
}

func shotsFor(shots []shot, label string) []shot {
	var out []shot
	for _, s := range shots {
		if s.Label == label {
			out = append(out, s)
		}
	}
	return out
}

func (p *Pipeline) decideStatus(review *spec.Review) {
	r := p.rep
	if p.Opt.Chrome == nil {
		r.Status = "unverified"
		r.StatusReasons = append(r.StatusReasons, "rendering and visual review were skipped")
	}
	if n := check.Count(r.Findings, check.Blocker); n > 0 {
		r.StatusReasons = append(r.StatusReasons, fmt.Sprintf("%d blocker findings remain after repair", n))
	}
	if r.Repair != nil && len(r.Repair.Failed) > 0 {
		r.StatusReasons = append(r.StatusReasons, fmt.Sprintf("%d repair edits could not be applied", len(r.Repair.Failed)))
	}
	if review != nil && review.Verdict == "fix" {
		r.StatusReasons = append(r.StatusReasons, "repairs were applied but not re-reviewed visually; check the final screenshots")
	}
	if r.Status == "" {
		switch {
		case check.Count(r.Findings, check.Blocker) > 0 || (r.Repair != nil && len(r.Repair.Failed) > 0):
			r.Status = "needs_review"
		default:
			r.Status = "ready"
		}
	}
}

// ---- helpers ----

// started tells OnStage that a stage began.
func (p *Pipeline) started(stage string) {
	if p.Opt.OnStage != nil {
		p.Opt.OnStage(stage)
	}
}

func (p *Pipeline) call(ctx context.Context, stage string, parts []llm.Part, attempt int) (llm.Response, error) {
	p.started(stage)
	cfg := p.Opt.Profile[stage]
	system, err := prompts.Load(stage, p.Opt.PromptDir)
	if err != nil {
		return llm.Response{}, err
	}
	timeout := p.Opt.StageTimeout
	if timeout == 0 {
		timeout = 15 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := p.LLM.Complete(cctx, llm.Request{
		Stage: stage, Model: cfg.Model, System: system, Effort: cfg.Effort, MaxTokens: cfg.MaxTokens, Parts: parts,
	})
	run := StageRun{
		Stage: stage, Model: cfg.Model, Effort: cfg.Effort, Attempt: attempt,
		InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens,
		CacheRead: resp.Usage.CacheReadInputTokens, CacheWrite: resp.Usage.CacheCreationInputTokens,
		CostUSD: llm.Cost(cfg.Model, resp.Usage), Seconds: resp.Duration.Seconds(),
	}
	if err != nil {
		run.Error = err.Error()
		if errors.Is(err, llm.ErrTruncated) {
			err = fmt.Errorf("%s: %w (max_tokens %d)", stage, err, cfg.MaxTokens)
		}
	}
	p.rep.Stages = append(p.rep.Stages, run)
	p.rep.TotalCostUSD += run.CostUSD
	fmt.Fprintf(p.Opt.Log, "%-9s %-18s try %d  %6.1fs  in %7d  out %6d  $%.4f\n",
		stage, cfg.Model, attempt, run.Seconds, run.InputTokens+run.CacheRead+run.CacheWrite, run.OutputTokens, run.CostUSD)
	return resp, err
}

// callJSON calls a stage, decodes its JSON and validates it. Invalid output
// gets one retry that quotes the validation error back to the model.
func callJSON[T any](ctx context.Context, p *Pipeline, stage string, parts []llm.Part, validate func(*T) error) (*T, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		ps := parts
		if lastErr != nil {
			ps = append(append([]llm.Part{}, parts...), llm.Part{Text: fmt.Sprintf(
				"# Correction\n\nA previous attempt returned output that failed validation: %v\n\nReturn the complete corrected JSON object only, inside a single ```json block.", lastErr)})
		}
		resp, err := p.call(ctx, stage, ps, attempt)
		if err != nil {
			return nil, err
		}
		v := new(T)
		if err := llm.DecodeJSON(resp.Text, v); err != nil {
			lastErr = err
			continue
		}
		if err := validate(v); err != nil {
			lastErr = err
			continue
		}
		return v, nil
	}
	return nil, fmt.Errorf("%s: output failed validation twice: %w", stage, lastErr)
}

func (p *Pipeline) visionParts(max int) []llm.Part {
	var parts []llm.Part
	for _, a := range p.Opt.Assets {
		if len(parts)/2 >= max {
			break
		}
		if a.VisionReady() {
			parts = append(parts, llm.Part{Text: "Attached file: " + a.Name}, llm.Part{Image: a.Data, MediaType: a.MediaType})
		}
	}
	return parts
}

func (p *Pipeline) inline(source string) string {
	max := p.Opt.InlineMax
	if max == 0 {
		max = 1 << 20
	}
	page, skipped := assets.Inline(source, p.Opt.Assets, max)
	p.rep.AssetsNotInlined = skipped
	if len(skipped) > 0 {
		dir := filepath.Join(p.Opt.OutDir, "assets")
		_ = os.MkdirAll(dir, 0o755)
		for _, a := range p.Opt.Assets {
			for _, s := range skipped {
				if a.Name == s && len(a.Data) > 0 {
					_ = os.WriteFile(filepath.Join(dir, a.Name), a.Data, 0o644)
				}
			}
		}
	}
	return page
}

func (p *Pipeline) writeJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return p.writeFile(name, string(b)+"\n")
}

func (p *Pipeline) writeFile(name, content string) error {
	path := filepath.Join(p.Opt.OutDir, name)
	p.rep.Outputs[name] = path
	return os.WriteFile(path, []byte(content), 0o644)
}

// block renders a titled JSON section for a user message.
func block(title string, v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		b = []byte(fmt.Sprintf("%q", err.Error()))
	}
	return "# " + title + "\n\n```json\n" + string(b) + "\n```"
}

func recentWork(recent []history.Entry) string {
	if len(recent) == 0 {
		return "# Recent studio work\n\nNone yet."
	}
	var b strings.Builder
	b.WriteString("# Recent studio work (do not repeat these choices)\n\n")
	for _, e := range recent {
		fmt.Fprintf(&b, "- %s: display %s, body %s, background %s, accent %s; bold move: %s\n",
			e.Subject, e.DisplayFont, e.BodyFont, e.Background, e.Accent, e.BoldMove)
	}
	return b.String()
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
