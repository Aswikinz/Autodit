package auth

import (
	"context"
	"github.com/Aswikinz/Autodit/internal/platform"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSecureCookiePolicy(t *testing.T) {
	for _, tc := range []struct {
		mode, origin string
		secure       bool
	}{
		{"demo", "http://localhost:8088", false},
		{"demo", "http://127.0.0.1:8088", false},
		{"demo", "https://localhost:8088", true},
		{"demo", "https://audit.example", true},
		{"oidc", "https://audit.example", true},
		{"oidc", "http://localhost:8088", true},
		{"demo", "http://localhost.attacker.invalid", true},
		{"unknown", "http://localhost:8088", true},
		{"demo", ":invalid", true},
	} {
		t.Run(tc.mode+tc.origin, func(t *testing.T) {
			m := Manager{cfg: platform.Config{AuthMode: tc.mode, PublicURL: tc.origin}}
			response := httptest.NewRecorder()
			m.cookie(response, "autodit_session", "opaque", 60)
			cookie := response.Result().Cookies()[0]
			if cookie.Secure != tc.secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatal("cookie security policy violated")
			}
		})
	}
}

func TestRoleSeparation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		role, op string
		want     bool
	}{{"admin", "exceptions.write", false}, {"implementer", "exceptions.read", false}, {"auditor", "rules.write", false}, {"audit_manager", "rules.write", true}, {"rule_engineer", "rules.write", true}, {"auditor", "exceptions.write", true}, {"admin", "runs.read", true}, {"unknown", "runs.read", false}, {"audit_manager", "unknown", false}} {
		if got := Allowed(Identity{Roles: []string{tc.role}}, tc.op); got != tc.want {
			t.Errorf("role %s operation %s = %v", tc.role, tc.op, got)
		}
	}
}
func TestDemoSessionAndCSRF(t *testing.T) {
	t.Parallel()
	cfg := platform.Config{AuthMode: "demo", PublicURL: "http://localhost:8088", DemoToken: strings.Repeat("x", 40), TenantID: "11111111-1111-4111-8111-111111111111"}
	m, e := New(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"token":"`+cfg.DemoToken+`"}`))
	req.Header.Set("Origin", cfg.PublicURL)
	w := httptest.NewRecorder()
	m.DemoLogin(w, req)
	if w.Code != 204 {
		t.Fatal("login failed", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly {
		t.Fatal("session readable from JS")
	}
	req = httptest.NewRequest("POST", "/api/test", nil)
	req.AddCookie(cookie)
	identity, ok := m.Current(req)
	if !ok {
		t.Fatal("session unavailable")
	}
	if m.Mutation(req, identity) {
		t.Fatal("missing CSRF accepted")
	}
	req.Header.Set("Origin", cfg.PublicURL)
	req.Header.Set("X-CSRF-Token", identity.CSRF)
	if !m.Mutation(req, identity) {
		t.Fatal("valid CSRF rejected")
	}
	req.Header.Set("Origin", "https://attacker.invalid")
	if m.Mutation(req, identity) {
		t.Fatal("cross-site mutation accepted")
	}
	req.Header.Set("Origin", cfg.PublicURL)
	w = httptest.NewRecorder()
	m.Logout(w, req)
	if w.Code != 204 {
		t.Fatal("logout failed")
	}
	if _, ok = m.Current(req); ok {
		t.Fatal("logout did not invalidate session")
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"token":"wrong"}`))
	req.Header.Set("Origin", cfg.PublicURL)
	m.DemoLogin(w, req)
	if w.Code != 401 {
		t.Fatal("bad credential accepted")
	}
	w = httptest.NewRecorder()
	m.Login(w, req)
	if w.Code != 404 {
		t.Fatal("unconfigured oidc allowed")
	}
	w = httptest.NewRecorder()
	m.Callback(w, req)
	if w.Code != 404 {
		t.Fatal("unconfigured callback allowed")
	}
	w = httptest.NewRecorder()
	m.Session(w, req)
	if w.Code != 200 {
		t.Fatal("session endpoint failed")
	}
	m.Now = func() time.Time { return time.Now().Add(9 * time.Hour) }
	if _, ok = m.Current(req); ok {
		t.Fatal("expired session accepted")
	}
}
