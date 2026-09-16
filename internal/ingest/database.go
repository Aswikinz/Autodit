package ingest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Connection is request scoped. Credentials are never persisted or returned.
type Connection struct {
	Kind           string `json:"kind"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Database       string `json:"database"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	AllowPlaintext bool   `json:"allow_plaintext"`
}
type Table struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}
type Preview struct {
	Columns   []string   `json:"columns"`
	Sample    [][]string `json:"sample"`
	Truncated bool       `json:"truncated"`
}

func (c Connection) open() (*sql.DB, error) {
	if c.Host == "" || strings.ContainsAny(c.Host, "/\\@?# \t\r\n") || len(c.Host) > 253 || c.Port < 1 || c.Port > 65535 || c.Database == "" || len(c.Database) > 128 || len(c.Username) > 128 || len(c.Password) > 1024 {
		return nil, domain.ErrInvalid
	}
	host := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	driver := ""
	dsn := ""
	switch c.Kind {
	case "postgres":
		driver = "pgx"
		u := url.URL{Scheme: "postgres", Host: host, User: url.UserPassword(c.Username, c.Password), Path: "/" + c.Database}
		q := url.Values{"sslmode": {"verify-full"}, "connect_timeout": {"5"}}
		if c.AllowPlaintext {
			q.Set("sslmode", "disable")
		}
		u.RawQuery = q.Encode()
		dsn = u.String()
	case "mysql":
		driver = "mysql"
		conf := mysql.NewConfig()
		conf.User = c.Username
		conf.Passwd = c.Password
		conf.Net = "tcp"
		conf.Addr = host
		conf.DBName = c.Database
		conf.Timeout = 5 * time.Second
		conf.ReadTimeout = 8 * time.Second
		conf.WriteTimeout = 8 * time.Second
		conf.TLSConfig = "true"
		if c.AllowPlaintext {
			conf.TLSConfig = "false"
		}
		dsn = conf.FormatDSN()
	case "sqlserver":
		driver = "sqlserver"
		u := url.URL{Scheme: "sqlserver", Host: host, User: url.UserPassword(c.Username, c.Password)}
		q := url.Values{"database": {c.Database}, "encrypt": {"true"}, "TrustServerCertificate": {"false"}, "connection timeout": {"5"}}
		if c.AllowPlaintext {
			q.Set("encrypt", "disable")
		}
		u.RawQuery = q.Encode()
		dsn = u.String()
	default:
		return nil, domain.ErrInvalid
	}
	db, e := sql.Open(driver, dsn)
	if e != nil {
		return nil, errors.New("invalid connection settings")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return db, nil
}
func tableQuery(kind string) string {
	switch kind {
	case "mysql":
		return "select table_schema,table_name from information_schema.tables where table_schema=database() order by table_schema,table_name limit 500"
	case "sqlserver":
		return "select top (500) table_schema,table_name from information_schema.tables order by table_schema,table_name"
	default:
		return "select table_schema,table_name from information_schema.tables where table_schema not in ('pg_catalog','information_schema') order by table_schema,table_name limit 500"
	}
}
func (c Connection) Tables(ctx context.Context) ([]Table, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	db, e := c.open()
	if e != nil {
		return nil, e
	}
	defer db.Close()
	if e = db.PingContext(ctx); e != nil {
		return nil, errors.New("connection failed; check host, credentials, TLS and network access")
	}
	rows, e := db.QueryContext(ctx, tableQuery(c.Kind))
	if e != nil {
		return nil, errors.New("connected, but table discovery was denied")
	}
	defer rows.Close()
	out := []Table{}
	for rows.Next() {
		var t Table
		if e = rows.Scan(&t.Schema, &t.Name); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func quotedIdentifier(kind, value string) string {
	if kind == "mysql" {
		return "`" + strings.ReplaceAll(value, "`", "``") + "`"
	}
	if kind == "sqlserver" {
		return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
func (c Connection) Preview(ctx context.Context, table Table) (Preview, error) {
	return c.read(ctx, table, 50, true)
}

// Read imports a complete bounded table; it never represents a truncated preview
// as a dataset. Credentials remain confined to the request.
func (c Connection) Read(ctx context.Context, table Table) (Preview, error) {
	return c.read(ctx, table, 10000, false)
}

func (c Connection) read(ctx context.Context, table Table, limit int, preview bool) (Preview, error) {
	var out Preview
	tables, e := c.Tables(ctx)
	if e != nil {
		return out, e
	}
	allowed := false
	for _, t := range tables {
		if t == table {
			allowed = true
			break
		}
	}
	if !allowed {
		return out, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	db, e := c.open()
	if e != nil {
		return out, e
	}
	defer db.Close()
	name := quotedIdentifier(c.Kind, table.Schema) + "." + quotedIdentifier(c.Kind, table.Name)
	query := "select * from " + name + " limit " + strconv.Itoa(limit+1)
	if c.Kind == "sqlserver" {
		query = "select top (" + strconv.Itoa(limit+1) + ") * from " + name
	}
	// Query text comes only from a discovered, quoted identifier and a fixed row limit.
	rows, e := db.QueryContext(ctx, query)
	if e != nil {
		return out, errors.New("preview failed; verify read permission and column types")
	}
	defer rows.Close()
	out.Columns, e = rows.Columns()
	if e != nil {
		return out, e
	}
	if len(out.Columns) > 200 {
		return out, errors.New("preview supports up to 200 columns")
	}
	out.Sample = [][]string{}
	bytesRead := 0
	for rows.Next() {
		if len(out.Sample) == limit {
			if !preview {
				return Preview{}, errors.New("table exceeds 10,000 rows; select a smaller source view")
			}
			out.Truncated = true
			break
		}
		values := make([]any, len(out.Columns))
		targets := make([]any, len(values))
		for i := range values {
			targets[i] = &values[i]
		}
		if e = rows.Scan(targets...); e != nil {
			return out, e
		}
		row := make([]string, len(values))
		for i, v := range values {
			switch val := v.(type) {
			case nil:
				row[i] = ""
			case []byte:
				row[i] = string(val)
			case time.Time:
				row[i] = val.Format(time.RFC3339Nano)
			default:
				row[i] = fmt.Sprint(val)
			}
			if preview && len(row[i]) > 4096 {
				row[i] = row[i][:4096] + "..."
			}
			bytesRead += len(row[i])
			if bytesRead > 16*1024*1024 {
				return Preview{}, errors.New("table exceeds 16 MiB; select a smaller source view")
			}
		}
		out.Sample = append(out.Sample, row)
	}
	return out, rows.Err()
}
