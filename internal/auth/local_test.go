package auth

import (
	"context"
	"github.com/Aswikinz/Autodit/internal/platform"
	"github.com/Aswikinz/Autodit/internal/storage"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocalSessionIntegration(t *testing.T) {
	if os.Getenv("TEST_OWNER_DATABASE_URL") == "" {
		t.Skip("integration database required")
	}
	ctx := context.Background()
	if e := storage.Migrate(ctx, os.Getenv("TEST_OWNER_DATABASE_URL")); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	id := uuid.NewString()
	if e = store.BootstrapLocal(ctx, id, "Local auth", nil); e != nil {
		t.Fatal(e)
	}
	tenant, _ := store.ForTenant(id)
	if e = tenant.BootstrapAdmin(ctx, "initial secret password"); e != nil {
		t.Fatal(e)
	}
	m, e := New(ctx, platform.Config{AuthMode: "local", TenantID: id, PublicURL: "http://localhost:8088"})
	if e != nil {
		t.Fatal(e)
	}
	m.Users = tenant
	req := httptest.NewRequest("POST", "/api/password-login", strings.NewReader(`{"username":"admin","password":"initial secret password"}`))
	req.Header.Set("Origin", "http://localhost:8088")
	rec := httptest.NewRecorder()
	m.PasswordLogin(rec, req)
	if rec.Code != 204 {
		t.Fatal("login failed", rec.Code)
	}
	r := httptest.NewRequest("GET", "/api/session", nil)
	r.AddCookie(rec.Result().Cookies()[0])
	identity, ok := m.Current(r)
	if !ok || !identity.MustChange || Allowed(identity, "admin.read") {
		t.Fatal("initial password has full access")
	}
	if e = tenant.ChangePassword(ctx, "admin", "initial secret password", "replacement password secret"); e != nil {
		t.Fatal(e)
	}
	if _, ok = m.Current(r); ok {
		t.Fatal("password change retained old session")
	}
	u, _ := tenant.User(ctx, "admin")
	rec = httptest.NewRecorder()
	m.establish(rec, Identity{Subject: "admin", TenantID: id, Roles: u.Roles, LocalRevision: u.Revision}, time.Now().Add(time.Hour))
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(rec.Result().Cookies()[0])
	identity, ok = m.Current(r)
	if !ok || !Allowed(identity, "admin.read") || Allowed(identity, "sources.write") {
		t.Fatal("role separation failed")
	}
	u.DisplayName = "Changed admin"
	if e = tenant.SaveUser(ctx, u, "", "admin"); e != nil {
		t.Fatal(e)
	}
	if _, ok = m.Current(r); ok {
		t.Fatal("account edit retained old session")
	}
	bad := httptest.NewRequest(http.MethodPost, "/api/password-login", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	m.PasswordLogin(rec, bad)
	if rec.Code != 403 {
		t.Fatal("cross origin login accepted")
	}
	for range 8 {
		m.permitLogin("blocked")
	}
	if m.permitLogin("blocked") {
		t.Fatal("rate limit missing")
	}
}
