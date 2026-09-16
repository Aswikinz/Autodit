package storage

import (
	"context"
	"encoding/json"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/language"
	"strings"
	"time"
	_ "time/tzdata"
)

type WorkspaceSettings struct {
	Name              string `json:"name"`
	Timezone          string `json:"timezone"`
	Locale            string `json:"locale"`
	ReportingCurrency string `json:"reporting_currency"`
	FiscalStartMonth  int    `json:"fiscal_start_month"`
}

func (t *Tenant) Workspace(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select jsonb_build_object('revision',coalesce(s.revision,0),'settings',coalesce(s.content,jsonb_build_object('name',t.name,'timezone','','locale','','reporting_currency','','fiscal_start_month',0))) from tenant t left join workspace_setting s on s.tenant_id=t.id where t.id=$1`, t.ID).Scan(&out)
	})
	return out, e
}
func (t *Tenant) SaveWorkspace(ctx context.Context, s WorkspaceSettings, revision int, actor string) error {
	if len(strings.TrimSpace(s.Name)) < 1 || len(s.Name) > 100 || s.FiscalStartMonth < 1 || s.FiscalStartMonth > 12 || len(s.ReportingCurrency) != 3 || s.ReportingCurrency != strings.ToUpper(s.ReportingCurrency) {
		return domain.ErrInvalid
	}
	for _, r := range s.ReportingCurrency {
		if r < 'A' || r > 'Z' {
			return domain.ErrInvalid
		}
	}
	if s.Timezone == "" || s.Locale == "" {
		return domain.ErrInvalid
	}
	if _, e := time.LoadLocation(s.Timezone); e != nil {
		return domain.ErrInvalid
	}
	if _, e := language.Parse(s.Locale); e != nil {
		return domain.ErrInvalid
	}
	content, _ := json.Marshal(s)
	return t.Tx(ctx, func(tx pgx.Tx) error {
		var e error
		if revision == 0 {
			tag, err := tx.Exec(ctx, `insert into workspace_setting(tenant_id,content) values($1,$2) on conflict do nothing`, t.ID, content)
			e = err
			if e == nil && tag.RowsAffected() != 1 {
				return ErrConflict
			}
		} else {
			tag, err := tx.Exec(ctx, `update workspace_setting set content=$2,revision=revision+1 where tenant_id=$1 and revision=$3`, t.ID, content, revision)
			e = err
			if e == nil && tag.RowsAffected() != 1 {
				return ErrConflict
			}
		}
		if e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "workspace.settings", t.ID)
	})
}
