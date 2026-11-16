package product

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GraHms/faz/internal/assets"
	"github.com/GraHms/faz/internal/jobs"
	"github.com/GraHms/faz/internal/payments"
	"github.com/GraHms/faz/internal/pipeline"
	"github.com/GraHms/faz/internal/spec"
	"github.com/GraHms/faz/internal/store"
)

// Job kinds.
const (
	JobBuild  = "site.build"
	JobEdit   = "site.edit"
	JobSettle = "payment.settle"
)

// SiteView is a site with what the editor screen needs.
type SiteView struct {
	Site      store.Site      `json:"site"`
	Questions []spec.Question `json:"questions,omitempty"`
	// Understood is what the intake understood, shown with the questions so
	// the owner can correct it before paying for the build.
	Understood string          `json:"understood,omitempty"`
	Messages   []store.Message `json:"messages"`
	Versions   []store.Version `json:"versions"`
	Assets     []store.Asset   `json:"assets"`
	LiveURL    string          `json:"live_url,omitempty"`
	Busy       bool            `json:"busy"`
}

func (s *Service) siteBrief(site store.Site) (*spec.Brief, error) {
	if len(site.Brief) == 0 {
		return nil, fmt.Errorf("site %s has no brief", site.ID)
	}
	var b spec.Brief
	return &b, json.Unmarshal(site.Brief, &b)
}

// CorrectionKey holds, among a site's stored answers, the owner's
// correction to what the intake understood. Question ids never start with _.
const CorrectionKey = "_correction"

// composeBrief writes the text the intake stage reads: the business, the
// owner's request, their answers to the questions, and their correction to
// what the first intake understood. brief is nil before the first intake.
func composeBrief(site store.Site, brief *spec.Brief, answers map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Negócio: %s\nTipo de negócio: %s\n\nPedido do cliente:\n%s\n", site.BusinessName, site.BusinessType, site.Prompt)
	if brief == nil {
		return b.String()
	}
	wrote := false
	for _, q := range brief.Questions {
		if a := answers[q.ID]; a != "" {
			if !wrote {
				b.WriteString("\nRespostas do cliente às perguntas:\n")
				wrote = true
			}
			fmt.Fprintf(&b, "- %s %s\n", q.Text, a)
		}
	}
	if c := answers[CorrectionKey]; c != "" {
		fmt.Fprintf(&b, "\nO que dissemos ao cliente que percebemos:\n%s\n\nA correcção do cliente, que manda sobre o pedido e o resumo:\n%s\n", brief.Understood, c)
	}
	return b.String()
}

// Upload is a file sent with a request, before it is checked.
type Upload struct {
	Name string
	Data []byte
}

// NewSite is what the owner sends to start a site. Photos come with the
// request so the questions can take them into account.
type NewSite struct {
	Prompt       string
	BusinessName string
	BusinessType string
	Files        []Upload
}

// MaxNewSiteFiles caps the photos sent with a new site; more can be added later.
const MaxNewSiteFiles = 8

