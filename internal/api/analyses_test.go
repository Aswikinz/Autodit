package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Aswikinz/Autodit/internal/analysis"
	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/ingest"
	"github.com/Aswikinz/Autodit/internal/platform"
	"github.com/Aswikinz/Autodit/internal/storage"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

func TestMain(m *testing.M) {
	if analysis.WorkerMain() {
		return
	}
	os.Exit(m.Run())
}

func TestAnalysisWorkspaceIntegration(t *testing.T) {
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
	tenantID := uuid.NewString()
	if e = store.BootstrapLocal(ctx, tenantID, "Analysis API", nil); e != nil {
		t.Fatal(e)
	}
	cfg := platform.Config{AuthMode: "demo", PublicURL: "http://localhost:8088", TenantID: tenantID, DemoToken: strings.Repeat("a", 40)}
	manager, e := auth.New(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer((&Server{Store: store, Auth: manager, WebDir: t.TempDir(), Now: time.Now}).Handler())
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
			t.Fatalf("%s %s got %d want %d: %s", method, path, response.StatusCode, want, b)
		}
		return b
	}
	request("GET", "/api/analyses", nil, 401)
	request("POST", "/api/login", map[string]string{"token": cfg.DemoToken}, 204)
	var session struct {
		Identity auth.Identity `json:"identity"`
	}
	_ = json.Unmarshal(request("GET", "/api/session", nil, 200), &session)
	request("POST", "/api/analyses/preview", map[string]string{"format": "csv", "data": "id,amount\n1,10\n"}, 403)
	csrf = session.Identity.CSRF
	var dataset analysis.Dataset
	_ = json.Unmarshal(request("POST", "/api/analyses/preview", map[string]string{"format": "csv", "name": "Payments", "data": "id,amount\n001,10\n002,20\n"}, 200), &dataset)
	if dataset.RowCount != 2 || dataset.Rows[0]["id"] != "001" {
		t.Fatal("CSV changed")
	}
	request("POST", "/api/analyses/preview", map[string]string{"format": "json", "data": "[{\"id\":\"001\",\"amount\":10}]"}, 200)
	for _, body := range []any{map[string]string{"unknown": "bad"}, map[string]string{"format": "bad"}, map[string]string{"format": "json", "data": "not json"}, map[string]string{"format": "csv", "data": "id,id\na,b\n"}, map[string]string{"format": "csv", "name": strings.Repeat("x", 251)}, map[string]string{"format": "xlsx", "data": "%%"}, map[string]string{"format": "xlsx", "data": base64.StdEncoding.EncodeToString([]byte("broken"))}} {
		request("POST", "/api/analyses/preview", body, 400)
	}
	book := excelize.NewFile()
	defer book.Close()
	_ = book.SetSheetRow("Sheet1", "A1", &[]any{"id", "value"})
	_ = book.SetSheetRow("Sheet1", "A2", &[]any{"A", 10})
	buffer, e := book.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	xlsx := map[string]string{"format": "xlsx", "data": base64.StdEncoding.EncodeToString(buffer.Bytes())}
	if !bytes.Contains(request("POST", "/api/analyses/preview", xlsx, 200), []byte("Sheet1")) {
		t.Fatal("missing sheets")
	}
	xlsx["sheet"] = "Sheet1"
	request("POST", "/api/analyses/preview", xlsx, 200)
	dbURL, _ := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	port, _ := strconv.Atoi(dbURL.Port())
	if port == 0 {
		port = 5432
	}
	password, _ := dbURL.User.Password()
	connection := ingest.Connection{Kind: "postgres", Host: dbURL.Hostname(), Port: port, Database: strings.TrimPrefix(dbURL.Path, "/"), Username: dbURL.User.Username(), Password: password, AllowPlaintext: true}
	request("POST", "/api/analyses/connection", connection, 200)
	request("POST", "/api/analyses/connection", ingest.Connection{}, 400)
	request("POST", "/api/analyses/connection", map[string]string{"invalid": "field"}, 400)
	request("POST", "/api/analyses/preview", map[string]any{"format": "database", "connection": connection, "table": ingest.Table{Schema: "public", Name: "schema_version"}}, 200)
	request("POST", "/api/analyses/preview", map[string]any{"format": "database", "connection": connection, "table": ingest.Table{Schema: "public", Name: "missing"}}, 400)
	model := json.RawMessage(`{"nodes":[{"id":"in","name":"Input","type":"inputNode"},{"id":"out","name":"Output","type":"outputNode"}],"edges":[{"id":"edge","sourceId":"in","targetId":"out"}]}`)
	in := storage.SavedAnalysis{Name: "Payments", Dataset: dataset, SelectedColumns: dataset.Columns, Model: model}
	var saved storage.SavedAnalysis
	_ = json.Unmarshal(request("POST", "/api/analyses", in, 200), &saved)
	request("GET", "/api/analyses", nil, 200)
	request("GET", "/api/analyses?page=2", nil, 200)
	request("GET", "/api/analyses?page=x", nil, 400)
	request("GET", "/api/analyses?page=0", nil, 400)
	request("GET", "/api/analyses/"+saved.ID, nil, 200)
	request("GET", "/api/analyses/"+saved.ID+"?revision=1", nil, 200)
	request("GET", "/api/analyses/"+saved.ID+"?revision=9", nil, 404)
	request("GET", "/api/analyses/"+saved.ID+"?revision=x", nil, 400)
	request("GET", "/api/analyses/missing", nil, 400)
	saved.Name = "Payments revised"
	request("POST", "/api/analyses", saved, 200)
	request("POST", "/api/analyses", saved, 409)
	request("POST", "/api/analyses", map[string]string{"unknown": "field"}, 400)
	var report analysis.Report
	_ = json.Unmarshal(request("POST", "/api/analyses/test", map[string]any{"dataset": dataset, "selected_columns": dataset.Columns, "model": model}, 200), &report)
	if report.Total != 2 || report.Errors != 0 || len(report.Results) != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	request("POST", "/api/analyses/test", map[string]any{"dataset": dataset, "selected_columns": dataset.Columns, "model": map[string]any{}}, 400)
	request("POST", "/api/analyses/test", map[string]any{"dataset": dataset, "model": model}, 400)
	request("POST", "/api/analyses/test", map[string]string{"unknown": "field"}, 400)
	badSyntax := json.RawMessage(`{"nodes":[{"id":"in","type":"inputNode","name":"Request"},{"id":"rule","name":"Bad condition","type":"decisionTableNode","content":{"hitPolicy":"first","inputs":[{"id":"amount","field":"data.amount"}],"outputs":[{"id":"flag","field":"flag"}],"rules":[{"_id":"r1","amount":">>> 10","flag":"true"},{"_id":"r2","amount":"","flag":"false"}]}},{"id":"out","name":"Response","type":"outputNode"}],"edges":[{"id":"a","sourceId":"in","targetId":"rule"},{"id":"b","sourceId":"rule","targetId":"out"}]}`)
	in.Model = badSyntax
	request("POST", "/api/analyses", in, 400)
	request("POST", "/api/analyses/test", map[string]any{"dataset": dataset, "selected_columns": dataset.Columns, "model": badSyntax}, 400)
	tenant, _ := store.ForTenant(tenantID)
	cases, e := tenant.Cases(ctx, "active", 1)
	if e != nil || len(cases) != 0 {
		t.Fatal("exploratory data produced governed cases", e)
	}
}

func TestAnalysisPermissions(t *testing.T) {
	for _, role := range []string{"admin", "auditor", "implementer", "rule_engineer", "audit_manager", "unknown"} {
		for _, operation := range []string{"analytics.read", "analytics.test", "analytics.write"} {
			want := role == "implementer" || role == "rule_engineer" || role == "audit_manager" || (role == "auditor" && operation != "analytics.write")
			if auth.Allowed(auth.Identity{Roles: []string{role}}, operation) != want {
				t.Fatalf("permission %s %s", role, operation)
			}
			if auth.Allowed(auth.Identity{Roles: []string{role}, MustChange: true}, operation) {
				t.Fatal("password gate bypass")
			}
		}
	}
}
