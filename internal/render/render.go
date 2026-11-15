// Package render loads a page in headless Chrome, measures it and takes
// screenshots. It drives the Chrome command line directly (--dump-dom and
// --screenshot), so it needs a Chrome or Chromium binary and nothing else.
package render

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed capture.js
var captureJS string

//go:embed measure.js
var measureJS string

// Metrics is what measure.js reports for one width and theme.
type Metrics struct {
	Label         string   `json:"label"`
	ViewportWidth int      `json:"viewport_width"`
	ScrollWidth   int      `json:"scroll_width"`
	DocHeight     int      `json:"doc_height"`
	Lang          string   `json:"lang"`
	Theme         string   `json:"theme"`
	Errors        []string `json:"errors"`
	Overflow      []struct {
		Where string `json:"where"`
		Right int    `json:"right"`
		Width int    `json:"width"`
	} `json:"overflow"`
	HiddenText []struct {
		Where string `json:"where"`
		Text  string `json:"text"`
	} `json:"hidden_text"`
	LowContrast []struct {
		Where  string  `json:"where"`
		Text   string  `json:"text"`
		Ratio  float64 `json:"ratio"`
		Need   float64 `json:"need"`
		SizePx float64 `json:"size_px"`
	} `json:"low_contrast"`
	ContrastChecked int `json:"contrast_checked"`
	ContrastSkipped int `json:"contrast_skipped"`
	SmallTargets    []struct {
		Where string `json:"where"`
		W     int    `json:"w"`
		H     int    `json:"h"`
	} `json:"small_targets"`
	ImagesNoAlt       []string `json:"images_no_alt"`
	ImagesBroken      []string `json:"images_broken"`
	UnlabeledControls []string `json:"unlabeled_controls"`
	H1Count           int      `json:"h1_count"`
	HeadingSkips      []string `json:"heading_skips"`
	Fonts             []struct {
		Family string `json:"family"`
		Weight string `json:"weight"`
		Status string `json:"status"`
	} `json:"fonts"`
	PlaceholdersShown int `json:"placeholders_shown"`
	PlaceholdersTotal int `json:"placeholders_total"`
}

// Chrome runs headless Chrome from the command line.
type Chrome struct {
	Path       string        // binary; found automatically when empty
	NoSandbox  bool          // needed when running as root, as in most containers
	Timeout    time.Duration // per Chrome process; default 60s
	ExtraFlags []string      // for example --proxy-server=...
	WaitMS     int           // virtual time budget; default 6000
}

// FindChrome looks for a Chrome or Chromium binary on PATH.
func FindChrome() (string, error) {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	for _, p := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no Chrome or Chromium binary found; install one or pass -chrome")
}

func (c *Chrome) args(width, height int, dpr float64, extra ...string) ([]string, string, error) {
	profile, err := os.MkdirTemp("", "faz-chrome-")
	if err != nil {
		return nil, "", err
	}
	wait := c.WaitMS
	if wait == 0 {
		wait = 6000
	}
	a := []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--hide-scrollbars",
		"--mute-audio",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-sync",
		"--user-data-dir=" + profile,
		fmt.Sprintf("--window-size=%d,%d", width, height),
		fmt.Sprintf("--force-device-scale-factor=%g", dpr),
		fmt.Sprintf("--virtual-time-budget=%d", wait),
	}
	if c.NoSandbox {
		a = append(a, "--no-sandbox")
	}
	a = append(a, c.ExtraFlags...)
	a = append(a, extra...)
	return a, profile, nil
}

