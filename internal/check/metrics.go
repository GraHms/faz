package check

import (
	"fmt"
	"strings"

	"github.com/GraHms/faz/internal/render"
)

// Rendered turns browser measurements into findings. mobile marks the pass
// taken at phone width, where tap target size matters.
func Rendered(m render.Metrics, mobile bool, wantLang string) []Finding {
	var fs []Finding
	add := func(sev, check, where, msg string) {
		fs = append(fs, Finding{Check: check, Severity: sev, Where: m.Label + ": " + where, Message: msg})
	}
	if m.ScrollWidth > m.ViewportWidth+1 {
		var who []string
		for _, o := range m.Overflow {
			who = append(who, fmt.Sprintf("%s (right edge %dpx)", o.Where, o.Right))
		}
		add(Blocker, "render.overflow", strings.Join(first(who, 4), ", "),
			fmt.Sprintf("page scrolls sideways: content is %dpx wide in a %dpx viewport", m.ScrollWidth, m.ViewportWidth))
	}
	for _, e := range m.Errors {
		switch {
		case strings.HasPrefix(e, "failed to load") && (strings.Contains(e, "fonts.googleapis.com") || strings.Contains(e, "fonts.gstatic.com")):
			// Font hosts may be unreachable where the check runs; the
			// fallback stack still renders. Report it, do not block on it.
			add(Info, "render.fonts", "", e)
		case strings.HasPrefix(e, "failed to load"):
			add(Major, "render.resources", "", e)
		default:
			add(Blocker, "render.errors", "", e)
		}
	}
	for _, h := range m.HiddenText {
		add(Blocker, "render.hidden", h.Where, fmt.Sprintf("text is invisible at rest: %q", h.Text))
	}
	for _, lc := range m.LowContrast {
		sev := Major
		if lc.Ratio < 3 && lc.Need > 3 {
			sev = Blocker
		}
		add(sev, "render.contrast", lc.Where, fmt.Sprintf("%q measures %.2f:1, needs %.1f:1 at %.0fpx", lc.Text, lc.Ratio, lc.Need, lc.SizePx))
	}
	if mobile {
		for _, t := range m.SmallTargets {
			add(Minor, "render.targets", t.Where, fmt.Sprintf("tap target is %dx%dpx; aim for at least 44px tall", t.W, t.H))
		}
	}
	for _, s := range m.ImagesNoAlt {
		add(Major, "render.alt", s, "image has no alt attribute")
	}
	for _, s := range m.ImagesBroken {
		add(Blocker, "render.images", s, "image failed to load")
	}
	for _, s := range m.UnlabeledControls {
		add(Major, "render.labels", s, "form control has no label")
	}
	if m.H1Count != 1 {
		add(Major, "render.headings", "", fmt.Sprintf("page has %d h1 elements; it needs exactly one", m.H1Count))
	}
	for _, s := range m.HeadingSkips {
		add(Minor, "render.headings", "", "heading level skipped: "+s)
	}
	if wantLang != "" && !strings.EqualFold(primary(m.Lang), primary(wantLang)) {
		add(Major, "render.lang", "<html>", fmt.Sprintf("lang is %q; the brief asks for %q", m.Lang, wantLang))
	}
	var failed []string
	for _, f := range m.Fonts {
		if f.Status == "error" {
			failed = append(failed, f.Family+" "+f.Weight)
		}
	}
	if len(failed) > 0 {
		add(Info, "render.fonts", "", "fonts failed to load here (may be the render environment): "+strings.Join(first(failed, 6), ", "))
	}
	if m.PlaceholdersTotal > m.PlaceholdersShown {
		add(Major, "render.placeholders", "", fmt.Sprintf("%d of %d placeholder marks are not visible", m.PlaceholdersTotal-m.PlaceholdersShown, m.PlaceholdersTotal))
	}
	return fs
}

func primary(lang string) string {
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		return lang[:i]
	}
	return lang
}

func first(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
