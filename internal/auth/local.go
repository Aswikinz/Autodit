package auth

import (
	"encoding/json"
	"github.com/Aswikinz/Autodit/internal/storage"
	"net/http"
	"strings"
	"time"
)

type attempt struct {
	count int
	until time.Time
}

func (m *Manager) TenantID() string { return m.cfg.TenantID }
func (m *Manager) permitLogin(username string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attempts == nil {
		m.attempts = map[string]attempt{}
	}
	for k, a := range m.attempts {
		if !a.until.After(m.Now()) {
			delete(m.attempts, k)
		}
	}
	a := m.attempts[username]
	if a.count >= 8 || len(m.attempts) >= 1000 {
		return false
	}
	if a.count == 0 {
		a.until = m.Now().Add(5 * time.Minute)
	}
	a.count++
	m.attempts[username] = a
	return true
}
func (m *Manager) PasswordLogin(w http.ResponseWriter, r *http.Request) {
	if m.cfg.AuthMode != "local" || m.Users == nil || r.Header.Get("Origin") != m.cfg.PublicURL {
		http.Error(w, "forbidden", 403)
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		http.Error(w, "invalid credential", 401)
		return
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	if !m.permitLogin(input.Username) {
		http.Error(w, "Too many attempts. Try again in five minutes.", 429)
		return
	}
	u, e := m.Users.User(r.Context(), input.Username)
	// A missing account still pays the same derivation cost as an existing one.
	hash := u.PasswordHash
	if e != nil {
		hash = "pbkdf2-sha256$600000$00000000000000000000000000000000$0000000000000000000000000000000000000000000000000000000000000000"
	}
	valid := storage.CheckPassword(hash, input.Password)
	if e != nil || !u.Enabled || !valid {
		http.Error(w, "invalid credential", 401)
		return
	}
	m.mu.Lock()
	delete(m.attempts, input.Username)
	m.mu.Unlock()
	m.establish(w, Identity{Subject: u.Username, TenantID: m.Users.ID, Roles: u.Roles, MustChange: u.MustChange, LocalRevision: u.Revision}, m.Now().Add(8*time.Hour))
	w.WriteHeader(204)
}
func (m *Manager) PasswordChange(w http.ResponseWriter, r *http.Request) {
	i, ok := m.Current(r)
	if !ok || i.LocalRevision == 0 || !m.Mutation(r, i) {
		http.Error(w, "forbidden", 403)
		return
	}
	var input struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if json.NewDecoder(r.Body).Decode(&input) != nil || !m.permitLogin(i.Subject+":password") {
		http.Error(w, "invalid request or too many attempts", 400)
		return
	}
	if e := m.Users.ChangePassword(r.Context(), i.Subject, input.Current, input.Next); e != nil {
		http.Error(w, "Use the current password and a different new password of 12 to 256 characters.", 400)
		return
	}
	cookie, _ := r.Cookie("autodit_session")
	m.mu.Lock()
	delete(m.sessions, cookie.Value)
	m.mu.Unlock()
	m.cookie(w, "autodit_session", "", -1)
	w.WriteHeader(204)
}
