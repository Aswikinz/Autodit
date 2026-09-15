package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/jackc/pgx/v5"
)

// SourceConfig controls a file source and its expected delivery interval.
type SourceConfig struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	IntervalMinutes int               `json:"interval_minutes"`
	Mapping         map[string]string `json:"mapping"`
}

// SaveSource registers a stable source identity and declared extract schedule.
func (t *Tenant) SaveSource(ctx context.Context, c SourceConfig, actor string) error {
	if !domain.ValidID(c.ID) || len(strings.TrimSpace(c.Name)) < 1 || len(c.Name) > 100 || c.IntervalMinutes < 1 || c.IntervalMinutes > 525600 {
		return domain.ErrInvalid
	}
	if len(c.Mapping) > 30 {
		return domain.ErrInvalid
	}
	mapping, _ := json.Marshal(c.Mapping)
	return t.Tx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, "insert into source(tenant_id,id,name,interval_minutes,mapping) values($1,$2,$3,$4,$5) on conflict(tenant_id,id) do update set name=excluded.name,interval_minutes=excluded.interval_minutes,mapping=excluded.mapping", t.ID, c.ID, c.Name, c.IntervalMinutes, mapping)
		if e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "source.configure", c.ID)
	})
}

// Sources reports data freshness and missing expected successful runs.
func (t *Tenant) Sources(ctx context.Context) ([]json.RawMessage, error) {
	var out []json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `select jsonb_build_object('id',s.id,'name',s.name,'kind',s.kind,'mapping',s.mapping,'interval_minutes',s.interval_minutes,'last_landed_at',s.last_landed_at,'watermark',s.watermark,'last_completed_at',(select max(r.completed_at) from audit_run r where r.tenant_id=s.tenant_id and r.source_id=s.id and r.status='completed'),'overdue',coalesce((select max(r.completed_at) from audit_run r where r.tenant_id=s.tenant_id and r.source_id=s.id and r.status='completed'),s.created_at)<now()-make_interval(mins=>s.interval_minutes)) from source s where tenant_id=$1 order by s.name,s.id`, t.ID)
		if e != nil {
			return e
		}
		out, e = JSONRows(rows)
		return e
	})
	return out, e
}

