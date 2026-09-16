// Package auth provides an OIDC browser session and server-side role checks.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Aswikinz/Autodit/internal/platform"
	"github.com/Aswikinz/Autodit/internal/storage"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Identity binds a verified subject to a tenant and role set.
type Identity struct {
	Subject       string   `json:"subject"`
	TenantID      string   `json:"tenant_id"`
	Roles         []string `json:"roles"`
	CSRF          string   `json:"csrf_token"`
	MustChange    bool     `json:"must_change_password"`
	LocalRevision int      `json:"-"`
}
type session struct {
	identity Identity
	expires  time.Time
}
type login struct {
	verifier, nonce string
	expires         time.Time
}

// Manager keeps short-lived opaque browser sessions; restart invalidates all sessions.
type Manager struct {
	cfg      platform.Config
	mu       sync.Mutex
	sessions map[string]session
	logins   map[string]login
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	Now      func() time.Time
	Users    *storage.Tenant
	attempts map[string]attempt
}

// New discovers the explicitly configured identity provider.
func New(ctx context.Context, c platform.Config) (*Manager, error) {
	if e := c.ValidateAPI(); e != nil {
		return nil, e
	}
	m := &Manager{cfg: c, sessions: map[string]session{}, logins: map[string]login{}, Now: time.Now}
	if c.AuthMode == "oidc" {
		p, e := oidc.NewProvider(ctx, c.Issuer)
		if e != nil {
			return nil, errors.New("OIDC discovery failed")
		}
		m.oauth = oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, Endpoint: p.Endpoint(), RedirectURL: c.PublicURL + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile"}}
		m.verifier = p.Verifier(&oidc.Config{ClientID: c.ClientID})
	}
	return m, nil
}
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic("system entropy unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Allowed maps explicit operations to roles; no admin bypass exists.
func Allowed(i Identity, operation string) bool {
	if i.MustChange {
		return false
	}
	if operation == "workspace.read" {
		return len(i.Roles) > 0
	}
	if operation == "cases.read" || operation == "cases.write" {
		for _, r := range i.Roles {
			if r == "auditor" || r == "audit_manager" || r == "implementer" || r == "rule_engineer" {
				return true
			}
		}
		return false
	}
	if operation == "parameters.read" {
		for _, r := range i.Roles {
			if r == "admin" || r == "auditor" || r == "audit_manager" || r == "rule_engineer" {
				return true
			}
		}
		return false
	}
	if operation == "admin.write" || operation == "admin.read" {
		for _, r := range i.Roles {
			if r == "admin" {
				return true
			}
		}
		return false
	}
	roles := map[string][]string{"exceptions.read": {"auditor", "audit_manager"}, "exceptions.write": {"auditor", "audit_manager"}, "runs.read": {"auditor", "audit_manager", "implementer", "admin"}, "runs.write": {"implementer"}, "sources.read": {"implementer", "audit_manager", "admin"}, "sources.write": {"implementer"}, "rules.read": {"auditor", "audit_manager", "rule_engineer"}, "rules.simulate": {"auditor", "audit_manager", "rule_engineer"}, "rules.write": {"audit_manager", "rule_engineer"}, "parameters.write": {"admin", "audit_manager", "rule_engineer"}, "assurance.read": {"auditor", "audit_manager"}}
	for _, have := range i.Roles {
		for _, want := range roles[operation] {
			if have == want {
				return true
			}
		}
	}
	return false
}

// Current authenticates the opaque cookie without trusting browser-supplied claims.
func (m *Manager) Current(r *http.Request) (Identity, bool) {
	cookie, e := r.Cookie("autodit_session")
	if e != nil {
		return Identity{}, false
	}
	m.mu.Lock()
	s, ok := m.sessions[cookie.Value]
	if !ok || !s.expires.After(m.Now()) {
		delete(m.sessions, cookie.Value)
		m.mu.Unlock()
		return Identity{}, false
	}
	m.mu.Unlock()
	if s.identity.LocalRevision > 0 {
		if m.Users == nil {
			return Identity{}, false
		}
		u, e := m.Users.User(r.Context(), s.identity.Subject)
		if e != nil || !u.Enabled || u.Revision != s.identity.LocalRevision {
			return Identity{}, false
		}
		s.identity.Roles = u.Roles
		s.identity.MustChange = u.MustChange
	}
	return s.identity, true
}

// Mutation protects authenticated changes against cross-site requests.
func (m *Manager) Mutation(r *http.Request, i Identity) bool {
	return r.Header.Get("Origin") == m.cfg.PublicURL && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(i.CSRF)) == 1
}

