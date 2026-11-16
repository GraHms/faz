package product

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/GraHms/faz/internal/auth"
	"github.com/GraHms/faz/internal/store"
)

// Account is the signed in customer with their balance.
type Account struct {
	User    store.User `json:"user"`
	Balance int        `json:"balance"`
}

// Session is returned on signup and login. The token goes in the
// Authorization header; Faz uses no cookies, so pages on other faz.co.mz
// subdomains cannot ride a customer's session.
type Session struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Account   Account   `json:"account"`
}

var errBadLogin = &Error{Status: http.StatusUnauthorized, Code: "bad_login", Message: "Email ou palavra-passe errados."}

// ErrUnauthenticated means no valid session.
var ErrUnauthenticated = &Error{Status: http.StatusUnauthorized, Code: "unauthenticated", Message: "Entra na tua conta para continuar."}

// errSuspended is returned for suspended accounts: 401 on a session, so the
// app signs out, and 403 on login.
var errSuspended = &Error{Status: http.StatusUnauthorized, Code: "account_suspended", Message: "A tua conta está suspensa. Fala com a equipa Faz para saber porquê."}

var (
	errInviteRequired = &Error{Status: http.StatusForbidden, Code: "invite_required", Message: "A Faz está em beta fechado: precisas de um código de convite para criar conta."}
	errInviteInvalid  = &Error{Status: http.StatusForbidden, Code: "invite_invalid", Message: "Este código de convite não é válido, expirou ou já foi usado todas as vezes."}
)

func normalizeEmail(e string) (string, error) {
	e = strings.ToLower(clean(e))
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e || !strings.Contains(e[strings.Index(e, "@"):], ".") {
		return "", badRequest("invalid_email", "Escreve um email válido.")
	}
	return e, nil
}

