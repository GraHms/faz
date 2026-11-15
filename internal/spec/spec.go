// Package spec defines the data contracts passed between pipeline stages.
//
// Each stage returns JSON that must decode into one of these types and pass
// Validate before the next stage runs. Field names match the shapes given in
// the stage prompts.
package spec

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Brief is the intake stage's structured reading of the client's request.
type Brief struct {
	Language          string   `json:"language"`
	PageKind          string   `json:"page_kind"`
	Subject           string   `json:"subject"`
	Audience          string   `json:"audience"`
	PageJob           string   `json:"page_job"`
	PrimaryAction     Action   `json:"primary_action"`
	ProvidedFacts     []Fact   `json:"provided_facts"`
	Brand             Brand    `json:"brand"`
	RequestedStyle    *string  `json:"requested_style"`
	SectionsSuggested []string `json:"sections_suggested"`
	Assets            []Asset  `json:"assets"`
	Gaps              []string `json:"gaps"`
	Assumptions       []string `json:"assumptions"`
}

type Action struct {
	Label  *string `json:"label"`
	Target *string `json:"target"`
}

type Fact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Brand struct {
	Name           *string  `json:"name"`
	ExistingColors []string `json:"existing_colors"`
	ExistingFonts  []string `json:"existing_fonts"`
	Tone           *string  `json:"tone"`
	MustUse        []string `json:"must_use"`
	MustAvoid      []string `json:"must_avoid"`
}

type Asset struct {
	File string `json:"file"`
	Kind string `json:"kind"`
	Note string `json:"note"`
}

var pageKinds = map[string]bool{
	"landing": true, "product": true, "event": true, "portfolio": true, "menu": true,
	"organization": true, "campaign": true, "tool": true, "other": true,
}

func (b *Brief) Validate() error {
	var errs []string
	if strings.TrimSpace(b.Language) == "" {
		errs = append(errs, "language is empty")
	}
	if !pageKinds[b.PageKind] {
		errs = append(errs, fmt.Sprintf("page_kind %q is not one of the allowed values", b.PageKind))
	}
	if strings.TrimSpace(b.Subject) == "" {
		errs = append(errs, "subject is empty")
	}
	if strings.TrimSpace(b.PageJob) == "" {
		errs = append(errs, "page_job is empty")
	}
	return joinErrs(errs)
}

// StyleText returns the client's own style words, or an empty string.
func (b *Brief) StyleText() string {
	if b.RequestedStyle == nil {
		return ""
	}
	return *b.RequestedStyle
}

// Copy is the content stage's output: every word on the page.
type Copy struct {
	PageTitle       string        `json:"page_title"`
	MetaDescription string        `json:"meta_description"`
	PrimaryCTA      Link          `json:"primary_cta"`
	Sections        []Section     `json:"sections"`
	Footer          Footer        `json:"footer"`
	Placeholders    []Placeholder `json:"placeholders"`
	SubjectDetail   string        `json:"subject_detail"`
}

type Link struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type Section struct {
	ID      string   `json:"id"`
	Purpose string   `json:"purpose"`
	Heading string   `json:"heading"`
	Body    []string `json:"body"`
	Items   []Item   `json:"items,omitempty"`
	CTA     *Link    `json:"cta,omitempty"`
}

type Item struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type Footer struct {
	Text  string `json:"text"`
	Links []Link `json:"links"`
}

type Placeholder struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

var placeholderRe = regexp.MustCompile(`\[\[([a-z0-9_\-]+):[^\]]*\]\]`)

// PlaceholderKeys returns every placeholder key used anywhere in a string.
func PlaceholderKeys(s string) []string {
	var keys []string
	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		keys = append(keys, m[1])
	}
	return keys
}

