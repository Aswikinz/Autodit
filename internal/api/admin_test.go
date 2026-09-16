package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/ingest"
	"github.com/Aswikinz/Autodit/internal/platform"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestAdminWorkspaceIntegration(t *testing.T) {
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
	models, e := rules.LoadPack("../../rulepack")
	if e != nil {
		t.Fatal(e)
	}
	if e = store.BootstrapLocal(ctx, id, "Admin API", models); e != nil {
		t.Fatal(e)
	}
	tenant, _ := store.ForTenant(id)
	if e = tenant.BootstrapAdmin(ctx, "initial admin password"); e != nil {
		t.Fatal(e)
	}
	u, _ := tenant.User(ctx, "admin")
	u.Roles = []string{"admin", "auditor", "audit_manager", "implementer", "rule_engineer"}
	if e = tenant.SaveUser(ctx, u, "", "bootstrap"); e != nil {
		t.Fatal(e)
	}
	cfg := platform.Config{AuthMode: "local", PublicURL: "http://localhost:8088", TenantID: id}
	manager, e := auth.New(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	objects := objectstore.Files{Root: t.TempDir()}
	server := httptest.NewServer((&Server{Store: store, Auth: manager, Objects: objects, WebDir: t.TempDir(), Now: time.Now}).Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	request := func(method, path string, body any, want int) []byte {
		t.Helper()
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		req.Header.Set("Origin", cfg.PublicURL)
		req.Header.Set("X-CSRF-Token", csrf)
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		b, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, response.StatusCode, want, b)
		}
		return b
	}
	session := func() {
		var s struct {
			Identity auth.Identity `json:"identity"`
		}
		_ = json.Unmarshal(request("GET", "/api/session", nil, 200), &s)
		csrf = s.Identity.CSRF
	}
	request("GET", "/api/admin/users", nil, 401)
	request("POST", "/api/password-login", map[string]string{"username": "admin", "password": "wrong"}, 401)
	request("POST", "/api/password-login", map[string]string{"username": "admin", "password": "initial admin password"}, 204)
	session()
	request("GET", "/api/admin/users", nil, 403)
	request("POST", "/api/password", map[string]string{"current_password": "initial admin password", "new_password": "short"}, 400)
	request("POST", "/api/password", map[string]string{"current_password": "initial admin password", "new_password": "replacement admin password"}, 204)
	request("GET", "/api/admin/users", nil, 401)
	request("POST", "/api/password-login", map[string]string{"username": "admin", "password": "replacement admin password"}, 204)
	session()
	request("GET", "/api/admin/users", nil, 200)
	person := map[string]any{"username": "reviewer", "display_name": "Reviewer", "roles": []string{"auditor"}, "password": "initial reviewer password", "enabled": true, "revision": 0}
	request("POST", "/api/admin/users", person, 204)
	request("POST", "/api/admin/users", person, 409)
	request("POST", "/api/admin/users", map[string]string{"unknown": "value"}, 400)
	request("POST", "/api/admin/users", map[string]string{"username": "bad"}, 400)
	request("GET", "/api/workspace", nil, 200)
	settings := storage.WorkspaceSettings{Name: "Audit", Timezone: "Asia/Colombo", Locale: "en-LK", ReportingCurrency: "LKR", FiscalStartMonth: 4}
	request("PUT", "/api/admin/settings", map[string]any{"settings": settings, "revision": 0}, 204)
	request("PUT", "/api/admin/settings", map[string]any{"settings": settings, "revision": 0}, 409)
	settings.Name = "Revised"
	request("PUT", "/api/admin/settings", map[string]any{"settings": settings, "revision": 1}, 204)
	request("PUT", "/api/admin/settings", map[string]any{"settings": settings, "revision": 1}, 409)
	request("PUT", "/api/admin/settings", map[string]string{"bad": "field"}, 400)
	for _, change := range []func(*storage.WorkspaceSettings){func(s *storage.WorkspaceSettings) { s.Name = "" }, func(s *storage.WorkspaceSettings) { s.ReportingCurrency = "12X" }, func(s *storage.WorkspaceSettings) { s.Timezone = "missing/zone" }, func(s *storage.WorkspaceSettings) { s.Locale = "??" }, func(s *storage.WorkspaceSettings) { s.Locale = "" }} {
		bad := settings
		change(&bad)
		request("PUT", "/api/admin/settings", map[string]any{"settings": bad, "revision": 2}, 400)
	}
	request("GET", "/api/admin/workflow", nil, 200)
	workflow := storage.Workflow{Name: "Review", Steps: []storage.WorkflowStep{{Name: "Review", Role: "auditor"}, {Name: "Approve", Role: "audit_manager", DifferentActor: true}}}
	request("PUT", "/api/admin/workflow", map[string]any{"workflow": workflow, "revision": 0}, 204)
	request("PUT", "/api/admin/workflow", map[string]any{"workflow": workflow, "revision": 0}, 409)
	request("PUT", "/api/admin/workflow", map[string]string{"bad": "field"}, 400)
	request("PUT", "/api/admin/workflow", map[string]any{"workflow": storage.Workflow{}, "revision": 1}, 400)
	request("GET", "/api/admin/workflow", nil, 200)
	request("GET", "/api/cases", nil, 200)
	request("GET", "/api/cases?status=invalid", nil, 400)
	request("GET", "/api/cases/people", nil, 200)
	p := rules.Defaults()
	p.ExplicitCurrencies = true
	p.CurrencyThresholds = map[string]string{"USD": "1000"}
	if e = tenant.SaveParameters(ctx, p, 1, "admin"); e != nil {
		t.Fatal(e)
	}
	if e = tenant.SaveSource(ctx, storage.SourceConfig{ID: "test-source", Name: "Test", IntervalMinutes: 60}, "admin"); e != nil {
		t.Fatal(e)
	}
	data, _ := os.ReadFile("../../test/fixtures/population.json")
	var population domain.Population
	_ = json.Unmarshal(data, &population)
	if _, e = tenant.Submit(ctx, population, "admin"); e != nil {
		t.Fatal(e)
	}
	if _, e = tenant.ProcessNext(ctx, objects, time.Now); e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Key string `json:"exception_key"`
	}
	_ = json.Unmarshal(request("GET", "/api/cases?status=active&page=1", nil, 200), &cases)
	if len(cases) != 4 {
		t.Fatal("missing cases")
	}
	key := cases[0].Key
	request("POST", "/api/cases/"+key+"/actions", storage.CaseAction{Action: "assign", Assignee: "reviewer", Note: "Please review", Revision: 1}, 204)
	request("POST", "/api/cases/"+key+"/actions", storage.CaseAction{Action: "advance", Note: "Not my assignment", Revision: 2}, 400)
	request("POST", "/api/cases/"+key+"/actions", storage.CaseAction{Action: "assign", Note: "Release assignment", Revision: 2}, 204)
	request("POST", "/api/cases/"+key+"/actions", storage.CaseAction{Action: "advance", Note: "Evidence reviewed", Revision: 3}, 204)
	request("POST", "/api/cases/"+key+"/actions", map[string]string{"bad": "field"}, 400)
	request("GET", "/api/cases/"+key+"/events", nil, 200)
	dbURL, _ := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	port, _ := strconv.Atoi(dbURL.Port())
	if port == 0 {
		port = 5432
	}
	password, _ := dbURL.User.Password()
	connection := ingest.Connection{Kind: "postgres", Host: dbURL.Hostname(), Port: port, Database: "autodit", Username: dbURL.User.Username(), Password: password, AllowPlaintext: true}
	request("POST", "/api/sources/connection", connection, 200)
	request("POST", "/api/sources/connection", map[string]string{"bad": "field"}, 400)
	request("POST", "/api/sources/connection", ingest.Connection{}, 400)
	request("POST", "/api/sources/preview", map[string]any{"connection": connection, "table": ingest.Table{Schema: "public", Name: "schema_version"}}, 200)
	request("POST", "/api/sources/preview", map[string]any{"connection": connection, "table": ingest.Table{Schema: "public", Name: "missing"}}, 400)
	request("POST", "/api/sources/preview", map[string]string{"bad": "field"}, 400)
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetRow("Sheet1", "A1", &[]any{"id", "amount"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]any{"P1", "1.2345"})
	workbook, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	request("POST", "/api/sources/workbook", map[string]any{"data": workbook.Bytes(), "sheet": "Sheet1"}, 200)
	request("POST", "/api/sources/workbook", map[string]any{"data": []byte("broken"), "sheet": "Sheet1"}, 400)
	request("POST", "/api/sources/workbook", map[string]string{"bad": "field"}, 400)
	request("POST", "/api/logout", nil, 204)
	request("POST", "/api/password-login", map[string]string{"username": "reviewer", "password": "initial reviewer password"}, 204)
	session()
	request("POST", "/api/password", map[string]string{"current_password": "initial reviewer password", "new_password": "replacement reviewer password"}, 204)
	request("POST", "/api/password-login", map[string]string{"username": "reviewer", "password": "replacement reviewer password"}, 204)
	session()
	request("GET", "/api/admin/users", nil, 403)
	request("POST", "/api/sources/connection", connection, 403)
	request("GET", "/api/cases", nil, 200)
}
