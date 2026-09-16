package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Aswikinz/Autodit/internal/analysis"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestSavedAnalysesIntegration(t *testing.T) {
	if os.Getenv("TEST_OWNER_DATABASE_URL") == "" {
		t.Skip("integration database required")
	}
	ctx := context.Background()
	if e := Migrate(ctx, os.Getenv("TEST_OWNER_DATABASE_URL")); e != nil {
		t.Fatal(e)
	}
	store, e := Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	id, otherID := uuid.NewString(), uuid.NewString()
	for _, tenantID := range []string{id, otherID} {
		if e = store.BootstrapLocal(ctx, tenantID, "Analysis", nil); e != nil {
			t.Fatal(e)
		}
	}
	tenant, _ := store.ForTenant(id)
	other, _ := store.ForTenant(otherID)
	dataset, e := analysis.ParseCSV(ctx, "reference,amount\n001,125.50\n002,50\n")
	if e != nil {
		t.Fatal(e)
	}
	in := SavedAnalysis{Name: "Payments", Dataset: dataset, SelectedColumns: dataset.Columns, Model: json.RawMessage(`{"nodes":[{"id":"in","name":"Input","type":"inputNode"},{"id":"out","name":"Output","type":"outputNode"}],"edges":[{"id":"edge","sourceId":"in","targetId":"out"}]}`), Origin: "csv"}
	for _, edit := range []func(*SavedAnalysis){
		func(v *SavedAnalysis) { v.Name = " " }, func(v *SavedAnalysis) { v.Name = strings.Repeat("x", 121) }, func(v *SavedAnalysis) { v.Origin = strings.Repeat("x", 251) },
		func(v *SavedAnalysis) { v.Revision = -1 }, func(v *SavedAnalysis) { v.ID = "bad" }, func(v *SavedAnalysis) { v.Revision = 1 },
		func(v *SavedAnalysis) { v.Dataset = analysis.Dataset{} }, func(v *SavedAnalysis) { v.SelectedColumns = nil }, func(v *SavedAnalysis) { v.Model = json.RawMessage(`{}`) },
	} {
		bad := in
		edit(&bad)
		if _, e = tenant.SaveAnalysis(ctx, bad, "writer"); e == nil {
			t.Fatal("invalid analysis accepted")
		}
	}
	if _, e = tenant.SaveAnalysis(ctx, in, ""); e == nil {
		t.Fatal("missing actor accepted")
	}
	first, e := tenant.SaveAnalysis(ctx, in, "writer")
	if e != nil {
		t.Fatal(e)
	}
	if first.Revision != 1 || first.ID == "" || first.UpdatedAt.IsZero() {
		t.Fatal("missing version metadata")
	}
	latest, e := tenant.Analysis(ctx, first.ID, 0)
	if e != nil || latest.Name != "Payments" || latest.Dataset.Rows[0]["reference"] != "001" {
		t.Fatal("saved input changed", e)
	}
	if _, e = other.Analysis(ctx, first.ID, 0); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross-tenant read", e)
	}
	if _, e = tenant.Analysis(ctx, "invalid", 0); e == nil {
		t.Fatal("invalid UUID")
	}
	if _, e = tenant.Analysis(ctx, first.ID, -1); e == nil {
		t.Fatal("negative revision")
	}
	if _, e = tenant.Analysis(ctx, first.ID, 9); !errors.Is(e, ErrNotFound) {
		t.Fatal("missing revision", e)
	}
	first.Name = "Updated payments"
	second, e := tenant.SaveAnalysis(ctx, first, "writer")
	if e != nil || second.Revision != 2 {
		t.Fatal("new revision", e)
	}
	if _, e = tenant.SaveAnalysis(ctx, first, "writer"); !errors.Is(e, ErrConflict) {
		t.Fatal("stale save accepted", e)
	}
	historical, e := tenant.Analysis(ctx, first.ID, 1)
	if e != nil || historical.Name != "Payments" {
		t.Fatal("old version changed", e)
	}
	list, e := tenant.Analyses(ctx, 1)
	if e != nil || len(list) != 1 || !strings.Contains(string(list[0]), "Updated payments") {
		t.Fatal("latest listing", e)
	}
	list, e = tenant.Analyses(ctx, 2)
	if e != nil || len(list) != 0 {
		t.Fatal("pagination", e)
	}
	list, e = other.Analyses(ctx, 1)
	if e != nil || len(list) != 0 {
		t.Fatal("tenant listing leaked", e)
	}
	if _, e = tenant.Analyses(ctx, 0); e == nil {
		t.Fatal("bad page accepted")
	}
	e = tenant.Tx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `update analysis_version set name='tampered' where tenant_id=$1 and id=$2`, id, first.ID)
		return e
	})
	if e == nil {
		t.Fatal("immutable analysis version changed")
	}
	e = other.Tx(ctx, func(tx pgx.Tx) error {
		var count int
		e := tx.QueryRow(ctx, `select count(*) from analysis_version where id=$1`, first.ID).Scan(&count)
		if e == nil && count != 0 {
			t.Fatal("RLS exposed another tenant")
		}
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
}
