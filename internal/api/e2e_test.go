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

const adminToken = "test-admin-token-0123456789"

func (h *harness) admin(path, key string, out any) int {
	req, _ := http.NewRequest("GET", h.srv.URL+path, nil)
	if key != "" {
		req.Header.Set("X-Faz-Admin", key)
	}
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
	srv := httptest.NewServer((&api.Server{Svc: svc, AdminToken: adminToken}).Handler())
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, svc: svc, model: model,
		worker: &jobs.Worker{DB: db.DB, ID: "test", Handlers: svc.Handlers(), OnFailed: svc.OnJobFailed}}
}

func pngUpload(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	img := image.NewRGBA(image.Rect(0, 0, 2400, 1800))
	for y := 0; y < 1800; y++ {
		for x := 0; x < 2400; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 251), uint8(y % 241), uint8((x * y) % 239), 255})
		}
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	part, _ := w.CreateFormFile("file", "Colmeias Lichinga.PNG")
	if err := png.Encode(part, img); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

func (h *harness) multipart(path, token string, body *bytes.Buffer, ctype string, out any) int {
	req, _ := http.NewRequest("POST", h.srv.URL+path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ctype)
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

// intakeCalls returns what each intake call sent the model: its text and
// how many images it carried.
func (h *harness) intakeCalls() (texts []string, images []int) {
	for _, c := range h.model.Client.(*llm.Fake).Calls {
		if c.Stage != "intake" {
			continue
		}
		var text strings.Builder
		n := 0
		for _, p := range c.Parts {
			text.WriteString(p.Text)
			if len(p.Image) > 0 {
				n++
			}
		}
		texts, images = append(texts, text.String()), append(images, n)
	}
	return texts, images
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

	// Questions before the first build, free. The photo comes with the
	// request, so the questions are written with it in view.
	var site product.SiteView
	body, ctype := pngUpload(t, map[string]string{"prompt": string(brief), "business_name": "Cooperativa Apícola de Lichinga", "business_type": "Loja"})
	if code := h.multipart("/v1/sites", tok, body, ctype, &site); code != 201 {
		t.Fatalf("create site: %d", code)
	}
	id := site.Site.ID
	if site.Site.Status != "questions" || len(site.Questions) != 3 || h.balance(tok) != 19 {
		t.Fatalf("site after create: status %s, %d questions, balance %d", site.Site.Status, len(site.Questions), h.balance(tok))
	}
	if !strings.HasPrefix(site.Understood, "Percebi que") {
		t.Errorf("understood %q", site.Understood)
	}
	if len(site.Assets) != 1 || site.Assets[0].Name != "colmeias-lichinga.jpg" || site.Assets[0].Size > 900_000 {
		t.Fatalf("photo sent with the site: %+v", site.Assets)
	}
	if texts, images := h.intakeCalls(); len(texts) != 1 || images[0] != 1 || !strings.Contains(texts[0], "colmeias-lichinga.jpg") {
		t.Fatalf("intake did not see the photo: %d calls, images %v", len(texts), images)
	}

	// A photo added later is scaled down too.
	body, ctype = pngUpload(t, nil)
	var asset store.Asset
	if code := h.multipart("/v1/sites/"+id+"/assets", tok, body, ctype, &asset); code != 201 || asset.Name != "colmeias-lichinga.jpg" || asset.Size > 900_000 {
		t.Fatalf("upload: %d %+v", code, asset)
	}

	// Answers are validated, then the first page is charged and queued.
	if code := h.call("POST", "/v1/sites/"+id+"/answers", tok, map[string]any{"answers": map[string]string{"sentir": "neon"}}, nil); code != 400 {
		t.Fatalf("invalid option: %d", code)
	}
	var queued struct {
		JobID int64 `json:"job_id"`
	}
	correction := "É para revendedores também, não só famílias."
	if code := h.call("POST", "/v1/sites/"+id+"/answers", tok, map[string]any{"answers": map[string]string{"entrega": "50 MT", "fotos": "sim", "sentir": "rotulo"}, "correction": correction}, &queued); code != 202 || queued.JobID == 0 {
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
	var job store.JobView
	if code := h.call("GET", fmt.Sprintf("/v1/jobs/%d", queued.JobID), tok, nil, &job); code != 200 || job.Status != "succeeded" || job.Progress == nil {
		t.Fatalf("build job: %d %+v", code, job)
	}
	// The build read the answers and the correction, next to what it corrects.
	texts, images := h.intakeCalls()
	if build := texts[len(texts)-1]; len(texts) != 2 || images[1] != 1 || !strings.Contains(build, "50 MT") ||
		!strings.Contains(build, correction) || !strings.Contains(build, site.Understood) && !strings.Contains(build, "Percebi que") {
		t.Fatalf("build intake input:\n%s", build)
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
	h.call("POST", "/v1/sites/"+site2.Site.ID+"/answers", other.Token, map[string]any{"answers": map[string]string{"sentir": "rotulo"}, "correction": "Só vendemos em Maputo."}, nil)
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
	// The answers survive the failure, so the owner can retry in one tap.
	var kept map[string]string
	json.Unmarshal(site2.Site.Answers, &kept)
	if kept["sentir"] == "" || kept[product.CorrectionKey] != "Só vendemos em Maputo." || site2.Understood == "" || len(site2.Questions) != 3 {
		t.Fatalf("after failed build: answers %v, understood %q, %d questions", kept, site2.Understood, len(site2.Questions))
	}

	// Margins are measurable: every stage's spend is recorded.
	cost, err := h.svc.Store.SiteCost(context.Background(), id)
	if err != nil || cost <= 0 {
		t.Fatalf("site cost %v %v", cost, err)
	}

	// Telemetry: per site and per period, only with the admin key.
	if code := h.admin("/v1/admin/sites/"+id+"/telemetry", "", nil); code != 401 {
		t.Fatalf("telemetry without the admin key: %d", code)
	}
	if code := h.admin("/v1/admin/sites/"+id+"/telemetry", "wrong", nil); code != 401 {
		t.Fatalf("telemetry with a wrong key: %d", code)
	}
	if code := h.call("GET", "/v1/admin/sites/"+id+"/telemetry", tok, nil, nil); code != 401 {
		t.Fatalf("a customer token is not an admin key: %d", code)
	}
	var tel product.SiteTelemetry
	if code := h.admin("/v1/admin/sites/"+id+"/telemetry", adminToken, &tel); code != 200 {
		t.Fatalf("telemetry: %d", code)
	}
	var rows int
	h.svc.Store.DB.QueryRow(`SELECT COUNT(*) FROM model_usage WHERE site_id = $1`, id).Scan(&rows)
	if tel.Totals.Calls != rows || len(tel.Calls) != rows || tel.OwnerEmail != "ana@exemplo.co.mz" {
		t.Fatalf("telemetry calls %d (%d listed), rows %d, owner %q", tel.Totals.Calls, len(tel.Calls), rows, tel.OwnerEmail)
	}
	if d := tel.Totals.CostUSD - cost; d > 1e-6 || d < -1e-6 {
		t.Fatalf("telemetry cost %f, ledger of usage %f", tel.Totals.CostUSD, cost)
	}
	seen := map[string]product.StageTelemetry{}
	var stageSum float64
	for _, st := range tel.ByStage {
		seen[st.Operation+"/"+st.Stage] = st
		stageSum += st.CostUSD
		if st.Model == "" || st.Effort == "" || st.InputTokens <= 0 || st.OutputTokens <= 0 {
			t.Errorf("stage without detail: %+v", st)
		}
	}
	for _, want := range []string{"questions/intake", "build/intake", "build/content", "build/plan", "build/critique", "build/build", "classify/classify", "edit/edit"} {
		if _, ok := seen[want]; !ok {
			t.Errorf("telemetry is missing %s; has %v", want, seen)
		}
	}
	if m := seen["questions/intake"].Model; m != pipeline.Sonnet {
		t.Errorf("intake ran on %s", m)
	}
	if m := seen["build/plan"].Model; m != pipeline.Opus {
		t.Errorf("plan ran on %s", m)
	}
	if d := stageSum - tel.Totals.CostUSD; d > 1e-6 || d < -1e-6 {
		t.Errorf("stages add up to %f, total %f", stageSum, tel.Totals.CostUSD)
	}
	if tel.Money.CreditsCharged < 19 || tel.Money.CostMZN != float64(int64(tel.Totals.CostUSD*product.DefaultUSDToMZN*100+0.5))/100 {
		t.Errorf("money %+v", tel.Money)
	}
	if tel.Jobs.Builds != 1 || tel.Jobs.Edits < 1 {
		t.Errorf("jobs %+v", tel.Jobs)
	}
	var ov product.Overview
	if code := h.admin("/v1/admin/telemetry", adminToken, &ov); code != 200 || len(ov.Sites) == 0 || ov.Totals.CostUSD < tel.Totals.CostUSD {
		t.Fatalf("overview: %d %+v", code, ov.Totals)
	}
	if code := h.admin("/v1/admin/telemetry?from=2026-13-01", adminToken, nil); code != 400 {
		t.Errorf("bad period: %d", code)
	}
	rec := httptest.NewRecorder()
	(&api.Server{Svc: h.svc}).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/v1/admin/telemetry", nil))
	if rec.Code != 404 {
		t.Errorf("admin routes must not exist without FAZ_ADMIN_TOKEN: %d", rec.Code)
	}
	var ledgerSum int
	h.svc.Store.DB.QueryRow(`SELECT COALESCE(SUM(delta),0) FROM credit_ledger WHERE user_id = $1`, sess.Account.User.ID).Scan(&ledgerSum)
	if ledgerSum != 195 {
		t.Fatalf("ledger sum %d", ledgerSum)
	}
	t.Logf("site %s: 2 versions, estimated model cost $%.3f (fake token counts)", id, cost)
}
