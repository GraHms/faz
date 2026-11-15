// Command faz turns a client brief into a verified single page website.
//
//	faz -brief brief.md -assets ./client-files -out ./out
//
// It needs ANTHROPIC_API_KEY, and a Chrome or Chromium binary for the
// render and review stages. Run with -fake examples/fake to exercise the
// whole pipeline with recorded model output and no spend.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/GraHms/faz/internal/assets"
	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/pipeline"
	"github.com/GraHms/faz/internal/render"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "faz:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		briefPath   = flag.String("brief", "", "path to the client's brief (text or Markdown); required")
		assetsDir   = flag.String("assets", "", "directory of client files (logo, photos)")
		outDir      = flag.String("out", "out", "output directory")
		profileName = flag.String("profile", "quality", "model routing: "+pipeline.ProfileNames())
		promptDir   = flag.String("prompts", "", "directory with prompt overrides (same file names as internal/prompts)")
		historyPath = flag.String("history", defaultHistory(), "run history file used to avoid repeating recent designs; empty disables")
		chromePath  = flag.String("chrome", "", "Chrome or Chromium binary; found on PATH when empty")
		chromeFlags = flag.String("chrome-flags", os.Getenv("FAZ_CHROME_FLAGS"), "extra Chrome flags, space separated")
		noRender    = flag.Bool("no-render", false, "skip headless rendering and visual review")
		fakeDir     = flag.String("fake", "", "use recorded responses from this directory instead of the API")
		baseURL     = flag.String("base-url", os.Getenv("ANTHROPIC_BASE_URL"), "API base URL")
		cache1h     = flag.Bool("cache-1h", false, "cache system prompts for one hour instead of five minutes (worth it for batches)")
		timeout     = flag.Duration("timeout", 45*time.Minute, "overall time limit")
	)
	flag.Parse()
	if *briefPath == "" {
		flag.Usage()
		return errors.New("-brief is required")
	}
	briefText, err := os.ReadFile(*briefPath)
	if err != nil {
		return err
	}
	profile, ok := pipeline.Profiles[*profileName]
	if !ok {
		return fmt.Errorf("unknown profile %q; choose one of %s", *profileName, pipeline.ProfileNames())
	}
	files, err := assets.Load(*assetsDir, 10<<20)
	if err != nil {
		return fmt.Errorf("assets: %w", err)
	}

	var client llm.Client
	if *fakeDir != "" {
		client = &llm.Fake{Dir: *fakeDir}
	} else {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return errors.New("ANTHROPIC_API_KEY is not set (or use -fake for a dry run)")
		}
		a := &llm.Anthropic{APIKey: key, BaseURL: *baseURL}
		if *cache1h {
			a.CacheTTL = "1h"
		}
		client = a
	}

	var chrome *render.Chrome
	if !*noRender {
		path := *chromePath
		if path == "" {
			if path, err = render.FindChrome(); err != nil {
				return fmt.Errorf("%w (or pass -no-render)", err)
			}
		}
		chrome = &render.Chrome{Path: path, NoSandbox: os.Geteuid() == 0, ExtraFlags: strings.Fields(*chromeFlags)}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	p := &pipeline.Pipeline{LLM: client, Opt: pipeline.Options{
		BriefText:   string(briefText),
		Assets:      files,
		OutDir:      *outDir,
		Profile:     profile,
		ProfileName: *profileName,
		PromptDir:   *promptDir,
		HistoryPath: *historyPath,
		Chrome:      chrome,
		Log:         os.Stderr,
	}}
	rep, err := p.Run(ctx)
	if err != nil {
		return err
	}
	if rep.Status != "ready" {
		for _, r := range rep.StatusReasons {
			fmt.Fprintln(os.Stderr, "  -", r)
		}
	}
	return nil
}

func defaultHistory() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "faz", "history.jsonl")
}
