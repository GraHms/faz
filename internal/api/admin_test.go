package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/GraHms/faz/internal/product"
	"github.com/GraHms/faz/internal/store"
)

// adminDo calls an admin route with the admin key and an operator name.
func (h *harness) adminDo(method, path, operator string, in, out any) int {
	h.t.Helper()
	var body bytes.Buffer
	if in != nil {
		json.NewEncoder(&body).Encode(in)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, &body)
	req.Header.Set("X-Faz-Admin", adminToken)
	if operator != "" {
		req.Header.Set("X-Faz-Operator", operator)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

type apiErr struct {
	Error struct{ Code, Message string } `json:"error"`
}

func TestAdminOperations(t *testing.T) {
	h := setup(t)
	const ismael = "Ismael"
	why := func(s string) map[string]any { return map[string]any{"reason": s} }

	// Writes need an operator and a reason; reads only the key.
	var st product.Status
	if code := h.adminDo("GET", "/v1/admin/status", "", nil, &st); code != 200 || !st.ModelsOpen || st.BetaRequired {
		t.Fatalf("status: %d %+v", code, st)
	}
	var e apiErr
	if code := h.adminDo("POST", "/v1/admin/settings", "", map[string]any{"beta_required": true, "reason": "abrir o beta"}, &e); code != 400 || e.Error.Code != "operator_required" {
		t.Fatalf("settings without operator: %d %+v", code, e)
	}
	if code := h.adminDo("POST", "/v1/admin/settings", ismael, map[string]any{"beta_required": true}, &e); code != 400 || e.Error.Code != "reason_required" {
		t.Fatalf("settings without reason: %d %+v", code, e)
	}

	// Closed beta: sign up needs a live code, and the code's bonus is added.
	if code := h.adminDo("POST", "/v1/admin/settings", ismael, map[string]any{"beta_required": true, "reason": "Beta fechado em Maputo"}, &st); code != 200 || !st.BetaRequired {
		t.Fatalf("enable beta: %d %+v", code, st)
	}
	var cfg product.PublicConfig
	if h.call("GET", "/v1/config", "", nil, &cfg); !cfg.BetaRequired {
		t.Fatal("public config should say the beta is closed")
	}
	if code := h.call("POST", "/v1/auth/signup", "", map[string]string{"name": "Sem Código", "email": "sem@exemplo.co.mz", "password": "palavra-passe"}, &e); code != 403 || e.Error.Code != "invite_required" {
		t.Fatalf("signup without code: %d %+v", code, e)
	}
	var inv store.Invite
	if code := h.adminDo("POST", "/v1/admin/invites", ismael, map[string]any{"code": "maputo-beta", "note": "Grupo de WhatsApp", "max_uses": 1, "bonus_credits": 100, "reason": "Primeiros testes"}, &inv); code != 201 || inv.Code != "MAPUTO-BETA" {
		t.Fatalf("create invite: %d %+v", code, inv)
	}
	var generated store.Invite
	if code := h.adminDo("POST", "/v1/admin/invites", ismael, map[string]any{"max_uses": 5, "reason": "Código gerado"}, &generated); code != 201 || !strings.HasPrefix(generated.Code, "FAZ-") {
		t.Fatalf("generated invite: %d %+v", code, generated)
	}
	var sess product.Session
	if code := h.call("POST", "/v1/auth/signup", "", map[string]string{"name": "Rita Cossa", "email": "rita@exemplo.co.mz", "password": "palavra-passe", "invite_code": " maputo-beta "}, &sess); code != 201 {
		t.Fatalf("signup with code: %d", code)
	}
	tok, uid := sess.Token, sess.Account.User.ID
	if sess.Account.Balance != 119 || sess.Account.User.InviteCode == nil || *sess.Account.User.InviteCode != "MAPUTO-BETA" {
		t.Fatalf("account with invite: %+v", sess.Account)
	}
	if code := h.call("POST", "/v1/auth/signup", "", map[string]string{"name": "Outra", "email": "outra@exemplo.co.mz", "password": "palavra-passe", "invite_code": "MAPUTO-BETA"}, &e); code != 403 || e.Error.Code != "invite_invalid" {
		t.Fatalf("used up code: %d %+v", code, e)
	}
	if code := h.adminDo("POST", "/v1/admin/invites/"+generated.Code+"/disable", ismael, why("Partilhado por engano"), &generated); code != 200 || generated.DisabledAt == nil {
		t.Fatalf("disable: %d %+v", code, generated)
	}
	if code := h.call("POST", "/v1/auth/signup", "", map[string]string{"name": "Outra", "email": "outra@exemplo.co.mz", "password": "palavra-passe", "invite_code": generated.Code}, &e); code != 403 || e.Error.Code != "invite_invalid" {
		t.Fatalf("disabled code: %d %+v", code, e)
	}

	// Support finds the account by the phone number of a payment.
	if code := h.call("POST", "/v1/payments", tok, map[string]any{"amount_mzn": 200, "msisdn": "84 123 4567"}, nil); code != 202 {
		t.Fatalf("top up: %d", code)
	}
	var found struct{ Users []store.UserRow }
	if h.adminDo("GET", "/v1/admin/users?q=%2B258%2084%20123%204567", "", nil, &found); len(found.Users) != 1 || found.Users[0].ID != uid {
		t.Fatalf("search by phone: %+v", found)
	}
	if h.adminDo("GET", "/v1/admin/users?q=rita%40", "", nil, &found); len(found.Users) != 1 {
		t.Fatalf("search by email: %+v", found)
	}

	// Credits: given once per request id, never below zero, explained to the owner.
	var bal struct{ Balance int }
	grant := map[string]any{"delta": 50, "request_id": "req-0001-abc", "reason": "Compensação pela falha de ontem"}
	if code := h.adminDo("POST", "/v1/admin/users/"+uid+"/credits", ismael, grant, &bal); code != 200 || bal.Balance != 169 {
		t.Fatalf("grant: %d %+v", code, bal)
	}
	if code := h.adminDo("POST", "/v1/admin/users/"+uid+"/credits", ismael, grant, &bal); code != 200 || bal.Balance != 169 || h.balance(tok) != 169 {
		t.Fatalf("repeated grant must not pay twice: %d %+v", code, bal)
	}
	if code := h.adminDo("POST", "/v1/admin/users/"+uid+"/credits", ismael, map[string]any{"delta": -500, "request_id": "req-0002-abc", "reason": "Erro de teste"}, &e); code != 400 || e.Error.Code != "would_go_negative" {
		t.Fatalf("negative balance: %d %+v", code, e)
	}
	var credits struct{ Ledger []store.LedgerEntry }
	h.call("GET", "/v1/credits", tok, nil, &credits)
	if credits.Ledger[0].Reason != "Créditos oferecidos pela equipa Faz" || strings.Contains(credits.Ledger[0].Reason, "falha") {
		t.Errorf("the owner sees the public note, not the internal reason: %q", credits.Ledger[0].Reason)
	}

	// Kill switches: paused by hand, then by the daily cap. Nothing is charged.
	newSite := map[string]string{"prompt": "Site para a minha marisqueira na Costa do Sol", "business_name": "Mar Azul", "business_type": "Restaurante"}
	h.adminDo("POST", "/v1/admin/settings", ismael, map[string]any{"builds_paused": true, "pause_reason": "Falha no fornecedor", "reason": "Incidente"}, nil)
	if code := h.call("POST", "/v1/sites", tok, newSite, &e); code != 503 || e.Error.Code != "builds_paused" || h.balance(tok) != 169 {
		t.Fatalf("paused: %d %+v", code, e)
	}
	h.adminDo("POST", "/v1/admin/settings", ismael, map[string]any{"builds_paused": false, "reason": "Fornecedor recuperado"}, nil)
	var site product.SiteView
	if code := h.call("POST", "/v1/sites", tok, newSite, &site); code != 201 {
		t.Fatalf("after unpause: %d", code)
	}
	h.adminDo("POST", "/v1/admin/settings", ismael, map[string]any{"daily_spend_cap_usd": 0.000001, "reason": "Testar o limite"}, &st)
	if !st.CapReached || st.ModelsOpen {
		t.Fatalf("cap should be reached: %+v", st)
	}
	if code := h.call("POST", "/v1/sites/"+site.Site.ID+"/answers", tok, map[string]any{"skip": true}, &e); code != 503 || h.balance(tok) != 169 {
		t.Fatalf("over the cap: %d %+v", code, e)
	}
	h.adminDo("POST", "/v1/admin/settings", ismael, map[string]any{"daily_spend_cap_usd": 0, "reason": "Sem limite"}, nil)
	var queued struct {
		JobID int64 `json:"job_id"`
	}
	if code := h.call("POST", "/v1/sites/"+site.Site.ID+"/answers", tok, map[string]any{"skip": true}, &queued); code != 202 {
		t.Fatalf("answers: %d", code)
	}
	h.drain()

	// Address rules an operator can extend.
	h.adminDo("POST", "/v1/admin/settings", ismael, map[string]any{"reserved_names": []string{"Faz-Oficial"}, "blocked_words": []string{"golpe"}, "reason": "Proteger a marca"}, nil)
	var av struct {
		Available bool
		Reason    string
	}
	for _, name := range []string{"faz-oficial", "golpe-mpesa"} {
		if h.call("GET", "/v1/subdomains/"+name, tok, nil, &av); av.Available {
			t.Errorf("%s should not be available", name)
		}
	}

	// Takedown: off the air, owner cannot republish, restore puts it back.
	if code := h.call("POST", "/v1/sites/"+site.Site.ID+"/publish", tok, map[string]string{"subdomain": "mar-azul"}, nil); code != 200 {
		t.Fatalf("publish: %d", code)
	}
	var down store.Site
	if code := h.adminDo("POST", "/v1/admin/sites/"+site.Site.ID+"/takedown", ismael, why("Denúncia de burla"), &down); code != 200 || down.TakenDownAt == nil {
		t.Fatalf("takedown: %d %+v", code, down)
	}
	if ok, _ := h.svc.R2.Exists(context.Background(), "live/mar-azul/index.html"); ok {
		t.Fatal("a taken down site must not be served")
	}
	if code := h.call("POST", "/v1/sites/"+site.Site.ID+"/publish", tok, map[string]string{"subdomain": "mar-azul"}, &e); code != 403 || e.Error.Code != "site_taken_down" {
		t.Fatalf("republish after takedown: %d %+v", code, e)
	}
	if h.call("GET", "/v1/sites/"+site.Site.ID, tok, nil, &site); site.Site.TakenDownAt == nil {
		t.Fatal("the owner should see the takedown")
	}
	var sites struct{ Sites []store.SiteRow }
	if h.adminDo("GET", "/v1/admin/sites?q=mar-az", "", nil, &sites); len(sites.Sites) != 1 || sites.Sites[0].OwnerEmail != "rita@exemplo.co.mz" {
		t.Fatalf("site search: %+v", sites)
	}
	var restored store.Site
	if code := h.adminDo("POST", "/v1/admin/sites/"+site.Site.ID+"/restore", ismael, why("Falso alarme"), &restored); code != 200 || restored.TakenDownAt != nil {
		t.Fatalf("restore: %d %+v", code, restored)
	}
	if ok, _ := h.svc.R2.Exists(context.Background(), "live/mar-azul/index.html"); !ok {
		t.Fatal("restore should put the published version back")
	}

	// Refunding a job pays once.
	before := h.balance(tok)
	var refunded struct{ Credits int }
	if code := h.adminDo("POST", "/v1/admin/jobs/"+itoa(queued.JobID)+"/refund", ismael, why("Site saiu mal"), &refunded); code != 200 || refunded.Credits != 19 || h.balance(tok) != before+19 {
		t.Fatalf("refund: %d %+v", code, refunded)
	}
	if code := h.adminDo("POST", "/v1/admin/jobs/"+itoa(queued.JobID)+"/refund", ismael, why("Outra vez"), &e); code != 409 || e.Error.Code != "already_refunded" {
		t.Fatalf("second refund: %d %+v", code, e)
	}

	// Suspension signs the owner out and keeps them out until lifted.
	if code := h.adminDo("POST", "/v1/admin/users/"+uid+"/suspend", ismael, map[string]any{"suspended": true, "reason": "Burla confirmada"}, nil); code != 200 {
		t.Fatalf("suspend: %d", code)
	}
	if code := h.call("GET", "/v1/me", tok, nil, &e); code != 401 {
		t.Fatalf("old session after suspension: %d", code)
	}
	if code := h.call("POST", "/v1/auth/login", "", map[string]string{"email": "rita@exemplo.co.mz", "password": "palavra-passe"}, &e); code != 403 || e.Error.Code != "account_suspended" {
		t.Fatalf("login while suspended: %d %+v", code, e)
	}
	h.adminDo("POST", "/v1/admin/users/"+uid+"/suspend", ismael, map[string]any{"suspended": false, "reason": "Recurso aceite"}, nil)
	if code := h.call("POST", "/v1/auth/login", "", map[string]string{"email": "rita@exemplo.co.mz", "password": "palavra-passe"}, &sess); code != 200 {
		t.Fatalf("login after unsuspend: %d", code)
	}
	var signedOut struct {
		SessionsEnded int `json:"sessions_ended"`
	}
	if code := h.adminDo("POST", "/v1/admin/users/"+uid+"/sign-out", ismael, why("Telemóvel perdido"), &signedOut); code != 200 || signedOut.SessionsEnded != 1 {
		t.Fatalf("sign out everywhere: %d %+v", code, signedOut)
	}

	// Everything above is in the audit log, with who and why.
	var detail product.UserDetail
	if code := h.adminDo("GET", "/v1/admin/users/"+uid, "", nil, &detail); code != 200 || len(detail.Sites) != 1 || len(detail.Payments) != 1 {
		t.Fatalf("user detail: %d %+v", code, detail)
	}
	actions := map[string]bool{}
	for _, a := range detail.Audit {
		actions[a.Action] = true
		if a.Operator != ismael || a.Reason == "" {
			t.Errorf("audit entry without who or why: %+v", a)
		}
	}
	for _, want := range []string{"credits.adjust", "credits.refund_job", "user.suspend", "user.unsuspend", "user.sign_out"} {
		if !actions[want] {
			t.Errorf("audit is missing %s: %v", want, actions)
		}
	}
	var log struct{ Entries []store.AuditEntry }
	h.adminDo("GET", "/v1/admin/audit", "", nil, &log)
	if len(log.Entries) < 14 {
		t.Errorf("audit has %d entries", len(log.Entries))
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
