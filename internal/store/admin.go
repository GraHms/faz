package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// ---- Settings ----

// Settings returns every runtime setting as raw JSON, keyed by name.
func (s *Store) Settings(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) PutSetting(ctx context.Context, q Q, key string, value json.RawMessage) error {
	_, err := q.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, []byte(value))
	return err
}

// SpendSince sums model spend since a moment, in US dollars.
func (s *Store) SpendSince(ctx context.Context, since time.Time) (float64, error) {
	var v float64
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost_usd), 0)::float8 FROM model_usage WHERE created_at >= $1`, since).Scan(&v)
	return v, err
}

// ---- Invite codes ----

// Invite is a beta invite code. A code can carry bonus credits.
type Invite struct {
	Code         string     `json:"code"`
	Note         string     `json:"note"`
	MaxUses      int        `json:"max_uses"`
	Uses         int        `json:"uses"`
	BonusCredits int        `json:"bonus_credits"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	DisabledAt   *time.Time `json:"disabled_at,omitempty"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
}

const inviteCols = `code, note, max_uses, uses, bonus_credits, expires_at, disabled_at, created_by, created_at`

func scanInvite(r interface{ Scan(...any) error }) (Invite, error) {
	var i Invite
	err := r.Scan(&i.Code, &i.Note, &i.MaxUses, &i.Uses, &i.BonusCredits, &i.ExpiresAt, &i.DisabledAt, &i.CreatedBy, &i.CreatedAt)
	return i, notFound(err)
}

func (s *Store) CreateInvite(ctx context.Context, q Q, i Invite) (Invite, error) {
	out, err := scanInvite(q.QueryRowContext(ctx, `INSERT INTO invite_codes (code, note, max_uses, bonus_credits, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+inviteCols, i.Code, i.Note, i.MaxUses, i.BonusCredits, i.ExpiresAt, i.CreatedBy))
	if isUnique(err) {
		return Invite{}, ErrTaken
	}
	return out, err
}

func (s *Store) Invites(ctx context.Context) ([]Invite, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+inviteCols+` FROM invite_codes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		i, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) DisableInvite(ctx context.Context, q Q, code string) (Invite, error) {
	return scanInvite(q.QueryRowContext(ctx, `UPDATE invite_codes SET disabled_at = COALESCE(disabled_at, now())
		WHERE code = $1 RETURNING `+inviteCols, code))
}

// UseInvite takes one use of a live code. ErrNotFound means the code does
// not exist, is disabled, has expired or is used up.
func (s *Store) UseInvite(ctx context.Context, q Q, code string) (Invite, error) {
	return scanInvite(q.QueryRowContext(ctx, `UPDATE invite_codes SET uses = uses + 1
		WHERE code = $1 AND disabled_at IS NULL AND (expires_at IS NULL OR expires_at > now()) AND uses < max_uses
		RETURNING `+inviteCols, code))
}

func (s *Store) SetUserInvite(ctx context.Context, q Q, userID, code string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET invite_code = $2 WHERE id = $1`, userID, code)
	return err
}

// ---- Audit ----

