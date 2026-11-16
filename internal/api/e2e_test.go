package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GraHms/faz/internal/api"
	"github.com/GraHms/faz/internal/jobs"
	"github.com/GraHms/faz/internal/llm"
	"github.com/GraHms/faz/internal/payments"
	"github.com/GraHms/faz/internal/pipeline"
	"github.com/GraHms/faz/internal/product"
	"github.com/GraHms/faz/internal/r2"
	"github.com/GraHms/faz/internal/render"
	"github.com/GraHms/faz/internal/store"
)

// failingBuild wraps the fake model and fails the build stage on demand.
type failingBuild struct {
	llm.Client
	fail bool
}

func (f *failingBuild) Complete(ctx context.Context, r llm.Request) (llm.Response, error) {
	if f.fail && r.Stage == "build" {
		return llm.Response{}, errors.New("simulated model outage")
	}
	return f.Client.Complete(ctx, r)
}

type harness struct {
	t      *testing.T
	srv    *httptest.Server
	svc    *product.Service
	worker *jobs.Worker
	model  *failingBuild
}

// call sends a JSON request and decodes the response into out.
func (h *harness) call(method, path, token string, in, out any) int {
	h.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			h.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	return resp.StatusCode
}

// drain runs queued jobs until none are ready.
func (h *harness) drain() {
	h.t.Helper()
	for i := 0; i < 20; i++ {
		err := h.worker.RunOne(context.Background())
		if errors.Is(err, jobs.ErrNoJob) {
			return
		}
		if err != nil {
			h.t.Fatal(err)
		}
	}
	h.t.Fatal("queue did not drain")
}

func (h *harness) balance(token string) int {
	var me product.Account
	if code := h.call("GET", "/v1/me", token, nil, &me); code != 200 {
		h.t.Fatalf("me: %d", code)
	}
	return me.Balance
}

func setup(t *testing.T) *harness {
	dsn, s3 := os.Getenv("FAZ_TEST_DATABASE_URL"), os.Getenv("FAZ_TEST_S3")
	if dsn == "" || s3 == "" {
		t.Skip("set FAZ_TEST_DATABASE_URL and FAZ_TEST_S3 to run the end to end test")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	bucket := &r2.Client{Endpoint: s3, Bucket: fmt.Sprintf("faz-e2e-%d", time.Now().UnixNano()), Region: "us-east-1", AccessKey: "test", SecretKey: "test"}
	if err := r2.CreateBucket(ctx, bucket); err != nil {
		t.Fatal(err)
	}
	var chrome *render.Chrome
	if path, err := render.FindChrome(); err == nil {
		chrome = &render.Chrome{Path: path, NoSandbox: os.Geteuid() == 0}
	}
	model := &failingBuild{Client: &llm.Fake{Dir: "../../examples/fake"}}
	svc := &product.Service{Store: db, R2: bucket, LLM: model, Pay: &payments.Mock{Secret: "s3cret"}, Cfg: product.Config{
		Pricing: product.DefaultPricing, Profile: pipeline.Profiles["quality"], ProfileName: "quality",
		SitesDomain: "faz.co.mz", Chrome: chrome, PasswordIterations: 1000,
	}}
	srv := httptest.NewServer((&api.Server{Svc: svc}).Handler())
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, svc: svc, model: model,
		worker: &jobs.Worker{DB: db.DB, ID: "test", Handlers: svc.Handlers(), OnFailed: svc.OnJobFailed}}
}

