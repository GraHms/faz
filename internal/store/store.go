// Package store is Faz's Postgres access layer: plain SQL over database/sql
// with the lib/pq driver. Business rules live in package product; this
// package only reads and writes rows.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrNotFound   = errors.New("not found")
	ErrEmailTaken = errors.New("email already registered")
	ErrTaken      = errors.New("already taken")
)

// Q is satisfied by both *sql.DB and *sql.Tx.
type Q interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct{ DB *sql.DB }

// Open connects and checks the connection.
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return &Store{DB: db}, nil
}

// Migrate applies embedded migrations in order, each in its own transaction.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version int PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		v, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: name must start with a number", e.Name())
		}
		var done bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, v).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		err = s.Tx(ctx, func(tx *sql.Tx) error {
			// Serialize concurrent migrators.
			if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7212026)`); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", e.Name(), err)
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`, v)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Tx runs fn in a transaction, committing on nil and rolling back otherwise.
func (s *Store) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func isUnique(err error) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23505"
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func jsonb(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// ---- users and sessions ----

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

const userCols = `id, email, name, password_hash, created_at`

func scanUser(r interface{ Scan(...any) error }) (User, error) {
	var u User
	err := r.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	return u, notFound(err)
}

func (s *Store) CreateUser(ctx context.Context, q Q, email, name, hash string) (User, error) {
	u, err := scanUser(q.QueryRowContext(ctx,
		`INSERT INTO users (email, name, password_hash) VALUES ($1, $2, $3) RETURNING `+userCols, email, name, hash))
	if isUnique(err) {
		return User{}, ErrEmailTaken
	}
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = $1`, email))
}

func (s *Store) UserByID(ctx context.Context, id string) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID string, expires time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, tokenHash, userID, expires)
	return err
}

func (s *Store) UserBySession(ctx context.Context, tokenHash []byte) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT u.id, u.email, u.name, u.password_hash, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash))
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// ---- credits ----

type LedgerEntry struct {
	ID             int64     `json:"id"`
	UserID         string    `json:"-"`
	Delta          int       `json:"delta"`
	Kind           string    `json:"kind"`
	Reason         string    `json:"reason"`
	JobID          *int64    `json:"job_id,omitempty"`
	PaymentID      *string   `json:"payment_id,omitempty"`
	IdempotencyKey *string   `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}

// LockUser takes a row lock on the user, serializing balance changes.
func (s *Store) LockUser(ctx context.Context, tx *sql.Tx, userID string) error {
	var id string
	return notFound(tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&id))
}

func (s *Store) Balance(ctx context.Context, q Q, userID string) (int, error) {
	var b int
	err := q.QueryRowContext(ctx, `SELECT COALESCE(SUM(delta), 0) FROM credit_ledger WHERE user_id = $1`, userID).Scan(&b)
	return b, err
}

// AddLedger appends an entry. With an idempotency key already used, it
// writes nothing and reports false.
func (s *Store) AddLedger(ctx context.Context, q Q, e LedgerEntry) (bool, error) {
	res, err := q.ExecContext(ctx, `INSERT INTO credit_ledger (user_id, delta, kind, reason, job_id, payment_id, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (idempotency_key) DO NOTHING`,
		e.UserID, e.Delta, e.Kind, e.Reason, e.JobID, e.PaymentID, e.IdempotencyKey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *Store) Ledger(ctx context.Context, userID string, limit int) ([]LedgerEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, delta, kind, reason, job_id, payment_id, created_at
		FROM credit_ledger WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Delta, &e.Kind, &e.Reason, &e.JobID, &e.PaymentID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- sites ----

type Site struct {
	ID               string          `json:"id"`
	UserID           string          `json:"-"`
	BusinessName     string          `json:"business_name"`
	BusinessType     string          `json:"business_type"`
	Prompt           string          `json:"prompt"`
	Status           string          `json:"status"`
	Brief            json.RawMessage `json:"-"`
	Answers          json.RawMessage `json:"answers,omitempty"`
	CurrentVersion   int             `json:"current_version"`
	PublishedVersion *int            `json:"published_version,omitempty"`
	Subdomain        *string         `json:"subdomain,omitempty"`
	PublishedAt      *time.Time      `json:"published_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

const siteCols = `id, user_id, business_name, business_type, prompt, status, brief, answers, current_version,
	published_version, subdomain, published_at, created_at, updated_at`

func scanSite(r interface{ Scan(...any) error }) (Site, error) {
	var s Site
	var brief, answers []byte
	err := r.Scan(&s.ID, &s.UserID, &s.BusinessName, &s.BusinessType, &s.Prompt, &s.Status, &brief, &answers,
		&s.CurrentVersion, &s.PublishedVersion, &s.Subdomain, &s.PublishedAt, &s.CreatedAt, &s.UpdatedAt)
	s.Brief, s.Answers = brief, answers
	return s, notFound(err)
}

func (s *Store) CreateSite(ctx context.Context, q Q, site Site) (Site, error) {
	return scanSite(q.QueryRowContext(ctx, `INSERT INTO sites (user_id, business_name, business_type, prompt, status)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+siteCols, site.UserID, site.BusinessName, site.BusinessType, site.Prompt, site.Status))
}

