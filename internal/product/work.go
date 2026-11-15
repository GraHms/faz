package product

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GraHms/faz/internal/assets"
	"github.com/GraHms/faz/internal/jobs"
	"github.com/GraHms/faz/internal/pipeline"
	"github.com/GraHms/faz/internal/spec"
	"github.com/GraHms/faz/internal/store"
)

type sitePayload struct {
	UserID      string `json:"user_id"`
	SiteID      string `json:"site_id"`
	Credits     int    `json:"credits"`
	Instruction string `json:"instruction,omitempty"`
	Structural  bool   `json:"structural,omitempty"`
	Request     string `json:"request,omitempty"`
}

// Handlers returns the job handlers for the worker.
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{JobBuild: s.handleBuild, JobEdit: s.handleEdit, JobSettle: s.handleSettle}
}

// fetchAssets downloads the site's uploads into dir/assets and loads them.
func (s *Service) fetchAssets(ctx context.Context, siteID, dir string) ([]assets.Asset, error) {
	list, err := s.Store.Assets(ctx, siteID)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	adir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(adir, 0o755); err != nil {
		return nil, err
	}
	for _, a := range list {
		obj, err := s.R2.Get(ctx, "sites/"+siteID+"/assets/"+a.Name)
		if err != nil {
			return nil, fmt.Errorf("asset %s: %w", a.Name, err)
		}
		if err := os.WriteFile(filepath.Join(adir, a.Name), obj.Body, 0o644); err != nil {
			return nil, err
		}
	}
	return assets.Load(adir, 10<<20)
}

func versionPrefix(siteID string, n int) string { return fmt.Sprintf("sites/%s/v%d", siteID, n) }

// handleBuild runs the full pipeline for a site's first version.
// progress returns a stage hook that records a job's stage, skipping
// repeats. A failed write only costs the editor a progress tick.
func (s *Service) progress(ctx context.Context, jobID int64) func(string) {
	last := ""
	return func(stage string) {
		if stage == last {
			return
		}
		last = stage
		if err := s.Store.SetJobProgress(ctx, jobID, stage); err != nil {
			s.log().Warn("job progress", "job", jobID, "err", err)
		}
	}
}

func (s *Service) handleBuild(ctx context.Context, j jobs.Job) (any, error) {
	var pl sitePayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return nil, err
	}
	site, err := s.Store.Site(ctx, s.Store.DB, pl.SiteID, "")
	if err != nil {
		return nil, err
	}
	brief, err := s.siteBrief(site)
	if err != nil {
		return nil, err
	}
	var answers map[string]string
	_ = json.Unmarshal(site.Answers, &answers)

	dir, done, err := tempDir("faz-build-")
	if err != nil {
		return nil, err
	}
	defer done()
	files, err := s.fetchAssets(ctx, site.ID, dir)
	if err != nil {
		return nil, err
	}
	out := filepath.Join(dir, "out")
	p := s.pipe(out, composeBrief(site, brief.Questions, answers))
	p.Opt.Assets = files
	p.Opt.OnStage = s.progress(ctx, j.ID)
	rep, err := p.Run(ctx)
	// Spend is recorded even when the build fails.
	s.recordUsage(ctx, s.Store.DB, p.Report(), pl.UserID, &site.ID, &j.ID, "build")
	if err != nil {
		return nil, err
	}

	n := site.CurrentVersion + 1
	prefix := versionPrefix(site.ID, n)
	if _, err := s.R2.PutDir(ctx, prefix, out); err != nil {
		return nil, err
	}
	text := "Pronto. Pede alterações aqui ou carrega em Publicar para escolher o endereço do site."
	if k := len(rep.Placeholders); k > 0 {
		text = fmt.Sprintf("Pronto. Há %d informações marcadas no site para completares; diz-me aqui e eu ponho-as. Quando quiseres, carrega em Publicar.", k)
	}
	if rep.Status == "needs_review" {
		text += " A verificação automática encontrou um problema que a nossa equipa vai rever."
	}
	jobID := j.ID
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if err := s.Store.AddVersion(ctx, tx, store.Version{SiteID: site.ID, Number: n, Kind: "build", R2Prefix: prefix, Status: rep.Status, JobID: &jobID}); err != nil {
			return err
		}
		if err := s.Store.SetCurrentVersion(ctx, tx, site.ID, n, "ready"); err != nil {
			return err
		}
		_, err := s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "assistant", Text: text, JobID: &jobID})
		return err
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": n, "status": rep.Status, "cost_usd": rep.TotalCostUSD}, nil
}

