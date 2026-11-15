package product

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/GraHms/faz/internal/jobs"
	"github.com/GraHms/faz/internal/payments"
	"github.com/GraHms/faz/internal/store"
)

// TopUp starts a mobile money payment for credits. The payment stays
// pending until the provider settles it, through its webhook or, for the
// mock, a scheduled job.
func (s *Service) TopUp(ctx context.Context, u store.User, amountMZN int, msisdn, nuit string) (*store.Payment, error) {
	pr := s.Cfg.Pricing
	if amountMZN < pr.TopupMinMZN || (pr.TopupMaxMZN > 0 && amountMZN > pr.TopupMaxMZN) {
		return nil, badRequest("invalid_amount", fmt.Sprintf("Escolhe um valor entre %d e %d MZN.", pr.TopupMinMZN, pr.TopupMaxMZN))
	}
	number, method, err := payments.ParseMSISDN(msisdn)
	if err != nil {
		return nil, badRequest("invalid_msisdn", "Usa um número M-Pesa (84, 85) ou e-Mola (86, 87).")
	}
	var nuitPtr *string
	if n := clean(nuit); n != "" {
		if len(n) != 9 {
			return nil, badRequest("invalid_nuit", "O NUIT tem 9 dígitos.")
		}
		nuitPtr = &n
	}
	pay, err := s.Store.CreatePayment(ctx, s.Store.DB, store.Payment{UserID: u.ID, Method: method, MSISDN: number, AmountMZN: amountMZN,
		VATMZN: payments.VATIncluded(amountMZN), Credits: pr.CreditsFor(amountMZN), NUIT: nuitPtr, Provider: s.Pay.Name()})
	if err != nil {
		return nil, err
	}
	ref, err := s.Pay.Initiate(ctx, payments.Request{PaymentID: pay.ID, MSISDN: number, Method: method, AmountMZN: amountMZN, Reference: "FAZ " + pay.ID[:8]})
	if err != nil {
		reason := "Não foi possível iniciar o pagamento."
		_ = s.Store.SetPaymentStatus(ctx, s.Store.DB, pay.ID, "failed", &reason)
		return nil, &Error{Status: http.StatusBadGateway, Code: "payment_unavailable", Message: reason + " Tenta outra vez."}
	}
	if err := s.Store.SetPaymentRef(ctx, pay.ID, ref); err != nil {
		return nil, err
	}
	if sim, ok := s.Pay.(payments.Simulator); ok {
		st, delay := sim.Simulate(ref)
		if _, err := jobs.Enqueue(ctx, s.Store.DB, JobSettle, map[string]any{"user_id": u.ID, "provider": s.Pay.Name(), "settlement": st},
			jobs.Options{Delay: delay, MaxAttempts: 5}); err != nil {
			return nil, err
		}
	}
	p, err := s.Store.Payment(ctx, pay.ID, u.ID)
	return &p, err
}

// Settle applies a provider's outcome once. A success adds the credits; a
// repeated or late settlement changes nothing.
func (s *Service) Settle(ctx context.Context, provider string, st payments.Settlement) error {
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		pay, err := s.Store.LockPaymentByRef(ctx, tx, provider, st.ProviderRef)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no payment with reference %s", st.ProviderRef)
		}
		if err != nil {
			return err
		}
		if pay.Status != "pending" {
			return nil
		}
		if !st.Success {
			reason := st.Reason
			if reason == "" {
				reason = "O pagamento não foi confirmado."
			}
			return s.Store.SetPaymentStatus(ctx, tx, pay.ID, "failed", &reason)
		}
		if err := s.Store.SetPaymentStatus(ctx, tx, pay.ID, "succeeded", nil); err != nil {
			return err
		}
		key := "topup:" + pay.ID
		_, err = s.Store.AddLedger(ctx, tx, store.LedgerEntry{UserID: pay.UserID, Delta: pay.Credits, Kind: "topup",
			Reason: fmt.Sprintf("Recarga de %d MZN (%s)", pay.AmountMZN, pay.Method), PaymentID: &pay.ID, IdempotencyKey: &key})
		return err
	})
}

// Callback handles a provider webhook.
func (s *Service) Callback(ctx context.Context, provider string, r *http.Request) error {
	if provider != s.Pay.Name() {
		return errNotFound
	}
	st, err := s.Pay.ParseCallback(r)
	if err != nil {
		return &Error{Status: http.StatusUnauthorized, Code: "bad_callback", Message: err.Error()}
	}
	return s.Settle(ctx, provider, st)
}

func (s *Service) handleSettle(ctx context.Context, j jobs.Job) (any, error) {
	var pl struct {
		Provider   string              `json:"provider"`
		Settlement payments.Settlement `json:"settlement"`
	}
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return nil, err
	}
	return nil, s.Settle(ctx, pl.Provider, pl.Settlement)
}

// Payment returns one of the customer's payments.
func (s *Service) Payment(ctx context.Context, u store.User, id string) (*store.Payment, error) {
	p, err := s.Store.Payment(ctx, id, u.ID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	return &p, nil
}

// Payments lists the customer's payments.
func (s *Service) Payments(ctx context.Context, u store.User) ([]store.Payment, error) {
	ps, err := s.Store.Payments(ctx, u.ID, 100)
	if ps == nil {
		ps = []store.Payment{}
	}
	return ps, err
}