func (m *Manager) cookie(w http.ResponseWriter, name, value string, maxAge int) {
	cookie := &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge}
	origin, err := url.Parse(m.cfg.PublicURL)
	// The explicit local-only evaluation mode supports HTTP clients. OIDC and
	// every HTTPS deployment always retain Secure, regardless of request headers.
	if err == nil && (m.cfg.AuthMode == "demo" || m.cfg.AuthMode == "local") && origin.Scheme == "http" && (origin.Hostname() == "localhost" || origin.Hostname() == "127.0.0.1") {
		cookie.Secure = false
	}
	http.SetCookie(w, cookie)
}
func (m *Manager) establish(w http.ResponseWriter, i Identity, expires time.Time) {
	id := token()
	i.CSRF = token()
	m.mu.Lock()
	for k, s := range m.sessions {
		if !s.expires.After(m.Now()) {
			delete(m.sessions, k)
		}
	}
	if len(m.sessions) >= 10000 {
		m.mu.Unlock()
		http.Error(w, "session capacity reached", http.StatusServiceUnavailable)
		return
	}
	m.sessions[id] = session{identity: i, expires: expires}
	m.mu.Unlock()
	m.cookie(w, "autodit_session", id, int(expires.Sub(m.Now()).Seconds()))
}

// Session returns only the active identity and available authentication method.
func (m *Manager) Session(w http.ResponseWriter, r *http.Request) {
	i, ok := m.Current(r)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": ok, "identity": i, "mode": m.cfg.AuthMode, "login_url": "/auth/login"})
}

// DemoLogin is available only in explicit demo mode with a generated local secret.
func (m *Manager) DemoLogin(w http.ResponseWriter, r *http.Request) {
	if m.cfg.AuthMode != "demo" || r.Header.Get("Origin") != m.cfg.PublicURL {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if json.NewDecoder(r.Body).Decode(&input) != nil || subtle.ConstantTimeCompare([]byte(input.Token), []byte(m.cfg.DemoToken)) != 1 {
		http.Error(w, "invalid credential", http.StatusUnauthorized)
		return
	}
	m.establish(w, Identity{Subject: "demo-operator", TenantID: m.cfg.TenantID, Roles: []string{"audit_manager", "implementer", "rule_engineer"}}, m.Now().Add(8*time.Hour))
	w.WriteHeader(http.StatusNoContent)
}

// Login begins authorization code flow with state, nonce and PKCE.
func (m *Manager) Login(w http.ResponseWriter, r *http.Request) {
	if m.cfg.AuthMode != "oidc" {
		http.Error(w, "OIDC not configured", http.StatusNotFound)
		return
	}
	state := token()
	nonce := token()
	verifier := oauth2.GenerateVerifier()
	m.mu.Lock()
	for k, l := range m.logins {
		if !l.expires.After(m.Now()) {
			delete(m.logins, k)
		}
	}
	if len(m.logins) >= 1000 {
		m.mu.Unlock()
		http.Error(w, "login capacity reached", 503)
		return
	}
	m.logins[state] = login{verifier: verifier, nonce: nonce, expires: m.Now().Add(5 * time.Minute)}
	m.mu.Unlock()
	m.cookie(w, "autodit_login", state, 300)
	http.Redirect(w, r, m.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

// Callback verifies issuer, audience, expiry, nonce and the configured tenant claim.
func (m *Manager) Callback(w http.ResponseWriter, r *http.Request) {
	if m.verifier == nil {
		http.Error(w, "not configured", 404)
		return
	}
	state := r.URL.Query().Get("state")
	cookie, e := r.Cookie("autodit_login")
	if e != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		http.Error(w, "invalid login state", 401)
		return
	}
	m.mu.Lock()
	l, ok := m.logins[state]
	delete(m.logins, state)
	m.mu.Unlock()
	m.cookie(w, "autodit_login", "", -1)
	if !ok || !l.expires.After(m.Now()) {
		http.Error(w, "login expired", 401)
		return
	}
	t, e := m.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(l.verifier))
	if e != nil {
		http.Error(w, "identity exchange failed", 401)
		return
	}
	raw, ok := t.Extra("id_token").(string)
	if !ok {
		http.Error(w, "ID token missing", 401)
		return
	}
	id, e := m.verifier.Verify(r.Context(), raw)
	if e != nil || id.Nonce != l.nonce {
		http.Error(w, "identity verification failed", 401)
		return
	}
	var claims struct {
		Tenant string   `json:"autodit_tenant"`
		Roles  []string `json:"autodit_roles"`
	}
	if id.Claims(&claims) != nil || claims.Tenant != m.cfg.TenantID || len(claims.Roles) == 0 {
		http.Error(w, "workspace access not assigned", 403)
		return
	}
	expires := m.Now().Add(8 * time.Hour)
	if id.Expiry.Before(expires) {
		expires = id.Expiry
	}
	m.establish(w, Identity{Subject: id.Subject, TenantID: claims.Tenant, Roles: claims.Roles}, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout invalidates the session server-side.
func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) {
	i, ok := m.Current(r)
	if !ok || !m.Mutation(r, i) {
		http.Error(w, "forbidden", 403)
		return
	}
	cookie, _ := r.Cookie("autodit_session")
	m.mu.Lock()
	delete(m.sessions, cookie.Value)
	m.mu.Unlock()
	m.cookie(w, "autodit_session", "", -1)
	w.WriteHeader(204)
}
