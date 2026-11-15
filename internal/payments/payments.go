// Package payments defines how Faz takes mobile money, and a mock provider
// that behaves like the real ones: a payment starts pending, the customer
// confirms on their phone, and the provider reports the outcome later.
//
// Real M-Pesa (Vodacom, 84 and 85) and e-Mola (Movitel, 86 and 87) adapters
// implement Provider; nothing else in Faz changes when they arrive.
package payments

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	MPesa = "mpesa"
	EMola = "emola"
)

// VATRate is Mozambique's standard IVA rate. Prices include it.
const VATRate = 0.16

// VATIncluded returns the IVA part of a price that already includes it.
func VATIncluded(amountMZN int) int {
	return int(math.Round(float64(amountMZN) * VATRate / (1 + VATRate)))
}

// ParseMSISDN normalizes a Mozambican mobile number to 258XXXXXXXXX and
// returns the wallet it belongs to.
func ParseMSISDN(s string) (string, string, error) {
	var digits strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	d := strings.TrimPrefix(digits.String(), "258")
	if len(d) != 9 || d[0] != '8' {
		return "", "", errors.New("enter a 9 digit Mozambican mobile number, for example 84 123 4567")
	}
	switch d[1] {
	case '4', '5':
		return "258" + d, MPesa, nil
	case '6', '7':
		return "258" + d, EMola, nil
	}
	return "", "", errors.New("use an M-Pesa number (84 or 85) or an e-Mola number (86 or 87)")
}

// Request starts a payment.
type Request struct {
	PaymentID string
	MSISDN    string
	Method    string
	AmountMZN int
	Reference string // shown to the customer on their phone
}

// Settlement is the provider's final word on a payment.
type Settlement struct {
	ProviderRef string `json:"provider_ref"`
	Success     bool   `json:"success"`
	Reason      string `json:"reason,omitempty"`
}

// Provider is one mobile money integration.
type Provider interface {
	Name() string
	// Initiate asks the provider to push a payment prompt to the customer's
	// phone and returns the provider's reference for it.
	Initiate(ctx context.Context, r Request) (string, error)
	// ParseCallback reads the provider's webhook for a settled payment.
	ParseCallback(r *http.Request) (Settlement, error)
}

// Simulator is implemented by providers that settle payments themselves,
// like the mock. The product schedules Settle after Delay.
type Simulator interface {
	Simulate(ref string) (Settlement, time.Duration)
}

// Mock settles every payment with a fixed outcome after a delay, and accepts
// callbacks signed with a shared secret so the webhook path is exercised.
type Mock struct {
	Secret  string
	Fail    bool
	Delay   time.Duration
	Reason  string
	refSeed func() string
}

func (m *Mock) Name() string { return "mock" }

func (m *Mock) Initiate(_ context.Context, r Request) (string, error) {
	if r.AmountMZN <= 0 {
		return "", errors.New("amount must be positive")
	}
	if m.refSeed != nil {
		return m.refSeed(), nil
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "MOCK-" + strings.ToUpper(hex.EncodeToString(b)), nil
}

func (m *Mock) Simulate(ref string) (Settlement, time.Duration) {
	if m.Fail {
		reason := m.Reason
		if reason == "" {
			reason = "O pagamento não foi confirmado. Verifica o saldo e tenta outra vez."
		}
		return Settlement{ProviderRef: ref, Success: false, Reason: reason}, m.Delay
	}
	return Settlement{ProviderRef: ref, Success: true}, m.Delay
}

func (m *Mock) ParseCallback(r *http.Request) (Settlement, error) {
	got := r.Header.Get("X-Faz-Mock-Secret")
	if m.Secret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(m.Secret)) != 1 {
		return Settlement{}, errors.New("callback signature does not match")
	}
	var s Settlement
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&s); err != nil {
		return Settlement{}, fmt.Errorf("callback body: %w", err)
	}
	if s.ProviderRef == "" {
		return Settlement{}, errors.New("callback has no provider_ref")
	}
	return s, nil
}
