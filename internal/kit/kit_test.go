package kit

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const page = `<!DOCTYPE html><html lang="pt-MZ"><head><meta charset="utf-8">
<style type="text/tailwindcss">
@theme { --color-accent: #0f766e; --font-display: "Fraunces", serif; }
</style></head>
<body class="bg-background text-foreground">
<a class="btn btn-primary btn-lg" href="#"><i data-icon="phone" class="size-4"></i>Ligar</a>
<span class="icon-tile"><i data-icon="wrench" aria-label="Oficina"></i></span>
<i data-icon="no-such-icon"></i>
<h1 class="font-display text-5xl tracking-tight md:text-6xl">Olá</h1>
</body></html>`

func TestIcons(t *testing.T) {
	res, err := Compile(context.Background(), "", strings.Replace(page, `<style type="text/tailwindcss">`, `<style>`, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `<svg class="size-4 shrink-0" aria-hidden="true"`) {
		t.Errorf("decorative icon not expanded: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `role="img" aria-label="Oficina"`) {
		t.Error("labelled icon lost its label")
	}
	if len(res.UnknownIcons) != 1 || res.UnknownIcons[0] != "no-such-icon" {
		t.Errorf("unknown icons = %v", res.UnknownIcons)
	}
	if strings.Contains(res.HTML, "data-icon") {
		t.Error("an <i data-icon> tag survived")
	}
	if n := len(Icons()); n < 100 {
		t.Errorf("only %d icons embedded", n)
	}
}

func TestCompile(t *testing.T) {
	bin := os.Getenv("FAZ_TAILWIND")
	if bin == "" {
		if p, err := exec.LookPath("tailwindcss"); err == nil {
			bin = p
		}
	}
	if bin == "" {
		t.Skip("no Tailwind binary; set FAZ_TAILWIND")
	}
	res, err := Compile(context.Background(), bin, page)
	if err != nil {
		t.Fatal(err)
	}
	if Uses(res.HTML) {
		t.Fatal("tailwind block left in the compiled page")
	}
	for _, want := range []string{".btn-primary", ".icon-tile", "#0f766e", "Fraunces", ".md\\:text-6xl", "prefers-reduced-motion"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("compiled CSS lacks %q", want)
		}
	}
	if _, err := Compile(context.Background(), bin, strings.Replace(page, "@theme", "@import \"x.css\"; @theme", 1)); err == nil {
		t.Error("a page block with @import was accepted")
	}
}
