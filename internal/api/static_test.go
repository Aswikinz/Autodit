package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontendConfinement(t *testing.T) {
	base := t.TempDir()
	web := filepath.Join(base, "web")
	if err := os.MkdirAll(filepath.Join(web, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"web/index.html": "<html>workspace</html>", "web/assets/app.js": "window.ready=true", "private.txt": "PRIVATE_DATA", "web/.env": "PRIVATE_DATA"} {
		if err := os.WriteFile(filepath.Join(base, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler := serveFrontend(web)
	for _, tc := range []struct {
		path   string
		status int
		body   string
		cached bool
	}{
		{"/", 200, "workspace", false},
		{"/queue", 200, "workspace", false},
		{"/.env", 200, "workspace", false},
		{"/assets/app.js", 200, "window.ready", true},
		{"/assets/missing.js", 404, "", false},
		{"/assets/../private.txt", 400, "", false},
		{"/assets/%2e%2e/%2e%2e/private.txt", 400, "", false},
		{"/assets/..%5cprivate.txt", 400, "", false},
		{"/assets/app.js:stream", 400, "", false},
		{"/assets/", 400, "", false},
		{"/api/unknown", 404, "", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler(response, httptest.NewRequest("GET", tc.path, nil))
			if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.body) {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "PRIVATE_DATA") {
				t.Fatal("private file escaped the frontend boundary")
			}
			if strings.Contains(response.Header().Get("Cache-Control"), "immutable") != tc.cached {
				t.Fatal("incorrect asset caching")
			}
		})
	}
	t.Run("symlink escape", func(t *testing.T) {
		if err := os.Symlink(filepath.Join(base, "private.txt"), filepath.Join(web, "assets", "escape.txt")); err != nil {
			t.Skip("symlink creation unavailable", err)
		}
		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest("GET", "/assets/escape.txt", nil))
		if response.Code != 404 || strings.Contains(response.Body.String(), "PRIVATE_DATA") {
			t.Fatal("symlink escaped web root")
		}
	})
	t.Run("missing build", func(t *testing.T) {
		response := httptest.NewRecorder()
		serveFrontend(filepath.Join(base, "missing"))(response, httptest.NewRequest("GET", "/", nil))
		if response.Code != 503 {
			t.Fatal("missing frontend must fail closed")
		}
	})
}
