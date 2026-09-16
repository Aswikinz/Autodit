package storage

import (
	"context"
	"encoding/json"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/exceptions"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
	"github.com/google/uuid"
	"os"
	"testing"
	"time"
)

func TestWorkflowIntegration(t *testing.T) {
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
	id := uuid.NewString()
	models, e := rules.LoadPack("../../rulepack")
	if e != nil {
		t.Fatal(e)
	}
	if e = store.Bootstrap(ctx, id, "Workflow", models); e != nil {
		t.Fatal(e)
	}
	tenant, _ := store.ForTenant(id)
	workflow := Workflow{Name: "Review and approval", Steps: []WorkflowStep{{Name: "Review", Role: "auditor"}, {Name: "Approve", Role: "audit_manager", DifferentActor: true}}}
	if e = tenant.SaveWorkflow(ctx, workflow, 0, "admin"); e != nil {
		t.Fatal(e)
	}
	if e = tenant.SaveSource(ctx, SourceConfig{ID: "test-source", Name: "Test", IntervalMinutes: 60}, "engineer"); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile("../../test/fixtures/population.json")
	if e != nil {
		t.Fatal(e)
	}
	var p domain.Population
	if e = json.Unmarshal(data, &p); e != nil {
		t.Fatal(e)
	}
	if _, e = tenant.Submit(ctx, p, "engineer"); e != nil {
		t.Fatal(e)
	}
	if _, e = tenant.ProcessNext(ctx, objectstore.Files{Root: t.TempDir()}, time.Now); e != nil {
		t.Fatal(e)
	}
	cases, e := tenant.Cases(ctx, "active")
	if e != nil || len(cases) != 4 {
		t.Fatal("new findings did not enter workflow", len(cases), e)
	}
	var c struct {
		Key      string `json:"exception_key"`
		Revision int    `json:"revision"`
	}
	_ = json.Unmarshal(cases[0], &c)
	if e = tenant.Dispose(ctx, c.Key, "reviewer", "try bypass", exceptions.InReview, 1, nil, time.Now()); e == nil {
		t.Fatal("legacy endpoint bypassed case workflow")
	}
	advance := CaseAction{Action: "advance", Note: "Evidence reviewed", Revision: 1}
	if e = tenant.ActOnCase(ctx, c.Key, "implementer", []string{"implementer"}, advance); e == nil {
		t.Fatal("wrong role advanced")
	}
	if e = tenant.ActOnCase(ctx, c.Key, "reviewer", []string{"auditor"}, advance); e != nil {
		t.Fatal(e)
	}
	if e = tenant.ActOnCase(ctx, c.Key, "reviewer", []string{"auditor"}, advance); e != ErrConflict {
		t.Fatal("stale action accepted", e)
	}
	advance.Revision = 2
	advance.Resolution = "accepted"
	if e = tenant.ActOnCase(ctx, c.Key, "reviewer", []string{"audit_manager"}, advance); e == nil {
		t.Fatal("self approval accepted")
	}
	back := CaseAction{Action: "return", Note: "More evidence required", Revision: 2}
	if e = tenant.ActOnCase(ctx, c.Key, "approver", []string{"audit_manager"}, back); e != nil {
		t.Fatal(e)
	}
	advance.Revision = 3
	if e = tenant.ActOnCase(ctx, c.Key, "reviewer", []string{"auditor"}, advance); e != nil {
		t.Fatal(e)
	}
	workflow.Steps = append(workflow.Steps, WorkflowStep{Name: "Extra control", Role: "audit_manager"})
	if e = tenant.SaveWorkflow(ctx, workflow, 1, "admin"); e != nil {
		t.Fatal(e)
	}
	advance.Revision = 4
	if e = tenant.ActOnCase(ctx, c.Key, "approver", []string{"audit_manager"}, advance); e != nil {
		t.Fatal(e)
	}
	closed, e := tenant.Cases(ctx, "closed")
	if e != nil || len(closed) != 1 {
		t.Fatal("old case did not retain two-step workflow", e)
	}
	history, e := tenant.CaseEvents(ctx, c.Key)
	if e != nil || len(history) != 4 {
		t.Fatal("history incomplete", e)
	}
	reopen := CaseAction{Action: "reopen", Note: "Follow-up required", Revision: 5}
	if e = tenant.ActOnCase(ctx, c.Key, "reviewer", []string{"auditor"}, reopen); e == nil {
		t.Fatal("auditor reopened closed case")
	}
	if e = tenant.ActOnCase(ctx, c.Key, "approver", []string{"audit_manager"}, reopen); e != nil {
		t.Fatal(e)
	}
	other, _ := store.ForTenant(uuid.NewString())
	hidden, e := other.Cases(ctx, "active")
	if e != nil || len(hidden) != 0 {
		t.Fatal("cross tenant cases exposed", e)
	}
}
