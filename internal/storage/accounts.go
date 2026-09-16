package storage

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/jackc/pgx/v5"
)

// RoleDescriptions is also the permission guide displayed by the admin page.
var RoleDescriptions = map[string]string{
	"admin":         "Manage accounts, workspace settings and review workflows. Assign operational roles separately.",
	"implementer":   "Connect data sources, preview records, map fields and start audit runs.",
	"rule_engineer": "Edit decision graphs, simulate rules and release rule versions.",
	"auditor":       "Review findings, add evidence notes and complete assigned review steps.",
	"audit_manager": "Manage findings, approve case steps, release rules and view assurance reports.",
}
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._@-]{1,79}$`)

type User struct {
	Username     string   `json:"username"`
	DisplayName  string   `json:"display_name"`
	Roles        []string `json:"roles"`
	Enabled      bool     `json:"enabled"`
	MustChange   bool     `json:"must_change_password"`
	Revision     int      `json:"revision"`
	PasswordHash string   `json:"-"`
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 256 {
		return "", domain.ErrInvalid
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key, e := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	if e != nil {
		return "", e
	}
	return "pbkdf2-sha256$600000$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(key), nil
}
func CheckPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || parts[1] != "600000" || len(password) > 256 {
		return false
	}
	salt, e := hex.DecodeString(parts[2])
	if e != nil || len(salt) != 16 {
		return false
	}
	want, e := hex.DecodeString(parts[3])
	if e != nil || len(want) != 32 {
		return false
	}
	got, e := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	return e == nil && subtle.ConstantTimeCompare(want, got) == 1
}
func (t *Tenant) User(ctx context.Context, username string) (User, error) {
	var u User
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select username,display_name,roles,enabled,must_change_password,revision,password_hash from local_user where tenant_id=$1 and username=$2`, t.ID, username).Scan(&u.Username, &u.DisplayName, &u.Roles, &u.Enabled, &u.MustChange, &u.Revision, &u.PasswordHash)
	})
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return u, e
}
func (t *Tenant) Users(ctx context.Context) ([]User, error) {
	out := []User{}
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `select username,display_name,roles,enabled,must_change_password,revision from local_user where tenant_id=$1 order by username`, t.ID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var u User
			if e = rows.Scan(&u.Username, &u.DisplayName, &u.Roles, &u.Enabled, &u.MustChange, &u.Revision); e != nil {
				return e
			}
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, e
}
func (t *Tenant) BootstrapAdmin(ctx context.Context, password string) error {
	if _, e := t.User(ctx, "admin"); e == nil {
		return nil
	} else if !errors.Is(e, ErrNotFound) {
		return e
	}
	hash, e := HashPassword(password)
	if e != nil {
		return e
	}
	return t.Tx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `insert into local_user(tenant_id,username,display_name,password_hash,roles) values($1,'admin','Administrator',$2,array['admin']) on conflict do nothing`, t.ID, hash)
		return e
	})
}
func (t *Tenant) SaveUser(ctx context.Context, u User, password, actor string) error {
	if !usernamePattern.MatchString(u.Username) || len(strings.TrimSpace(u.DisplayName)) < 1 || len(u.DisplayName) > 100 || len(u.Roles) < 1 || len(u.Roles) > len(RoleDescriptions) {
		return domain.ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range u.Roles {
		if _, ok := RoleDescriptions[r]; !ok || seen[r] {
			return domain.ErrInvalid
		}
		seen[r] = true
	}
	hash := ""
	var e error
	if password != "" {
		hash, e = HashPassword(password)
		if e != nil {
			return e
		}
	}
	if u.Revision == 0 && hash == "" {
		return domain.ErrInvalid
	}
	return t.Tx(ctx, func(tx pgx.Tx) error {
		// Serializing account changes prevents two admins from simultaneously removing the last admin.
		if _, e := tx.Exec(ctx, `select id from tenant where id=$1 for update`, t.ID); e != nil {
			return e
		}
		if u.Revision == 0 {
			tag, e := tx.Exec(ctx, `insert into local_user(tenant_id,username,display_name,password_hash,roles,enabled) values($1,$2,$3,$4,$5,$6) on conflict do nothing`, t.ID, u.Username, u.DisplayName, hash, u.Roles, u.Enabled)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return ErrConflict
			}
		} else {
			if !u.Enabled || !slices.Contains(u.Roles, "admin") {
				var count int
				if e := tx.QueryRow(ctx, `select count(*) from local_user where tenant_id=$1 and username<>$2 and enabled and 'admin'=any(roles)`, t.ID, u.Username).Scan(&count); e != nil {
					return e
				}
				var wasAdmin bool
				if e := tx.QueryRow(ctx, `select enabled and 'admin'=any(roles) from local_user where tenant_id=$1 and username=$2`, t.ID, u.Username).Scan(&wasAdmin); e != nil {
					return e
				}
				if wasAdmin && count == 0 {
					return domain.ErrInvalid
				}
			}
			tag, e := tx.Exec(ctx, `update local_user set display_name=$3,roles=$4,enabled=$5,password_hash=case when $6='' then password_hash else $6 end,must_change_password=case when $6='' then must_change_password else true end,revision=revision+1 where tenant_id=$1 and username=$2 and revision=$7`, t.ID, u.Username, u.DisplayName, u.Roles, u.Enabled, hash, u.Revision)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return ErrConflict
			}
		}
		return t.Audit(ctx, tx, actor, "user.save", u.Username)
	})
}
func (t *Tenant) ChangePassword(ctx context.Context, username, old, next string) error {
	u, e := t.User(ctx, username)
	if e != nil || !u.Enabled || !CheckPassword(u.PasswordHash, old) || old == next {
		return domain.ErrInvalid
	}
	hash, e := HashPassword(next)
	if e != nil {
		return e
	}
	return t.Tx(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `update local_user set password_hash=$3,must_change_password=false,revision=revision+1 where tenant_id=$1 and username=$2 and revision=$4`, t.ID, username, hash, u.Revision)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
		return t.Audit(ctx, tx, username, "user.password", username)
	})
}
