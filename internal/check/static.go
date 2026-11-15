package check

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/GraHms/faz/internal/spec"
)

// AllowedHosts may serve fonts, styles and scripts to a generated page.
var AllowedHosts = map[string]bool{
	"fonts.googleapis.com": true,
	"fonts.gstatic.com":    true,
	"cdnjs.cloudflare.com": true,
	"cdn.jsdelivr.net":     true,
	"unpkg.com":            true,
}

var (
	reDoctype     = regexp.MustCompile(`(?is)^\s*<!doctype html>`)
	reLang        = regexp.MustCompile(`(?is)<html[^>]*\slang="([^"]+)"`)
	reCharset     = regexp.MustCompile(`(?is)<meta[^>]+charset=`)
	reViewport    = regexp.MustCompile(`(?is)<meta[^>]+name="viewport"[^>]+width=device-width`)
	reTitle       = regexp.MustCompile(`(?is)<title>\s*([^<]*?)\s*</title>`)
	reDescription = regexp.MustCompile(`(?is)<meta[^>]+name="description"`)
	reStyle       = regexp.MustCompile(`(?is)<style[^>]*>(.*?)</style>`)
	reScriptTag   = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	reSrcAttr     = regexp.MustCompile(`(?is)\ssrc="([^"]+)"`)
	reLinkTag     = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reHrefAttr    = regexp.MustCompile(`(?is)\shref="([^"]+)"`)
	reRelAttr     = regexp.MustCompile(`(?is)\srel="([^"]+)"`)
	reImgSrc      = regexp.MustCompile(`(?is)<(?:img|source)\b[^>]*\s(?:src|srcset)="([^"]+)"`)
	reIframe      = regexp.MustCompile(`(?is)<(?:iframe|embed|object)\b`)
	reCSSURL      = regexp.MustCompile(`(?is)url\(\s*['"]?(https?://[^'")\s]+)`)
	reImport      = regexp.MustCompile(`(?is)@import\s+(?:url\()?['"]?(https?://[^'")\s;]+)`)
	reExactVer    = regexp.MustCompile(`@\d+\.\d+\.\d+|/\d+\.\d+\.\d+/`)
	reLorem       = regexp.MustCompile(`(?i)lorem ipsum`)
	reRawPH       = regexp.MustCompile(`\[\[[a-z0-9_\-]+:[^\]]*\]\]`)
	reMarkPH      = regexp.MustCompile(`(?is)<mark[^>]*class="[^"]*\bph\b[^"]*"[^>]*data-ph="([^"]+)"|<mark[^>]*data-ph="([^"]+)"[^>]*class="[^"]*\bph\b`)
	reViewportH   = regexp.MustCompile(`(?i)\b100(?:vh|dvh|svh|lvh)\b`)
	reMotion      = regexp.MustCompile(`(?i)@keyframes|animation\s*:|animation-name\s*:|transition\s*:`)
	reTags        = regexp.MustCompile(`(?is)<[^>]+>`)
	reStripBlocks = regexp.MustCompile(`(?is)<(script|style|svg)\b.*?</(script|style|svg)>`)
	reDataURI     = regexp.MustCompile(`data:[a-z+/.\-]+;base64,[A-Za-z0-9+/=]+`)
)