func (c *Copy) Validate() error {
	var errs []string
	if strings.TrimSpace(c.PageTitle) == "" {
		errs = append(errs, "page_title is empty")
	}
	if strings.TrimSpace(c.PrimaryCTA.Label) == "" {
		errs = append(errs, "primary_cta.label is empty")
	}
	if n := len(c.Sections); n < 3 || n > 7 {
		errs = append(errs, fmt.Sprintf("there are %d sections; write between 3 and 7", n))
	}
	seen := map[string]bool{}
	for i, s := range c.Sections {
		if s.ID == "" {
			errs = append(errs, fmt.Sprintf("section %d has no id", i))
		}
		if seen[s.ID] {
			errs = append(errs, fmt.Sprintf("section id %q is used twice", s.ID))
		}
		seen[s.ID] = true
	}
	// Every placeholder used in the text must be declared.
	declared := map[string]bool{}
	for _, p := range c.Placeholders {
		declared[p.Key] = true
	}
	for _, key := range PlaceholderKeys(c.allText()) {
		if !declared[key] {
			errs = append(errs, fmt.Sprintf("placeholder %q is used but not listed in placeholders", key))
		}
	}
	return joinErrs(errs)
}

func (c *Copy) allText() string {
	var b strings.Builder
	b.WriteString(c.PageTitle + " " + c.MetaDescription + " " + c.PrimaryCTA.Label + " " + c.PrimaryCTA.Href)
	for _, s := range c.Sections {
		b.WriteString(" " + s.Heading + " " + strings.Join(s.Body, " "))
		for _, it := range s.Items {
			b.WriteString(" " + it.Title + " " + it.Text)
		}
		if s.CTA != nil {
			b.WriteString(" " + s.CTA.Label + " " + s.CTA.Href)
		}
	}
	b.WriteString(" " + c.Footer.Text)
	for _, l := range c.Footer.Links {
		b.WriteString(" " + l.Label + " " + l.Href)
	}
	return b.String()
}

// SectionIDs lists section ids in page order.
func (c *Copy) SectionIDs() []string {
	ids := make([]string, len(c.Sections))
	for i, s := range c.Sections {
		ids[i] = s.ID
	}
	return ids
}

// Plan is the design plan, from the plan stage and revised by the critique.
type Plan struct {
	Concept       string        `json:"concept"`
	BoldMove      string        `json:"bold_move"`
	SubjectDetail string        `json:"subject_detail"`
	ThemeMode     string        `json:"theme_mode"`
	ThemeReason   string        `json:"theme_reason,omitempty"`
	Colors        Palettes      `json:"colors"`
	Type          Typography    `json:"type"`
	Layout        Layout        `json:"layout"`
	Sections      []SectionPlan `json:"sections"`
	Images        []ImagePlan   `json:"images"`
	Motion        string        `json:"motion"`
	Principles    []string      `json:"principles"`
}

type Palettes struct {
	Light []Color `json:"light"`
	Dark  []Color `json:"dark,omitempty"`
}

type Color struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Hex    string `json:"hex"`
	Source string `json:"source"`
}

type Typography struct {
	Display Face      `json:"display"`
	Body    Face      `json:"body"`
	Scale   []float64 `json:"scale_rem"`
}

type Face struct {
	Family   string `json:"family"`
	Weights  []int  `json:"weights"`
	Fallback string `json:"fallback"`
	Source   string `json:"source"`
}

type Layout struct {
	Concept          string `json:"concept"`
	Alignment        string `json:"alignment"`
	AlignmentReason  string `json:"alignment_reason"`
	WireframeDesktop string `json:"wireframe_desktop"`
	WireframeMobile  string `json:"wireframe_mobile"`
}

type SectionPlan struct {
	ID        string `json:"id"`
	Treatment string `json:"treatment"`
}

