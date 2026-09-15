package storage

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/exceptions"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPipelineIntegration(t *testing.T) {
	ownerURL := os.Getenv("TEST_OWNER_DATABASE_URL")
	if ownerURL == "" {
		t.Skip("integration database not requested; run make test-int")
	}
	ctx := context.Background()
	if e := Migrate(ctx, ownerURL); e != nil {
		t.Fatal(e)
	}
	if e := Migrate(ctx, ownerURL); e != nil {
		t.Fatal("migration not idempotent", e)
	}
	store, e := Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	models, e := rules.LoadPack("../../rulepack")
	if e != nil {
		t.Fatal(e)
	}
	tenantID := uuid.NewString()
	if e = store.Bootstrap(ctx, tenantID, "Integration", models); e != nil {
		t.Fatal(e)
	}
	tenant, _ := store.ForTenant(tenantID)
	if e = tenant.SaveSource(ctx, SourceConfig{ID: "test-source", Name: "Test file", IntervalMinutes: 60}, "implementer"); e != nil {
		t.Fatal(e)
	}
	var population domain.Population
	b, e := os.ReadFile("../../test/fixtures/population.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &population); e != nil {
		t.Fatal(e)
	}
	objects := objectstore.Files{Root: t.TempDir()}
	now := func() time.Time { return time.Now().UTC() }
	run := func(p domain.Population, want string) string {
		t.Helper()
		id, e := tenant.Submit(ctx, p, "implementer")
		if e != nil {
			t.Fatal(e)
		}
		worked, e := tenant.ProcessNext(ctx, objects, now)
		if !worked {
			t.Fatal("worker did not claim run")
		}
		if e != nil && want == "completed" {
			t.Fatal(e)
		}
		out, e := tenant.Run(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		var r struct {
			Status     string `json:"status"`
			SnapshotID string `json:"snapshot_id"`
		}
		_ = json.Unmarshal(out, &r)
		if r.Status != want {
			t.Fatalf("run status %s, wanted %s", r.Status, want)
		}
		return id
	}
	first := run(population, "completed")
	page, e := tenant.Queue(ctx, QueueFilter{Page: 1, Size: 100, Sort: "oldest"})
	if e != nil {
		t.Fatal(e)
	}
	if page.Total != 4 {
		t.Fatalf("expected four exceptions, got %d", page.Total)
	}
	wantKeys := map[string]bool{}
	for _, spec := range []struct {
		rule, entity string
		ids          []string
	}{{"AP-01", "payment", []string{"P1", "P2"}}, {"JE-01", "journal_entry", []string{"J1"}}, {"JE-01", "journal_entry", []string{"J2"}}, {"AP-02", "approval", []string{"A1"}}} {
		wantKeys[domain.ExceptionKey(spec.rule, spec.entity, domain.EntityID(population.SourceID, spec.ids...), population.Period.ID)] = true
	}
	for _, b := range page.Items {
		var v struct {
			Key string `json:"key"`
		}
		_ = json.Unmarshal(b, &v)
		if !wantKeys[v.Key] {
			t.Fatalf("unexpected exception key %s", v.Key)
		}
	}
	key := domain.ExceptionKey("AP-01", "payment", domain.EntityID(population.SourceID, "P1", "P2"), population.Period.ID)
	if e = tenant.Dispose(ctx, key, "auditor", "investigating", exceptions.InReview, 1, nil, now()); e != nil {
		t.Fatal(e)
	}
	if e = tenant.Dispose(ctx, key, "auditor", "legitimate separate payments", exceptions.Dismissed, 2, nil, now()); e != nil {
		t.Fatal(e)
	}
	run(population, "completed")
	detail, e := tenant.Detail(ctx, key)
	if e != nil {
		t.Fatal(e)
	}
	var d struct {
		Exception struct {
			State      string `json:"state"`
			SnapshotID string `json:"snapshot_id"`
		} `json:"exception"`
		Observations []struct {
			ID string `json:"id"`
		} `json:"observations"`
	}
	_ = json.Unmarshal(detail, &d)
	if d.Exception.State != "dismissed" || d.Exception.SnapshotID != first || len(d.Observations) != 2 {
		t.Fatal("rerun lost disposition or original evidence")
	}
	if _, e = tenant.Replay(ctx, d.Observations[0].ID, objects); e != nil {
		t.Fatal("replay", e)
	}
	// Third run removes the approval breach while leaving the complete population.
	population.Records[4].Limit = "3000"
	run(population, "completed")
	resolved, e := tenant.Queue(ctx, QueueFilter{State: "resolved_in_source", Page: 1, Size: 100})
	if e != nil || resolved.Total != 1 {
		t.Fatal("source resolution failed", e, resolved.Total)
	}
	population.Controls[0].Amount = "1"
	run(population, "tieout_failed")
	still, e := tenant.Queue(ctx, QueueFilter{Page: 1, Size: 100})
	if e != nil || still.Total != 4 {
		t.Fatal("failed tie-out changed exceptions")
	}
	otherID := uuid.NewString()
	if e = store.Bootstrap(ctx, otherID, "Other", models); e != nil {
		t.Fatal(e)
	}
	other, _ := store.ForTenant(otherID)
	if _, e = other.Detail(ctx, key); e != ErrNotFound {
		t.Fatal("cross-tenant evidence exposed")
	}
	// Runtime role cannot mutate append-only evidence, even using direct SQL.
	e = tenant.Tx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "update observation set classification='tampered' where tenant_id=$1", tenantID)
		return e
	})
	if e == nil {
		t.Fatal("immutable evidence was writable")
	}
	// RLS is independent of the storage-layer tenant predicate.
	e = other.Tx(ctx, func(tx pgx.Tx) error {
		var count int
		if e := tx.QueryRow(ctx, "select count(*) from exception where tenant_id=$1", tenantID).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			t.Fatal("RLS isolation failed")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