// CreateSite reads the owner's request and its photos and returns the
// clarifying questions with what was understood. Nothing is charged: this is
// one Sonnet call.
func (s *Service) CreateSite(ctx context.Context, u store.User, in NewSite) (*SiteView, error) {
	prompt, businessName, businessType := clean(in.Prompt), clean(in.BusinessName), clean(in.BusinessType)
	switch {
	case len([]rune(prompt)) < 10 || len([]rune(prompt)) > 4000:
		return nil, badRequest("invalid_prompt", "Descreve o site em 10 a 4000 caracteres.")
	case businessName == "" || len([]rune(businessName)) > 80:
		return nil, badRequest("invalid_business_name", "Escreve o nome do negócio.")
	case len([]rune(businessType)) > 40:
		return nil, badRequest("invalid_business_type", "O tipo de negócio é demasiado longo.")
	}
	if len(in.Files) > MaxNewSiteFiles {
		return nil, badRequest("too_many_files", fmt.Sprintf("Envia até %d fotos agora; podes juntar mais depois.", MaxNewSiteFiles))
	}
	// Photos are checked and scaled before the intake, so it sees what the
	// site will use and does not ask for photos it already has.
	var photos []assets.Asset
	seen := map[string]int{}
	for _, f := range in.Files {
		if len(f.Data) == 0 || len(f.Data) > 15<<20 {
			return nil, &Error{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: "Envia imagens até 15 MB."}
		}
		name, body, mediaType, err := prepareImage(f.Name, f.Data)
		if err != nil {
			return nil, badRequest("unsupported_image", err.Error())
		}
		a := assets.Asset{Name: name, MediaType: mediaType, Size: int64(len(body)), Data: body}
		if i, ok := seen[name]; ok {
			photos[i] = a
			continue
		}
		seen[name] = len(photos)
		photos = append(photos, a)
	}
	draft := store.Site{UserID: u.ID, BusinessName: businessName, BusinessType: businessType, Prompt: prompt, Status: "questions"}

	dir, done, err := tempDir("faz-intake-")
	if err != nil {
		return nil, err
	}
	defer done()
	p := s.pipe(dir, composeBrief(draft, nil, nil))
	p.Opt.Assets = photos
	brief, err := p.Intake(ctx)
	if err != nil {
		return nil, fmt.Errorf("questions: %w", err)
	}
	var site store.Site
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if site, err = s.Store.CreateSite(ctx, tx, draft); err != nil {
			return err
		}
		if err := s.Store.SetSiteBrief(ctx, tx, site.ID, brief); err != nil {
			return err
		}
		s.recordUsage(ctx, tx, p.Report(), u.ID, &site.ID, nil, "questions")
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The site exists now, so a photo that fails to store is logged rather
	// than failing the request; the view lists the photos that made it.
	for _, a := range photos {
		if err := s.storeAsset(ctx, site.ID, a.Name, a.Data, a.MediaType); err != nil {
			s.log().Error("store photo sent with new site", "site", site.ID, "photo", a.Name, "err", err)
		}
	}
	return s.GetSite(ctx, u, site.ID)
}

// GetSite returns the editor view of a site.
func (s *Service) GetSite(ctx context.Context, u store.User, id string) (*SiteView, error) {
	site, err := s.Store.Site(ctx, s.Store.DB, id, u.ID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	v := &SiteView{Site: site, Messages: []store.Message{}, Versions: []store.Version{}, Assets: []store.Asset{}}
	if b, err := s.siteBrief(site); err == nil && site.Status == "questions" {
		v.Questions, v.Understood = b.Questions, b.Understood
	}
	if m, err := s.Store.Messages(ctx, id, 100); err == nil && m != nil {
		v.Messages = m
	}
	if vs, err := s.Store.Versions(ctx, id); err == nil && vs != nil {
		v.Versions = vs
	}
	if as, err := s.Store.Assets(ctx, id); err == nil && as != nil {
		v.Assets = as
	}
	if site.Subdomain != nil {
		v.LiveURL = s.liveURL(*site.Subdomain)
	}
	v.Busy, _ = s.Store.ActiveJob(ctx, s.Store.DB, id)
	return v, nil
}

// ListSites returns the customer's sites, newest first.
func (s *Service) ListSites(ctx context.Context, u store.User) ([]store.Site, error) {
	sites, err := s.Store.SitesByUser(ctx, u.ID)
	if sites == nil {
		sites = []store.Site{}
	}
	return sites, err
}

// Answer records the answers (or a skip) and the owner's correction to what
// was understood, charges the first page and queues the build. Answers are
// keyed by question id; multi choice answers are comma separated values.
// They stay stored if the build fails, so the owner can retry without
// answering again.
func (s *Service) Answer(ctx context.Context, u store.User, siteID string, answers map[string]string, skip bool, correction string) (int64, error) {
	correction = clean(correction)
	if len([]rune(correction)) > 1000 {
		return 0, badRequest("correction_too_long", "Escreve a correcção em até 1000 caracteres.")
	}
	var jobID int64
	err := s.Store.Tx(ctx, func(tx *sql.Tx) error {
		site, err := s.Store.LockSite(ctx, tx, siteID, u.ID)
		if err != nil {
			return mapStoreErr(err)
		}
		if site.Status != "questions" {
			return conflict("not_waiting_for_answers", "Este site já está a ser construído.")
		}
		brief, err := s.siteBrief(site)
		if err != nil {
			return err
		}
		labeled := map[string]string{}
		if !skip {
			if labeled, err = labelAnswers(brief.Questions, answers); err != nil {
				return err
			}
		}
		if correction != "" {
			labeled[CorrectionKey] = correction
		}
		if err := s.Store.SetSiteAnswers(ctx, tx, site.ID, labeled); err != nil {
			return err
		}
		price := s.Cfg.Pricing.FirstPage
		if jobID, err = jobs.Enqueue(ctx, tx, JobBuild, map[string]any{"user_id": u.ID, "site_id": site.ID, "credits": price}, jobs.Options{MaxAttempts: 2}); err != nil {
			return err
		}
		if err := s.charge(ctx, tx, u.ID, price, jobID, "Primeiro site: "+site.BusinessName); err != nil {
			return err
		}
		if err := s.Store.SetSiteStatus(ctx, tx, site.ID, "building"); err != nil {
			return err
		}
		_, err = s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "assistant", JobID: &jobID, CreditsCharged: price,
			Text: "Obrigado. Estou a construir o teu site; demora um ou dois minutos."})
		return err
	})
	return jobID, err
}

