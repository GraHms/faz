package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GraHms/faz/internal/assets"
	"github.com/GraHms/faz/internal/check"
	"github.com/GraHms/faz/internal/edit"
	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/prompts"
	"github.com/GraHms/faz/internal/spec"
)

// The entry points below serve the product: questions before the first
// build, sorting chat messages, and applying one owner request to a page.
// Each call records its model usage in Report(), like Run does.

// Report returns what the last call recorded, including stage costs.
func (p *Pipeline) Report() *Report { return p.rep }

func (p *Pipeline) begin() error {
	if p.Opt.Log == nil {
		p.Opt.Log = io.Discard
	}
	if err := p.Opt.Profile.Validate(); err != nil {
		return err
	}
	if p.rep == nil {
		p.rep = &Report{Profile: p.Opt.ProfileName, Outputs: map[string]string{}}
	}
	if p.Opt.OutDir != "" {
		return os.MkdirAll(p.Opt.OutDir, 0o755)
	}
	return nil
}

// Intake reads the client's request and returns the structured brief,
// including up to three clarifying questions. It costs one Haiku call.
func (p *Pipeline) Intake(ctx context.Context) (*spec.Brief, error) {
	if err := p.begin(); err != nil {
		return nil, err
	}
	if p.Opt.OutDir == "" {
		return nil, fmt.Errorf("intake needs an output directory")
	}
	return p.intake(ctx)
}

// ChatTurn is one earlier message in the owner's chat.
type ChatTurn struct {
	Role string `json:"role"` // user or assistant
	Text string `json:"text"`
}

// ClassifyInput is what the classify stage sees.
type ClassifyInput struct {
	Brief     *spec.Brief
	Copy      *spec.Copy
	Plan      *spec.Plan
	History   []ChatTurn
	Message   string
	PriceList string
}

// Classify decides whether a chat message is a clear edit, a structural
// change, too vague to act on, or not a change at all. It writes no files.
func (p *Pipeline) Classify(ctx context.Context, in ClassifyInput) (*spec.Classification, error) {
	if err := p.begin(); err != nil {
		return nil, err
	}
	var sections []string
	if in.Copy != nil {
		for _, s := range in.Copy.Sections {
			sections = append(sections, s.ID+" ("+s.Heading+")")
		}
	}
	summary := fmt.Sprintf("# Page\n\nLanguage: %s\nSubject: %s\nSections: %s\n", in.Brief.Language, in.Brief.Subject, strings.Join(sections, ", "))
	if in.Plan != nil {
		summary += fmt.Sprintf("Concept: %s\nBold move: %s\n", in.Plan.Concept, in.Plan.BoldMove)
	}
	history := in.History
	if len(history) > 6 {
		history = history[len(history)-6:]
	}
	text := summary + "\n" + block("Recent chat", nonNil(history)) +
		"\n\n# Price list\n\n" + in.PriceList + "\n\n# New message\n\n" + in.Message
	return callJSON(ctx, p, prompts.Classify, []llm.Part{{Text: text}}, (*spec.Classification).Validate)
}

// EditInput is one owner request against the current page.
type EditInput struct {
	Source      string // the page with assets/ references, as stored
	Brief       *spec.Brief
	Copy        *spec.Copy
	Plan        *spec.Plan
	Instruction string
	Structural  bool
}

// EditResult is what an edit produced. When Rejected is true the edit was
// not kept: Source and Page hold the unchanged page.
type EditResult struct {
	Source   string          `json:"-"`
	Page     string          `json:"-"`
	Summary  string          `json:"summary"`
	Mode     string          `json:"mode"`
	Applied  int             `json:"applied"`
	Failed   []edit.Failure  `json:"failed,omitempty"`
	Repaired bool            `json:"repaired"`
	Rejected bool            `json:"rejected"`
	Reason   string          `json:"reason,omitempty"`
	Findings []check.Finding `json:"findings"`
}

