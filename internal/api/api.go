// Package api is Faz's HTTP interface: JSON over HTTPS, authenticated with
// a bearer token. It holds no business rules; it decodes requests, calls
// package product and encodes the answer.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GraHms/faz/internal/product"
	"github.com/GraHms/faz/internal/store"
)

// Server routes requests to the product service.
type Server struct {
	Svc        *product.Service
	AppOrigins []string // allowed CORS origins for the web app
	Log        *slog.Logger
	// AdminToken unlocks /v1/admin, sent as X-Faz-Admin. Empty disables
	// those routes. It is separate from user sessions: accounts have no
	// verified emails, so no account can be trusted to be the operator.
	AdminToken string
}

type ctxKey struct{}

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	open := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, h) }
	authed := func(pattern string, h func(w http.ResponseWriter, r *http.Request, u store.User)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			u, err := s.Svc.Authenticate(r.Context(), strings.TrimSpace(token))
			if err != nil {
				s.fail(w, r, err)
				return
			}
			h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)), u)
		})
	}

	admin := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if s.AdminToken == "" {
				s.fail(w, r, &product.Error{Status: http.StatusNotFound, Code: "not_found", Message: "Não encontrado."})
				return
			}
			got := r.Header.Get("X-Faz-Admin")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.AdminToken)) != 1 {
				s.fail(w, r, &product.Error{Status: http.StatusUnauthorized, Code: "admin_unauthorized", Message: "Chave de administração errada."})
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			h(w, r)
		})
	}

	// Telemetry: tokens, cost per stage and model, and credits, per site
	// and per period. ?from=2026-11-01&to=2026-11-30 (inclusive days, UTC);
	// the default is the last 30 days.
	admin("GET /v1/admin/telemetry", func(w http.ResponseWriter, r *http.Request) {
		from, to, err := period(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		v, err := s.Svc.Telemetry(r.Context(), from, to, limit)
		s.respond(w, r, http.StatusOK, v, err)
	})
	admin("GET /v1/admin/sites/{id}/telemetry", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Svc.SiteTelemetry(r.Context(), r.PathValue("id"))
		s.respond(w, r, http.StatusOK, v, err)
	})

	open("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Svc.Store.DB.PingContext(r.Context()); err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	open("GET /v1/pricing", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Svc.Cfg.Pricing)
	})
	open("POST /v1/auth/signup", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name, Email, Password string }
		if !s.decode(w, r, &in) {
			return
		}
		sess, err := s.Svc.Signup(r.Context(), in.Name, in.Email, in.Password)
		s.respond(w, r, http.StatusCreated, sess, err)
	})
	open("POST /v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Email, Password string }
		if !s.decode(w, r, &in) {
			return
		}
		sess, err := s.Svc.Login(r.Context(), in.Email, in.Password)
		s.respond(w, r, http.StatusOK, sess, err)
	})
	authed("POST /v1/auth/logout", func(w http.ResponseWriter, r *http.Request, u store.User) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		s.respond(w, r, http.StatusOK, map[string]bool{"ok": true}, s.Svc.Logout(r.Context(), token))
	})
	authed("GET /v1/me", func(w http.ResponseWriter, r *http.Request, u store.User) {
		a, err := s.Svc.Me(r.Context(), u)
		s.respond(w, r, http.StatusOK, a, err)
	})
	authed("GET /v1/credits", func(w http.ResponseWriter, r *http.Request, u store.User) {
		c, err := s.Svc.Credits(r.Context(), u)
		s.respond(w, r, http.StatusOK, c, err)
	})

	// JSON, or multipart with the same fields plus up to 8 "file" photos, so
	// the questions are written with the photos in view.
	authed("POST /v1/sites", func(w http.ResponseWriter, r *http.Request, u store.User) {
		var in product.NewSite
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			var ok bool
			if in, ok = s.newSiteForm(w, r); !ok {
				return
			}
		} else {
			var body struct {
				Prompt       string `json:"prompt"`
				BusinessName string `json:"business_name"`
				BusinessType string `json:"business_type"`
			}
			if !s.decode(w, r, &body) {
				return
			}
			in = product.NewSite{Prompt: body.Prompt, BusinessName: body.BusinessName, BusinessType: body.BusinessType}
		}
		v, err := s.Svc.CreateSite(r.Context(), u, in)
		s.respond(w, r, http.StatusCreated, v, err)
	})
	authed("GET /v1/sites", func(w http.ResponseWriter, r *http.Request, u store.User) {
		v, err := s.Svc.ListSites(r.Context(), u)
		s.respond(w, r, http.StatusOK, map[string]any{"sites": v}, err)
	})
	authed("GET /v1/sites/{id}", func(w http.ResponseWriter, r *http.Request, u store.User) {
		v, err := s.Svc.GetSite(r.Context(), u, r.PathValue("id"))
		s.respond(w, r, http.StatusOK, v, err)
	})
	authed("POST /v1/sites/{id}/answers", func(w http.ResponseWriter, r *http.Request, u store.User) {
		var in struct {
			Answers    map[string]string `json:"answers"`
			Skip       bool              `json:"skip"`
			Correction string            `json:"correction"`
		}
		if !s.decode(w, r, &in) {
			return
		}
		id, err := s.Svc.Answer(r.Context(), u, r.PathValue("id"), in.Answers, in.Skip, in.Correction)
		s.respond(w, r, http.StatusAccepted, map[string]any{"job_id": id}, err)
	})
	authed("POST /v1/sites/{id}/messages", func(w http.ResponseWriter, r *http.Request, u store.User) {
		var in struct{ Text string }
		if !s.decode(w, r, &in) {
			return
		}
		res, err := s.Svc.SendMessage(r.Context(), u, r.PathValue("id"), in.Text)
		status := http.StatusOK
		if err == nil && res.JobID != nil {
			status = http.StatusAccepted
		}
		s.respond(w, r, status, res, err)
	})
	authed("GET /v1/sites/{id}/preview", func(w http.ResponseWriter, r *http.Request, u store.User) {
		version, _ := strconv.Atoi(r.URL.Query().Get("version"))
		v, err := s.Svc.Preview(r.Context(), u, r.PathValue("id"), version)
		s.respond(w, r, http.StatusOK, v, err)
	})
	authed("POST /v1/sites/{id}/publish", func(w http.ResponseWriter, r *http.Request, u store.User) {
		var in struct{ Subdomain string }
		if !s.decode(w, r, &in) {
			return
		}
		v, err := s.Svc.Publish(r.Context(), u, r.PathValue("id"), in.Subdomain)
		s.respond(w, r, http.StatusOK, v, err)
	})
	authed("POST /v1/sites/{id}/assets", func(w http.ResponseWriter, r *http.Request, u store.User) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
		f, hdr, err := r.FormFile("file")
		if err != nil {
			s.fail(w, r, &product.Error{Status: http.StatusBadRequest, Code: "missing_file", Message: "Envia a imagem no campo file."})
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		a, err := s.Svc.UploadAsset(r.Context(), u, r.PathValue("id"), hdr.Filename, data)
		s.respond(w, r, http.StatusCreated, a, err)
	})
	authed("GET /v1/subdomains/{name}", func(w http.ResponseWriter, r *http.Request, u store.User) {
		ok, why, err := s.Svc.Availability(r.Context(), r.PathValue("name"), r.URL.Query().Get("site_id"))
		s.respond(w, r, http.StatusOK, map[string]any{"name": strings.ToLower(r.PathValue("name")), "available": ok, "reason": why}, err)
	})
	authed("GET /v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request, u store.User) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			s.fail(w, r, &product.Error{Status: http.StatusNotFound, Code: "not_found", Message: "Não encontrado."})
			return
		}
		j, err := s.Svc.Store.Job(r.Context(), id, u.ID)
		if errors.Is(err, store.ErrNotFound) {
			err = &product.Error{Status: http.StatusNotFound, Code: "not_found", Message: "Não encontrado."}
		}
		s.respond(w, r, http.StatusOK, j, err)
	})

	authed("POST /v1/payments", func(w http.ResponseWriter, r *http.Request, u store.User) {
		var in struct {
			AmountMZN int    `json:"amount_mzn"`
			MSISDN    string `json:"msisdn"`
			NUIT      string `json:"nuit"`
		}
		if !s.decode(w, r, &in) {
			return
		}
		p, err := s.Svc.TopUp(r.Context(), u, in.AmountMZN, in.MSISDN, in.NUIT)
		s.respond(w, r, http.StatusAccepted, p, err)
	})
	authed("GET /v1/payments", func(w http.ResponseWriter, r *http.Request, u store.User) {
		p, err := s.Svc.Payments(r.Context(), u)
		s.respond(w, r, http.StatusOK, map[string]any{"payments": p}, err)
	})
	authed("GET /v1/payments/{id}", func(w http.ResponseWriter, r *http.Request, u store.User) {
		p, err := s.Svc.Payment(r.Context(), u, r.PathValue("id"))
		s.respond(w, r, http.StatusOK, p, err)
	})
	open("POST /v1/payments/callback/{provider}", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
		err := s.Svc.Callback(r.Context(), r.PathValue("provider"), r)
		s.respond(w, r, http.StatusOK, map[string]bool{"ok": true}, err)
	})

	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				s.log().Error("panic", "path", r.URL.Path, "panic", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]string{"code": "internal", "message": "Erro interno."}})
			}
		}()
		if origin := r.Header.Get("Origin"); origin != "" && s.allowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Faz-Admin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
		s.log().Debug("request", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) allowed(origin string) bool {
	for _, o := range s.AppOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

// period reads ?from and ?to as inclusive UTC days and returns [from, to).
func period(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	to := now
	from := now.AddDate(0, 0, -30)
	bad := &product.Error{Status: http.StatusBadRequest, Code: "invalid_period", Message: "Usa datas no formato 2026-11-30."}
	if v := r.URL.Query().Get("from"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return from, to, bad
		}
		from = d
	}
	if v := r.URL.Query().Get("to"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return from, to, bad
		}
		to = d.AddDate(0, 0, 1)
	}
	if !from.Before(to) {
		return from, to, bad
	}
	return from, to, nil
}

