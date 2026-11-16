package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/render"
	"github.com/GraHms/faz/internal/spec"
)

// scripted answers each stage with the next response in its list.
type scripted struct {
	byStage map[string][]string
	calls   []llm.Request
}

func (s *scripted) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	s.calls = append(s.calls, req)
	list := s.byStage[req.Stage]
	if len(list) == 0 {
		return llm.Response{}, fmt.Errorf("no scripted response for %s", req.Stage)
	}
	s.byStage[req.Stage] = list[1:]
	return llm.Response{Text: list[0], StopReason: "end_turn", Model: req.Model, Usage: llm.Usage{InputTokens: 1000, OutputTokens: 200}}, nil
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../examples/fake/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decode[T any](t *testing.T, name string) *T {
	t.Helper()
	v := new(T)
	if err := llm.DecodeJSON(fixture(t, name), v); err != nil {
		t.Fatal(err)
	}
	return v
}

// builtPage returns the sample page after the recorded build and repair.
func builtPage(t *testing.T) string {
	t.Helper()
	src, err := llm.ExtractHTML(fixture(t, "build.txt"))
	if err != nil {
		t.Fatal(err)
	}
	out, a, err := applyResponse(fixture(t, "repair.txt"), src)
	if err != nil || a.Applied != 3 {
		t.Fatalf("repair fixture: applied %d, err %v", a.Applied, err)
	}
	return out
}

func chrome(t *testing.T) *render.Chrome {
	path, err := render.FindChrome()
	if err != nil {
		return nil
	}
	return &render.Chrome{Path: path, NoSandbox: os.Geteuid() == 0, ExtraFlags: strings.Fields(os.Getenv("FAZ_CHROME_FLAGS"))}
}

func TestIntakeReturnsQuestions(t *testing.T) {
	p := &Pipeline{LLM: &llm.Fake{Dir: "../../examples/fake"}, Opt: Options{
		BriefText: "mel", OutDir: t.TempDir(), Profile: Profiles["quality"],
	}}
	b, err := p.Intake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Questions) != 3 || b.Questions[0].Kind != "text" {
		t.Fatalf("questions = %+v", b.Questions)
	}
	if p.Report().TotalCostUSD <= 0 {
		t.Error("intake cost not recorded")
	}
}

func TestClassifyValidatesAndRetries(t *testing.T) {
	s := &scripted{byStage: map[string][]string{"classify": {
		"```json\n{\"kind\": \"vague\", \"reply\": \"Que cor?\", \"options\": []}\n```", // invalid: no options
		"```json\n{\"kind\": \"vague\", \"reply\": \"Que cor preferes?\", \"options\": [\"Verde\", \"Azul\"]}\n```",
	}}}
	p := &Pipeline{LLM: s, Opt: Options{Profile: Profiles["quality"]}}
	c, err := p.Classify(context.Background(), ClassifyInput{
		Brief: decode[spec.Brief](t, "intake.txt"), Copy: decode[spec.Copy](t, "content.txt"), Message: "muda as cores", PriceList: "Alteração: 5 créditos",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != "vague" || c.Chargeable() || len(s.calls) != 2 {
		t.Fatalf("classification %+v after %d calls", c, len(s.calls))
	}
	if !strings.Contains(s.calls[1].Parts[len(s.calls[1].Parts)-1].Text, "failed validation") {
		t.Error("retry did not quote the validation error")
	}
}

func TestEditFillsPlaceholderWithCachedPage(t *testing.T) {
	fake := &llm.Fake{Dir: "../../examples/fake"}
	p := &Pipeline{LLM: fake, Opt: Options{OutDir: t.TempDir(), Profile: Profiles["quality"], Chrome: chrome(t)}}
	res, err := p.Edit(context.Background(), EditInput{
		Source: builtPage(t), Brief: decode[spec.Brief](t, "intake.txt"), Copy: decode[spec.Copy](t, "content.txt"),
		Plan: &decode[spec.Critique](t, "critique.txt").RevisedPlan, Instruction: "A entrega custa 50 MT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rejected || res.Applied != 1 || !strings.Contains(res.Page, "50 MT em Maputo") || strings.Contains(res.Page, `data-ph="entrega"`) {
		t.Fatalf("edit result %+v", res)
	}
	if !strings.HasPrefix(res.Summary, "Pus o custo") {
		t.Errorf("summary %q", res.Summary)
	}
	call := fake.Calls[len(fake.Calls)-1]
	if !call.Parts[0].Cache || !strings.Contains(call.Parts[0].Text, "<!DOCTYPE html>") {
		t.Error("the current page should be the cached first part")
	}
}

func TestEditThatBreaksThePageIsRejected(t *testing.T) {
	c := chrome(t)
	if c == nil {
		t.Skip("needs Chrome to detect the overflow")
	}
	breaking := "```json\n{\"summary\": \"Adicionei uma faixa.\", \"edits\": [{\"find\": \"</main>\", \"replace\": \"<div style=\\\"width:900px\\\">faixa</div></main>\"}]}\n```"
	noop := "```json\n{\"edits\": [{\"find\": \"/* Harvest band */\", \"replace\": \"/* Harvest band (checked) */\"}]}\n```"
	s := &scripted{byStage: map[string][]string{"edit": {breaking}, "repair": {noop}}}
	p := &Pipeline{LLM: s, Opt: Options{OutDir: t.TempDir(), Profile: Profiles["quality"], Chrome: c}}
	source := builtPage(t)
	res, err := p.Edit(context.Background(), EditInput{
		Source: source, Brief: decode[spec.Brief](t, "intake.txt"), Copy: decode[spec.Copy](t, "content.txt"),
		Plan: &decode[spec.Critique](t, "critique.txt").RevisedPlan, Instruction: "Adiciona uma faixa",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Rejected || !res.Repaired || res.Source != source {
		t.Fatalf("expected a rejected edit that keeps the old page; got rejected=%v repaired=%v reason=%q", res.Rejected, res.Repaired, res.Reason)
	}
	if !strings.Contains(res.Reason, "scrolls sideways") {
		t.Errorf("reason %q should name the overflow", res.Reason)
	}
}