// Site returns a site owned by userID. An empty userID skips the owner check
// (for background jobs).
func (s *Store) Site(ctx context.Context, q Q, id, userID string) (Site, error) {
	if userID == "" {
		return scanSite(q.QueryRowContext(ctx, `SELECT `+siteCols+` FROM sites WHERE id = $1`, id))
	}
	return scanSite(q.QueryRowContext(ctx, `SELECT `+siteCols+` FROM sites WHERE id = $1 AND user_id = $2`, id, userID))
}

// LockSite is Site with a row lock, for state transitions.
func (s *Store) LockSite(ctx context.Context, tx *sql.Tx, id, userID string) (Site, error) {
	if userID == "" {
		return scanSite(tx.QueryRowContext(ctx, `SELECT `+siteCols+` FROM sites WHERE id = $1 FOR UPDATE`, id))
	}
	return scanSite(tx.QueryRowContext(ctx, `SELECT `+siteCols+` FROM sites WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID))
}

func (s *Store) SitesByUser(ctx context.Context, userID string) ([]Site, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+siteCols+` FROM sites WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Site
	for rows.Next() {
		site, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, site)
	}
	return out, rows.Err()
}

func (s *Store) SetSiteStatus(ctx context.Context, q Q, id, status string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

func (s *Store) SetSiteBrief(ctx context.Context, q Q, id string, brief any) error {
	b, err := jsonb(brief)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE sites SET brief = $2, updated_at = now() WHERE id = $1`, id, b)
	return err
}

func (s *Store) SetSiteAnswers(ctx context.Context, q Q, id string, answers any) error {
	b, err := jsonb(answers)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE sites SET answers = $2, updated_at = now() WHERE id = $1`, id, b)
	return err
}

func (s *Store) SetCurrentVersion(ctx context.Context, q Q, id string, version int, status string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET current_version = $2, status = $3, updated_at = now() WHERE id = $1`, id, version, status)
	return err
}

// Publish records the live subdomain and version. A subdomain held by
// another site returns ErrTaken.
func (s *Store) Publish(ctx context.Context, q Q, id, subdomain string, version int) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET subdomain = $2, published_version = $3, published_at = now(), updated_at = now() WHERE id = $1`,
		id, subdomain, version)
	if isUnique(err) {
		return ErrTaken
	}
	return err
}

