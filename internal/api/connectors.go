package api

import (
	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/ingest"
	"github.com/Aswikinz/Autodit/internal/storage"
	"net/http"
)

func (s *Server) connectorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/sources/connection", s.route("sources.write", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var in ingest.Connection
		if e := decode(w, r, &in); e != nil {
			return e
		}
		out, e := in.Tables(r.Context())
		if e != nil {
			write(w, 400, map[string]string{"error": e.Error()})
			return nil
		}
		write(w, 200, map[string]any{"tables": out, "status": "Connection successful"})
		return nil
	}))
	mux.HandleFunc("POST /api/sources/preview", s.route("sources.write", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var in struct {
			Connection ingest.Connection `json:"connection"`
			Table      ingest.Table      `json:"table"`
		}
		if e := decode(w, r, &in); e != nil {
			return e
		}
		out, e := in.Connection.Preview(r.Context(), in.Table)
		if e != nil {
			write(w, 400, map[string]string{"error": "Table preview failed. Check access, connection settings and table selection."})
			return nil
		}
		write(w, 200, out)
		return nil
	}))
	mux.HandleFunc("POST /api/sources/workbook", s.route("sources.write", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var in struct {
			Data  []byte `json:"data"`
			Sheet string `json:"sheet"`
		}
		if e := decode(w, r, &in); e != nil {
			return e
		}
		out, e := ingest.ReadWorkbook(r.Context(), in.Data, in.Sheet)
		if e != nil {
			write(w, 400, map[string]string{"error": e.Error()})
			return nil
		}
		write(w, 200, out)
		return nil
	}))
}