// newSiteForm reads a multipart new site request: the text fields and the
// "file" parts. Each photo may be up to 15 MB; the request up to 64 MB.
func (s *Server) newSiteForm(w http.ResponseWriter, r *http.Request) (product.NewSite, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			s.fail(w, r, &product.Error{Status: http.StatusRequestEntityTooLarge, Code: "request_too_large", Message: "As fotos juntas passam de 64 MB. Envia menos agora e junta as outras depois."})
		} else {
			s.fail(w, r, &product.Error{Status: http.StatusBadRequest, Code: "invalid_form", Message: "O pedido não veio no formato esperado."})
		}
		return product.NewSite{}, false
	}
	defer r.MultipartForm.RemoveAll()
	in := product.NewSite{
		Prompt:       r.FormValue("prompt"),
		BusinessName: r.FormValue("business_name"),
		BusinessType: r.FormValue("business_type"),
	}
	files := r.MultipartForm.File["file"]
	if len(files) > product.MaxNewSiteFiles {
		s.fail(w, r, &product.Error{Status: http.StatusBadRequest, Code: "too_many_files", Message: fmt.Sprintf("Envia até %d fotos agora; podes juntar mais depois.", product.MaxNewSiteFiles)})
		return product.NewSite{}, false
	}
	for _, fh := range files {
		if fh.Size > 15<<20 {
			s.fail(w, r, &product.Error{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: "Envia imagens até 15 MB."})
			return product.NewSite{}, false
		}
		f, err := fh.Open()
		if err != nil {
			s.fail(w, r, err)
			return product.NewSite{}, false
		}
		data, err := io.ReadAll(io.LimitReader(f, 15<<20+1))
		f.Close()
		if err != nil {
			s.fail(w, r, err)
			return product.NewSite{}, false
		}
		in.Files = append(in.Files, product.Upload{Name: fh.Filename, Data: data})
	}
	return in, true
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		s.fail(w, r, &product.Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "O pedido não é JSON válido."})
		return false
	}
	return true
}

func (s *Server) respond(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var pe *product.Error
	if errors.As(err, &pe) {
		writeJSON(w, pe.Status, map[string]any{"error": pe})
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	s.log().Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]string{"code": "internal", "message": "Erro interno. Tenta outra vez."}})
}

func (s *Server) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
