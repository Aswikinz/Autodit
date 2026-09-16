package storage

import (
	"context"
	"errors"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestPasswords(t *testing.T) {
	hash, e := HashPassword("correct horse battery staple")
	if e != nil {
		t.Fatal(e)
	}
	if !CheckPassword(hash, "correct horse battery staple") || CheckPassword(hash, "wrong") {
		t.Fatal("password verification failed")
	}
	if _, e = HashPassword("short"); e == nil {
		t.Fatal("short password accepted")
	}
	if CheckPassword("malformed", "password") {
		t.Fatal("bad hash accepted")
	}
}
func TestAccountsIntegration(t *testing.T) {
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
	if e = store.BootstrapLocal(ctx, id, "Accounts", nil); e != nil {
		t.Fatal(e)
	}
	tenant, _ := store.ForTenant(id)
	if e = tenant.BootstrapAdmin(ctx, "initial password secret"); e != nil {
		t.Fatal(e)
	}
	if e = tenant.BootstrapAdmin(ctx, "different initial secret"); e != nil {
		t.Fatal(e)
	}
	admin, e := tenant.User(ctx, "admin")
	if e != nil || !admin.MustChange || !CheckPassword(admin.PasswordHash, "initial password secret") {
		t.Fatal("bootstrap replaced or omitted account", e)
	}
	admin.Enabled = false
	if e = tenant.SaveUser(ctx, admin, "", "admin"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("last admin disabled", e)
	}
	admin.Enabled = true
	admin.Roles = []string{"made_up"}
	if e = tenant.SaveUser(ctx, admin, "", "admin"); e == nil {
		t.Fatal("unknown role accepted")
	}
	if e = tenant.ChangePassword(ctx, "admin", "wrong", "replacement password"); e == nil {
		t.Fatal("wrong current password accepted")
	}
	if e = tenant.ChangePassword(ctx, "admin", "initial password secret", "replacement password"); e != nil {
		t.Fatal(e)
	}
	changed, _ := tenant.User(ctx, "admin")
	if changed.MustChange || changed.Revision == admin.Revision {
		t.Fatal("password change did not invalidate sessions")
	}
	reviewer := User{Username: "reviewer", DisplayName: "Reviewer", Roles: []string{"auditor"}, Enabled: true}
	if e = tenant.SaveUser(ctx, reviewer, "temporary password", "admin"); e != nil {
		t.Fatal(e)
	}
	reviewer, _ = tenant.User(ctx, "reviewer")
	reviewer.Enabled = false
	if e = tenant.SaveUser(ctx, reviewer, "", "admin"); e != nil {
		t.Fatal(e)
	}
	if e = tenant.SaveUser(ctx, reviewer, "", "admin"); !errors.Is(e, ErrConflict) {
		t.Fatal("stale account write succeeded", e)
	}
	other, _ := store.ForTenant(uuid.NewString())
	if _, e = other.User(ctx, "admin"); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross tenant account exposed", e)
	}
}
