package product

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GraHms/faz/internal/store"
)

// Operator controls: runtime settings and kill switches, beta invite codes,
// credit adjustments, account suspension, site takedowns, and an audit log
// of every one of these actions. All of it sits behind the admin key; each
// write names its operator and gives a reason.

// Operator identifies who acted. With one shared admin key this is the name
// the operator typed; it becomes an account when operator logins exist.
type Operator struct {
	Name string
}

func (o Operator) check(reason string) error {
	if n := len([]rune(strings.TrimSpace(o.Name))); n < 2 || n > 60 {
		return badRequest("operator_required", "Diz quem és: o nome do operador vai para o registo.")
	}
	if len([]rune(strings.TrimSpace(reason))) < 5 {
		return badRequest("reason_required", "Escreve o motivo (pelo menos 5 caracteres); fica no registo.")
	}
	return nil
}

func (s *Service) audit(ctx context.Context, q store.Q, op Operator, action, targetType, targetID, reason string, details any) error {
	return s.Store.AddAudit(ctx, q, store.AuditEntry{Operator: strings.TrimSpace(op.Name), Action: action, TargetType: targetType,
		TargetID: targetID, Reason: strings.TrimSpace(reason), Details: mustJSON(details)})
}

// ---- Settings and kill switches ----

// Settings are runtime switches an operator changes without a redeploy.
type Settings struct {
	// BetaRequired makes sign up need a live invite code.
	BetaRequired bool `json:"beta_required"`
	// BuildsPaused stops every new request that would call a model:
	// questions, builds, chat. Jobs already queued still run.
	BuildsPaused bool   `json:"builds_paused"`
	PauseReason  string `json:"pause_reason"`
	// DailySpendCapUSD pauses the same requests once today's model spend
	// (UTC) reaches it. 0 means no cap.
	DailySpendCapUSD float64 `json:"daily_spend_cap_usd"`
	// ReservedNames and BlockedWords extend the built in subdomain rules.
	ReservedNames []string `json:"reserved_names"`
	BlockedWords  []string `json:"blocked_words"`
}

// settingsCache keeps settings for a few seconds; several fazd copies see
// a change within that time.
type settingsCache struct {
	mu  sync.Mutex
	at  time.Time
	val Settings
	ok  bool
}

const settingsTTL = 5 * time.Second

func (s *Service) cache() *settingsCache { return &s.settings }

// Settings reads the runtime settings, cached for a few seconds.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	c := s.cache()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ok && time.Since(c.at) < settingsTTL {
		return c.val, nil
	}
	raw, err := s.Store.Settings(ctx)
	if err != nil {
		return Settings{}, err
	}
	v := Settings{ReservedNames: []string{}, BlockedWords: []string{}}
	for k, r := range raw {
		var err error
		switch k {
		case "beta_required":
			err = json.Unmarshal(r, &v.BetaRequired)
		case "builds_paused":
			err = json.Unmarshal(r, &v.BuildsPaused)
		case "pause_reason":
			err = json.Unmarshal(r, &v.PauseReason)
		case "daily_spend_cap_usd":
			err = json.Unmarshal(r, &v.DailySpendCapUSD)
		case "reserved_names":
			err = json.Unmarshal(r, &v.ReservedNames)
		case "blocked_words":
			err = json.Unmarshal(r, &v.BlockedWords)
		}
		if err != nil {
			return Settings{}, fmt.Errorf("setting %s: %w", k, err)
		}
	}
	c.val, c.at, c.ok = v, time.Now(), true
	return v, nil
}

// SettingsPatch changes some settings; nil fields stay as they are.
type SettingsPatch struct {
	BetaRequired     *bool     `json:"beta_required"`
	BuildsPaused     *bool     `json:"builds_paused"`
	PauseReason      *string   `json:"pause_reason"`
	DailySpendCapUSD *float64  `json:"daily_spend_cap_usd"`
	ReservedNames    *[]string `json:"reserved_names"`
	BlockedWords     *[]string `json:"blocked_words"`
}

var wordRe = regexp.MustCompile(`^[a-z0-9-]{2,40}$`)

