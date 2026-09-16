package api

import (
	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/storage"
	"net/http"
)

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspace", s.route("workspace.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		out, e := t.Workspace(r.Context())
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("PUT /api/admin/settings", s.route("admin.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var in struct {
			Settings storage.WorkspaceSettings `json:"settings"`
			Revision int                       `json:"revision"`
		}
		if e := decode(w, r, &in); e != nil {
			return e
		}
		if e := t.SaveWorkspace(r.Context(), in.Settings, in.Revision, i.Subject); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
	mux.HandleFunc("GET /api/admin/users", s.route("admin.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		users, e := t.Users(r.Context())
		if e == nil {
			write(w, 200, map[string]any{"users": users, "roles": storage.RoleDescriptions})
		}
		return e
	}))
	mux.HandleFunc("POST /api/admin/users", s.route("admin.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var in struct {
			storage.User
			Password string `json:"password"`
		}
		if e := decode(w, r, &in); e != nil {
			return e
		}
		if e := t.SaveUser(r.Context(), in.User, in.Password, i.Subject); e != nil {
			return e
		}
		w.WriteHeader(204)
		return nil
	}))
}