// labelAnswers validates answers against the questions and turns choice
// values into their labels, which read better in the brief.
func labelAnswers(qs []spec.Question, answers map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, q := range qs {
		a := clean(answers[q.ID])
		if a == "" {
			continue
		}
		if len([]rune(a)) > 500 {
			return nil, badRequest("answer_too_long", "Uma resposta é demasiado longa.")
		}
		switch q.Kind {
		case "phone":
			n, _, err := payments.ParseMSISDN(a)
			if err != nil {
				return nil, badRequest("invalid_phone", "Escreve um número de telemóvel moçambicano, por exemplo 84 123 4567.")
			}
			out[q.ID] = "+" + n
		case "choice", "multi":
			var labels []string
			for _, v := range strings.Split(a, ",") {
				v = clean(v)
				found := false
				for _, o := range q.Options {
					if o.Value == v {
						labels, found = append(labels, o.Label), true
					}
				}
				if !found {
					return nil, badRequest("invalid_option", fmt.Sprintf("Opção desconhecida para %q.", q.Text))
				}
			}
			if q.Kind == "choice" && len(labels) > 1 {
				return nil, badRequest("one_option", fmt.Sprintf("Escolhe só uma opção para %q.", q.Text))
			}
			out[q.ID] = strings.Join(labels, ", ")
		default:
			out[q.ID] = a
		}
	}
	return out, nil
}

// MessageResult is what the chat shows after the owner sends a message.
type MessageResult struct {
	Messages []store.Message `json:"messages"`
	JobID    *int64          `json:"job_id,omitempty"`
	Charged  int             `json:"credits_charged"`
	Kind     string          `json:"kind"`
}

