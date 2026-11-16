package product

import (
	"context"
	"sort"
	"time"

	"github.com/GraHms/faz/internal/store"
)

// Telemetry answers, per site and per period: how many tokens, how much
// each stage cost and on which models, and what was charged for it. It is
// for operators, behind the admin token; owners only ever see credits.
//
// Costs are estimates made at call time from the price table in
// internal/llm/cost.go, so later price changes do not rewrite history.
// Credits are valued at list price without IVA, including the signup gift,
// so a site paid entirely with gift credits shows the value given away.

// DefaultUSDToMZN converts model spend to meticais when FAZ_USD_MZN is unset.
const DefaultUSDToMZN = 64.0

// VATRate is the IVA included in every credit price.
const VATRate = 0.16

// Money puts model spend and credits side by side in meticais.
type Money struct {
	USDToMZN        float64 `json:"usd_to_mzn"`
	CostUSD         float64 `json:"cost_usd"`
	CostMZN         float64 `json:"cost_mzn"`
	CreditsCharged  int     `json:"credits_charged"`
	CreditsRefunded int     `json:"credits_refunded"`
	CreditsNet      int     `json:"credits_net"`
	CreditsMZN      float64 `json:"credits_mzn"`        // as charged, IVA included
	CreditsMZNExVAT float64 `json:"credits_mzn_ex_vat"` // what the business keeps
	MarginMZN       float64 `json:"margin_mzn"`         // credits without IVA minus model cost
	MarginPercent   float64 `json:"margin_percent"`     // of credits without IVA; 0 when nothing was charged
}

func (s *Service) money(costUSD float64, charged, refunded int) Money {
	rate := s.Cfg.USDToMZN
	if rate <= 0 {
		rate = DefaultUSDToMZN
	}
	per := s.Cfg.Pricing.MZNPerCredit
	m := Money{USDToMZN: rate, CostUSD: round(costUSD, 6), CreditsCharged: charged, CreditsRefunded: refunded, CreditsNet: charged - refunded}
	m.CostMZN = round(costUSD*rate, 2)
	m.CreditsMZN = round(float64(m.CreditsNet)*per, 2)
	m.CreditsMZNExVAT = round(m.CreditsMZN/(1+VATRate), 2)
	m.MarginMZN = round(m.CreditsMZNExVAT-m.CostMZN, 2)
	if m.CreditsMZNExVAT > 0 {
		m.MarginPercent = round(100*m.MarginMZN/m.CreditsMZNExVAT, 1)
	}
	return m
}

func round(v float64, places int) float64 {
	p := 1.0
	for range places {
		p *= 10
	}
	if v < 0 {
		return -float64(int64(-v*p+0.5)) / p
	}
	return float64(int64(v*p+0.5)) / p
}