// AuditEntry records one admin action: who, what, on what, and why.
type AuditEntry struct {
	ID         int64           `json:"id"`
	Operator   string          `json:"operator"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Reason     string          `json:"reason"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (s *Store) AddAudit(ctx context.Context, q Q, e AuditEntry) error {
	if len(e.Details) == 0 {
		e.Details = json.RawMessage(`{}`)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO admin_audit (operator, action, target_type, target_id, reason, details)
		VALUES ($1, $2, $3, $4, $5, $6)`, e.Operator, e.Action, e.TargetType, e.TargetID, e.Reason, []byte(e.Details))
	return err
}

// Audit lists admin actions, newest first, optionally for one target.
func (s *Store) Audit(ctx context.Context, targetType, targetID string, limit int) ([]AuditEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, operator, action, target_type, target_id, reason, details, created_at
		FROM admin_audit WHERE ($1 = '' OR target_type = $1) AND ($2 = '' OR target_id = $2)
		ORDER BY id DESC LIMIT $3`, targetType, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var d []byte
		if err := rows.Scan(&e.ID, &e.Operator, &e.Action, &e.TargetType, &e.TargetID, &e.Reason, &d, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Details = d
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- Users ----

// UserRow is a user as the admin search lists them.
type UserRow struct {
	User
	Balance    int     `json:"balance"`
	Sites      int     `json:"sites"`
	LastMSISDN *string `json:"last_msisdn,omitempty"`
}

// SearchUsers finds users by email, name, or the phone number of any of
// their payments (support in Mozambique arrives with a phone number).
func (s *Store) SearchUsers(ctx context.Context, query string, limit int) ([]UserRow, error) {
	query = strings.TrimSpace(query)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, query)
	if len(digits) > 9 && strings.HasPrefix(digits, "258") {
		digits = digits[3:]
	}
	if len(digits) < 6 {
		digits = ""
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT u.id, u.email, u.name, u.password_hash, u.created_at, u.invite_code, u.suspended_at, u.suspended_reason,
		COALESCE((SELECT SUM(delta) FROM credit_ledger l WHERE l.user_id = u.id), 0),
		(SELECT COUNT(*) FROM sites x WHERE x.user_id = u.id),
		(SELECT msisdn FROM payments p WHERE p.user_id = u.id ORDER BY created_at DESC LIMIT 1)
		FROM users u
		WHERE $1 = '' OR u.email ILIKE '%' || $1 || '%' OR u.name ILIKE '%' || $1 || '%' OR u.id::text = $1
			OR ($2 <> '' AND EXISTS (SELECT 1 FROM payments p WHERE p.user_id = u.id AND p.msisdn LIKE '%' || $2 || '%'))
		ORDER BY u.created_at DESC LIMIT $3`, query, digits, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		var r UserRow
		if err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.PasswordHash, &r.CreatedAt, &r.InviteCode, &r.SuspendedAt, &r.SuspendedReason,
			&r.Balance, &r.Sites, &r.LastMSISDN); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SetUserSuspended(ctx context.Context, q Q, userID string, reason *string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET suspended_at = CASE WHEN $2::text IS NULL THEN NULL ELSE now() END,
		suspended_reason = $2 WHERE id = $1`, userID, reason)
	return err
}

// DeleteUserSessions signs a user out everywhere and says how many sessions ended.
func (s *Store) DeleteUserSessions(ctx context.Context, q Q, userID string) (int64, error) {
	res, err := q.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ActiveSessions(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = $1 AND expires_at > now()`, userID).Scan(&n)
	return n, err
}

// JobCharge returns who was charged for a job and how much, from the ledger.
func (s *Store) JobCharge(ctx context.Context, q Q, jobID int64) (userID string, amount int, err error) {
	err = q.QueryRowContext(ctx, `SELECT user_id, -delta FROM credit_ledger WHERE job_id = $1 AND kind = 'charge'`, jobID).Scan(&userID, &amount)
	return userID, amount, notFound(err)
}

// ---- Sites ----

// SiteRow is a site as the admin search lists them.
type SiteRow struct {
	Site
	OwnerEmail string `json:"owner_email"`
}

// SearchSites finds sites by name, address or id.
func (s *Store) SearchSites(ctx context.Context, query string, limit int) ([]SiteRow, error) {
	query = strings.TrimSpace(strings.ToLower(query))
	rows, err := s.DB.QueryContext(ctx, `SELECT `+prefixed("s.", siteCols)+`, u.email
		FROM sites s JOIN users u ON u.id = s.user_id
		WHERE $1 = '' OR s.business_name ILIKE '%' || $1 || '%' OR s.subdomain ILIKE '%' || $1 || '%' OR s.id::text = $1
		ORDER BY s.updated_at DESC LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SiteRow{}
	for rows.Next() {
		var r SiteRow
		var brief, answers []byte
		if err := rows.Scan(&r.ID, &r.UserID, &r.BusinessName, &r.BusinessType, &r.Prompt, &r.Status, &brief, &answers,
			&r.CurrentVersion, &r.PublishedVersion, &r.Subdomain, &r.PublishedAt, &r.CreatedAt, &r.UpdatedAt, &r.TakenDownAt, &r.TakedownReason,
			&r.OwnerEmail); err != nil {
			return nil, err
		}
		r.Answers = answers
		out = append(out, r)
	}
	return out, rows.Err()
}

func prefixed(p, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = p + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}

func (s *Store) SetSiteTakedown(ctx context.Context, q Q, siteID string, reason *string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET taken_down_at = CASE WHEN $2::text IS NULL THEN NULL ELSE now() END,
		takedown_reason = $2, updated_at = now() WHERE id = $1`, siteID, reason)
	return err
}
