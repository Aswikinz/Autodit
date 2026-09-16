package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Aswikinz/Autodit/internal/analysis"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SavedAnalysis keeps the input data and decision graph together in an immutable version.
type SavedAnalysis struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Revision        int               `json:"revision"`
	Dataset         analysis.Dataset  `json:"dataset"`
	SelectedColumns []analysis.Column `json:"selected_columns"`
	Model           json.RawMessage   `json:"model"`
	Origin          string            `json:"origin,omitempty"`
	SavedBy         string            `json:"saved_by,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at,omitempty"`
}

func (t *Tenant) Analyses(ctx context.Context, page int) ([]json.RawMessage, error) {
	if page < 1 || page > 100000 {
		return nil, domain.ErrInvalid
	}
	var out []json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `select jsonb_build_object('id',id,'name',name,'revision',revision,'row_count',(dataset->>'row_count')::int,'updated_at',updated_at,'saved_by',saved_by,'origin',origin) from (select distinct on (id) * from analysis_version where tenant_id=$1 order by id,revision desc) current_version order by updated_at desc,id limit 100 offset $2`, t.ID, (page-1)*100)
		if e != nil {
			return e
		}
		out, e = JSONRows(rows)
		return e
	})
	return out, e
}

func (t *Tenant) Analysis(ctx context.Context, id string, revision int) (SavedAnalysis, error) {
	var out SavedAnalysis
	if _, e := uuid.Parse(id); e != nil || revision < 0 {
		return out, domain.ErrInvalid
	}
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		var data, columns []byte
		e := tx.QueryRow(ctx, `select id,name,revision,dataset,selected_columns,model,origin,saved_by,updated_at from analysis_version where tenant_id=$1 and id=$2 and ($3=0 or revision=$3) order by revision desc limit 1`, t.ID, id, revision).Scan(&out.ID, &out.Name, &out.Revision, &data, &columns, &out.Model, &out.Origin, &out.SavedBy, &out.UpdatedAt)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if e = json.Unmarshal(data, &out.Dataset); e != nil {
			return e
		}
		return json.Unmarshal(columns, &out.SelectedColumns)
	})
	return out, e
}

func (t *Tenant) SaveAnalysis(ctx context.Context, in SavedAnalysis, actor string) (SavedAnalysis, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 || len(in.Origin) > 250 || in.Revision < 0 || strings.TrimSpace(actor) == "" {
		return in, domain.ErrInvalid
	}
	if e := analysis.ValidateDataset(in.Dataset); e != nil {
		return in, e
	}
	if _, e := analysis.SelectColumns(in.Dataset, in.SelectedColumns); e != nil {
		return in, e
	}
	if e := analysis.ValidateModel(in.Model); e != nil {
		return in, e
	}
	if in.ID == "" {
		if in.Revision != 0 {
			return in, domain.ErrInvalid
		}
		in.ID = uuid.NewString()
	} else if _, e := uuid.Parse(in.ID); e != nil {
		return in, domain.ErrInvalid
	}
	data, e := json.Marshal(in.Dataset)
	if e != nil {
		return in, domain.ErrInvalid
	}
	columns, e := json.Marshal(in.SelectedColumns)
	if e != nil {
		return in, domain.ErrInvalid
	}
	in.SavedBy = actor
	e = t.Tx(ctx, func(tx pgx.Tx) error {
		// Serialize version checks within this tenant; stale browser tabs cannot overwrite a revision.
		var locked string
		if e := tx.QueryRow(ctx, `select id from tenant where id=$1 for update`, t.ID).Scan(&locked); e != nil {
			return e
		}
		var current int
		if e := tx.QueryRow(ctx, `select coalesce(max(revision),0) from analysis_version where tenant_id=$1 and id=$2`, t.ID, in.ID).Scan(&current); e != nil {
			return e
		}
		if current != in.Revision {
			return ErrConflict
		}
		in.Revision++
		if e := tx.QueryRow(ctx, `insert into analysis_version(tenant_id,id,revision,name,dataset,selected_columns,model,origin,saved_by) values($1,$2,$3,$4,$5,$6,$7,$8,$9) returning updated_at`, t.ID, in.ID, in.Revision, in.Name, data, columns, in.Model, in.Origin, actor).Scan(&in.UpdatedAt); e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "analysis.save", in.ID)
	})
	return in, e
}
