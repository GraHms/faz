package store

import (
	"context"
	"time"
)

// UsageCall is one recorded model call, as telemetry reads it back.
type UsageCall struct {
	ID           int64     `json:"id"`
	JobID        *int64    `json:"job_id,omitempty"`
	Operation    string    `json:"operation"`
	Stage        string    `json:"stage"`
	Model        string    `json:"model"`
	Effort       string    `json:"effort"`
	Attempt      int       `json:"attempt"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CacheRead    int       `json:"cache_read_tokens"`
	CacheWrite   int       `json:"cache_write_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	LatencyMS    int       `json:"latency_ms"`
	Error        *string   `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SiteUsageCalls returns every model call made for a site, oldest first.
func (s *Store) SiteUsageCalls(ctx context.Context, siteID string) ([]UsageCall, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, job_id, operation, stage, model, effort, attempt,
		input_tokens, output_tokens, cache_read, cache_write, cost_usd::float8, latency_ms, error, created_at
		FROM model_usage WHERE site_id = $1 ORDER BY id`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageCall{}
	for rows.Next() {
		var c UsageCall
		if err := rows.Scan(&c.ID, &c.JobID, &c.Operation, &c.Stage, &c.Model, &c.Effort, &c.Attempt,
			&c.InputTokens, &c.OutputTokens, &c.CacheRead, &c.CacheWrite, &c.CostUSD, &c.LatencyMS, &c.Error, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SiteCredits sums the credits charged for a site's jobs and the credits
// refunded for them.
func (s *Store) SiteCredits(ctx context.Context, siteID string) (charged, refunded int, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(-l.delta) FILTER (WHERE l.kind = 'charge'), 0),
		COALESCE(SUM(l.delta) FILTER (WHERE l.kind = 'refund'), 0)
		FROM credit_ledger l JOIN jobs j ON j.id = l.job_id
		WHERE j.payload->>'site_id' = $1`, siteID).Scan(&charged, &refunded)
	return charged, refunded, err
}

// CreditsBetween sums the credits charged for jobs, and refunded, in [from, to).
func (s *Store) CreditsBetween(ctx context.Context, from, to time.Time) (charged, refunded int, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(-delta) FILTER (WHERE kind = 'charge'), 0),
		COALESCE(SUM(delta) FILTER (WHERE kind = 'refund'), 0)
		FROM credit_ledger WHERE job_id IS NOT NULL AND created_at >= $1 AND created_at < $2`, from, to).Scan(&charged, &refunded)
	return charged, refunded, err
}

// SiteJobs counts a site's build and edit jobs by status.
// AvgBuildSecs runs from the owner's request to the finished page, queue
// time included, over successful builds.
type SiteJobs struct {
	Builds       int `json:"builds"`
	Edits        int `json:"edits"`
	Failed       int `json:"failed"`
	AvgBuildSecs int `json:"avg_build_seconds"`
}

func (s *Store) SiteJobCounts(ctx context.Context, siteID, buildKind, editKind string) (SiteJobs, error) {
	var j SiteJobs
	err := s.DB.QueryRowContext(ctx, `SELECT
		COUNT(*) FILTER (WHERE kind = $2),
		COUNT(*) FILTER (WHERE kind = $3),
		COUNT(*) FILTER (WHERE status = 'failed' AND kind IN ($2, $3)),
		COALESCE(AVG(EXTRACT(EPOCH FROM updated_at - created_at)) FILTER (WHERE kind = $2 AND status = 'succeeded'), 0)::int
		FROM jobs WHERE payload->>'site_id' = $1`, siteID, buildKind, editKind).Scan(&j.Builds, &j.Edits, &j.Failed, &j.AvgBuildSecs)
	return j, err
}

// UsageTotals adds up model calls.
type UsageTotals struct {
	Calls        int     `json:"calls"`
	Errors       int     `json:"errors"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheRead    int64   `json:"cache_read_tokens"`
	CacheWrite   int64   `json:"cache_write_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	LatencyMS    int64   `json:"latency_ms"`
}

// UsageGroup is usage summed by operation, stage and model.
type UsageGroup struct {
	Operation string `json:"operation"`
	Stage     string `json:"stage"`
	Model     string `json:"model"`
	UsageTotals
}

// UsageByStage sums the calls made in [from, to) by operation, stage and model.
func (s *Store) UsageByStage(ctx context.Context, from, to time.Time) ([]UsageGroup, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT operation, stage, model, COUNT(*), COUNT(error),
		SUM(input_tokens), SUM(output_tokens), SUM(cache_read), SUM(cache_write), SUM(cost_usd)::float8, SUM(latency_ms)
		FROM model_usage WHERE created_at >= $1 AND created_at < $2
		GROUP BY operation, stage, model ORDER BY SUM(cost_usd) DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageGroup{}
	for rows.Next() {
		var g UsageGroup
		if err := rows.Scan(&g.Operation, &g.Stage, &g.Model, &g.Calls, &g.Errors, &g.InputTokens, &g.OutputTokens,
			&g.CacheRead, &g.CacheWrite, &g.CostUSD, &g.LatencyMS); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SiteUsageRow is one site's spend in a period, with the credits charged
// for its jobs in that period.
type SiteUsageRow struct {
	SiteID         string    `json:"site_id"`
	BusinessName   string    `json:"business_name"`
	OwnerEmail     string    `json:"owner_email"`
	Status         string    `json:"status"`
	CurrentVersion int       `json:"current_version"`
	PublishedVer   *int      `json:"published_version,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	CreditsCharged int       `json:"credits_charged"`
	CreditsRefund  int       `json:"credits_refunded"`
	UsageTotals
}

// SitesUsage lists the sites with model spend in [from, to), costliest first.
func (s *Store) SitesUsage(ctx context.Context, from, to time.Time, limit int) ([]SiteUsageRow, error) {
	rows, err := s.DB.QueryContext(ctx, `WITH u AS (
			SELECT site_id, COUNT(*) calls, COUNT(error) errors, SUM(input_tokens) i, SUM(output_tokens) o,
				SUM(cache_read) cr, SUM(cache_write) cw, SUM(cost_usd)::float8 cost, SUM(latency_ms) ms
			FROM model_usage WHERE site_id IS NOT NULL AND created_at >= $1 AND created_at < $2 GROUP BY site_id),
		c AS (
			SELECT j.payload->>'site_id' site_id,
				COALESCE(SUM(-l.delta) FILTER (WHERE l.kind = 'charge'), 0) charged,
				COALESCE(SUM(l.delta) FILTER (WHERE l.kind = 'refund'), 0) refunded
			FROM credit_ledger l JOIN jobs j ON j.id = l.job_id
			WHERE l.created_at >= $1 AND l.created_at < $2 GROUP BY 1)
		SELECT s.id, s.business_name, us.email, s.status, s.current_version, s.published_version, s.created_at,
			COALESCE(c.charged, 0), COALESCE(c.refunded, 0),
			u.calls, u.errors, u.i, u.o, u.cr, u.cw, u.cost, u.ms
		FROM u JOIN sites s ON s.id = u.site_id JOIN users us ON us.id = s.user_id
		LEFT JOIN c ON c.site_id = s.id::text
		ORDER BY u.cost DESC LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SiteUsageRow{}
	for rows.Next() {
		var r SiteUsageRow
		if err := rows.Scan(&r.SiteID, &r.BusinessName, &r.OwnerEmail, &r.Status, &r.CurrentVersion, &r.PublishedVer, &r.CreatedAt,
			&r.CreditsCharged, &r.CreditsRefund, &r.Calls, &r.Errors, &r.InputTokens, &r.OutputTokens,
			&r.CacheRead, &r.CacheWrite, &r.CostUSD, &r.LatencyMS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
