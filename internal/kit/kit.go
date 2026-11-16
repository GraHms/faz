// Package kit turns a kit page into a plain self contained page.
//
// A kit page is HTML written with Tailwind utility classes and the Faz
// component classes in base.css (shadcn-style tokens, buttons, cards,
// inputs). It carries its identity in one <style type="text/tailwindcss">
// block that overrides the @theme tokens, and names icons with
// <i data-icon="name"></i>. Compile inlines the icons, runs the Tailwind
// standalone binary over the page, and replaces the block with the compiled
// CSS, so the result needs no build step or framework in the browser.
package kit

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed base.css
var baseCSS string

//go:embed icons/*.svg
var iconFS embed.FS

var (
	reBlock = regexp.MustCompile(`(?is)<style\s+type=["']text/tailwindcss["']\s*>(.*?)</style>`)
	reIcon  = regexp.MustCompile(`(?is)<i\s([^>]*?)\bdata-icon=["']([^"']+)["']([^>]*)>\s*</i>`)
	reClass = regexp.MustCompile(`(?i)\bclass=["']([^"']*)["']`)
	reLabel = regexp.MustCompile(`(?i)\baria-label=["']([^"']*)["']`)
)

// Uses reports whether a page is a kit page.
func Uses(html string) bool { return reBlock.MatchString(html) }

// Icons lists the icon names a page may use.
func Icons() []string {
	entries, _ := iconFS.ReadDir("icons")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".svg"))
	}
	sort.Strings(names)
	return names
}

// Result is a compiled page and what it could not resolve.
type Result struct {
	HTML         string
	UnknownIcons []string // data-icon names that are not in the set; dropped
}

// Compile expands icons and compiles the page's Tailwind block with the
// binary at bin. A page without the block only gets its icons expanded.
func Compile(ctx context.Context, bin, page string) (Result, error) {
	var res Result
	unknown := map[string]bool{}
	page = reIcon.ReplaceAllStringFunc(page, func(tag string) string {
		m := reIcon.FindStringSubmatch(tag)
		name, attrs := strings.ToLower(strings.TrimSpace(m[2])), m[1]+" "+m[3]
		svg, err := iconFS.ReadFile("icons/" + name + ".svg")
		if err != nil {
			unknown[name] = true
			return ""
		}
		class := "shrink-0"
		if c := reClass.FindStringSubmatch(attrs); c != nil {
			class = strings.TrimSpace(c[1] + " shrink-0")
		}
		a11y := `aria-hidden="true" focusable="false"`
		if l := reLabel.FindStringSubmatch(attrs); l != nil && strings.TrimSpace(l[1]) != "" {
			a11y = fmt.Sprintf(`role="img" aria-label="%s"`, l[1])
		}
		return strings.Replace(strings.TrimSpace(string(svg)), "<svg ", fmt.Sprintf(`<svg class="%s" %s `, class, a11y), 1)
	})
	for n := range unknown {
		res.UnknownIcons = append(res.UnknownIcons, n)
	}
	sort.Strings(res.UnknownIcons)

	block := reBlock.FindStringSubmatchIndex(page)
	if block == nil {
		res.HTML = page
		return res, nil
	}
	if bin == "" {
		return res, errors.New("kit page needs the Tailwind binary; set FAZ_TAILWIND")
	}
	dir, err := os.MkdirTemp("", "faz-kit-")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(dir)
	own := page[block[2]:block[3]]
	if strings.Contains(own, "@import") || strings.Contains(own, "@source") {
		return res, errors.New("the page's tailwind block may not use @import or @source")
	}
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(page[:block[0]]+page[block[1]:]), 0o600); err != nil {
		return res, err
	}
	if err := os.WriteFile(filepath.Join(dir, "input.css"), []byte(baseCSS+"\n/* page */\n"+own), 0o600); err != nil {
		return res, err
	}
	cctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "-i", "input.css", "-o", "out.css", "--minify")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return res, fmt.Errorf("tailwind: %v: %s", err, lastLines(stderr.String(), 6))
	}
	css, err := os.ReadFile(filepath.Join(dir, "out.css"))
	if err != nil {
		return res, err
	}
	res.HTML = page[:block[0]] + "<style>" + strings.ReplaceAll(string(css), "</style", `<\/style`) + "</style>" + page[block[1]:]
	return res, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