// Edit applies one instruction with the edit stage, then checks the page in
// the browser. An edit that introduces a blocker gets one repair; if the
// blocker is still there, the edit is rejected and the old page stays live.
// The current document is sent as a cached prefix, so a run of edits on the
// same page pays full input price only once.
func (p *Pipeline) Edit(ctx context.Context, in EditInput) (*EditResult, error) {
	if err := p.begin(); err != nil {
		return nil, err
	}
	res := &EditResult{Source: in.Source, Page: p.inline(in.Source)}
	before, _, err := p.verify(ctx, in.Brief, nil, in.Plan, res.Page, "edit-before", false)
	if err != nil {
		return nil, err
	}

	kind := "a small change"
	if in.Structural {
		kind = "a structural change (sections or overall look)"
	}
	plan := map[string]any{}
	if in.Plan != nil {
		plan = map[string]any{"concept": in.Plan.Concept, "bold_move": in.Plan.BoldMove, "theme_mode": in.Plan.ThemeMode,
			"colors": in.Plan.Colors, "type": in.Plan.Type, "principles": in.Plan.Principles}
	}
	parts := []llm.Part{
		{Text: "# Current document\n\n```html\n" + in.Source + "\n```", Cache: true},
		{Text: block("Brief", in.Brief) + "\n\n" + block("Copy", in.Copy) + "\n\n" + block("Design plan summary", plan) +
			"\n\n# Files available under assets/\n\n" + assets.Listing(p.Opt.Assets) +
			"\n\n# Instruction (" + kind + ")\n\n" + in.Instruction},
	}
	resp, err := p.call(ctx, prompts.Edit, parts, 1)
	if err != nil {
		return nil, err
	}
	out, a, err := applyResponse(resp.Text, in.Source)
	if err != nil {
		return nil, fmt.Errorf("edit: %w", err)
	}
	res.Summary, res.Mode, res.Applied, res.Failed = a.Summary, a.Mode, a.Applied, a.Failed
	if a.Mode == "edits" && a.Applied == 0 {
		res.Rejected, res.Reason = true, "none of the edits matched the current page"
		res.Findings = before
		return res, p.finishEdit(res)
	}

	page := p.inline(out)
	after, _, err := p.verify(ctx, in.Brief, nil, in.Plan, page, "edit-after", false)
	if err != nil {
		return nil, err
	}
	if fresh := newBlockers(before, after); len(fresh) > 0 {
		res.Repaired = true
		fixed, _, err := p.repair(ctx, out, nil, fresh)
		if err != nil {
			return nil, err
		}
		out, page = fixed, p.inline(fixed)
		if after, _, err = p.verify(ctx, in.Brief, nil, in.Plan, page, "edit-repaired", false); err != nil {
			return nil, err
		}
		if still := newBlockers(before, after); len(still) > 0 {
			res.Rejected = true
			res.Reason = fmt.Sprintf("the change broke the page (%s) and the repair did not fix it", still[0].Message)
			res.Findings = before
			return res, p.finishEdit(res)
		}
	}
	res.Source, res.Page, res.Findings = out, page, after
	return res, p.finishEdit(res)
}

func (p *Pipeline) finishEdit(res *EditResult) error {
	p.rep.Findings = nonNil(res.Findings)
	p.rep.Status = "ready"
	if res.Rejected {
		p.rep.Status, p.rep.StatusReasons = "rejected", []string{res.Reason}
	}
	if p.Opt.OutDir == "" {
		return nil
	}
	if err := p.writeFile("source.html", res.Source); err != nil {
		return err
	}
	if err := p.writeFile("index.html", res.Page); err != nil {
		return err
	}
	if err := p.writeJSON("edit.json", res); err != nil {
		return err
	}
	return p.writeJSON("report.json", p.rep)
}

// newBlockers lists blocker findings in after that were not in before, so an
// edit is judged only on what it broke.
func newBlockers(before, after []check.Finding) []check.Finding {
	had := map[string]bool{}
	for _, f := range before {
		had[f.Check+"|"+f.Message] = true
	}
	var out []check.Finding
	for _, f := range after {
		if f.Severity == check.Blocker && !had[f.Check+"|"+f.Message] {
			out = append(out, f)
		}
	}
	return out
}