func pngUpload(t *testing.T) (*bytes.Buffer, string) {
	img := image.NewRGBA(image.Rect(0, 0, 2400, 1800))
	for y := 0; y < 1800; y++ {
		for x := 0; x < 2400; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 251), uint8(y % 241), uint8((x * y) % 239), 255})
		}
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "Colmeias Lichinga.PNG")
	if err := png.Encode(part, img); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func TestProductEndToEnd(t *testing.T) {
	h := setup(t)
	brief, _ := os.ReadFile("../../examples/briefs/mel-niassa.md")

	// Accounts and the signup gift.
	var sess product.Session
	if code := h.call("POST", "/v1/auth/signup", "", map[string]string{"name": "Ana Muianga", "email": "Ana@Exemplo.co.mz", "password": "mel-do-niassa"}, &sess); code != 201 {
		t.Fatalf("signup: %d", code)
	}
	tok := sess.Token
	if sess.Account.Balance != 19 || sess.Account.User.Email != "ana@exemplo.co.mz" {
		t.Fatalf("account after signup: %+v", sess.Account)
	}
	if code := h.call("POST", "/v1/auth/signup", "", map[string]string{"name": "Ana", "email": "ana@exemplo.co.mz", "password": "outra-senha"}, nil); code != 409 {
		t.Fatalf("duplicate signup: %d", code)
	}
	if code := h.call("POST", "/v1/auth/login", "", map[string]string{"email": "ana@exemplo.co.mz", "password": "errada!!"}, nil); code != 401 {
		t.Fatalf("bad login: %d", code)
	}
	if code := h.call("GET", "/v1/me", "not-a-token", nil, nil); code != 401 {
		t.Fatalf("bad token: %d", code)
	}

	// Questions before the first build, free.
	var site product.SiteView
	if code := h.call("POST", "/v1/sites", tok, map[string]string{"prompt": string(brief), "business_name": "Cooperativa Apícola de Lichinga", "business_type": "Loja"}, &site); code != 201 {
		t.Fatalf("create site: %d", code)
	}
	id := site.Site.ID
	if site.Site.Status != "questions" || len(site.Questions) != 3 || h.balance(tok) != 19 {
		t.Fatalf("site after create: status %s, %d questions, balance %d", site.Site.Status, len(site.Questions), h.balance(tok))
	}

	// A large photo is scaled down to fit inline.
	body, ctype := pngUpload(t)
	req, _ := http.NewRequest("POST", h.srv.URL+"/v1/sites/"+id+"/assets", body)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", ctype)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var asset store.Asset
	json.NewDecoder(resp.Body).Decode(&asset)
	resp.Body.Close()
	if resp.StatusCode != 201 || asset.Name != "colmeias-lichinga.jpg" || asset.Size > 900_000 {
		t.Fatalf("upload: %d %+v", resp.StatusCode, asset)
	}

	// Answers are validated, then the first page is charged and queued.
	if code := h.call("POST", "/v1/sites/"+id+"/answers", tok, map[string]any{"answers": map[string]string{"sentir": "neon"}}, nil); code != 400 {
		t.Fatalf("invalid option: %d", code)
	}
	var queued struct {
		JobID int64 `json:"job_id"`
	}
	if code := h.call("POST", "/v1/sites/"+id+"/answers", tok, map[string]any{"answers": map[string]string{"entrega": "50 MT", "fotos": "sim", "sentir": "rotulo"}}, &queued); code != 202 || queued.JobID == 0 {
		t.Fatalf("answers: %d", code)
	}
	if b := h.balance(tok); b != 0 {
		t.Fatalf("balance after first page charge: %d", b)
	}
	if code := h.call("POST", "/v1/sites/"+id+"/messages", tok, map[string]string{"text": "muda a cor"}, nil); code != 409 {
		t.Fatalf("message while building: %d", code)
	}

	h.drain()
	if code := h.call("GET", "/v1/sites/"+id, tok, nil, &site); code != 200 || site.Site.Status != "ready" || site.Site.CurrentVersion != 1 {
		t.Fatalf("after build: %d %+v", code, site.Site)
	}
	last := site.Messages[len(site.Messages)-1]
	if !strings.Contains(last.Text, "2 informações") {
		t.Errorf("build message %q should mention the 2 placeholders", last.Text)
	}
	for _, f := range []string{"index.html", "source.html", "brief.json", "copy.json", "plan.json", "report.json"} {
		if ok, _ := h.svc.R2.Exists(context.Background(), "sites/"+id+"/v1/"+f); !ok {
			t.Errorf("R2 is missing v1/%s", f)
		}
	}

	// Preview comes from the storage origin through a presigned URL.
	var prev struct{ URL string }
	h.call("GET", "/v1/sites/"+id+"/preview", tok, nil, &prev)
	pr, err := http.Get(prev.URL)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(pr.Body)
	pr.Body.Close()
	if !strings.Contains(string(page), "Mel cru do Niassa") || strings.HasPrefix(prev.URL, h.srv.URL) {
		t.Fatalf("preview %s returned %d bytes", prev.URL, len(page))
	}

	// An edit with no credits is refused before anything is queued.
	var errBody struct{ Error product.Error }
	if code := h.call("POST", "/v1/sites/"+id+"/messages", tok, map[string]string{"text": "A entrega custa 50 MT"}, &errBody); code != 402 || errBody.Error.Code != "insufficient_credits" {
		t.Fatalf("edit without credits: %d %+v", code, errBody)
	}

	// Top up with the mock wallet: validation, settlement, idempotency.
	if code := h.call("POST", "/v1/payments", tok, map[string]any{"amount_mzn": 200, "msisdn": "82 123 4567"}, nil); code != 400 {
		t.Fatalf("tmcel number: %d", code)
	}
	if code := h.call("POST", "/v1/payments", tok, map[string]any{"amount_mzn": 100, "msisdn": "84 123 4567"}, nil); code != 400 {
		t.Fatalf("below minimum: %d", code)
	}
	var pay store.Payment
	if code := h.call("POST", "/v1/payments", tok, map[string]any{"amount_mzn": 200, "msisdn": "+258 84 123 4567", "nuit": "123456789"}, &pay); code != 202 || pay.Status != "pending" || pay.Method != "mpesa" || pay.VATMZN != 28 {
		t.Fatalf("top up: %d %+v", code, pay)
	}
	h.drain()
	h.call("GET", "/v1/payments/"+pay.ID, tok, nil, &pay)
	if pay.Status != "succeeded" || h.balance(tok) != 200 {
		t.Fatalf("after settlement: %s, balance %d", pay.Status, h.balance(tok))
	}
	cb := func(secret string) int {
		b, _ := json.Marshal(payments.Settlement{ProviderRef: *pay.ProviderRef, Success: true})
		r, _ := http.NewRequest("POST", h.srv.URL+"/v1/payments/callback/mock", bytes.NewReader(b))
		r.Header.Set("X-Faz-Mock-Secret", secret)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := cb("wrong"); code != 401 {
		t.Fatalf("callback with a bad secret: %d", code)
	}
	if code := cb("s3cret"); code != 200 || h.balance(tok) != 200 {
		t.Fatalf("repeated callback must not credit twice: %d, balance %d", code, h.balance(tok))
	}

	// A clear edit is charged, applied and becomes version 2.
	var msg product.MessageResult
	if code := h.call("POST", "/v1/sites/"+id+"/messages", tok, map[string]string{"text": "A entrega custa 50 MT"}, &msg); code != 202 || msg.Charged != 5 {
		t.Fatalf("edit: %d %+v", code, msg)
	}
	h.drain()
	h.call("GET", "/v1/sites/"+id, tok, nil, &site)
	if site.Site.CurrentVersion != 2 || h.balance(tok) != 195 {
		t.Fatalf("after edit: version %d, balance %d", site.Site.CurrentVersion, h.balance(tok))
	}
	if last := site.Messages[len(site.Messages)-1]; !strings.HasPrefix(last.Text, "Pus o custo de entrega") {
		t.Errorf("edit summary %q", last.Text)
	}
	v2, err := h.svc.R2.Get(context.Background(), "sites/"+id+"/v2/index.html")
	if err != nil || !strings.Contains(string(v2.Body), "50 MT em Maputo") {
		t.Fatalf("v2 page: %v", err)
	}

	// Publishing.
	for name, want := range map[string]bool{"www": false, "Mel Niassa!": false, "a--b": false, "mel-niassa": true} {
		var av struct{ Available bool }
		h.call("GET", "/v1/subdomains/"+strings.ReplaceAll(name, " ", "%20"), tok, nil, &av)
		if av.Available != want {
			t.Errorf("availability of %q = %v, want %v", name, av.Available, want)
		}
	}
	var pub struct{ URL string }
	if code := h.call("POST", "/v1/sites/"+id+"/publish", tok, map[string]string{"subdomain": "mel-niassa"}, &pub); code != 200 || pub.URL != "https://mel-niassa.faz.co.mz" {
		t.Fatalf("publish: %d %+v", code, pub)
	}
	live, err := h.svc.R2.Get(context.Background(), "live/mel-niassa/index.html")
	if err != nil || !bytes.Equal(live.Body, v2.Body) {
		t.Fatalf("live page is not version 2: %v", err)
	}

	// Another customer cannot take the name.
	var other product.Session
	h.call("POST", "/v1/auth/signup", "", map[string]string{"name": "Outro", "email": "outro@exemplo.co.mz", "password": "outra-senha-longa"}, &other)
	var av struct{ Available bool }
	h.call("GET", "/v1/subdomains/mel-niassa", other.Token, nil, &av)
	if av.Available {
		t.Error("a published subdomain must not be available to another customer")
	}
	if code := h.call("GET", "/v1/sites/"+id, other.Token, nil, nil); code != 404 {
		t.Errorf("another customer's site: %d", code)
	}

	// A build that keeps failing refunds the charge and reopens the questions.
	var site2 product.SiteView
	h.call("POST", "/v1/sites", other.Token, map[string]string{"prompt": string(brief), "business_name": "Mel Dois", "business_type": "Loja"}, &site2)
	h.call("POST", "/v1/sites/"+site2.Site.ID+"/answers", other.Token, map[string]any{"skip": true}, nil)
	if b := h.balance(other.Token); b != 0 {
		t.Fatalf("second customer after charge: %d", b)
	}
	h.model.fail = true
	h.drain()
	if _, err := h.svc.Store.DB.Exec(`UPDATE jobs SET run_after = now() WHERE status = 'queued'`); err != nil {
		t.Fatal(err)
	}
	h.drain()
	h.model.fail = false
	h.call("GET", "/v1/sites/"+site2.Site.ID, other.Token, nil, &site2)
	if site2.Site.Status != "questions" || h.balance(other.Token) != 19 {
		t.Fatalf("after failed build: status %s, balance %d", site2.Site.Status, h.balance(other.Token))
	}
	if last := site2.Messages[len(site2.Messages)-1]; !strings.Contains(last.Text, "créditos foram devolvidos") {
		t.Errorf("failure message %q", last.Text)
	}

	// Margins are measurable: every stage's spend is recorded.
	cost, err := h.svc.Store.SiteCost(context.Background(), id)
	if err != nil || cost <= 0 {
		t.Fatalf("site cost %v %v", cost, err)
	}
	var ledgerSum int
	h.svc.Store.DB.QueryRow(`SELECT COALESCE(SUM(delta),0) FROM credit_ledger WHERE user_id = $1`, sess.Account.User.ID).Scan(&ledgerSum)
	if ledgerSum != 195 {
		t.Fatalf("ledger sum %d", ledgerSum)
	}
	t.Logf("site %s: 2 versions, estimated model cost $%.3f (fake token counts)", id, cost)
}