func cleanWords(in []string, what string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, w := range in {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" || seen[w] {
			continue
		}
		if !wordRe.MatchString(w) {
			return nil, badRequest("invalid_"+what, fmt.Sprintf("%q não serve: usa letras, números e hífens.", w))
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	if len(out) > 1000 {
		return nil, badRequest("invalid_"+what, "No máximo 1000 entradas.")
	}
	return out, nil
}

// UpdateSettings applies a patch and records it, with before and after.
func (s *Service) UpdateSettings(ctx context.Context, op Operator, reason string, p SettingsPatch) (Settings, error) {
	if err := op.check(reason); err != nil {
		return Settings{}, err
	}
	before, err := s.Settings(ctx)
	if err != nil {
		return Settings{}, err
	}
	writes := map[string]any{}
	if p.BetaRequired != nil {
		writes["beta_required"] = *p.BetaRequired
	}
	if p.BuildsPaused != nil {
		writes["builds_paused"] = *p.BuildsPaused
	}
	if p.PauseReason != nil {
		writes["pause_reason"] = clean(*p.PauseReason)
	}
	if p.DailySpendCapUSD != nil {
		if *p.DailySpendCapUSD < 0 || *p.DailySpendCapUSD > 100000 {
			return Settings{}, badRequest("invalid_cap", "O limite diário vai de 0 (sem limite) a 100 000 USD.")
		}
		writes["daily_spend_cap_usd"] = *p.DailySpendCapUSD
	}
	if p.ReservedNames != nil {
		w, err := cleanWords(*p.ReservedNames, "reserved_names")
		if err != nil {
			return Settings{}, err
		}
		writes["reserved_names"] = w
	}
	if p.BlockedWords != nil {
		w, err := cleanWords(*p.BlockedWords, "blocked_words")
		if err != nil {
			return Settings{}, err
		}
		writes["blocked_words"] = w
	}
	if len(writes) == 0 {
		return before, badRequest("nothing_to_change", "Nada para mudar.")
	}
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		for k, v := range writes {
			if err := s.Store.PutSetting(ctx, tx, k, mustJSON(v)); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, op, "settings.update", "settings", "global", reason, map[string]any{"changes": writes})
	})
	if err != nil {
		return Settings{}, err
	}
	c := s.cache()
	c.mu.Lock()
	c.ok = false
	c.mu.Unlock()
	return s.Settings(ctx)
}

// Status is the settings plus what they mean right now.
type Status struct {
	Settings
	SpendTodayUSD float64 `json:"spend_today_usd"`
	CapReached    bool    `json:"cap_reached"`
	ModelsOpen    bool    `json:"models_open"` // false when paused by hand or by the cap
}

func (s *Service) Status(ctx context.Context) (*Status, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	spend, err := s.Store.SpendSince(ctx, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		return nil, err
	}
	out := &Status{Settings: st, SpendTodayUSD: round(spend, 4)}
	out.CapReached = st.DailySpendCapUSD > 0 && spend >= st.DailySpendCapUSD
	out.ModelsOpen = !st.BuildsPaused && !out.CapReached
	return out, nil
}

// errModelsPaused is what owners see while models are paused. Nothing has
// been charged when it is returned.
var errModelsPaused = &Error{Status: http.StatusServiceUnavailable, Code: "builds_paused",
	Message: "A Faz está em manutenção por momentos. Tenta outra vez daqui a pouco; não gastaste créditos."}

// modelsOpen refuses new model work while builds are paused or today's
// spend has reached the cap.
func (s *Service) modelsOpen(ctx context.Context) error {
	st, err := s.Status(ctx)
	if err != nil {
		return err
	}
	if !st.ModelsOpen {
		return errModelsPaused
	}
	return nil
}

// PublicConfig is what the web app needs before anyone signs in.
type PublicConfig struct {
	BetaRequired bool `json:"beta_required"`
}

func (s *Service) PublicConfig(ctx context.Context) (*PublicConfig, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return &PublicConfig{BetaRequired: st.BetaRequired}, nil
}

// ---- Invite codes ----

var inviteRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{2,39}$`)

// NormalizeInvite uppercases a code and drops spaces, as people type it.
func NormalizeInvite(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
}

// NewInvite is what an operator fills in to create a code.
type NewInvite struct {
	Code         string     `json:"code"` // empty: generated
	Note         string     `json:"note"`
	MaxUses      int        `json:"max_uses"`
	BonusCredits int        `json:"bonus_credits"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

const inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O or 1/I

func randomInvite() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := []byte("FAZ-")
	for i, x := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, inviteAlphabet[int(x)%len(inviteAlphabet)])
	}
	return string(out)
}

func (s *Service) CreateInvite(ctx context.Context, op Operator, reason string, in NewInvite) (*store.Invite, error) {
	if err := op.check(reason); err != nil {
		return nil, err
	}
	code := NormalizeInvite(in.Code)
	if code == "" {
		code = randomInvite()
	}
	switch {
	case !inviteRe.MatchString(code):
		return nil, badRequest("invalid_code", "O código leva 3 a 40 letras maiúsculas, números e hífens.")
	case in.MaxUses < 1 || in.MaxUses > 10000:
		return nil, badRequest("invalid_max_uses", "Usos: de 1 a 10 000.")
	case in.BonusCredits < 0 || in.BonusCredits > 5000:
		return nil, badRequest("invalid_bonus", "Bónus: de 0 a 5 000 créditos.")
	case in.ExpiresAt != nil && in.ExpiresAt.Before(time.Now()):
		return nil, badRequest("invalid_expiry", "A data de fim já passou.")
	}
	var inv store.Invite
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		inv, err = s.Store.CreateInvite(ctx, tx, store.Invite{Code: code, Note: clean(in.Note), MaxUses: in.MaxUses,
			BonusCredits: in.BonusCredits, ExpiresAt: in.ExpiresAt, CreatedBy: strings.TrimSpace(op.Name)})
		if errors.Is(err, store.ErrTaken) {
			return conflict("code_taken", "Esse código já existe.")
		}
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, op, "invite.create", "invite", code, reason, in)
	})
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Service) Invites(ctx context.Context) ([]store.Invite, error) { return s.Store.Invites(ctx) }

func (s *Service) DisableInvite(ctx context.Context, op Operator, reason, code string) (*store.Invite, error) {
	if err := op.check(reason); err != nil {
		return nil, err
	}
	var inv store.Invite
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if inv, err = s.Store.DisableInvite(ctx, tx, NormalizeInvite(code)); err != nil {
			return mapStoreErr(err)
		}
		return s.audit(ctx, tx, op, "invite.disable", "invite", inv.Code, reason, nil)
	})
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// ---- Users ----

func (s *Service) AdminUsers(ctx context.Context, query string) ([]store.UserRow, error) {
	return s.Store.SearchUsers(ctx, query, 50)
}

// UserDetail is everything support needs about one account.
type UserDetail struct {
	User     store.User          `json:"user"`
	Balance  int                 `json:"balance"`
	Sessions int                 `json:"sessions"`
	Sites    []store.Site        `json:"sites"`
	Ledger   []store.LedgerEntry `json:"ledger"`
	Payments []store.Payment     `json:"payments"`
	Audit    []store.AuditEntry  `json:"audit"`
}

func (s *Service) AdminUser(ctx context.Context, id string) (*UserDetail, error) {
	u, err := s.Store.UserByID(ctx, id)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	d := &UserDetail{User: u}
	if d.Balance, err = s.Store.Balance(ctx, s.Store.DB, id); err != nil {
		return nil, err
	}
	if d.Sessions, err = s.Store.ActiveSessions(ctx, id); err != nil {
		return nil, err
	}
	if d.Sites, err = s.Store.SitesByUser(ctx, id); err != nil {
		return nil, err
	}
	if d.Ledger, err = s.Store.Ledger(ctx, id, 200); err != nil {
		return nil, err
	}
	if d.Payments, err = s.Store.Payments(ctx, id, 100); err != nil {
		return nil, err
	}
	if d.Audit, err = s.Store.Audit(ctx, "user", id, 100); err != nil {
		return nil, err
	}
	if d.Sites == nil {
		d.Sites = []store.Site{}
	}
	if d.Ledger == nil {
		d.Ledger = []store.LedgerEntry{}
	}
	if d.Payments == nil {
		d.Payments = []store.Payment{}
	}
	return d, nil
}

