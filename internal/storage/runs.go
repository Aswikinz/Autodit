package storage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Submit freezes the population, parameters and rule manifest before queueing work.
func (t *Tenant) Submit(ctx context.Context, p domain.Population, actor string) (string, error) {
	return t.submit(ctx, p, actor, "")
}

// SubmitInbox deduplicates a landed file transactionally across worker restarts.
func (t *Tenant) SubmitInbox(ctx context.Context, p domain.Population, receipt string) (string, error) {
	if len(receipt) != 64 {
		return "", domain.ErrInvalid
	}
	return t.submit(ctx, p, "scheduled-file-extract", receipt)
}

func (t *Tenant) submit(ctx context.Context, p domain.Population, actor, receipt string) (string, error) {
	if e := p.Validate(); e != nil {
		return "", e
	}
	id := uuid.NewString()
	b, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	e = t.Tx(ctx, func(tx pgx.Tx) error {
		if receipt != "" {
			if _, e := tx.Exec(ctx, "select pg_advisory_xact_lock(hashtextextended($1,1))", t.ID+receipt); e != nil {
				return e
			}
			var existingID string
			err := tx.QueryRow(ctx, "select run_id::text from ingestion_receipt where tenant_id=$1 and receipt_hash=$2", t.ID, receipt).Scan(&existingID)
			if err == nil {
				id = existingID
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		var existing bool
		if e := tx.QueryRow(ctx, "select exists(select 1 from source where tenant_id=$1 and id=$2)", t.ID, p.SourceID).Scan(&existing); e != nil {
			return e
		}
		if !existing {
			return ErrNotFound
		}
		if _, e := tx.Exec(ctx, "insert into fiscal_period(tenant_id,id,start_date,end_date) values($1,$2,$3,$4) on conflict do nothing", t.ID, p.Period.ID, p.Period.Start, p.Period.End); e != nil {
			return e
		}
		var start, end string
		if e := tx.QueryRow(ctx, "select start_date::text,end_date::text from fiscal_period where tenant_id=$1 and id=$2", t.ID, p.Period.ID).Scan(&start, &end); e != nil {
			return e
		}
		if start != p.Period.Start || end != p.Period.End {
			return ErrConflict
		}
		var params string
		var manifest json.RawMessage
		if e := tx.QueryRow(ctx, "select parameter_hash from tenant_setting where tenant_id=$1", t.ID).Scan(&params); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, "select coalesce(jsonb_object_agg(rule_id,version),'{}'::jsonb) from rule_release where tenant_id=$1 and enabled", t.ID).Scan(&manifest); e != nil {
			return e
		}
		if string(manifest) == "{}" {
			return domain.ErrInvalid
		}
		_, e := tx.Exec(ctx, "insert into audit_run(tenant_id,id,source_id,period_id,status,input,input_hash,parameter_set_hash,rule_manifest,submitted_by,record_count) values($1,$2,$3,$4,'queued',$5,$6,$7,$8,$9,$10)", t.ID, id, p.SourceID, p.Period.ID, b, domain.Hash(b), params, manifest, actor, len(p.Records))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "update source set last_landed_at=now() where tenant_id=$1 and id=$2", t.ID, p.SourceID)
		if e != nil {
			return e
		}
		if receipt != "" {
			if _, e = tx.Exec(ctx, "insert into ingestion_receipt(tenant_id,receipt_hash,run_id) values($1,$2,$3)", t.ID, receipt, id); e != nil {
				return e
			}
		}
		return t.Audit(ctx, tx, actor, "run.submit", id)
	})
	return id, e
}

// Runs lists metadata without returning original source populations.
func (t *Tenant) Runs(ctx context.Context) ([]json.RawMessage, error) {
	var out []json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `select jsonb_build_object('id',id,'source_id',source_id,'period_id',period_id,'status',status,'stage',stage,'record_count',record_count,'exception_count',exception_count,'snapshot_id',snapshot_id,'created_at',created_at,'completed_at',completed_at,'error_code',error_code,'differences',differences) from audit_run where tenant_id=$1 order by created_at desc,id desc limit 100`, t.ID)
		if e != nil {
			return e
		}
		out, e = JSONRows(rows)
		return e
	})
	return out, e
}

// Run returns an authorized operational record and its stage history.
func (t *Tenant) Run(ctx context.Context, id string) (json.RawMessage, error) {
	var out json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `select jsonb_build_object('id',r.id,'source_id',r.source_id,'period_id',r.period_id,'status',r.status,'stage',r.stage,'record_count',r.record_count,'exception_count',r.exception_count,'snapshot_id',r.snapshot_id,'created_at',r.created_at,'completed_at',r.completed_at,'error_code',r.error_code,'differences',r.differences,'stages',coalesce((select jsonb_agg(jsonb_build_object('stage',s.stage,'status',s.status,'at',s.created_at) order by s.created_at) from run_stage s where s.tenant_id=r.tenant_id and s.run_id=r.id),'[]')) from audit_run r where r.tenant_id=$1 and r.id=$2`, t.ID, id).Scan(&out)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return e
	})
	return out, e
}
