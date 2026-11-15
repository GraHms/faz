package check

import (
	"fmt"
	"strings"

	"github.com/GraHms/faz/internal/history"
	"github.com/GraHms/faz/internal/spec"
)

// pair is one contrast requirement between two palette roles.
type pair struct {
	fg, bg   string
	min      float64
	severity string // severity when below min
	hardMin  float64
}

// Required contrast between palette roles. Muted text is often small, so it
// gets the same bar as body text.
var pairs = []pair{
	{fg: "text", bg: "bg", min: 4.5, severity: Major, hardMin: 3},
	{fg: "muted", bg: "bg", min: 4.5, severity: Major, hardMin: 3},
	{fg: "text", bg: "surface", min: 4.5, severity: Major, hardMin: 3},
	{fg: "accent", bg: "bg", min: 3, severity: Major, hardMin: 2},
	{fg: "accent_ink", bg: "accent", min: 4.5, severity: Major, hardMin: 3},
}

// Generic looks the plan should not land on unless the brief asks.
var genericFonts = []string{"inter", "space grotesk"}

type genericColor struct {
	name   string
	role   string
	hex    string
	within float64
}

var genericCombos = [][]genericColor{
	{ // cream background with a terracotta accent
		{"warm cream background", "bg", "#F4F1EA", 14},
		{"terracotta accent", "accent", "#D97757", 30},
	},
}

// Plan runs every deterministic check on a design plan.
func Plan(p *spec.Plan, c *spec.Copy, b *spec.Brief, recent []history.Entry) []Finding {
	var fs []Finding
	fs = append(fs, planStructure(p, c)...)
	fs = append(fs, planContrast(p)...)
	fs = append(fs, planDefaults(p, b)...)
	fs = append(fs, planRepeats(p, recent)...)
	return fs
}

func themes(p *spec.Plan) []string {
	if p.ThemeMode == "dual" {
		return []string{"light", "dark"}
	}
	return []string{"light"}
}

func planStructure(p *spec.Plan, c *spec.Copy) []Finding {
	var fs []Finding
	copyIDs := map[string]bool{}
	for _, id := range c.SectionIDs() {
		copyIDs[id] = true
	}
	planIDs := map[string]bool{}
	for _, s := range p.Sections {
		planIDs[s.ID] = true
		if !copyIDs[s.ID] {
			fs = append(fs, Finding{Check: "plan.sections", Severity: Major, Where: s.ID,
				Message: fmt.Sprintf("plan section %q has no matching section in the copy", s.ID)})
		}
	}
	for id := range copyIDs {
		if !planIDs[id] {
			fs = append(fs, Finding{Check: "plan.sections", Severity: Major, Where: id,
				Message: fmt.Sprintf("copy section %q has no treatment in the plan", id)})
		}
	}
	if strings.EqualFold(p.Type.Display.Family, p.Type.Body.Family) {
		fs = append(fs, Finding{Check: "plan.type", Severity: Info,
			Message: "display and body use one family; make sure the weights and sizes do the work"})
	}
	return fs
}

func planContrast(p *spec.Plan) []Finding {
	var fs []Finding
	for _, theme := range themes(p) {
		for _, req := range pairs {
			fg, okF := p.ColorByRole(theme, req.fg)
			bg, okB := p.ColorByRole(theme, req.bg)
			if !okF || !okB {
				continue
			}
			a, errA := ParseHex(fg.Hex)
			b, errB := ParseHex(bg.Hex)
			if errA != nil || errB != nil {
				continue
			}
			ratio := Contrast(a, b)
			if ratio >= req.min {
				continue
			}
			sev := req.severity
			if ratio < req.hardMin {
				sev = Blocker
			}
			fs = append(fs, Finding{
				Check: "plan.contrast", Severity: sev, Where: theme,
				Message: fmt.Sprintf("%s (%s) on %s (%s) is %.2f:1; needs %.1f:1", req.fg, fg.Hex, req.bg, bg.Hex, ratio, req.min),
			})
		}
		// Without accent_ink, something must still be readable on the accent.
		if _, ok := p.ColorByRole(theme, "accent_ink"); !ok {
			acc, okA := p.ColorByRole(theme, "accent")
			if okA {
				a, _ := ParseHex(acc.Hex)
				best := 0.0
				for _, role := range []string{"bg", "text"} {
					if c, ok := p.ColorByRole(theme, role); ok {
						x, _ := ParseHex(c.Hex)
						if r := Contrast(x, a); r > best {
							best = r
						}
					}
				}
				if best < 4.5 {
					fs = append(fs, Finding{Check: "plan.contrast", Severity: Major, Where: theme,
						Message: fmt.Sprintf("no palette color reaches 4.5:1 on the accent (%s); add an accent_ink role", acc.Hex)})
				}
			}
		}
	}
	return fs
}