// CreditAdjustment gives (delta > 0) or removes (delta < 0) credits.
// RequestID makes a repeated click a no-op. PublicNote is what the owner
// reads in their movements; the reason stays in the audit log.
type CreditAdjustment struct {
	Delta      int    `json:"delta"`
	PublicNote string `json:"public_note"`
	RequestID  string `json:"request_id"`
}

func (s *Service) AdjustCredits(ctx context.Context, op Operator, reason, userID string, in CreditAdjustment) (int, error) {
	if err := op.check(reason); err != nil {
		return 0, err
	}
	switch {
	case in.Delta == 0 || in.Delta > 100000 || in.Delta < -100000:
		return 0, badRequest("invalid_delta", "Escolhe um valor entre −100 000 e 100 000 créditos, sem ser 0.")
	case len(in.RequestID) < 8 || len(in.RequestID) > 80:
		return 0, badRequest("request_id_required", "Falta o identificador do pedido.")
	}
	note := clean(in.PublicNote)
	if note == "" {
		note = "Créditos oferecidos pela equipa Faz"
		if in.Delta < 0 {
			note = "Acerto feito pela equipa Faz"
		}
	}
	var balance int
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := s.Store.UserByID(ctx, userID); err != nil {
			return mapStoreErr(err)
		}
		if err := s.Store.LockUser(ctx, tx, userID); err != nil {
			return err
		}
		bal, err := s.Store.Balance(ctx, tx, userID)
		if err != nil {
			return err
		}
		if bal+in.Delta < 0 {
			return badRequest("would_go_negative", fmt.Sprintf("O saldo é %d créditos; não dá para tirar %d.", bal, -in.Delta))
		}
		key := "admin:" + in.RequestID
		added, err := s.Store.AddLedger(ctx, tx, store.LedgerEntry{UserID: userID, Delta: in.Delta, Kind: "adjustment", Reason: note, IdempotencyKey: &key})
		if err != nil {
			return err
		}
		balance = bal
		if !added { // the same request again: report the balance, change nothing
			return nil
		}
		balance = bal + in.Delta
		return s.audit(ctx, tx, op, "credits.adjust", "user", userID, reason, map[string]any{"delta": in.Delta, "public_note": note, "balance_after": balance})
	})
	return balance, err
}

// RefundJob gives back what a build or edit charged. A job refunded before,
// by a failure or another operator, is refused rather than paid twice.
func (s *Service) RefundJob(ctx context.Context, op Operator, reason string, jobID int64) (int, error) {
	if err := op.check(reason); err != nil {
		return 0, err
	}
	var amount int
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		userID, charged, err := s.Store.JobCharge(ctx, tx, jobID)
		if err != nil {
			return mapStoreErr(err)
		}
		added, err := s.refund(ctx, tx, userID, charged, jobID, "Devolução feita pela equipa Faz")
		if err != nil {
			return err
		}
		if !added {
			return conflict("already_refunded", "Este trabalho já foi devolvido.")
		}
		amount = charged
		return s.audit(ctx, tx, op, "credits.refund_job", "user", userID, reason, map[string]any{"job_id": jobID, "credits": charged})
	})
	return amount, err
}

// SuspendUser blocks or unblocks an account. Suspending signs it out
// everywhere; its published sites stay up unless taken down separately.
func (s *Service) SuspendUser(ctx context.Context, op Operator, reason, userID string, suspend bool) (*store.User, error) {
	if err := op.check(reason); err != nil {
		return nil, err
	}
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := s.Store.UserByID(ctx, userID); err != nil {
			return mapStoreErr(err)
		}
		var why *string
		action := "user.unsuspend"
		if suspend {
			r := clean(reason)
			why, action = &r, "user.suspend"
		}
		if err := s.Store.SetUserSuspended(ctx, tx, userID, why); err != nil {
			return err
		}
		ended := int64(0)
		if suspend {
			var err error
			if ended, err = s.Store.DeleteUserSessions(ctx, tx, userID); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, op, action, "user", userID, reason, map[string]any{"sessions_ended": ended})
	})
	if err != nil {
		return nil, err
	}
	u, err := s.Store.UserByID(ctx, userID)
	return &u, err
}