// Static runs source level checks on the built HTML. They are cheap and
// catch what a browser would silently tolerate.
func Static(html string, p *spec.Plan, c *spec.Copy) []Finding {
	var fs []Finding
	add := func(sev, check, where, msg string) {
		fs = append(fs, Finding{Check: check, Severity: sev, Where: where, Message: msg})
	}

	if !reDoctype.MatchString(html) {
		add(Blocker, "html.doctype", "", "document does not start with <!DOCTYPE html>")
	}
	if !reLang.MatchString(html) {
		add(Major, "html.lang", "<html>", "html element has no lang attribute")
	}
	if !reCharset.MatchString(html) {
		add(Major, "html.charset", "<head>", "no meta charset")
	}
	if !reViewport.MatchString(html) {
		add(Blocker, "html.viewport", "<head>", "no responsive viewport meta tag; the page will render zoomed out on phones")
	}
	if m := reTitle.FindStringSubmatch(html); m == nil || strings.TrimSpace(m[1]) == "" {
		add(Major, "html.title", "<head>", "title is missing or empty")
	}
	if !reDescription.MatchString(html) {
		add(Minor, "html.description", "<head>", "no meta description")
	}

	css := ""
	for _, m := range reStyle.FindAllStringSubmatch(html, -1) {
		css += m[1] + "\n"
	}
	if !strings.Contains(css, ":root") {
		add(Major, "css.tokens", "<style>", "no :root token block")
	}
	if p != nil && p.ThemeMode == "dual" {
		if !regexp.MustCompile(`(?i)prefers-color-scheme\s*:\s*dark`).MatchString(css) {
			add(Major, "css.theme", "<style>", "plan is dual theme but there is no prefers-color-scheme: dark block")
		}
		if !regexp.MustCompile(`\[data-theme=["']?dark["']?\]`).MatchString(css) {
			add(Major, "css.theme", "<style>", `plan is dual theme but there is no :root[data-theme="dark"] block`)
		}
	}
	if !strings.Contains(css, "color-scheme") {
		add(Minor, "css.theme", "<style>", "color-scheme is never set")
	}
	if reMotion.MatchString(css) && !strings.Contains(css, "prefers-reduced-motion") {
		add(Major, "css.motion", "<style>", "animations or transitions are used without a prefers-reduced-motion guard")
	}
	if reViewportH.MatchString(css) {
		add(Minor, "css.hero", "<style>", "a viewport height unit is used; the hero must be sized to its content")
	}
	if !strings.Contains(css, ":focus-visible") && !strings.Contains(css, ":focus") {
		add(Major, "css.focus", "<style>", "no focus style; keyboard users cannot see where they are")
	}

	// Resources the page loads.
	checkURL := func(raw, kind string, needsVersion bool) {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return
		}
		if !AllowedHosts[u.Host] {
			sev := Major
			if kind == "script" {
				sev = Blocker
			}
			add(sev, "resource.host", raw, fmt.Sprintf("%s loads from %s, which is not on the allowed list", kind, u.Host))
			return
		}
		if needsVersion && !reExactVer.MatchString(u.Path) {
			add(Major, "resource.version", raw, kind+" URL is not pinned to an exact version")
		}
	}
	inlineScripts := 0
	for _, m := range reScriptTag.FindAllStringSubmatch(html, -1) {
		if src := reSrcAttr.FindStringSubmatch(m[1]); src != nil {
			checkURL(src[1], "script", true)
		} else if strings.TrimSpace(m[2]) != "" && !strings.Contains(m[1], "application/ld+json") {
			inlineScripts++
		}
	}
	if inlineScripts > 1 {
		add(Minor, "html.scripts", "<script>", fmt.Sprintf("%d inline scripts; keep JavaScript in one script at the end of the body", inlineScripts))
	}
	for _, tag := range reLinkTag.FindAllString(html, -1) {
		href := reHrefAttr.FindStringSubmatch(tag)
		if href == nil {
			continue
		}
		rel := ""
		if r := reRelAttr.FindStringSubmatch(tag); r != nil {
			rel = strings.ToLower(r[1])
		}
		if strings.Contains(rel, "preconnect") || strings.Contains(rel, "dns-prefetch") {
			continue
		}
		pinned := !strings.Contains(href[1], "fonts.googleapis.com")
		checkURL(href[1], "stylesheet", pinned && strings.Contains(rel, "stylesheet"))
	}
	for _, m := range reImgSrc.FindAllStringSubmatch(html, -1) {
		checkURL(m[1], "image", false)
	}
	for _, m := range reCSSURL.FindAllStringSubmatch(css, -1) {
		checkURL(m[1], "css url()", false)
	}
	for _, m := range reImport.FindAllStringSubmatch(css, -1) {
		checkURL(m[1], "css @import", false)
	}
	if reIframe.MatchString(html) {
		add(Blocker, "resource.embed", "", "page contains an iframe, embed or object; embeds are not allowed")
	}

	// Content integrity.
	text := VisibleText(html)
	if reLorem.MatchString(text) {
		add(Blocker, "content.lorem", "", "lorem ipsum found")
	}
	if raw := reRawPH.FindAllString(html, -1); len(raw) > 0 {
		add(Major, "content.placeholders", "", fmt.Sprintf("%d placeholder tokens were left as raw text instead of <mark class=\"ph\">", len(raw)))
	}
	if c != nil {
		rendered := map[string]bool{}
		for _, m := range reMarkPH.FindAllStringSubmatch(html, -1) {
			rendered[m[1]+m[2]] = true
		}
		for _, m := range regexp.MustCompile(`data-ph="([^"]+)"`).FindAllStringSubmatch(html, -1) {
			rendered[m[1]] = true
		}
		for _, ph := range c.Placeholders {
			if !rendered[ph.Key] {
				add(Major, "content.placeholders", ph.Key, fmt.Sprintf("placeholder %q from the copy does not appear in the page", ph.Key))
			}
		}
		lowerText := strings.ToLower(text)
		contains := func(s string) bool {
			return strings.Contains(lowerText, strings.ToLower(normalizeCopy(s)))
		}
		if !contains(c.PrimaryCTA.Label) {
			add(Blocker, "content.cta", "", fmt.Sprintf("primary action %q does not appear on the page", c.PrimaryCTA.Label))
		}
		for _, s := range c.Sections {
			if h := strings.TrimSpace(s.Heading); h != "" && !contains(h) {
				add(Major, "content.sections", s.ID, fmt.Sprintf("heading %q from the copy is missing", h))
			}
		}
	}
	if n := strings.Count(text, "\u2014"); n > 0 {
		add(Minor, "content.dashes", "", fmt.Sprintf("%d em dashes in visible text; the copy rules avoid dash asides", n))
	}

	// Weight without inlined assets.
	stripped := reDataURI.ReplaceAllString(html, "")
	if kb := len(stripped) / 1024; kb > 300 {
		add(Minor, "html.size", "", fmt.Sprintf("document is %d KB without inlined assets; look for duplicated CSS or SVG", kb))
	}
	return fs
}

var rePHParts = regexp.MustCompile(`\[\[[a-z0-9_\-]+:\s*([^\]]*)\]\]`)

// normalizeCopy turns copy text into what the page shows: placeholder tokens
// become their descriptions and whitespace collapses.
func normalizeCopy(s string) string {
	s = rePHParts.ReplaceAllString(s, "$1")
	return strings.Join(strings.Fields(s), " ")
}

// VisibleText strips scripts, styles, inline SVG and tags, leaving roughly
// what a reader sees.
func VisibleText(html string) string {
	t := reStripBlocks.ReplaceAllString(html, " ")
	t = reTags.ReplaceAllString(t, " ")
	t = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&mdash;", "\u2014").Replace(t)
	return strings.Join(strings.Fields(t), " ")
}
