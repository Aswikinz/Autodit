// Package api serves the tenant-scoped JSON API and bundled browser application.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/exceptions"
	"github.com/Aswikinz/Autodit/internal/ingest"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
)

// Server composes identity, storage and immutable evidence services.
type Server struct {
	Store   *storage.Store
	Auth    *auth.Manager
	Objects objectstore.Files
	WebDir  string
	Now     func() time.Time
}
type endpoint func(http.ResponseWriter, *http.Request, *storage.Tenant, auth.Identity) error

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 40*1024*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return domain.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}

func (s *Server) route(operation string, fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i, ok := s.Auth.Current(r)
		if !ok {
			write(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		if !auth.Allowed(i, operation) {
			write(w, 403, map[string]string{"error": "operation not permitted for this role"})
			return
		}
		if r.Method != "GET" && !s.Auth.Mutation(r, i) {
			write(w, 403, map[string]string{"error": "request verification failed"})
			return
		}
		t, e := s.Store.ForTenant(i.TenantID)
		if e == nil {
			e = fn(w, r, t, i)
		}
		if e != nil {
			status := 500
			message := "operation failed"
			switch {
			case errors.Is(e, domain.ErrInvalid), errors.Is(e, exceptions.ErrTransition):
				status = 400
				message = "invalid request; check the extraction contract or workflow"
			case errors.Is(e, storage.ErrConflict):
				status = 409
				message = e.Error()
			case errors.Is(e, storage.ErrNotFound):
				status = 404
				message = e.Error()
			}
			write(w, status, map[string]string{"error": message})
		}
	}
}

// Handler installs explicit role checks for every application endpoint.
func (s *Server) Handler() http.Handler {
	s.Auth.Users, _ = s.Store.ForTenant(s.Auth.TenantID())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.Store.Ping(ctx) != nil {
			write(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		write(w, 200, map[string]string{"status": "ok", "version": "0.1.0"})
	})
	mux.HandleFunc("GET /api/session", s.Auth.Session)
	mux.HandleFunc("POST /api/login", s.Auth.DemoLogin)
	mux.HandleFunc("POST /api/password-login", s.Auth.PasswordLogin)
	mux.HandleFunc("POST /api/password", s.Auth.PasswordChange)
	s.adminRoutes(mux)
	s.connectorRoutes(mux)
	s.workflowRoutes(mux)
	s.analysisRoutes(mux)
	mux.HandleFunc("POST /api/logout", s.Auth.Logout)
	mux.HandleFunc("GET /auth/login", s.Auth.Login)
	mux.HandleFunc("GET /auth/callback", s.Auth.Callback)
	mux.HandleFunc("GET /api/exceptions", s.route("exceptions.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page == 0 {
			page = 1
		}
		size, _ := strconv.Atoi(q.Get("size"))
		if size == 0 {
			size = 30
		}
		out, e := t.Queue(r.Context(), storage.QueueFilter{State: q.Get("state"), Rule: q.Get("rule"), Severity: q.Get("severity"), Search: q.Get("search"), Sort: q.Get("sort"), Page: page, Size: size})
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("GET /api/exceptions/{key}", s.route("exceptions.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Detail(r.Context(), r.PathValue("key"))
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("POST /api/exceptions/{key}/disposition", s.route("exceptions.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var input struct {
			State    exceptions.State `json:"state"`
			Reason   string           `json:"reason"`
			Revision int              `json:"revision"`
			Expiry   *time.Time       `json:"suppression_until"`
		}
		if e := decode(w, r, &input); e != nil {
			return e
		}
		if e := t.Dispose(r.Context(), r.PathValue("key"), i.Subject, input.Reason, input.State, input.Revision, input.Expiry, s.Now()); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
	mux.HandleFunc("POST /api/exceptions/{key}/comments", s.route("exceptions.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var input struct {
			Body string `json:"body"`
		}
		if e := decode(w, r, &input); e != nil {
			return e
		}
		if e := t.Comment(r.Context(), r.PathValue("key"), i.Subject, input.Body, s.Now()); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
	mux.HandleFunc("POST /api/observations/{id}/replay", s.route("exceptions.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Replay(r.Context(), r.PathValue("id"), s.Objects)
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("GET /api/runs", s.route("runs.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Runs(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("GET /api/runs/{id}", s.route("runs.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Run(r.Context(), r.PathValue("id"))
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("POST /api/runs", s.route("runs.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var input domain.Population
		if e := decode(w, r, &input); e != nil {
			return e
		}
		id, e := t.Submit(r.Context(), input, i.Subject)
		if e == nil {
			write(w, 202, map[string]string{"id": id})
		}
		return e
	}))
	mux.HandleFunc("POST /api/imports", s.route("runs.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var input struct {
			CSV        string            `json:"csv"`
			Mapping    map[string]string `json:"mapping"`
			Population domain.Population `json:"population"`
		}
		if e := decode(w, r, &input); e != nil {
			return e
		}
		result, e := (ingest.File{}).Extract(r.Context(), ingest.SourceConfig{CSV: input.CSV, Mapping: input.Mapping, Population: input.Population}, ingest.Watermark{})
		if e != nil {
			return domain.ErrInvalid
		}
		id, e := t.Submit(r.Context(), result.Population, i.Subject)
		if e == nil {
			write(w, 202, map[string]string{"id": id})
		}
		return e
	}))
	mux.HandleFunc("POST /api/sources/profile", s.route("sources.write", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var input struct {
			CSV string `json:"csv"`
		}
		if e := decode(w, r, &input); e != nil {
			return e
		}
		out, e := (ingest.File{}).Discover(r.Context(), ingest.SourceConfig{CSV: input.CSV})
		if e != nil {
			return domain.ErrInvalid
		}
		write(w, 200, out)
		return nil
	}))
	mux.HandleFunc("GET /api/sources", s.route("sources.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Sources(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("POST /api/sources", s.route("sources.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var input storage.SourceConfig
		if e := decode(w, r, &input); e != nil {
			return e
		}
		if e := t.SaveSource(r.Context(), input, i.Subject); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
	mux.HandleFunc("GET /api/rules", s.route("rules.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Catalog(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("GET /api/parameters", s.route("parameters.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Settings(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("PUT /api/parameters", s.route("parameters.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var input struct {
			Parameters rules.Parameters `json:"parameters"`
			Revision   int              `json:"revision"`
		}
		if e := decode(w, r, &input); e != nil {
			return e
		}
		if e := t.SaveParameters(r.Context(), input.Parameters, input.Revision, i.Subject); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
	mux.HandleFunc("POST /api/rules/simulate", s.route("rules.simulate", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var input struct {
			Model json.RawMessage `json:"model"`
		}
		if e := decode(w, r, &input); e != nil {
			return e
		}
		out, e := storage.Simulate(input.Model)
		if e != nil {
			write(w, 422, map[string]any{"error": "required simulation fixture failed", "evaluations": out})
			return nil
		}
		write(w, 200, map[string]any{"passed": true, "evaluations": out})
		return nil
	}))
	mux.HandleFunc("PUT /api/rules/{id}", s.route("rules.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var input struct {
			Model    json.RawMessage `json:"model"`
			Revision int             `json:"revision"`
			Enabled  bool            `json:"enabled"`
		}
		if e := decode(w, r, &input); e != nil {
			return e
		}
		if e := t.Release(r.Context(), r.PathValue("id"), input.Model, input.Enabled, input.Revision, i.Subject); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
	mux.HandleFunc("GET /api/assurance", s.route("assurance.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Assurance(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("GET /", serveFrontend(s.WebDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		// The bundled decision editor uses WebAssembly for column completion and
		// expression validation. JavaScript eval and external scripts remain blocked.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