func pinnedByClient(b *spec.Brief, needle string) bool {
	needle = strings.ToLower(needle)
	if strings.Contains(strings.ToLower(b.StyleText()), needle) {
		return true
	}
	for _, f := range append(append([]string{}, b.Brand.ExistingFonts...), b.Brand.MustUse...) {
		if strings.Contains(strings.ToLower(f), needle) {
			return true
		}
	}
	return false
}

func clientColorNear(b *spec.Brief, c RGB, within float64) bool {
	for _, h := range b.Brand.ExistingColors {
		if x, err := ParseHex(h); err == nil && Distance(x, c) <= within {
			return true
		}
	}
	return false
}

func planDefaults(p *spec.Plan, b *spec.Brief) []Finding {
	var fs []Finding
	for _, face := range []spec.Face{p.Type.Display, p.Type.Body} {
		fam := strings.ToLower(strings.TrimSpace(face.Family))
		for _, g := range genericFonts {
			if fam == g && !pinnedByClient(b, g) {
				fs = append(fs, Finding{Check: "plan.defaults", Severity: Major, Where: "type",
					Message: fmt.Sprintf("%s is the generic safe face; choose one for this subject", face.Family)})
			}
		}
	}
	for _, theme := range themes(p) {
		for _, combo := range genericCombos {
			hits := 0
			var names []string
			for _, gc := range combo {
				c, ok := p.ColorByRole(theme, gc.role)
				if !ok {
					continue
				}
				x, err := ParseHex(c.Hex)
				ref, _ := ParseHex(gc.hex)
				if err == nil && Distance(x, ref) <= gc.within && !clientColorNear(b, x, 20) {
					hits++
					names = append(names, gc.name)
				}
			}
			if hits == len(combo) {
				fs = append(fs, Finding{Check: "plan.defaults", Severity: Major, Where: theme,
					Message: "palette matches a generic default: " + strings.Join(names, " with ")})
			}
		}
		// Near black background with one loud green or vermilion accent.
		if bg, ok := p.ColorByRole(theme, "bg"); ok {
			if acc, ok := p.ColorByRole(theme, "accent"); ok {
				x, _ := ParseHex(bg.Hex)
				a, _ := ParseHex(acc.Hex)
				h, s, _ := a.HSL()
				loud := s > 0.7 && ((h >= 70 && h <= 160) || h <= 18 || h >= 350)
				if x.Luminance() < 0.012 && loud && !clientColorNear(b, a, 20) {
					fs = append(fs, Finding{Check: "plan.defaults", Severity: Major, Where: theme,
						Message: fmt.Sprintf("near black background (%s) with a single loud accent (%s) is a generic default", bg.Hex, acc.Hex)})
				}
			}
		}
	}
	text := strings.ToLower(strings.Join([]string{p.Concept, p.BoldMove, p.Motion, p.Layout.Concept, sectionText(p)}, " "))
	for _, kw := range []struct{ word, msg, sev string }{
		{"100vh", "the plan sizes something to the viewport height; size the hero to its content", Major},
		{"full viewport", "the plan sizes something to the viewport height; size the hero to its content", Major},
		{"eyebrow", "the plan uses eyebrow labels; keep them only if they carry information", Minor},
		{"fade up", "the plan mentions fade up entrances; keep at most one orchestrated moment", Minor},
		{"fade-up", "the plan mentions fade up entrances; keep at most one orchestrated moment", Minor},
		{"on scroll", "the plan ties motion to scrolling; content must be visible at rest", Minor},
		{"gradient", "the plan uses a gradient; make sure it comes from the subject, not decoration", Info},
		{"glassmorphism", "glassmorphism is a generic default", Major},
	} {
		if strings.Contains(text, kw.word) && !pinnedByClient(b, kw.word) {
			fs = append(fs, Finding{Check: "plan.defaults", Severity: kw.sev, Message: kw.msg})
		}
	}
	if strings.EqualFold(p.Layout.Alignment, "center") || strings.EqualFold(p.Layout.Alignment, "centered") {
		fs = append(fs, Finding{Check: "plan.defaults", Severity: Minor, Where: "layout",
			Message: "everything centered is a common default; confirm the subject calls for it"})
	}
	return fs
}

