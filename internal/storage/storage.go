// Package storage centralizes tenant-scoped PostgreSQL transactions.
package storage

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrConflict signals an optimistic locking conflict or immutable definition mismatch.
var ErrConflict = errors.New("state changed; refresh and retry")

// ErrNotFound hides both absent and inaccessible resources.
var ErrNotFound = errors.New("resource not found")

// Store has no unscoped query methods; all application access uses ForTenant.
type Store struct{ pool *pgxpool.Pool }

// Tenant is a validated tenant-bound storage handle.
type Tenant struct {
	store *Store
	ID    string
}

// Open connects without logging connection strings or credentials.
func Open(ctx context.Context, url string) (*Store, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, errors.New("database configuration invalid")
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, errors.New("database unavailable")
	}
	return &Store{pool: p}, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// Ping is a real readiness check.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// ForTenant validates a tenant identity before exposing any application query.
func (s *Store) ForTenant(id string) (*Tenant, error) {
	if _, e := uuid.Parse(id); e != nil {
		return nil, domain.ErrInvalid
	}
	return &Tenant{store: s, ID: id}, nil
}

// Tx guarantees tenant context is transaction-local, including pooled connections.
func (t *Tenant) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, e := t.store.pool.Begin(ctx)
	if e != nil {
		return errors.New("database transaction unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, e = tx.Exec(ctx, "select set_config('app.tenant_id',$1,true)", t.ID); e != nil {
		return errors.New("tenant context unavailable")
	}
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// Migrate runs forward-only embedded migrations using a separate owner connection.
func Migrate(ctx context.Context, url string) error {
	s, e := Open(ctx, url)
	if e != nil {
		return e
	}
	defer s.Close()
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, e = tx.Exec(ctx, "select pg_advisory_xact_lock(7420382)"); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, "select to_regclass('public.schema_version') is not null").Scan(&exists); e != nil {
		return e
	}
	version := 0
	if exists {
		if e = tx.QueryRow(ctx, "select coalesce(max(version),0) from schema_version").Scan(&version); e != nil {
			return e
		}
	}
	files, e := fs.Glob(migrations, "migrations/*.sql")
	if e != nil {
		return e
	}
	sort.Strings(files)
	if version > len(files) {
		return errors.New("database schema is newer than this application")
	}
	for _, name := range files {
		base := strings.TrimPrefix(name, "migrations/")
		n, _ := strconv.Atoi(base[:4])
		if n <= version {
			continue
		}
		b, e := migrations.ReadFile(name)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, string(b)); e != nil {
			return fmt.Errorf("migration %04d failed: %w", n, e)
		}
	}
	// The owner is used only by this command. Runtime credentials belong to a
	// separate, non-superuser role created by the PostgreSQL initialization script.
	var appExists bool
	if e = tx.QueryRow(ctx, "select exists(select 1 from pg_roles where rolname='autodit_app')").Scan(&appExists); e != nil {
		return e
	}
	if appExists {
		_, e = tx.Exec(ctx, `grant usage on schema public to autodit_app; grant select,insert,update on all tables in schema public to autodit_app; revoke update on snapshot,canonical_record,observation,exception_event,audit_event,run_stage,rule_version,parameter_set,invoice,vendor,employee,access_grant from autodit_app; grant usage,select on all sequences in schema public to autodit_app; grant select on schema_version to autodit_app`)
		if e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

// Bootstrap initializes an isolated tenant and immutable shipped rule versions.
func (s *Store) Bootstrap(ctx context.Context, id, name string, models []rules.Model) error {
	return s.bootstrap(ctx, id, name, models, rules.Defaults())
}

// BootstrapLocal starts with no currency policy. An administrator must configure it.
func (s *Store) BootstrapLocal(ctx context.Context, id, name string, models []rules.Model) error {
	p := rules.Defaults()
	p.ExplicitCurrencies = true
	p.Materiality = "0"
	p.CurrencyThresholds = map[string]string{}
	return s.bootstrap(ctx, id, name, models, p)
}
func (s *Store) bootstrap(ctx context.Context, id, name string, models []rules.Model, parameters rules.Parameters) error {
	t, e := s.ForTenant(id)
	if e != nil {
		return e
	}
	return t.Tx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "insert into tenant(id,name) values($1,$2) on conflict do nothing", id, name); e != nil {
			return e
		}
		params, _ := json.Marshal(parameters)
		hash := domain.Hash(params)
		if _, e := tx.Exec(ctx, "insert into parameter_set(tenant_id,hash,content) values($1,$2,$3) on conflict do nothing", id, hash, params); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "insert into tenant_setting(tenant_id,parameter_hash) values($1,$2) on conflict do nothing", id, hash); e != nil {
			return e
		}
		for _, m := range models {
			if _, e := tx.Exec(ctx, "insert into rule_version(rule_id,version,model,title) values($1,$2,$3,$4) on conflict do nothing", m.ID, m.Version, m.Content, m.Title); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, "insert into rule_release(tenant_id,rule_id,version,released_by) values($1,$2,$3,'bootstrap') on conflict do nothing", id, m.ID, m.Version); e != nil {
				return e
			}
		}
		return nil
	})
}

// Audit records successful mutating actions in durable storage.
func (t *Tenant) Audit(ctx context.Context, tx pgx.Tx, actor, action, target string) error {
	_, e := tx.Exec(ctx, "insert into audit_event(tenant_id,id,actor,action,target_id) values($1,$2,$3,$4,$5)", t.ID, uuid.NewString(), actor, action, target)
	return e
}

// JSONRows decodes database-produced objects without converting exact money to floats.
func JSONRows(rows pgx.Rows) ([]json.RawMessage, error) {
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b json.RawMessage
		if e := rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
