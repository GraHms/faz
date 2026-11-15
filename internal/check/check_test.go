package check

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/GraHms/faz/internal/history"
	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/spec"
)

func TestContrastKnownValues(t *testing.T) {
	black, _ := ParseHex("#000000")
	white, _ := ParseHex("#FFFFFF")
	if got := Contrast(black, white); math.Abs(got-21) > 0.01 {
		t.Fatalf("black on white = %.2f, want 21", got)
	}
	grey, _ := ParseHex("#777777")
	if got := Contrast(grey, white); math.Abs(got-4.48) > 0.02 {
		t.Fatalf("#777 on white = %.2f, want about 4.48", got)
	}
}

func TestFixContrastReachesTarget(t *testing.T) {
	fg, _ := ParseHex("#A39B8B")
	bg, _ := ParseHex("#F4F0E8")
	fixed, ok := FixContrast(fg, bg, 4.55)
	if !ok || Contrast(fixed, bg) < 4.5 {
		t.Fatalf("fixed %s reaches %.2f, want 4.5", fixed.Hex(), Contrast(fixed, bg))
	}
}

func load[T any](t *testing.T, path string) *T {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v := new(T)
	if err := llm.DecodeJSON(string(b), v); err != nil {
		t.Fatal(err)
	}
	return v
}

func messages(fs []Finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.Severity + " " + f.Check + " " + f.Message + "\n")
	}
	return b.String()
}

func TestDraftPlanIsFlaggedAsGeneric(t *testing.T) {
	brief := load[spec.Brief](t, "../../examples/fake/intake.txt")
	copyDoc := load[spec.Copy](t, "../../examples/fake/content.txt")
	draft := load[spec.Plan](t, "../../examples/fake/plan.txt")
	fs := Plan(draft, copyDoc, brief, nil)
	all := messages(fs)
	for _, want := range []string{"Inter is the generic safe face", "warm cream background with terracotta accent", "viewport height", "muted (#A39B8B)"} {
		if !strings.Contains(all, want) {
			t.Errorf("expected a finding containing %q; got:\n%s", want, all)
		}
	}
}

func TestRevisedPlanIsClean(t *testing.T) {
	brief := load[spec.Brief](t, "../../examples/fake/intake.txt")
	copyDoc := load[spec.Copy](t, "../../examples/fake/content.txt")
	crit := load[spec.Critique](t, "../../examples/fake/critique.txt")
	if err := crit.Validate(); err != nil {
		t.Fatal(err)
	}
	fs := Filter(Plan(&crit.RevisedPlan, copyDoc, brief, nil), Major)
	if len(fs) > 0 {
		t.Fatalf("revised plan should have no major findings:\n%s", messages(fs))
	}
}

func TestRepeatOfRecentWorkIsFlagged(t *testing.T) {
	brief := load[spec.Brief](t, "../../examples/fake/intake.txt")
	copyDoc := load[spec.Copy](t, "../../examples/fake/content.txt")
	crit := load[spec.Critique](t, "../../examples/fake/critique.txt")
	recent := []history.Entry{{Subject: "a bakery", DisplayFont: "Zilla Slab", Accent: "#9B5C02", Background: "#FFFFFF"}}
	all := messages(Plan(&crit.RevisedPlan, copyDoc, brief, recent))
	if !strings.Contains(all, "Zilla Slab was used recently") || !strings.Contains(all, "accent #9A5B00 is close") {
		t.Fatalf("expected repeat findings, got:\n%s", all)
	}
}

func TestFixPlanContrastRepairsDraft(t *testing.T) {
	draft := load[spec.Plan](t, "../../examples/fake/plan.txt")
	changes := FixPlanContrast(draft)
	if len(changes) == 0 {
		t.Fatal("expected contrast changes on the draft plan")
	}
	for _, f := range planContrast(draft) {
		if f.Severity != Info && !strings.Contains(f.Message, "accent_ink") {
			t.Errorf("still failing after fix: %s", f.Message)
		}
	}
}

func TestStaticChecksOnBuiltPage(t *testing.T) {
	raw, err := os.ReadFile("../../examples/fake/build.txt")
	if err != nil {
		t.Fatal(err)
	}
	html, err := llm.ExtractHTML(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	crit := load[spec.Critique](t, "../../examples/fake/critique.txt")
	copyDoc := load[spec.Copy](t, "../../examples/fake/content.txt")
	if fs := Filter(Static(html, &crit.RevisedPlan, copyDoc), Major); len(fs) > 0 {
		t.Fatalf("built page should pass static checks:\n%s", messages(fs))
	}
	broken := strings.Replace(html, `<meta name="viewport" content="width=device-width, initial-scale=1">`, "", 1)
	broken = strings.Replace(broken, "Encomendar pelo WhatsApp", "Saber mais", -1)
	broken += `<script src="https://cdn.jsdelivr.net/npm/alpinejs@3"></script><script src="https://evil.example/x.js"></script>`
	all := messages(Static(broken, &crit.RevisedPlan, copyDoc))
	for _, want := range []string{"viewport meta", "primary action", "not pinned", "evil.example"} {
		if !strings.Contains(all, want) {
			t.Errorf("expected %q in findings:\n%s", want, all)
		}
	}
}