type ImagePlan struct {
	Use   string  `json:"use"`
	File  *string `json:"file"`
	Drawn *string `json:"drawn"`
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// RequiredRoles must be present in every palette.
var RequiredRoles = []string{"bg", "text", "muted", "accent"}

func (p *Plan) Validate() error {
	var errs []string
	if p.ThemeMode != "dual" && p.ThemeMode != "single" {
		errs = append(errs, fmt.Sprintf("theme_mode %q must be dual or single", p.ThemeMode))
	}
	themes := map[string][]Color{"light": p.Colors.Light}
	if p.ThemeMode == "dual" {
		themes["dark"] = p.Colors.Dark
	}
	for theme, colors := range themes {
		if n := len(colors); n < 4 || n > 7 {
			errs = append(errs, fmt.Sprintf("%s palette has %d colors; give 4 to 6", theme, n))
		}
		roles := map[string]bool{}
		for _, c := range colors {
			if !hexRe.MatchString(c.Hex) {
				errs = append(errs, fmt.Sprintf("%s color %q has hex %q; use #RRGGBB", theme, c.Name, c.Hex))
			}
			roles[c.Role] = true
		}
		for _, r := range RequiredRoles {
			if !roles[r] {
				errs = append(errs, fmt.Sprintf("%s palette has no %q role", theme, r))
			}
		}
	}
	if strings.TrimSpace(p.Type.Display.Family) == "" || strings.TrimSpace(p.Type.Body.Family) == "" {
		errs = append(errs, "type.display.family and type.body.family are required")
	}
	if strings.TrimSpace(p.BoldMove) == "" {
		errs = append(errs, "bold_move is empty")
	}
	if len(p.Sections) == 0 {
		errs = append(errs, "sections is empty")
	}
	return joinErrs(errs)
}

// ColorByRole returns the first color with a role in a theme.
func (p *Plan) ColorByRole(theme, role string) (Color, bool) {
	colors := p.Colors.Light
	if theme == "dark" {
		colors = p.Colors.Dark
	}
	for _, c := range colors {
		if c.Role == role {
			return c, true
		}
	}
	return Color{}, false
}

// Critique is the critique stage's output.
type Critique struct {
	Axes []struct {
		Axis    string `json:"axis"`
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	} `json:"axes"`
	Changes []struct {
		What string `json:"what"`
		Why  string `json:"why"`
	} `json:"changes"`
	RevisedPlan Plan `json:"revised_plan"`
}

func (c *Critique) Validate() error {
	if err := c.RevisedPlan.Validate(); err != nil {
		return fmt.Errorf("revised_plan: %w", err)
	}
	return nil
}

// Review is the visual review stage's output.
type Review struct {
	Verdict string  `json:"verdict"`
	Issues  []Issue `json:"issues"`
	Notes   string  `json:"notes"`
}

type Issue struct {
	Severity string `json:"severity"`
	Where    string `json:"where"`
	Problem  string `json:"problem"`
	Fix      string `json:"fix"`
}

func (r *Review) Validate() error {
	var errs []string
	if r.Verdict != "pass" && r.Verdict != "fix" {
		errs = append(errs, fmt.Sprintf("verdict %q must be pass or fix", r.Verdict))
	}
	for i, is := range r.Issues {
		switch is.Severity {
		case "blocker", "major", "minor":
		default:
			errs = append(errs, fmt.Sprintf("issue %d has severity %q", i, is.Severity))
		}
	}
	if len(r.Issues) > 12 {
		errs = append(errs, fmt.Sprintf("%d issues reported; report at most 12", len(r.Issues)))
	}
	return joinErrs(errs)
}

// Edits is the repair stage's output when it returns edits.
type Edits struct {
	Edits []struct {
		Find    string `json:"find"`
		Replace string `json:"replace"`
	} `json:"edits"`
	Skipped []struct {
		Issue  string `json:"issue"`
		Reason string `json:"reason"`
	} `json:"skipped"`
}

func (e *Edits) Validate() error {
	for i, ed := range e.Edits {
		if ed.Find == "" {
			return fmt.Errorf("edit %d has an empty find", i)
		}
	}
	return nil
}

func joinErrs(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, "; "))
}