func (s *Store) SubdomainOwner(ctx context.Context, name string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM sites WHERE subdomain = $1`, name).Scan(&id)
	return id, notFound(err)
}

type Version struct {
	SiteID      string    `json:"-"`
	Number      int       `json:"number"`
	Kind        string    `json:"kind"`
	R2Prefix    string    `json:"-"`
	Status      string    `json:"status"`
	Instruction *string   `json:"instruction,omitempty"`
	JobID       *int64    `json:"job_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) AddVersion(ctx context.Context, q Q, v Version) error {
	_, err := q.ExecContext(ctx, `INSERT INTO site_versions (site_id, number, kind, r2_prefix, status, instruction, job_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, v.SiteID, v.Number, v.Kind, v.R2Prefix, v.Status, v.Instruction, v.JobID)
	return err
}

func (s *Store) Version(ctx context.Context, siteID string, number int) (Version, error) {
	var v Version
	err := s.DB.QueryRowContext(ctx, `SELECT site_id, number, kind, r2_prefix, status, instruction, job_id, created_at
		FROM site_versions WHERE site_id = $1 AND number = $2`, siteID, number).
		Scan(&v.SiteID, &v.Number, &v.Kind, &v.R2Prefix, &v.Status, &v.Instruction, &v.JobID, &v.CreatedAt)
	return v, notFound(err)
}

func (s *Store) Versions(ctx context.Context, siteID string) ([]Version, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT site_id, number, kind, r2_prefix, status, instruction, job_id, created_at
		FROM site_versions WHERE site_id = $1 ORDER BY number DESC`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.SiteID, &v.Number, &v.Kind, &v.R2Prefix, &v.Status, &v.Instruction, &v.JobID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type Asset struct {
	SiteID    string    `json:"-"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) UpsertAsset(ctx context.Context, a Asset) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO site_assets (site_id, name, media_type, size) VALUES ($1, $2, $3, $4)
		ON CONFLICT (site_id, name) DO UPDATE SET media_type = EXCLUDED.media_type, size = EXCLUDED.size, created_at = now()`,
		a.SiteID, a.Name, a.MediaType, a.Size)
	return err
}

func (s *Store) Assets(ctx context.Context, siteID string) ([]Asset, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT site_id, name, media_type, size, created_at FROM site_assets WHERE site_id = $1 ORDER BY name`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.SiteID, &a.Name, &a.MediaType, &a.Size, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- messages ----

type Message struct {
	ID             int64     `json:"id"`
	SiteID         string    `json:"-"`
	Role           string    `json:"role"`
	Text           string    `json:"text"`
	Options        []string  `json:"options,omitempty"`
	JobID          *int64    `json:"job_id,omitempty"`
	CreditsCharged int       `json:"credits_charged"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *Store) AddMessage(ctx context.Context, q Q, m Message) (Message, error) {
	var opts []byte
	if len(m.Options) > 0 {
		opts, _ = json.Marshal(m.Options)
	}
	err := q.QueryRowContext(ctx, `INSERT INTO messages (site_id, role, text, options, job_id, credits_charged)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`, m.SiteID, m.Role, m.Text, opts, m.JobID, m.CreditsCharged).
		Scan(&m.ID, &m.CreatedAt)
	return m, err
}

func (s *Store) Messages(ctx context.Context, siteID string, limit int) ([]Message, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, site_id, role, text, options, job_id, credits_charged, created_at FROM (
		SELECT * FROM messages WHERE site_id = $1 ORDER BY id DESC LIMIT $2) m ORDER BY id`, siteID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var opts []byte
		if err := rows.Scan(&m.ID, &m.SiteID, &m.Role, &m.Text, &opts, &m.JobID, &m.CreditsCharged, &m.CreatedAt); err != nil {
			return nil, err
		}
		if len(opts) > 0 {
			_ = json.Unmarshal(opts, &m.Options)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- payments ----

type Payment struct {
	ID            string    `json:"id"`
	UserID        string    `json:"-"`
	Method        string    `json:"method"`
	MSISDN        string    `json:"msisdn"`
	AmountMZN     int       `json:"amount_mzn"`
	VATMZN        int       `json:"vat_mzn"`
	Credits       int       `json:"credits"`
	NUIT          *string   `json:"nuit,omitempty"`
	Status        string    `json:"status"`
	Provider      string    `json:"provider"`
	ProviderRef   *string   `json:"provider_ref,omitempty"`
	FailureReason *string   `json:"failure_reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const paymentCols = `id, user_id, method, msisdn, amount_mzn, vat_mzn, credits, nuit, status, provider, provider_ref, failure_reason, created_at, updated_at`

func scanPayment(r interface{ Scan(...any) error }) (Payment, error) {
	var p Payment
	err := r.Scan(&p.ID, &p.UserID, &p.Method, &p.MSISDN, &p.AmountMZN, &p.VATMZN, &p.Credits, &p.NUIT, &p.Status,
		&p.Provider, &p.ProviderRef, &p.FailureReason, &p.CreatedAt, &p.UpdatedAt)
	return p, notFound(err)
}

func (s *Store) CreatePayment(ctx context.Context, q Q, p Payment) (Payment, error) {
	return scanPayment(q.QueryRowContext(ctx, `INSERT INTO payments (user_id, method, msisdn, amount_mzn, vat_mzn, credits, nuit, status, provider)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8) RETURNING `+paymentCols,
		p.UserID, p.Method, p.MSISDN, p.AmountMZN, p.VATMZN, p.Credits, p.NUIT, p.Provider))
}

func (s *Store) SetPaymentRef(ctx context.Context, id, ref string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE payments SET provider_ref = $2, updated_at = now() WHERE id = $1`, id, ref)
	return err
}

func (s *Store) Payment(ctx context.Context, id, userID string) (Payment, error) {
	return scanPayment(s.DB.QueryRowContext(ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1 AND user_id = $2`, id, userID))
}