// Settings returns the current immutable parameter set with an edit revision.
func (t *Tenant) Settings(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select jsonb_build_object('revision',s.revision,'hash',s.parameter_hash,'parameters',p.content) from tenant_setting s join parameter_set p on p.tenant_id=s.tenant_id and p.hash=s.parameter_hash where s.tenant_id=$1`, t.ID).Scan(&out)
	})
	return out, e
}

// SaveParameters creates a new version and leaves prior runs bound to their old version.
func (t *Tenant) SaveParameters(ctx context.Context, p rules.Parameters, revision int, actor string) error {
	if e := p.Validate(); e != nil {
		return e
	}
	content, _ := json.Marshal(p)
	hash := domain.Hash(content)
	return t.Tx(ctx, func(tx pgx.Tx) error {
		var current int
		if e := tx.QueryRow(ctx, "select revision from tenant_setting where tenant_id=$1 for update", t.ID).Scan(&current); e != nil {
			return e
		}
		if current != revision {
			return ErrConflict
		}
		if _, e := tx.Exec(ctx, "insert into parameter_set(tenant_id,hash,content) values($1,$2,$3) on conflict do nothing", t.ID, hash, content); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "update tenant_setting set parameter_hash=$2,revision=revision+1 where tenant_id=$1", t.ID, hash); e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "parameters.release", hash)
	})
}

// Catalog includes immutable models, active revisions, exception rates and precision.
func (t *Tenant) Catalog(ctx context.Context) ([]json.RawMessage, error) {
	var out []json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `select jsonb_build_object('rule_id',r.rule_id,'version',r.version,'enabled',r.enabled,'revision',r.revision,'title',v.title,'model',v.model,'released_at',r.released_at,'exceptions',(select count(*) from exception e where e.tenant_id=r.tenant_id and e.rule_id=r.rule_id),'accepted',(select count(*) from exception e where e.tenant_id=r.tenant_id and e.rule_id=r.rule_id and e.state='accepted'),'dismissed',(select count(*) from exception e where e.tenant_id=r.tenant_id and e.rule_id=r.rule_id and e.state='dismissed')) from rule_release r join rule_version v on v.rule_id=r.rule_id and v.version=r.version where r.tenant_id=$1 order by r.rule_id`, t.ID)
		if e != nil {
			return e
		}
		out, e = JSONRows(rows)
		return e
	})
	return out, e
}

// Simulate validates the graph and checks immutable positive, negative and null fixtures.
func Simulate(model []byte) ([]rules.Evaluation, error) {
	if e := rules.ValidateModel(model); e != nil {
		return nil, e
	}
	out := []rules.Evaluation{}
	for _, tc := range []struct {
		value any
		want  bool
	}{{true, true}, {false, false}, {nil, false}} {
		ev, e := rules.Evaluate(model, map[string]any{"qualifies": tc.value})
		if e != nil {
			return nil, e
		}
		out = append(out, ev)
		if ev.Result.Flag != tc.want {
			return out, errors.New("required simulation fixture failed")
		}
	}
	return out, nil
}

// Release validates all fixtures in the same request that promotes an immutable version.
func (t *Tenant) Release(ctx context.Context, id string, model json.RawMessage, enabled bool, revision int, actor string) error {
	if _, e := Simulate(model); e != nil {
		return e
	}
	var graph any
	if json.Unmarshal(model, &graph) != nil {
		return domain.ErrInvalid
	}
	canonical, _ := json.Marshal(graph)
	hash := domain.Hash(canonical)
	return t.Tx(ctx, func(tx pgx.Tx) error {
		var current int
		var title string
		e := tx.QueryRow(ctx, "select r.revision,v.title from rule_release r join rule_version v on v.rule_id=r.rule_id and v.version=r.version where r.tenant_id=$1 and r.rule_id=$2 for update of r", t.ID, id).Scan(&current, &title)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if current != revision {
			return ErrConflict
		}
		if _, e = tx.Exec(ctx, "insert into rule_version(rule_id,version,model,title) values($1,$2,$3,$4) on conflict do nothing", id, hash, canonical, title); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "update rule_release set version=$3,enabled=$4,revision=revision+1,released_by=$5,released_at=now() where tenant_id=$1 and rule_id=$2", t.ID, id, hash, enabled, actor); e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "rule.release", id+":"+hash)
	})
}

// Assurance summarizes coverage, queue aging and disposition mix without implying untested controls passed.
func (t *Tenant) Assurance(ctx context.Context) (json.RawMessage, error) {
	var out json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select jsonb_build_object('states',coalesce((select jsonb_object_agg(state,n) from (select state,count(*) n from exception where tenant_id=$1 group by state) s),'{}'),'severities',coalesce((select jsonb_object_agg(severity,n) from (select severity,count(*) n from exception where tenant_id=$1 and state in ('open','in_review','reopened') group by severity) s),'{}'),'completed_runs',(select count(*) from audit_run where tenant_id=$1 and status='completed'),'failed_runs',(select count(*) from audit_run where tenant_id=$1 and status in ('failed','tieout_failed','ceiling_exceeded')),'tested_records',coalesce((select sum(record_count) from audit_run where tenant_id=$1 and status='completed'),0),'overdue_exceptions',(select count(*) from exception where tenant_id=$1 and state in ('open','in_review','reopened') and first_seen_at<now()-interval '30 days'),'trend',coalesce((select jsonb_agg(x order by x.day) from (select created_at::date::text as day,count(*) runs,sum(exception_count) exceptions from audit_run where tenant_id=$1 and status='completed' group by created_at::date order by created_at::date desc limit 30) x),'[]'))`, t.ID).Scan(&out)
	})
	return out, e
}