// SignOutEverywhere ends every session of an account.
func (s *Service) SignOutEverywhere(ctx context.Context, op Operator, reason, userID string) (int64, error) {
	if err := op.check(reason); err != nil {
		return 0, err
	}
	var ended int64
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := s.Store.UserByID(ctx, userID); err != nil {
			return mapStoreErr(err)
		}
		var err error
		if ended, err = s.Store.DeleteUserSessions(ctx, tx, userID); err != nil {
			return err
		}
		return s.audit(ctx, tx, op, "user.sign_out", "user", userID, reason, map[string]any{"sessions_ended": ended})
	})
	return ended, err
}

// ---- Sites ----

func (s *Service) AdminSites(ctx context.Context, query string) ([]store.SiteRow, error) {
	return s.Store.SearchSites(ctx, query, 50)
}

// TakeDown removes a site from the air and stops its owner publishing it
// again. The address stays with the site, so nobody else can take it over.
func (s *Service) TakeDown(ctx context.Context, op Operator, reason, siteID string) (*store.Site, error) {
	if err := op.check(reason); err != nil {
		return nil, err
	}
	var site store.Site
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if site, err = s.Store.LockSite(ctx, tx, siteID, ""); err != nil {
			return mapStoreErr(err)
		}
		r := clean(reason)
		if err := s.Store.SetSiteTakedown(ctx, tx, siteID, &r); err != nil {
			return err
		}
		// Deleted before commit: if R2 fails, the site is not marked down while still live.
		if site.Subdomain != nil {
			if err := s.R2.Delete(ctx, "live/"+*site.Subdomain+"/index.html"); err != nil {
				return err
			}
			_ = s.R2.Delete(ctx, "live/"+*site.Subdomain+"/site.json")
		}
		return s.audit(ctx, tx, op, "site.takedown", "site", siteID, reason, map[string]any{"subdomain": site.Subdomain})
	})
	if err != nil {
		return nil, err
	}
	out, err := s.Store.Site(ctx, s.Store.DB, siteID, "")
	return &out, err
}

// Restore lifts a takedown and puts the last published version back live.
func (s *Service) Restore(ctx context.Context, op Operator, reason, siteID string) (*store.Site, error) {
	if err := op.check(reason); err != nil {
		return nil, err
	}
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		site, err := s.Store.LockSite(ctx, tx, siteID, "")
		if err != nil {
			return mapStoreErr(err)
		}
		if site.TakenDownAt == nil {
			return conflict("not_taken_down", "Este site não está retirado do ar.")
		}
		if err := s.Store.SetSiteTakedown(ctx, tx, siteID, nil); err != nil {
			return err
		}
		if site.Subdomain != nil && site.PublishedVersion != nil {
			ver, err := s.Store.Version(ctx, site.ID, *site.PublishedVersion)
			if err != nil {
				return err
			}
			if err := s.R2.Copy(ctx, ver.R2Prefix+"/index.html", "live/"+*site.Subdomain+"/index.html", "public, max-age=60"); err != nil {
				return err
			}
			meta := mustJSON(map[string]any{"site_id": site.ID, "version": *site.PublishedVersion, "published_at": time.Now().UTC()})
			if err := s.R2.Put(ctx, "live/"+*site.Subdomain+"/site.json", meta, "application/json", "no-store"); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, op, "site.restore", "site", siteID, reason, map[string]any{"subdomain": site.Subdomain})
	})
	if err != nil {
		return nil, err
	}
	out, err := s.Store.Site(ctx, s.Store.DB, siteID, "")
	return &out, err
}

func (s *Service) AuditLog(ctx context.Context, targetType, targetID string) ([]store.AuditEntry, error) {
	return s.Store.Audit(ctx, targetType, targetID, 300)
}