func sectionText(p *spec.Plan) string {
	var parts []string
	for _, s := range p.Sections {
		parts = append(parts, s.Treatment)
	}
	return strings.Join(parts, " ")
}

func planRepeats(p *spec.Plan, recent []history.Entry) []Finding {
	var fs []Finding
	bg, _ := p.ColorByRole("light", "bg")
	acc, _ := p.ColorByRole("light", "accent")
	bgRGB, errBg := ParseHex(bg.Hex)
	accRGB, errAcc := ParseHex(acc.Hex)
	for _, e := range recent {
		if strings.EqualFold(e.DisplayFont, p.Type.Display.Family) {
			fs = append(fs, Finding{Check: "plan.repeat", Severity: Major, Where: "type",
				Message: fmt.Sprintf("display face %s was used recently for %q", e.DisplayFont, e.Subject)})
		}
		if x, err := ParseHex(e.Accent); err == nil && errAcc == nil && Distance(x, accRGB) < 22 {
			fs = append(fs, Finding{Check: "plan.repeat", Severity: Minor, Where: "light",
				Message: fmt.Sprintf("accent %s is close to %s used recently for %q", acc.Hex, e.Accent, e.Subject)})
		}
		if x, err := ParseHex(e.Background); err == nil && errBg == nil && Distance(x, bgRGB) < 10 {
			fs = append(fs, Finding{Check: "plan.repeat", Severity: Minor, Where: "light",
				Message: fmt.Sprintf("background %s is nearly the same as %s used recently for %q", bg.Hex, e.Background, e.Subject)})
		}
	}
	return fs
}

// FixPlanContrast repairs any contrast pair still below its minimum by
// nudging the foreground color. It runs after the critique, as a safety net,
// and returns a description of every change.
func FixPlanContrast(p *spec.Plan) []string {
	var changes []string
	for _, theme := range themes(p) {
		colors := p.Colors.Light
		if theme == "dark" {
			colors = p.Colors.Dark
		}
		index := map[string]int{}
		for i, c := range colors {
			if _, seen := index[c.Role]; !seen {
				index[c.Role] = i
			}
		}
		for _, req := range pairs {
			fi, okF := index[req.fg]
			bi, okB := index[req.bg]
			if !okF || !okB {
				continue
			}
			fg, errF := ParseHex(colors[fi].Hex)
			bg, errB := ParseHex(colors[bi].Hex)
			if errF != nil || errB != nil || Contrast(fg, bg) >= req.min {
				continue
			}
			fixed, _ := FixContrast(fg, bg, req.min+0.05)
			old := colors[fi].Hex
			colors[fi].Hex = fixed.Hex()
			changes = append(changes, fmt.Sprintf("%s %s: %s changed from %s to %s to reach %.1f:1 on %s",
				theme, colors[fi].Name, req.fg, old, colors[fi].Hex, req.min, req.bg))
		}
	}
	return changes
}

// Entry summarizes a plan for the run history.
func Entry(p *spec.Plan, b *spec.Brief) history.Entry {
	bg, _ := p.ColorByRole("light", "bg")
	acc, _ := p.ColorByRole("light", "accent")
	return history.Entry{
		Subject:     b.Subject,
		PageKind:    b.PageKind,
		DisplayFont: p.Type.Display.Family,
		BodyFont:    p.Type.Body.Family,
		Background:  bg.Hex,
		Accent:      acc.Hex,
		BoldMove:    p.BoldMove,
	}
}
