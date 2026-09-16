package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Aswikinz/Autodit/internal/analysis"
	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/ingest"
	"github.com/Aswikinz/Autodit/internal/storage"
)

// Analysis errors are deliberately readable: inputs are authored by the current user.
func analysisError(w http.ResponseWriter, e error) error {
	write(w, 400, map[string]string{"error": e.Error()})
	return nil
}

func (s *Server) analysisRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/analyses", s.route("analytics.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			var e error
			page, e = strconv.Atoi(raw)
			if e != nil {
				return domain.ErrInvalid
			}
		}
		out, e := t.Analyses(r.Context(), page)
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("GET /api/analyses/{id}", s.route("analytics.read", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, _ auth.Identity) error {
		revision := 0
		if raw := r.URL.Query().Get("revision"); raw != "" {
			var e error
			revision, e = strconv.Atoi(raw)
			if e != nil {
				return domain.ErrInvalid
			}
		}
		out, e := t.Analysis(r.Context(), r.PathValue("id"), revision)
		if e == nil {
			write(w, 200, out)
		}
		return e
	}))
	mux.HandleFunc("POST /api/analyses", s.route("analytics.write", func(w http.ResponseWriter, r *http.Request, t *storage.Tenant, i auth.Identity) error {
		var in storage.SavedAnalysis
		if e := decode(w, r, &in); e != nil {
			return e
		}
		if e := analysis.ValidateDataset(in.Dataset); e != nil {
			return analysisError(w, e)
		}
		if _, e := analysis.SelectColumns(in.Dataset, in.SelectedColumns); e != nil {
			return analysisError(w, e)
		}
		if e := analysis.ValidateModel(in.Model); e != nil {
			return analysisError(w, e)
		}
		if e := analysis.ValidateSyntax(r.Context(), in.Model); e != nil {
			return analysisError(w, e)
		}
		out, e := t.SaveAnalysis(r.Context(), in, i.Subject)
		if e != nil {
			return e
		}
		write(w, 200, out)
		return nil
	}))
	mux.HandleFunc("POST /api/analyses/connection", s.route("analytics.test", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var in ingest.Connection
		if e := decode(w, r, &in); e != nil {
			return e
		}
		tables, e := in.Tables(r.Context())
		if e != nil {
			return analysisError(w, e)
		}
		write(w, 200, map[string]any{"tables": tables, "status": "Connection successful"})
		return nil
	}))
	mux.HandleFunc("POST /api/analyses/preview", s.route("analytics.test", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var in struct {
			Format     string            `json:"format"`
			Name       string            `json:"name"`
			Data       string            `json:"data"`
			Sheet      string            `json:"sheet"`
			Connection ingest.Connection `json:"connection"`
			Table      ingest.Table      `json:"table"`
		}
		if e := decode(w, r, &in); e != nil {
			return e
		}
		if len(in.Name) > 250 {
			return domain.ErrInvalid
		}
		var dataset analysis.Dataset
		var e error
		switch in.Format {
		case "csv":
			dataset, e = analysis.ParseCSV(r.Context(), in.Data)
		case "json":
			dataset, e = analysis.ParseJSON(r.Context(), []byte(in.Data))
		case "xlsx":
			var data []byte
			data, e = base64.StdEncoding.DecodeString(in.Data)
			if e == nil {
				var book ingest.Workbook
				book, e = ingest.ReadWorkbook(r.Context(), data, in.Sheet)
				if e == nil && in.Sheet == "" {
					write(w, 200, map[string]any{"sheets": book.Sheets})
					return nil
				}
				if e == nil {
					dataset, e = analysis.ParseCSV(r.Context(), book.CSV)
				}
			}
		case "database":
			var preview ingest.Preview
			preview, e = in.Connection.Read(r.Context(), in.Table)
			if e == nil {
				dataset, e = analysis.FromRows(r.Context(), preview.Columns, preview.Sample)
			}
		default:
			return domain.ErrInvalid
		}
		if e != nil {
			return analysisError(w, e)
		}
		write(w, 200, struct {
			analysis.Dataset
			Name   string `json:"name"`
			Origin string `json:"origin"`
		}{dataset, strings.TrimSpace(in.Name), in.Format})
		return nil
	}))
	mux.HandleFunc("POST /api/analyses/test", s.route("analytics.test", func(w http.ResponseWriter, r *http.Request, _ *storage.Tenant, _ auth.Identity) error {
		var in struct {
			Dataset         analysis.Dataset  `json:"dataset"`
			Model           json.RawMessage   `json:"model"`
			SelectedColumns []analysis.Column `json:"selected_columns"`
		}
		if e := decode(w, r, &in); e != nil {
			return e
		}
		dataset, e := analysis.SelectColumns(in.Dataset, in.SelectedColumns)
		if e != nil {
			return analysisError(w, e)
		}
		report, e := analysis.Evaluate(r.Context(), in.Model, dataset)
		if e != nil {
			return analysisError(w, e)
		}
		write(w, 200, report)
		return nil
	}))
}