// LockPaymentByRef finds a payment by the provider's reference, locked.
func (s *Store) LockPaymentByRef(ctx context.Context, tx *sql.Tx, provider, ref string) (Payment, error) {
	return scanPayment(tx.QueryRowContext(ctx, `SELECT `+paymentCols+` FROM payments WHERE provider = $1 AND provider_ref = $2 FOR UPDATE`, provider, ref))
}

func (s *Store) SetPaymentStatus(ctx context.Context, q Q, id, status string, reason *string) error {
	_, err := q.ExecContext(ctx, `UPDATE payments SET status = $2, failure_reason = $3, updated_at = now() WHERE id = $1`, id, status, reason)
	return err
}

func (s *Store) Payments(ctx context.Context, userID string, limit int) ([]Payment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+paymentCols+` FROM payments WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- jobs (read side; the queue itself is package jobs) ----

type JobView struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Status    string          `json:"status"`
	Attempts  int             `json:"attempts"`
	LastError *string         `json:"last_error,omitempty"`
	Progress  *string         `json:"progress,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Job returns a job whose payload belongs to userID.
func (s *Store) Job(ctx context.Context, id int64, userID string) (JobView, error) {
	var j JobView
	var result []byte
	err := s.DB.QueryRowContext(ctx, `SELECT id, kind, status, attempts, last_error, progress, result, created_at, updated_at
		FROM jobs WHERE id = $1 AND payload->>'user_id' = $2`, id, userID).
		Scan(&j.ID, &j.Kind, &j.Status, &j.Attempts, &j.LastError, &j.Progress, &result, &j.CreatedAt, &j.UpdatedAt)
	j.Result = result
	return j, notFound(err)
}

// SetJobProgress records the stage a running job has reached, so the editor
// can show real progress instead of a timer.
func (s *Store) SetJobProgress(ctx context.Context, id int64, stage string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE jobs SET progress = $2 WHERE id = $1 AND status = 'running'`, id, stage)
	return err
}

// ActiveJob reports whether a site has a queued or running job.
func (s *Store) ActiveJob(ctx context.Context, q Q, siteID string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM jobs WHERE payload->>'site_id' = $1 AND status IN ('queued', 'running'))`, siteID).Scan(&ok)
	return ok, err
}

// ---- model usage ----

// Usage is one model call: what it was for, which model, its tokens, cost
// and timing. Failed calls are recorded too; they are paid for.
type Usage struct {
	UserID       string
	SiteID       *string
	JobID        *int64
	Operation    string
	Stage        string
	Model        string
	Effort       string
	Attempt      int
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	CostUSD      float64
	LatencyMS    int
	Error        *string
}

func (s *Store) AddUsage(ctx context.Context, q Q, us []Usage) error {
	for _, u := range us {
		if _, err := q.ExecContext(ctx, `INSERT INTO model_usage (user_id, site_id, job_id, operation, stage, model, effort, attempt,
			input_tokens, output_tokens, cache_read, cache_write, cost_usd, latency_ms, error)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			u.UserID, u.SiteID, u.JobID, u.Operation, u.Stage, u.Model, u.Effort, u.Attempt,
			u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite, u.CostUSD, u.LatencyMS, u.Error); err != nil {
			return err
		}
	}
	return nil
}

// SiteCost sums model spend for a site in US dollars.
func (s *Store) SiteCost(ctx context.Context, siteID string) (float64, error) {
	var c float64
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost_usd), 0)::float8 FROM model_usage WHERE site_id = $1`, siteID).Scan(&c)
	return c, err
}
