package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/exceptions"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
	"github.com/jackc/pgx/v5"
)

// QueueFilter is constrained to server-side filtering, ordering and bounded pages.
type QueueFilter struct {
	State, Rule, Severity, Search, Sort string
	Page, Size                          int
}

// QueuePage includes the total matching count; the browser never filters just one page.
type QueuePage struct {
	Items []json.RawMessage `json:"items"`
	Total int               `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
}

// Queue returns only the current tenant's matching exceptions.
func (t *Tenant) Queue(ctx context.Context, f QueueFilter) (QueuePage, error) {
	out := QueuePage{Items: []json.RawMessage{}, Page: f.Page, Size: f.Size}
	if f.Page < 1 || f.Size < 1 || f.Size > 100 || len(f.Search) > 160 || f.Page > 1000000 {
		return out, domain.ErrInvalid
	}
	if f.Sort != "oldest" && f.Sort != "newest" && f.Sort != "severity" {
		f.Sort = "severity"
	}
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{t.ID, f.State, f.Rule, f.Severity, "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(f.Search) + "%"}
		where := ` where tenant_id=$1 and ($2='' or state=$2 or ($2='active' and state in ('open','in_review','reopened'))) and ($3='' or rule_id=$3) and ($4='' or severity=$4) and (entity_id ilike $5 or key ilike $5)`
		if e := tx.QueryRow(ctx, "select count(*) from exception"+where, args...).Scan(&out.Total); e != nil {
			return e
		}
		order := "first_seen_at asc,key"
		if f.Sort == "newest" {
			order = "first_seen_at desc,key"
		}
		if f.Sort == "severity" {
			order = "case severity when 'critical' then 0 when 'high' then 1 when 'medium' then 2 else 3 end,first_seen_at,key"
		}
		// Only constant allowlisted SQL fragments are composed here, never user strings.
		rows, e := tx.Query(ctx, `select jsonb_build_object('key',key,'rule_id',rule_id,'entity_type',entity_type,'entity_id',entity_id,'period_id',period_id,'state',state,'severity',severity,'owner_role',owner_role,'revision',revision,'first_seen_at',first_seen_at,'last_seen_at',last_seen_at,'suppression_until',suppression_until) from exception`+where+" order by "+order+" limit $6 offset $7", append(args, f.Size, (f.Page-1)*f.Size)...)
		if e != nil {
			return e
		}
		out.Items, e = JSONRows(rows)
		return e
	})
	return out, e
}

// Detail exposes original evidence and all subsequent observations and dispositions.
func (t *Tenant) Detail(ctx context.Context, key string) (json.RawMessage, error) {
	var out json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `select jsonb_build_object('exception',to_jsonb(e),'has_review_case',exists(select 1 from review_case c where c.tenant_id=e.tenant_id and c.exception_key=e.key),'observations',coalesce((select jsonb_agg(to_jsonb(o) order by o.created_at desc,o.id) from observation o where o.tenant_id=e.tenant_id and o.exception_key=e.key),'[]'),'events',coalesce((select jsonb_agg(to_jsonb(v) order by v.created_at,v.id) from exception_event v where v.tenant_id=e.tenant_id and v.exception_key=e.key),'[]'),'newer_snapshot_exists',exists(select 1 from audit_run r where r.tenant_id=e.tenant_id and r.source_id=e.source_id and r.period_id=e.period_id and r.status='completed' and r.snapshot_id<>e.snapshot_id and r.created_at>e.first_seen_at)) from exception e where e.tenant_id=$1 and e.key=$2`, t.ID, key).Scan(&out)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return e
	})
	return out, e
}

// Dispose appends an event and changes the queue state atomically under optimistic locking.
func (t *Tenant) Dispose(ctx context.Context, key, actor, reason string, to exceptions.State, revision int, expiry *time.Time, now time.Time) error {
	return t.Tx(ctx, func(tx pgx.Tx) error {
		var from exceptions.State
		var current int
		e := tx.QueryRow(ctx, "select state,revision from exception where tenant_id=$1 and key=$2 for update", t.ID, key).Scan(&from, &current)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if current != revision {
			return ErrConflict
		}
		var managed bool
		if e = tx.QueryRow(ctx, `select exists(select 1 from review_case where tenant_id=$1 and exception_key=$2)`, t.ID, key).Scan(&managed); e != nil {
			return e
		}
		if managed {
			return errors.Join(ErrConflict, errors.New("finding has a review workflow; use case actions"))
		}
		if e = exceptions.Transition(from, to, strings.TrimSpace(reason), expiry, now); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "update exception set state=$3,suppression_until=$4,revision=revision+1 where tenant_id=$1 and key=$2", t.ID, key, to, expiry)
		if e != nil {
			return e
		}
		if e = t.event(ctx, tx, key, actor, "disposition", reason, from, to, now.UTC()); e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "exception.dispose", key)
	})
}

// Comment adds a durable investigation note without altering the disposition.
func (t *Tenant) Comment(ctx context.Context, key, actor, body string, now time.Time) error {
	if len(strings.TrimSpace(body)) < 1 || len(body) > 4000 {
		return domain.ErrInvalid
	}
	return t.Tx(ctx, func(tx pgx.Tx) error {
		var state exceptions.State
		e := tx.QueryRow(ctx, "select state from exception where tenant_id=$1 and key=$2 for update", t.ID, key).Scan(&state)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		return t.event(ctx, tx, key, actor, "comment", body, state, state, now.UTC())
	})
}

// Replay checks the original snapshot and exact versioned evaluation artifacts.
func (t *Tenant) Replay(ctx context.Context, id string, objects objectstore.Files) (rules.Evaluation, error) {
	var result rules.Evaluation
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		var input, model, expected json.RawMessage
		var inputHash, version, engine, ref, hash string
		e := tx.QueryRow(ctx, `select o.input,o.input_hash,o.rule_version,o.engine_version,o.result,v.model,s.object_ref,s.content_hash from observation o join rule_version v on v.rule_id=o.rule_id and v.version=o.rule_version join snapshot s on s.tenant_id=o.tenant_id and s.id=o.snapshot_id where o.tenant_id=$1 and o.id=$2`, t.ID, id).Scan(&input, &inputHash, &version, &engine, &expected, &model, &ref, &hash)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		var in map[string]any
		var graph any
		if json.Unmarshal(input, &in) != nil || json.Unmarshal(model, &graph) != nil {
			return domain.ErrInvalid
		}
		canonicalInput, _ := json.Marshal(in)
		canonicalModel, _ := json.Marshal(graph)
		if domain.Hash(canonicalInput) != inputHash || domain.Hash(canonicalModel) != version || engine != rules.EngineVersion {
			return errors.New("evidence version or integrity mismatch")
		}
		if _, e = objects.ReadSnapshot(ctx, t.ID, ref, hash); e != nil {
			return e
		}
		result, e = rules.Evaluate(canonicalModel, in)
		if e != nil {
			return e
		}
		var expectedResult rules.Result
		if json.Unmarshal(expected, &expectedResult) != nil || expectedResult != result.Result {
			return errors.New("replay result mismatch")
		}
		return nil
	})
	return result, e
}