// SendMessage classifies a chat message. Questions and vague requests are
// answered for free; clear edits are charged and queued.
func (s *Service) SendMessage(ctx context.Context, u store.User, siteID, text string) (*MessageResult, error) {
	text = clean(text)
	if text == "" || len([]rune(text)) > 2000 {
		return nil, badRequest("invalid_message", "Escreve o pedido em até 2000 caracteres.")
	}
	site, err := s.Store.Site(ctx, s.Store.DB, siteID, u.ID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if site.CurrentVersion == 0 || site.Status != "ready" {
		return nil, conflict("site_busy", "Espera que o site termine a alteração em curso.")
	}
	ver, err := s.Store.Version(ctx, site.ID, site.CurrentVersion)
	if err != nil {
		return nil, err
	}
	var brief spec.Brief
	var copyDoc spec.Copy
	var plan spec.Plan
	if err := s.getJSON(ctx, ver.R2Prefix+"/brief.json", &brief); err != nil {
		return nil, err
	}
	_ = s.getJSON(ctx, ver.R2Prefix+"/copy.json", &copyDoc)
	_ = s.getJSON(ctx, ver.R2Prefix+"/plan.json", &plan)
	history, _ := s.Store.Messages(ctx, site.ID, 6)
	var turns []pipeline.ChatTurn
	for _, m := range history {
		turns = append(turns, pipeline.ChatTurn{Role: m.Role, Text: m.Text})
	}
	p := s.pipe("", "")
	c, err := p.Classify(ctx, pipeline.ClassifyInput{Brief: &brief, Copy: &copyDoc, Plan: &plan, History: turns, Message: text,
		PriceList: s.Cfg.Pricing.priceList(s.Cfg.SitesDomain)})
	if err != nil {
		return nil, fmt.Errorf("classify: %w", err)
	}
	// Classifying is never charged, so its cost is recorded whatever happens next.
	s.recordUsage(ctx, s.Store.DB, p.Report(), u.ID, &site.ID, nil, "classify")
	res := &MessageResult{Kind: c.Kind}
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if !c.Chargeable() {
			um, err := s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "user", Text: text})
			if err != nil {
				return err
			}
			am, err := s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "assistant", Text: c.Reply, Options: c.Options})
			res.Messages = []store.Message{um, am}
			return err
		}
		locked, err := s.Store.LockSite(ctx, tx, site.ID, u.ID)
		if err != nil {
			return err
		}
		if busy, err := s.Store.ActiveJob(ctx, tx, site.ID); err != nil || busy || locked.Status != "ready" {
			if err != nil {
				return err
			}
			return conflict("site_busy", "Espera que o site termine a alteração em curso.")
		}
		price := s.Cfg.Pricing.Edit
		if c.Kind == "structural" {
			price = s.Cfg.Pricing.Structural
		}
		jobID, err := jobs.Enqueue(ctx, tx, JobEdit, map[string]any{"user_id": u.ID, "site_id": site.ID, "credits": price,
			"instruction": c.Instruction, "structural": c.Kind == "structural", "request": text}, jobs.Options{MaxAttempts: 1})
		if err != nil {
			return err
		}
		if err := s.charge(ctx, tx, u.ID, price, jobID, "Alteração: "+truncate(text, 80)); err != nil {
			return err
		}
		if err := s.Store.SetSiteStatus(ctx, tx, site.ID, "editing"); err != nil {
			return err
		}
		um, err := s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "user", Text: text, JobID: &jobID, CreditsCharged: price})
		res.Messages, res.JobID, res.Charged = []store.Message{um}, &jobID, price
		return err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (s *Service) getJSON(ctx context.Context, key string, v any) error {
	obj, err := s.R2.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(obj.Body, v)
}

// ---- preview and publishing ----

