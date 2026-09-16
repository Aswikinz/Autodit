// Package platform owns runtime configuration and safe operational logging.
package platform

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

// Config is populated only from environment and mounted secret files.
type Config struct{ DatabaseURL, TenantID, TenantName, Listen, PublicURL, AuthMode, DemoToken, AdminPassword, Rulepack, SnapshotDir, Issuer, ClientID, ClientSecret string }

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func secret(key string) (string, error) {
	if path := os.Getenv(key + "_FILE"); path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			return "", errors.New("required secret file unavailable")
		}
		return strings.TrimSpace(string(b)), nil
	}
	return os.Getenv(key), nil
}

// Load refuses missing credentials and insecure production identity configuration.
func Load() (Config, error) {
	c := Config{TenantID: value("AUTODIT_TENANT_ID", "11111111-1111-4111-8111-111111111111"), TenantName: value("AUTODIT_TENANT_NAME", "Autodit workspace"), Listen: value("AUTODIT_LISTEN", ":8080"), PublicURL: value("AUTODIT_PUBLIC_URL", "http://localhost:8088"), AuthMode: value("AUTODIT_AUTH_MODE", "oidc"), Rulepack: value("AUTODIT_RULEPACK", "rulepack"), SnapshotDir: value("AUTODIT_SNAPSHOT_DIR", ".autodit/snapshots"), Issuer: os.Getenv("AUTODIT_OIDC_ISSUER"), ClientID: os.Getenv("AUTODIT_OIDC_CLIENT_ID")}
	var e error
	c.DatabaseURL, e = secret("DATABASE_URL")
	if e != nil {
		return c, e
	}
	if c.DatabaseURL == "" {
		password, e := secret("POSTGRES_PASSWORD")
		if e != nil {
			return c, e
		}
		if password == "" {
			return c, errors.New("database credentials required")
		}
		u := url.URL{Scheme: "postgres", Host: value("POSTGRES_HOST", "postgres:5432"), Path: "/" + value("POSTGRES_DB", "autodit"), User: url.UserPassword(value("POSTGRES_USER", "autodit_app"), password)}
		q := u.Query()
		q.Set("sslmode", value("POSTGRES_SSLMODE", "disable"))
		u.RawQuery = q.Encode()
		c.DatabaseURL = u.String()
	}
	c.DemoToken, e = secret("AUTODIT_DEMO_TOKEN")
	if e != nil {
		return c, e
	}
	c.ClientSecret, e = secret("AUTODIT_OIDC_CLIENT_SECRET")
	if e != nil {
		return c, e
	}
	c.AdminPassword, e = secret("AUTODIT_ADMIN_PASSWORD")
	if e != nil {
		return c, e
	}
	return c, nil
}

// ValidateAPI checks security-sensitive API-only configuration.
func (c Config) ValidateAPI() error {
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("public URL must be an origin")
	}
	switch c.AuthMode {
	case "local":
		if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
			return errors.New("local accounts require HTTPS outside loopback")
		}
	case "demo":
		if len(c.DemoToken) < 32 {
			return errors.New("demo token must contain at least 32 characters")
		}
		if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
			return errors.New("HTTP demo mode requires a loopback public URL")
		}
	case "oidc":
		if c.Issuer == "" || c.ClientID == "" || u.Scheme != "https" {
			return errors.New("OIDC issuer, client ID and HTTPS public URL required")
		}
	default:
		return errors.New("unsupported authentication mode")
	}
	return nil
}