// handleEdit applies one owner request to the current version. A rejected
// edit is not an error: the credits go back and the old page stays.
func (s *Service) handleEdit(ctx context.Context, j jobs.Job) (any, error) {
	var pl sitePayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return nil, err
	}
	site, err := s.Store.Site(ctx, s.Store.DB, pl.SiteID, "")
	if err != nil {
		return nil, err
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
	if err := s.getJSON(ctx, ver.R2Prefix+"/copy.json", &copyDoc); err != nil {
		return nil, err
	}
	if err := s.getJSON(ctx, ver.R2Prefix+"/plan.json", &plan); err != nil {
		return nil, err
	}
	src, err := s.R2.Get(ctx, ver.R2Prefix+"/source.html")
	if err != nil {
		return nil, err
	}

	dir, done, err := tempDir("faz-edit-")
	if err != nil {
		return nil, err
	}
	defer done()
	files, err := s.fetchAssets(ctx, site.ID, dir)
	if err != nil {
		return nil, err
	}
	out := filepath.Join(dir, "out")
	p := s.pipe(out, "")
	p.Opt.OnStage = s.progress(ctx, j.ID)
	p.Opt.Assets = files
	res, err := p.Edit(ctx, pipeline.EditInput{Source: string(src.Body), Brief: &brief, Copy: &copyDoc, Plan: &plan,
		Instruction: pl.Instruction, Structural: pl.Structural})
	s.recordUsage(ctx, s.Store.DB, p.Report(), pl.UserID, &site.ID, &j.ID, "edit")
	if err != nil {
		return nil, err
	}
	jobID := j.ID

	if res.Rejected {
		err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := s.refund(ctx, tx, pl.UserID, pl.Credits, jobID, "Alteração não aplicada"); err != nil {
				return err
			}
			if err := s.Store.SetSiteStatus(ctx, tx, site.ID, "ready"); err != nil {
				return err
			}
			_, err := s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "assistant", JobID: &jobID,
				Text: "Não apliquei esta alteração porque estragava a página. Os créditos foram devolvidos. Experimenta pedir de outra forma."})
			return err
		})
		return map[string]any{"rejected": true, "reason": res.Reason}, err
	}

	// The next edit needs the brief, copy and plan next to the new page.
	for name, v := range map[string]any{"brief.json": brief, "copy.json": copyDoc, "plan.json": plan} {
		b, _ := json.MarshalIndent(v, "", "  ")
		if err := os.WriteFile(filepath.Join(out, name), b, 0o644); err != nil {
			return nil, err
		}
	}
	n := site.CurrentVersion + 1
	prefix := versionPrefix(site.ID, n)
	if _, err := s.R2.PutDir(ctx, prefix, out); err != nil {
		return nil, err
	}
	summary := res.Summary
	if summary == "" {
		summary = "Feito. Actualizei o site com esse pedido."
	}
	if site.Subdomain != nil {
		summary += " Carrega em Publicar para pôr esta versão no ar."
	}
	instruction := pl.Request
	err = s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if err := s.Store.AddVersion(ctx, tx, store.Version{SiteID: site.ID, Number: n, Kind: "edit", R2Prefix: prefix, Status: "ready", Instruction: &instruction, JobID: &jobID}); err != nil {
			return err
		}
		if err := s.Store.SetCurrentVersion(ctx, tx, site.ID, n, "ready"); err != nil {
			return err
		}
		_, err := s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "assistant", Text: summary, JobID: &jobID})
		return err
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": n, "summary": summary, "applied": res.Applied}, nil
}

// OnJobFailed refunds the charge and tells the owner when a build or edit
// fails for the last time.
func (s *Service) OnJobFailed(ctx context.Context, j jobs.Job, cause error) error {
	if j.Kind != JobBuild && j.Kind != JobEdit {
		return nil
	}
	var pl sitePayload
	if err := json.Unmarshal(j.Payload, &pl); err != nil {
		return err
	}
	jobID := j.ID
	return s.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := s.refund(ctx, tx, pl.UserID, pl.Credits, jobID, "Devolução: "+j.Kind+" falhou"); err != nil {
			return err
		}
		site, err := s.Store.LockSite(ctx, tx, pl.SiteID, "")
		if err != nil {
			return err
		}
		status, text := "ready", "Não consegui fazer esta alteração por um erro nosso. Os créditos foram devolvidos; tenta outra vez."
		if j.Kind == JobBuild && site.CurrentVersion == 0 {
			status, text = "questions", "Não consegui construir o site por um erro nosso. Os créditos foram devolvidos; carrega em Construir para tentar outra vez."
		}
		if err := s.Store.SetSiteStatus(ctx, tx, site.ID, status); err != nil {
			return err
		}
		_, err = s.Store.AddMessage(ctx, tx, store.Message{SiteID: site.ID, Role: "assistant", Text: text, JobID: &jobID})
		return err
	})
}
