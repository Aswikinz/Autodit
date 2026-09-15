package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/platform"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
	"github.com/google/uuid"
)

func TestAPIIntegration(t *testing.T) {
	owner := os.Getenv("TEST_OWNER_DATABASE_URL")
	if owner == "" {
		t.Skip("run make test-int")
	}
	ctx := context.Background()
	if e := storage.Migrate(ctx, owner); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	models, e := rules.LoadPack("../../rulepack")
	if e != nil {
		t.Fatal(e)
	}
	id := uuid.NewString()
	if e = store.Bootstrap(ctx, id, "API integration", models); e != nil {
		t.Fatal(e)
	}
	cfg := platform.Config{AuthMode: "demo", PublicURL: "http://localhost:8088", TenantID: id, DemoToken: strings.Repeat("t", 40)}
	manager, e := auth.New(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	objects := objectstore.Files{Root: t.TempDir()}
	webDir := t.TempDir()
	_ = os.WriteFile(webDir+"/index.html", []byte("<html>Autodit</html>"), 0600)
	server := httptest.NewServer((&Server{Store: store, Auth: manager, Objects: objects, WebDir: webDir, Now: time.Now}).Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	request := func(method, path string, body any, want int) []byte {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(data))
		req.Header.Set("Origin", cfg.PublicURL)
		req.Header.Set("X-CSRF-Token", csrf)
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, resp.StatusCode, want, b)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing security headers")
		}
		return b
	}
	request("GET", "/healthz", nil, 200)
	request("GET", "/", nil, 200)
	request("GET", "/api/not-found", nil, 404)
	request("GET", "/api/exceptions", nil, 401)
	request("POST", "/api/login", map[string]string{"token": cfg.DemoToken}, 204)
	b := request("GET", "/api/session", nil, 200)
	var session struct {
		Identity auth.Identity `json:"identity"`
	}
	_ = json.Unmarshal(b, &session)
	csrf = session.Identity.CSRF
	if csrf == "" {
		t.Fatal("missing CSRF")
	}
	request("POST", "/api/sources", map[string]any{"id": "test-source", "name": "Synthetic", "interval_minutes": 60, "mapping": map[string]string{}}, 204)
	request("GET", "/api/sources", nil, 200)
	request("POST", "/api/sources/profile", map[string]string{"csv": "id,amount\na,1\n"}, 200)
	request("POST", "/api/sources/profile", map[string]string{"csv": "id,id\na,b\n"}, 400)
	population, e := os.ReadFile("../../test/fixtures/population.json")
	if e != nil {
		t.Fatal(e)
	}
	var pop map[string]any
	_ = json.Unmarshal(population, &pop)
	b = request("POST", "/api/runs", pop, 202)
	var run struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(b, &run)
	tenant, _ := store.ForTenant(id)
	if worked, e := tenant.ProcessNext(ctx, objects, time.Now); e != nil || !worked {
		t.Fatal("worker failed", e)
	}
	request("GET", "/api/runs", nil, 200)
	request("GET", "/api/runs/"+run.ID, nil, 200)
	request("GET", "/api/runs/"+uuid.NewString(), nil, 404)
	b = request("GET", "/api/exceptions?state=active&sort=newest&page=1&size=30", nil, 200)
	var queue struct {
		Items []struct {
			Key      string `json:"key"`
			Revision int    `json:"revision"`
		} `json:"items"`
	}
	_ = json.Unmarshal(b, &queue)
	if len(queue.Items) != 4 {
		t.Fatal("wrong queue")
	}
	key := queue.Items[0].Key
	b = request("GET", "/api/exceptions/"+key, nil, 200)
	var detail struct {
		Observations []struct {
			ID string `json:"id"`
		} `json:"observations"`
	}
	_ = json.Unmarshal(b, &detail)
	request("POST", "/api/observations/"+detail.Observations[0].ID+"/replay", nil, 200)
	request("POST", "/api/exceptions/"+key+"/disposition", map[string]any{"state": "in_review", "reason": "Investigate", "revision": 1, "suppression_until": nil}, 204)
	request("POST", "/api/exceptions/"+key+"/disposition", map[string]any{"state": "dismissed", "reason": "Stale request", "revision": 1, "suppression_until": nil}, 409)
	request("POST", "/api/exceptions/"+key+"/disposition", map[string]any{"state": "resolved_in_source", "reason": "Manual close prohibited", "revision": 2, "suppression_until": nil}, 400)
	request("POST", "/api/exceptions/"+key+"/comments", map[string]string{"body": "Investigation note"}, 204)
	request("POST", "/api/exceptions/"+key+"/comments", map[string]string{"body": ""}, 400)
	request("GET", "/api/exceptions?state=active&rule=AP-01&severity=high&sort=oldest&search=P1", nil, 200)
	request("GET", "/api/exceptions?page=-1", nil, 400)
	request("GET", "/api/exceptions/absent", nil, 404)
	request("GET", "/api/parameters", nil, 200)
	p := rules.Defaults()
	p.Materiality = "1100"
	request("PUT", "/api/parameters", map[string]any{"parameters": p, "revision": 1}, 204)
	request("PUT", "/api/parameters", map[string]any{"parameters": p, "revision": 1}, 409)
	request("GET", "/api/rules", nil, 200)
	request("POST", "/api/rules/simulate", map[string]any{"model": models[0].Content}, 200)
	request("POST", "/api/rules/simulate", map[string]any{"model": map[string]any{}}, 422)
	request("PUT", "/api/rules/AP-01", map[string]any{"model": models[0].Content, "revision": 1, "enabled": true}, 204)
	request("PUT", "/api/rules/AP-01", map[string]any{"model": models[0].Content, "revision": 1, "enabled": false}, 409)
	request("GET", "/api/assurance", nil, 200)
	request("POST", "/api/imports", map[string]any{"csv": "bad", "population": pop, "mapping": map[string]string{}}, 400)
	request("POST", "/api/sources", map[string]any{"unknown": true}, 400)
	saved := csrf
	csrf = "wrong"
	request("POST", "/api/sources", nil, 403)
	csrf = saved
	request("POST", "/api/logout", nil, 204)
	request("GET", "/api/session", nil, 200)
	request("GET", "/api/runs", nil, 401)
}