// StageTelemetry sums the calls of one stage on one model and effort.
type StageTelemetry struct {
	Operation string `json:"operation"` // questions, build, edit or classify
	Stage     string `json:"stage"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	Retries   int    `json:"retries"` // calls after the first attempt
	store.UsageTotals
}

// SiteTelemetry is everything spent on one site.
type SiteTelemetry struct {
	Site        store.Site        `json:"site"`
	OwnerEmail  string            `json:"owner_email"`
	Totals      store.UsageTotals `json:"totals"`
	Money       Money             `json:"money"`
	Jobs        store.SiteJobs    `json:"jobs"`
	ByOperation []StageTelemetry  `json:"by_operation"` // Stage, Model and Effort empty
	ByModel     []StageTelemetry  `json:"by_model"`     // Operation, Stage and Effort empty
	ByStage     []StageTelemetry  `json:"by_stage"`
	Calls       []store.UsageCall `json:"calls"`
}

// operationOrder sorts operations in the order a site goes through them.
var operationOrder = map[string]int{"questions": 0, "build": 1, "classify": 2, "edit": 3}

func add(t *store.UsageTotals, c store.UsageCall) {
	t.Calls++
	if c.Error != nil {
		t.Errors++
	}
	t.InputTokens += int64(c.InputTokens)
	t.OutputTokens += int64(c.OutputTokens)
	t.CacheRead += int64(c.CacheRead)
	t.CacheWrite += int64(c.CacheWrite)
	t.CostUSD += c.CostUSD
	t.LatencyMS += int64(c.LatencyMS)
}

// SiteTelemetry reports one site's model calls, grouped every useful way.
func (s *Service) SiteTelemetry(ctx context.Context, siteID string) (*SiteTelemetry, error) {
	site, err := s.Store.Site(ctx, s.Store.DB, siteID, "")
	if err != nil {
		return nil, mapStoreErr(err)
	}
	owner, err := s.Store.UserByID(ctx, site.UserID)
	if err != nil {
		return nil, err
	}
	calls, err := s.Store.SiteUsageCalls(ctx, siteID)
	if err != nil {
		return nil, err
	}
	charged, refunded, err := s.Store.SiteCredits(ctx, siteID)
	if err != nil {
		return nil, err
	}
	jobs, err := s.Store.SiteJobCounts(ctx, siteID, JobBuild, JobEdit)
	if err != nil {
		return nil, err
	}

	t := &SiteTelemetry{Site: site, OwnerEmail: owner.Email, Jobs: jobs, Calls: calls}
	stages, ops, models := map[[4]string]*StageTelemetry{}, map[string]*StageTelemetry{}, map[string]*StageTelemetry{}
	for _, c := range calls {
		add(&t.Totals, c)
		k := [4]string{c.Operation, c.Stage, c.Model, c.Effort}
		if stages[k] == nil {
			stages[k] = &StageTelemetry{Operation: c.Operation, Stage: c.Stage, Model: c.Model, Effort: c.Effort}
		}
		add(&stages[k].UsageTotals, c)
		if c.Attempt > 1 {
			stages[k].Retries++
		}
		if ops[c.Operation] == nil {
			ops[c.Operation] = &StageTelemetry{Operation: c.Operation}
		}
		add(&ops[c.Operation].UsageTotals, c)
		if c.Attempt > 1 {
			ops[c.Operation].Retries++
		}
		if models[c.Model] == nil {
			models[c.Model] = &StageTelemetry{Model: c.Model}
		}
		add(&models[c.Model].UsageTotals, c)
		if c.Attempt > 1 {
			models[c.Model].Retries++
		}
	}
	// First appearance keeps the pipeline's own stage order within an operation.
	first := map[[4]string]int64{}
	for _, c := range calls {
		k := [4]string{c.Operation, c.Stage, c.Model, c.Effort}
		if _, ok := first[k]; !ok {
			first[k] = c.ID
		}
	}
	for _, v := range stages {
		t.ByStage = append(t.ByStage, *v)
	}
	sort.Slice(t.ByStage, func(i, j int) bool {
		a, b := t.ByStage[i], t.ByStage[j]
		if a.Operation != b.Operation {
			return operationOrder[a.Operation] < operationOrder[b.Operation]
		}
		return first[[4]string{a.Operation, a.Stage, a.Model, a.Effort}] < first[[4]string{b.Operation, b.Stage, b.Model, b.Effort}]
	})
	for _, v := range ops {
		t.ByOperation = append(t.ByOperation, *v)
	}
	sort.Slice(t.ByOperation, func(i, j int) bool {
		return operationOrder[t.ByOperation[i].Operation] < operationOrder[t.ByOperation[j].Operation]
	})
	for _, v := range models {
		t.ByModel = append(t.ByModel, *v)
	}
	sort.Slice(t.ByModel, func(i, j int) bool { return t.ByModel[i].CostUSD > t.ByModel[j].CostUSD })
	t.Totals.CostUSD = round(t.Totals.CostUSD, 6)
	t.Money = s.money(t.Totals.CostUSD, charged, refunded)
	return t, nil
}

// Overview is spend across all sites in a period.
type Overview struct {
	From    time.Time          `json:"from"`
	To      time.Time          `json:"to"`
	Totals  store.UsageTotals  `json:"totals"`
	Money   Money              `json:"money"`
	ByStage []store.UsageGroup `json:"by_stage"`
	Sites   []OverviewSite     `json:"sites"`
}

// OverviewSite is one site's line in the overview.
type OverviewSite struct {
	store.SiteUsageRow
	Money Money `json:"money"`
}

// Telemetry reports spend in [from, to): by stage and model, and per site.
func (s *Service) Telemetry(ctx context.Context, from, to time.Time, limit int) (*Overview, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	groups, err := s.Store.UsageByStage(ctx, from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.SitesUsage(ctx, from, to, limit)
	if err != nil {
		return nil, err
	}
	o := &Overview{From: from, To: to, ByStage: groups, Sites: []OverviewSite{}}
	for _, g := range groups {
		o.Totals.Calls += g.Calls
		o.Totals.Errors += g.Errors
		o.Totals.InputTokens += g.InputTokens
		o.Totals.OutputTokens += g.OutputTokens
		o.Totals.CacheRead += g.CacheRead
		o.Totals.CacheWrite += g.CacheWrite
		o.Totals.CostUSD += g.CostUSD
		o.Totals.LatencyMS += g.LatencyMS
	}
	for _, r := range rows {
		o.Sites = append(o.Sites, OverviewSite{SiteUsageRow: r, Money: s.money(r.CostUSD, r.CreditsCharged, r.CreditsRefund)})
	}
	// Credits for the whole period, not only the sites listed.
	charged, refunded, err := s.Store.CreditsBetween(ctx, from, to)
	if err != nil {
		return nil, err
	}
	o.Totals.CostUSD = round(o.Totals.CostUSD, 6)
	o.Money = s.money(o.Totals.CostUSD, charged, refunded)
	return o, nil
}
