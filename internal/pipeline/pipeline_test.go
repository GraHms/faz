package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GraHms/faz/internal/assets"
	"github.com/GraHms/faz/internal/check"
	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/prompts"
	"github.com/GraHms/faz/internal/render"
)

// TestEndToEndWithFakeModel runs every stage against recorded model output.
// With Chrome installed it also renders, measures, screenshots, applies the
// recorded repair and verifies again.
func TestEndToEndWithFakeModel(t *testing.T) {
	brief, err := os.ReadFile("../../examples/briefs/mel-niassa.md")
	if err != nil {
		t.Fatal(err)
	}
	files, err := assets.Load("../../examples/assets", 10<<20)
	if err != nil {
		t.Fatal(err)
	}
	var chrome *render.Chrome
	if path, err := render.FindChrome(); err == nil {
		chrome = &render.Chrome{Path: path, NoSandbox: os.Geteuid() == 0, ExtraFlags: strings.Fields(os.Getenv("FAZ_CHROME_FLAGS"))}
	} else {
		t.Log("no Chrome found; render and review stages are skipped")
	}
	out := t.TempDir()
	fake := &llm.Fake{Dir: "../../examples/fake"}
	p := &Pipeline{LLM: fake, Opt: Options{
		BriefText: string(brief), Assets: files, OutDir: out,
		Profile: Profiles["quality"], ProfileName: "quality",
		HistoryPath: filepath.Join(out, "history.jsonl"), Chrome: chrome,
	}}
	rep, err := p.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Every stage used the model the profile assigns to it.
	for _, call := range fake.Calls {
		if want := Profiles["quality"][call.Stage].Model; call.Model != want {
			t.Errorf("%s ran on %s, want %s", call.Stage, call.Model, want)
		}
		if !strings.Contains(call.System, "# Role") || strings.Contains(call.System, "{{DEFAULTS}}") {
			t.Errorf("%s system prompt not loaded or not expanded", call.Stage)
		}
	}
	if len(rep.PlanFindingsDraft) == 0 {
		t.Error("draft plan should have produced findings")
	}
	if n := len(check.Filter(rep.PlanFindingsFinal, check.Major)); n > 0 {
		t.Errorf("final plan has %d major findings: %+v", n, rep.PlanFindingsFinal)
	}

	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "assets/logo.svg") || !strings.Contains(string(page), "data:image/svg+xml;base64,") {
		t.Error("logo was not inlined into index.html")
	}
	if src, _ := os.ReadFile(filepath.Join(out, "source.html")); !strings.Contains(string(src), "assets/logo.svg") {
		t.Error("source.html should keep the assets/ reference")
	}

	if chrome == nil {
		if rep.Status != "unverified" {
			t.Errorf("status %q, want unverified without Chrome", rep.Status)
		}
		return
	}
	var sawOverflow bool
	for _, f := range rep.FindingsBeforeRepair {
		if f.Check == "render.overflow" && strings.HasPrefix(f.Where, "phone") {
			sawOverflow = true
		}
	}
	if !sawOverflow {
		t.Errorf("expected the calendar overflow on phone before repair; findings: %+v", rep.FindingsBeforeRepair)
	}
	if rep.Repair == nil || rep.Repair.Mode != "edits" || rep.Repair.Applied != 3 {
		t.Fatalf("repair = %+v", rep.Repair)
	}
	for _, f := range rep.Findings {
		if f.Severity == check.Blocker {
			t.Errorf("blocker after repair: %+v", f)
		}
	}
	if rep.Status != "ready" {
		t.Errorf("status %q, reasons %v", rep.Status, rep.StatusReasons)
	}
	shots, _ := filepath.Glob(filepath.Join(out, "screenshots", "final", "*.jpg"))
	if len(shots) < 4 {
		t.Errorf("expected final screenshots, found %d", len(shots))
	}
	var reviewImages int
	for _, c := range fake.Calls {
		if c.Stage == prompts.Review {
			for _, part := range c.Parts {
				if len(part.Image) > 0 {
					reviewImages++
				}
			}
		}
	}
	if reviewImages < 4 {
		t.Errorf("review saw %d images, want at least 4", reviewImages)
	}
	t.Logf("status %s, %d stage calls, estimated $%.3f, %d review images", rep.Status, len(rep.Stages), rep.TotalCostUSD, reviewImages)
}