// Signup creates an account, grants the signup credits and opens a session.
// Signup creates an account. While the beta is on, a live invite code is
// required; a code's bonus credits are added on top of the signup gift.
func (s *Service) Signup(ctx context.Context, name, email, password, invite string) (*Session, error) {
	name = clean(name)
	if name == "" || len([]rune(name)) > 80 {
		return nil, badRequest("invalid_name", "Escreve o teu nome.")
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return nil, badRequest("weak_password", "A palavra-passe precisa de pelo menos 8 caracteres.")
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	invite = NormalizeInvite(invite)
	if settings.BetaRequired && invite == "" {
		return nil, errInviteRequired
	}
	hash, err := auth.HashPassword(password, s.Cfg.PasswordIterations)
	if err != nil {
		return nil, err
	}
	var user store.User
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		u, err := s.Store.CreateUser(ctx, tx, email, name, hash)
		if err != nil {
			return err
		}
		user = u
		if s.Cfg.Pricing.SignupGift > 0 {
			key := "gift:" + u.ID
			if _, err = s.Store.AddLedger(ctx, tx, store.LedgerEntry{UserID: u.ID, Delta: s.Cfg.Pricing.SignupGift, Kind: "signup_gift",
				Reason: "Créditos de oferta para o primeiro site", IdempotencyKey: &key}); err != nil {
				return err
			}
		}
		// A code typed by mistake is refused even when the beta is off, so
		// nobody signs up believing they got a bonus they did not get.
		if invite != "" {
			inv, err := s.Store.UseInvite(ctx, tx, invite)
			if errors.Is(err, store.ErrNotFound) {
				return errInviteInvalid
			}
			if err != nil {
				return err
			}
			if err := s.Store.SetUserInvite(ctx, tx, u.ID, inv.Code); err != nil {
				return err
			}
			user.InviteCode = &inv.Code
			if inv.BonusCredits > 0 {
				key := "invite:" + u.ID
				if _, err := s.Store.AddLedger(ctx, tx, store.LedgerEntry{UserID: u.ID, Delta: inv.BonusCredits, Kind: "adjustment",
					Reason: "Bónus do convite " + inv.Code, IdempotencyKey: &key}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if errors.Is(err, store.ErrEmailTaken) {
		return nil, conflict("email_taken", "Já existe uma conta com este email. Entra na tua conta.")
	}
	if err != nil {
		return nil, err
	}
	return s.openSession(ctx, user)
}

// Login checks the password and opens a session.
func (s *Service) Login(ctx context.Context, email, password string) (*Session, error) {
	email = strings.ToLower(clean(email))
	u, err := s.Store.UserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		// Spend the same time as a real check so timing does not reveal accounts.
		auth.CheckPassword("pbkdf2-sha256$600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
		return nil, errBadLogin
	}
	if err != nil {
		return nil, err
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		return nil, errBadLogin
	}
	if u.SuspendedAt != nil {
		return nil, &Error{Status: http.StatusForbidden, Code: errSuspended.Code, Message: errSuspended.Message}
	}
	return s.openSession(ctx, u)
}

func (s *Service) openSession(ctx context.Context, u store.User) (*Session, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return nil, err
	}
	ttl := s.Cfg.SessionTTL
	if ttl == 0 {
		ttl = 30 * 24 * time.Hour
	}
	exp := time.Now().Add(ttl).UTC()
	if err := s.Store.CreateSession(ctx, hash, u.ID, exp); err != nil {
		return nil, err
	}
	acct, err := s.account(ctx, u)
	if err != nil {
		return nil, err
	}
	return &Session{Token: token, ExpiresAt: exp, Account: *acct}, nil
}

// Authenticate resolves a session token to its user.
func (s *Service) Authenticate(ctx context.Context, token string) (store.User, error) {
	if token == "" {
		return store.User{}, ErrUnauthenticated
	}
	u, err := s.Store.UserBySession(ctx, auth.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, ErrUnauthenticated
	}
	if err == nil && u.SuspendedAt != nil {
		return store.User{}, errSuspended
	}
	return u, err
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Store.DeleteSession(ctx, auth.HashToken(token))
}

// Me returns the account with its current balance.
func (s *Service) Me(ctx context.Context, u store.User) (*Account, error) { return s.account(ctx, u) }

func (s *Service) account(ctx context.Context, u store.User) (*Account, error) {
	b, err := s.Store.Balance(ctx, s.Store.DB, u.ID)
	if err != nil {
		return nil, err
	}
	return &Account{User: u, Balance: b}, nil
}

// Credits returns the balance and recent ledger entries.
func (s *Service) Credits(ctx context.Context, u store.User) (map[string]any, error) {
	b, err := s.Store.Balance(ctx, s.Store.DB, u.ID)
	if err != nil {
		return nil, err
	}
	l, err := s.Store.Ledger(ctx, u.ID, 100)
	if err != nil {
		return nil, err
	}
	if l == nil {
		l = []store.LedgerEntry{}
	}
	return map[string]any{"balance": b, "ledger": l, "pricing": s.Cfg.Pricing}, nil
}

// charge debits credits inside tx after locking the user. It fails with a
// 402 when the balance is short.
func (s *Service) charge(ctx context.Context, tx *sql.Tx, userID string, amount int, jobID int64, reason string) error {
	if amount <= 0 {
		return nil
	}
	if err := s.Store.LockUser(ctx, tx, userID); err != nil {
		return err
	}
	bal, err := s.Store.Balance(ctx, tx, userID)
	if err != nil {
		return err
	}
	if bal < amount {
		return insufficient(amount, bal)
	}
	key := "charge:job:" + itoa(jobID)
	_, err = s.Store.AddLedger(ctx, tx, store.LedgerEntry{UserID: userID, Delta: -amount, Kind: "charge", Reason: reason, JobID: &jobID, IdempotencyKey: &key})
	return err
}

// refund returns a job's charge once, however many times it is called.
func (s *Service) refund(ctx context.Context, q store.Q, userID string, amount int, jobID int64, reason string) (bool, error) {
	if amount <= 0 {
		return false, nil
	}
	key := "refund:job:" + itoa(jobID)
	return s.Store.AddLedger(ctx, q, store.LedgerEntry{UserID: userID, Delta: amount, Kind: "refund", Reason: reason, JobID: &jobID, IdempotencyKey: &key})
}
