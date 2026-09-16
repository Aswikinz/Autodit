package api

import (
	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/storage"
	"net/http"
	"strconv"
)

func (s *Server) workflowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/workflow", s.route("admin.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Workflow(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("PUT /api/admin/workflow", s.route("admin.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var in struct {
			Workflow storage.Workflow `json:"workflow"`
			Revision int              `json:"revision"`
		}
		if e := decode(w, r, &in); e != nil {
			return e
		}
		if e := t.SaveWorkflow(r.Context(), in.Workflow, in.Revision, i.Subject); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
	mux.HandleFunc("GET /api/cases", s.route("cases.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		out, e := t.Cases(r.Context(), status, page)
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("GET /api/cases/people", s.route("cases.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		users, e := t.Users(r.Context())
		if e != nil {
			return e
		}
		out := []map[string]any{}
		for _, u := range users {
			if u.Enabled {
				out = append(out, map[string]any{"username": u.Username, "display_name": u.DisplayName, "roles": u.Roles})
			}
		}
		write(w, 200, out)
		return nil
	}))
	mux.HandleFunc("GET /api/cases/{key}/events", s.route("cases.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.CaseEvents(r.Context(), r.PathValue("key"))
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("POST /api/cases/{key}/actions", s.route("cases.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var in storage.CaseAction
		if e := decode(w, r, &in); e != nil {
			return e
		}
		if e := t.ActOnCase(r.Context(), r.PathValue("key"), i.Subject, i.Roles, in); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
}