func (c *Chrome) run(ctx context.Context, args []string) ([]byte, []byte, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	path := c.Path
	if path == "" {
		p, err := FindChrome()
		if err != nil {
			return nil, nil, err
		}
		path = p
	}
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

var metricsRe = regexp.MustCompile(`<pre id="__faz_metrics">([A-Za-z0-9+/=]+)</pre>`)

// Measure loads the page at a width and returns the metrics measure.js
// collected. theme is "", "light" or "dark"; it sets data-theme on <html>.
func (c *Chrome) Measure(ctx context.Context, html, workDir, label string, width int, theme string) (Metrics, error) {
	page := inject(WithTheme(html, theme))
	file, err := writeTemp(workDir, "measure-"+label+".html", frame(page, width, 900))
	if err != nil {
		return Metrics{}, err
	}
	defer os.Remove(file)
	args, profile, err := c.args(windowWidth(width), 900, 1, "--dump-dom", fileURL(file))
	if err != nil {
		return Metrics{}, err
	}
	defer os.RemoveAll(profile)
	stdout, stderr, err := c.run(ctx, args)
	if err != nil {
		return Metrics{}, fmt.Errorf("chrome dump-dom failed: %v: %s", err, tail(stderr))
	}
	m := metricsRe.FindSubmatch(stdout)
	if m == nil {
		return Metrics{}, fmt.Errorf("chrome returned no metrics for %s; the page may not have finished loading within the time budget", label)
	}
	raw, err := base64.StdEncoding.DecodeString(string(m[1]))
	if err != nil {
		return Metrics{}, err
	}
	var out Metrics
	if err := json.Unmarshal(raw, &out); err != nil {
		return Metrics{}, err
	}
	out.Label = label
	return out, nil
}

// Screenshot renders the page at width by height CSS pixels and returns a PNG.
func (c *Chrome) Screenshot(ctx context.Context, html, workDir, label string, width, height int, dpr float64, theme string) ([]byte, error) {
	file, err := writeTemp(workDir, "shot-"+label+".html", frame(WithTheme(html, theme), width, height))
	if err != nil {
		return nil, err
	}
	defer os.Remove(file)
	shot := filepath.Join(workDir, "shot-"+label+".png")
	defer os.Remove(shot)
	args, profile, err := c.args(windowWidth(width), height, dpr, "--screenshot="+shot, fileURL(file))
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(profile)
	if _, stderr, err := c.run(ctx, args); err != nil {
		return nil, fmt.Errorf("chrome screenshot failed: %v: %s", err, tail(stderr))
	}
	raw, err := os.ReadFile(shot)
	if err != nil {
		return nil, err
	}
	if width >= minWindowWidth {
		return raw, nil
	}
	return cropWidth(raw, int(float64(width)*dpr+0.5))
}

// Headless Chrome will not open a window narrower than this.
const minWindowWidth = 500

func windowWidth(w int) int {
	if w < minWindowWidth {
		return minWindowWidth
	}
	return w
}

// frame wraps the page in a srcdoc iframe of the exact width when the width
// is below what a headless window allows. Media queries, viewport units and
// innerWidth inside the iframe all see the requested width, and srcdoc keeps
// the frame same origin so the measure script can report to the top page.
func frame(page string, width, height int) string {
	if width >= minWindowWidth {
		return page
	}
	return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><style>html,body{margin:0;padding:0;background:#fff}iframe{display:block;border:0;width:%dpx;height:%dpx}</style></head><body><iframe scrolling="no" srcdoc="%s"></iframe></body></html>`,
		width, height, html.EscapeString(page))
}

func cropWidth(pngBytes []byte, w int) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if w >= b.Dx() {
		return pngBytes, nil
	}
	sub, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return pngBytes, nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, sub.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Min.X+w, b.Max.Y))); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var htmlOpenRe = regexp.MustCompile(`(?is)<html\b[^>]*>`)
var headOpenRe = regexp.MustCompile(`(?is)<head\b[^>]*>`)

// WithTheme sets data-theme on the html element, replacing any existing one.
func WithTheme(html, theme string) string {
	if theme == "" {
		return html
	}
	return htmlOpenRe.ReplaceAllStringFunc(html, func(tag string) string {
		tag = regexp.MustCompile(`\sdata-theme="[^"]*"`).ReplaceAllString(tag, "")
		return strings.TrimSuffix(tag, ">") + ` data-theme="` + theme + `">`
	})
}

func inject(html string) string {
	capture := "<script>" + captureJS + "</script>"
	if loc := headOpenRe.FindStringIndex(html); loc != nil {
		html = html[:loc[1]] + capture + html[loc[1]:]
	} else {
		html = capture + html
	}
	measure := "<script>" + measureJS + "</script>"
	lower := strings.ToLower(html)
	if i := strings.LastIndex(lower, "</body>"); i >= 0 {
		return html[:i] + measure + html[i:]
	}
	return html + measure
}

func writeTemp(dir, name, content string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	return p, os.WriteFile(p, []byte(content), 0o644)
}

func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file://" + filepath.ToSlash(abs)
}

func tail(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 600 {
		s = s[len(s)-600:]
	}
	return s
}
