package platform

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test.invalid/audit")
	t.Setenv("AUTODIT_AUTH_MODE", "demo")
	t.Setenv("AUTODIT_DEMO_TOKEN", strings.Repeat("x", 32))
	c, e := Load()
	if e != nil || c.ValidateAPI() != nil {
		t.Fatal("valid config failed", e)
	}
	c.PublicURL = "http://0.0.0.0:8088"
	if c.ValidateAPI() == nil {
		t.Fatal("exposed insecure demo accepted")
	}
	c.AuthMode = "oidc"
	if c.ValidateAPI() == nil {
		t.Fatal("incomplete OIDC accepted")
	}
	c.PublicURL = "https://audit.example"
	c.Issuer = "https://id.example"
	c.ClientID = "audit"
	if c.ValidateAPI() != nil {
		t.Fatal("valid OIDC rejected")
	}
	c.AuthMode = "unknown"
	if c.ValidateAPI() == nil {
		t.Fatal("unknown auth accepted")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "secret")
	_ = os.WriteFile(file, []byte(" mounted-value\n"), 0600)
	t.Setenv("TEST_SECRET_FILE", file)
	v, e := secret("TEST_SECRET")
	if e != nil || v != "mounted-value" {
		t.Fatal("secret load failed")
	}
	t.Setenv("TEST_SECRET_FILE", filepath.Join(dir, "absent"))
	if _, e = secret("TEST_SECRET"); e == nil {
		t.Fatal("missing secret accepted")
	}
}

func TestMountedDatabaseCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_URL_FILE", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("POSTGRES_PASSWORD_FILE", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing database credentials accepted")
	}
	path := filepath.Join(t.TempDir(), "password")
	password := "reserved:@/?#%+characters"
	if err := os.WriteFile(path, []byte(password), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POSTGRES_PASSWORD_FILE", path)
	t.Setenv("POSTGRES_HOST", "database:5432")
	t.Setenv("POSTGRES_USER", "audit")
	t.Setenv("POSTGRES_DB", "workspace")
	t.Setenv("POSTGRES_SSLMODE", "verify-full")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := u.User.Password()
	if got != password || u.User.Username() != "audit" || u.Host != "database:5432" || u.Path != "/workspace" || u.Query().Get("sslmode") != "verify-full" {
		t.Fatal("database credentials were not encoded correctly")
	}
}

func TestPublicOriginValidation(t *testing.T) {
	for _, origin := range []string{"https://", "https://audit.example/path", "https://audit.example?secret=value", "https://user:password@audit.example", "ftp://localhost"} {
		c := Config{AuthMode: "demo", DemoToken: strings.Repeat("x", 32), PublicURL: origin}
		if c.ValidateAPI() == nil {
			t.Errorf("invalid public origin accepted: %s", origin)
		}
	}
}