// Preview returns a short lived URL for a version's page on R2's own
// origin. Generated pages are never served from the app's origin.
func (s *Service) Preview(ctx context.Context, u store.User, siteID string, version int) (map[string]any, error) {
	site, err := s.Store.Site(ctx, s.Store.DB, siteID, u.ID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if version == 0 {
		version = site.CurrentVersion
	}
	if version == 0 {
		return nil, conflict("not_built", "O site ainda não foi construído.")
	}
	ver, err := s.Store.Version(ctx, site.ID, version)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	ttl := s.Cfg.PreviewTTL
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	url, err := s.R2.PresignGet(ver.R2Prefix+"/index.html", ttl)
	if err != nil {
		return nil, err
	}
	return map[string]any{"url": url, "version": version, "expires_at": time.Now().Add(ttl).UTC()}, nil
}

var subdomainRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,38}[a-z0-9])$`)

var reservedSubdomains = map[string]bool{
	"www": true, "api": true, "app": true, "admin": true, "mail": true, "email": true, "ftp": true, "faz": true,
	"static": true, "assets": true, "cdn": true, "status": true, "help": true, "ajuda": true, "suporte": true,
	"support": true, "blog": true, "docs": true, "dev": true, "staging": true, "test": true, "preview": true,
	"login": true, "entrar": true, "conta": true, "pagamentos": true, "billing": true, "mpesa": true, "emola": true,
}

// Availability says whether a subdomain can be used, and why not.
func (s *Service) Availability(ctx context.Context, name, forSiteID string) (bool, string, error) {
	name = strings.ToLower(clean(name))
	switch {
	case !subdomainRe.MatchString(name):
		return false, "Usa 3 a 40 letras, números e hífens, sem hífen no início ou no fim.", nil
	case strings.Contains(name, "--"):
		return false, "Não uses dois hífens seguidos.", nil
	case reservedSubdomains[name]:
		return false, "Este endereço está reservado.", nil
	}
	owner, err := s.Store.SubdomainOwner(ctx, name)
	if errors.Is(err, store.ErrNotFound) || (err == nil && owner == forSiteID) {
		return true, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return false, "Este endereço já tem dono.", nil
}

func (s *Service) liveURL(sub string) string { return "https://" + sub + "." + s.Cfg.SitesDomain }

// Publish puts the site's current version live at <subdomain>.<domain>.
// The Worker serves live/<subdomain>/index.html from R2.
func (s *Service) Publish(ctx context.Context, u store.User, siteID, subdomain string) (map[string]any, error) {
	subdomain = strings.ToLower(clean(subdomain))
	ok, why, err := s.Availability(ctx, subdomain, siteID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, conflict("subdomain_unavailable", why)
	}
	var old string
	var version int
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		site, err := s.Store.LockSite(ctx, tx, siteID, u.ID)
		if err != nil {
			return mapStoreErr(err)
		}
		if site.CurrentVersion == 0 {
			return conflict("not_built", "O site ainda não foi construído.")
		}
		if site.Subdomain != nil && *site.Subdomain != subdomain {
			old = *site.Subdomain
		}
		version = site.CurrentVersion
		if err := s.Store.Publish(ctx, tx, site.ID, subdomain, version); err != nil {
			if errors.Is(err, store.ErrTaken) {
				return conflict("subdomain_unavailable", "Este endereço já tem dono.")
			}
			return err
		}
		ver, err := s.Store.Version(ctx, site.ID, version)
		if err != nil {
			return err
		}
		// Written before commit: if the copy fails, the name is not taken.
		if err := s.R2.Copy(ctx, ver.R2Prefix+"/index.html", "live/"+subdomain+"/index.html", "public, max-age=60"); err != nil {
			return err
		}
		meta := mustJSON(map[string]any{"site_id": site.ID, "version": version, "published_at": time.Now().UTC()})
		return s.R2.Put(ctx, "live/"+subdomain+"/site.json", meta, "application/json", "no-store")
	})
	if err != nil {
		return nil, err
	}
	if old != "" {
		_ = s.R2.Delete(ctx, "live/"+old+"/index.html")
		_ = s.R2.Delete(ctx, "live/"+old+"/site.json")
	}
	return map[string]any{"url": s.liveURL(subdomain), "subdomain": subdomain, "version": version}, nil
}

// ---- assets ----

// UploadAsset stores a client photo or logo for the site. Large photos are
// scaled down so pages stay a single, light file.
func (s *Service) UploadAsset(ctx context.Context, u store.User, siteID, filename string, data []byte) (*store.Asset, error) {
	if _, err := s.Store.Site(ctx, s.Store.DB, siteID, u.ID); err != nil {
		return nil, mapStoreErr(err)
	}
	if len(data) == 0 || len(data) > 15<<20 {
		return nil, &Error{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: "Envia imagens até 15 MB."}
	}
	name, body, mediaType, err := prepareImage(filename, data)
	if err != nil {
		return nil, badRequest("unsupported_image", err.Error())
	}
	if err := s.storeAsset(ctx, siteID, name, body, mediaType); err != nil {
		return nil, err
	}
	return &store.Asset{SiteID: siteID, Name: name, MediaType: mediaType, Size: int64(len(body))}, nil
}

// storeAsset writes a prepared photo to R2 and lists it on the site.
func (s *Service) storeAsset(ctx context.Context, siteID, name string, body []byte, mediaType string) error {
	if err := s.R2.Put(ctx, "sites/"+siteID+"/assets/"+name, body, mediaType, ""); err != nil {
		return err
	}
	return s.Store.UpsertAsset(ctx, store.Asset{SiteID: siteID, Name: name, MediaType: mediaType, Size: int64(len(body))})
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
