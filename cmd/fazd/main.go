// Command fazd runs the Faz API and its job workers in one process.
//
// Configuration comes from the environment; see README.md for the list.
// Run several copies behind a load balancer: they share Postgres, and the
// job queue hands each job to exactly one worker.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GraHms/faz/internal/api"
	"github.com/GraHms/faz/internal/jobs"
	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/payments"
	"github.com/GraHms/faz/internal/pipeline"
	"github.com/GraHms/faz/internal/product"
	"github.com/GraHms/faz/internal/r2"
	"github.com/GraHms/faz/internal/render"
	"github.com/GraHms/faz/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level()}))
	if err := run(log); err != nil {
		log.Error("fazd stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, must("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	bucket := r2.New(env("R2_ACCOUNT_ID", ""), must("R2_BUCKET"), must("R2_ACCESS_KEY_ID"), must("R2_SECRET_ACCESS_KEY"))
	if ep := env("R2_ENDPOINT", ""); ep != "" { // S3 compatible server for local development
		bucket.Endpoint, bucket.Region = ep, env("R2_REGION", "us-east-1")
		if err := r2.CreateBucket(ctx, bucket); err != nil {
			return fmt.Errorf("bucket: %w", err)
		}
	} else if env("R2_ACCOUNT_ID", "") == "" {
		return errors.New("set R2_ACCOUNT_ID (or R2_ENDPOINT for a local S3 server)")
	}

	var model llm.Client
	if dir := env("FAZ_FAKE_DIR", ""); dir != "" {
		log.Warn("using recorded model responses", "dir", dir)
		model = &llm.Fake{Dir: dir}
	} else {
		model = &llm.Anthropic{APIKey: must("ANTHROPIC_API_KEY"), CacheTTL: env("FAZ_CACHE_TTL", "")}
	}

	profileName := env("FAZ_PROFILE", "quality")
	profile, ok := pipeline.Profiles[profileName]
	if !ok {
		return fmt.Errorf("unknown FAZ_PROFILE %q; choose %s", profileName, pipeline.ProfileNames())
	}

	var chrome *render.Chrome
	if env("FAZ_NO_RENDER", "") == "" {
		path := env("FAZ_CHROME", "")
		if path == "" {
			if path, err = render.FindChrome(); err != nil {
				return fmt.Errorf("%w (or set FAZ_NO_RENDER=1)", err)
			}
		}
		chrome = &render.Chrome{Path: path, NoSandbox: os.Geteuid() == 0, ExtraFlags: strings.Fields(env("FAZ_CHROME_FLAGS", ""))}
	}

	var pay payments.Provider
	switch env("FAZ_PAYMENTS", "mock") {
	case "mock":
		pay = &payments.Mock{Secret: env("FAZ_MOCK_PAYMENT_SECRET", ""), Fail: env("FAZ_MOCK_PAYMENT_OUTCOME", "success") == "failure",
			Delay: duration("FAZ_MOCK_PAYMENT_DELAY", 3*time.Second)}
	default:
		return fmt.Errorf("FAZ_PAYMENTS=%s is not available yet; use mock", env("FAZ_PAYMENTS", ""))
	}

	pricing := product.DefaultPricing
	pricing.SignupGift = integer("FAZ_SIGNUP_GIFT", pricing.SignupGift)
	pricing.FirstPage = integer("FAZ_PRICE_FIRST_PAGE", pricing.FirstPage)
	pricing.Edit = integer("FAZ_PRICE_EDIT", pricing.Edit)
	pricing.Structural = integer("FAZ_PRICE_STRUCTURAL", pricing.Structural)
	pricing.TopupMinMZN = integer("FAZ_TOPUP_MIN_MZN", pricing.TopupMinMZN)
	if v, err := strconv.ParseFloat(env("FAZ_MZN_PER_CREDIT", "1"), 64); err == nil && v > 0 {
		pricing.MZNPerCredit = v
	}

	dataDir := env("FAZ_DATA_DIR", filepath.Join(os.TempDir(), "faz"))
	svc := &product.Service{Store: db, R2: bucket, LLM: model, Pay: pay, Log: log, Cfg: product.Config{
		Pricing: pricing, Profile: profile, ProfileName: profileName, PromptDir: env("FAZ_PROMPTS_DIR", ""),
		HistoryPath: filepath.Join(dataDir, "history.jsonl"), SitesDomain: env("FAZ_SITES_DOMAIN", "faz.co.mz"),
		Chrome: chrome, PasswordIterations: integer("FAZ_PASSWORD_ITERATIONS", 0),
	}}

	workers := integer("FAZ_WORKERS", 2)
	for i := 0; i < workers; i++ {
		w := &jobs.Worker{DB: db.DB, ID: fmt.Sprintf("%s-%d", host(), i), Handlers: svc.Handlers(), OnFailed: svc.OnJobFailed, Log: log}
		go func() {
			if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("worker stopped", "err", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:              env("FAZ_HTTP_ADDR", ":8080"),
		Handler:           (&api.Server{Svc: svc, AppOrigins: strings.Fields(env("FAZ_APP_ORIGINS", "")), Log: log}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Info("fazd listening", "addr", srv.Addr, "workers", workers, "profile", profileName, "sites_domain", svc.Cfg.SitesDomain)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func must(k string) string {
	v := env(k, "")
	if v == "" {
		fmt.Fprintf(os.Stderr, "fazd: %s is required\n", k)
		os.Exit(2)
	}
	return v
}

func integer(k string, def int) int {
	if n, err := strconv.Atoi(env(k, "")); err == nil {
		return n
	}
	return def
}

func duration(k string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(env(k, "")); err == nil {
		return d
	}
	return def
}

func level() slog.Level {
	if env("FAZ_LOG_LEVEL", "") == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func host() string {
	h, _ := os.Hostname()
	return h
}
