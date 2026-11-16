// Package product holds Faz's business rules: accounts, credits, sites,
// questions, builds, chat edits, publishing and top ups. It ties together
// the pipeline, Postgres (package store), R2 and the job queue.
package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/payments"
	"github.com/GraHms/faz/internal/pipeline"
	"github.com/GraHms/faz/internal/r2"
	"github.com/GraHms/faz/internal/render"
	"github.com/GraHms/faz/internal/store"
)

// Pricing is everything that costs the customer credits. Prices include IVA.
type Pricing struct {
	MZNPerCredit float64 `json:"mzn_per_credit"`
	SignupGift   int     `json:"signup_gift"`
	FirstPage    int     `json:"first_page"`
	Edit         int     `json:"edit"`
	Structural   int     `json:"structural_edit"`
	TopupMinMZN  int     `json:"topup_min_mzn"`
	TopupMaxMZN  int     `json:"topup_max_mzn"`
}

// DefaultPricing mirrors the prototype. Check it against the model cost per
// job recorded in model_usage before launch: see the README.
var DefaultPricing = Pricing{MZNPerCredit: 1, SignupGift: 19, FirstPage: 19, Edit: 5, Structural: 20, TopupMinMZN: 200, TopupMaxMZN: 50_000}

// CreditsFor converts a top up in meticais to credits.
func (p Pricing) CreditsFor(amountMZN int) int {
	return int(math.Floor(float64(amountMZN) / p.MZNPerCredit))
}

func (p Pricing) priceList(domain string) string {
	return fmt.Sprintf("Cada alteração simples custa %d créditos. Mudar secções ou o aspecto geral custa %d créditos. "+
		"Perguntas e respostas no chat são grátis. 1 crédito = %g MZN. O alojamento em nome.%s é grátis. "+
		"Recarga mínima de %d MZN, paga com M-Pesa ou e-Mola.", p.Edit, p.Structural, p.MZNPerCredit, domain, p.TopupMinMZN)
}

// Config tunes the service.
type Config struct {
	Pricing            Pricing
	Profile            pipeline.Profile
	ProfileName        string
	PromptDir          string
	HistoryPath        string
	SitesDomain        string // nome.<SitesDomain>
	Chrome             *render.Chrome
	PasswordIterations int
	SessionTTL         time.Duration
	PreviewTTL         time.Duration
}

// Service is the product.
type Service struct {
	Store *store.Store
	R2    *r2.Client
	LLM   llm.Client
	Pay   payments.Provider
	Cfg   Config
	Log   *slog.Logger
}

// Error is a failure the API shows to the customer.
type Error struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func badRequest(code, msg string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Message: msg}
}
func conflict(code, msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: msg}
}

var errNotFound = &Error{Status: http.StatusNotFound, Code: "not_found", Message: "Não encontrado."}

func insufficient(need, have int) *Error {
	return &Error{Status: http.StatusPaymentRequired, Code: "insufficient_credits",
		Message: "Não tens créditos suficientes. Recarrega com M-Pesa ou e-Mola.",
		Details: map[string]any{"needed": need, "balance": have}}
}

func mapStoreErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return errNotFound
	}
	return err
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// pipe returns a pipeline for one operation.
func (s *Service) pipe(outDir, briefText string) *pipeline.Pipeline {
	return &pipeline.Pipeline{LLM: s.LLM, Opt: pipeline.Options{
		BriefText: briefText, OutDir: outDir, Profile: s.Cfg.Profile, ProfileName: s.Cfg.ProfileName,
		PromptDir: s.Cfg.PromptDir, HistoryPath: s.Cfg.HistoryPath, Chrome: s.Cfg.Chrome,
	}}
}

// recordUsage stores what a pipeline call spent.
func (s *Service) recordUsage(ctx context.Context, q store.Q, rep *pipeline.Report, userID string, siteID *string, jobID *int64, op string) {
	if rep == nil {
		return
	}
	var us []store.Usage
	for _, st := range rep.Stages {
		us = append(us, store.Usage{UserID: userID, SiteID: siteID, JobID: jobID, Operation: op, Stage: st.Stage, Model: st.Model,
			InputTokens: st.InputTokens, OutputTokens: st.OutputTokens, CacheRead: st.CacheRead, CacheWrite: st.CacheWrite, CostUSD: st.CostUSD})
	}
	if err := s.Store.AddUsage(ctx, q, us); err != nil {
		s.log().Error("record usage", "err", err)
	}
}

func tempDir(prefix string) (string, func(), error) {
	d, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", nil, err
	}
	return d, func() { os.RemoveAll(d) }, nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func clean(s string) string { return strings.TrimSpace(s) }
