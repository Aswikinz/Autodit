package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Aswikinz/Autodit/internal/analytics"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/exceptions"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ProcessNext executes one source/period job under a PostgreSQL advisory lock.
// On worker loss the lock is released automatically; a running job is retryable.
func (t *Tenant) ProcessNext(ctx context.Context, objects objectstore.Files, now func() time.Time) (bool, error) {
	processed := false
	runID := ""
	failure := ""
	var differences []domain.TieOutDifference
	err := t.Tx(ctx, func(tx pgx.Tx) error {
		var source string
		e := tx.QueryRow(ctx, "select id::text,source_id from audit_run where tenant_id=$1 and status in ('queued','running') order by created_at,id limit 1", t.ID).Scan(&runID, &source)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		var locked bool
		if e = tx.QueryRow(ctx, "select pg_try_advisory_xact_lock(hashtextextended($1,0))", t.ID+"|"+source).Scan(&locked); e != nil {
			return e
		}
		if !locked {
			return nil
		}
		var input, manifestJSON json.RawMessage
		var parameterHash, status string
		e = tx.QueryRow(ctx, "select input,rule_manifest,parameter_set_hash,status from audit_run where tenant_id=$1 and id=$2", t.ID, runID).Scan(&input, &manifestJSON, &parameterHash, &status)
		if e != nil {
			return e
		}
		if status != "queued" && status != "running" {
			return nil
		}
		processed = true
		var population domain.Population
		var manifest map[string]string
		var params rules.Parameters
		var paramsJSON json.RawMessage
		if json.Unmarshal(input, &population) != nil || json.Unmarshal(manifestJSON, &manifest) != nil {
			return domain.ErrInvalid
		}
		if e = tx.QueryRow(ctx, "select content from parameter_set where tenant_id=$1 and hash=$2", t.ID, parameterHash).Scan(&paramsJSON); e != nil {
			return e
		}
		if json.Unmarshal(paramsJSON, &params) != nil || params.Validate() != nil || population.Validate() != nil {
			return domain.ErrInvalid
		}
		for _, record := range population.Records {
			if _, e := params.MaterialityFor(record.Currency); e != nil {
				return errors.New("currency policy not configured")
			}
		}
		stage := func(name string) error { return t.progress(ctx, runID, name) }
		if e = stage("extract"); e != nil {
			return e
		}
		if e = stage("tie_out"); e != nil {
			return e
		}
		differences = domain.TieOut(population)
		if len(differences) > 0 {
			failure = "tieout_failed"
			return errors.New("tie-out failed")
		}
		if e = stage("snapshot"); e != nil {
			return e
		}
		ref, hash, e := objects.WriteSnapshot(ctx, t.ID, runID, population.Records)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "insert into snapshot(tenant_id,id,run_id,object_ref,content_hash,record_count) values($1,$2,$3,$4,$5,$6)", t.ID, runID, runID, ref, hash, len(population.Records)); e != nil {
			return e
		}
		if e = stage("transform"); e != nil {
			return e
		}
		if e = t.canonicalize(ctx, tx, runID, population); e != nil {
			return e
		}
		if e = stage("analyse"); e != nil {
			return e
		}
		candidates := []analytics.Candidate{}
		scope := map[string]string{}
		entities := map[string]bool{}
		for _, control := range population.Controls {
			entities[control.Entity] = true
		}
		for rule, version := range manifest {
			entity := map[string]string{"AP-01": "payment", "AP-02": "approval", "JE-01": "journal_entry"}[rule]
			if entities[entity] {
				scope[rule] = version
			}
		}
		if _, ok := scope["AP-01"]; ok {
			candidates, e = analytics.Duplicates(ctx, tx, t.ID, runID, source, params)
			if e != nil {
				return e
			}
		}
		for _, r := range population.Records {
			if c, ok := analytics.RowCandidate(source, r, params); ok {
				if _, enabled := scope[c.RuleID]; enabled {
					candidates = append(candidates, c)
				}
			}
		}
		if e = stage("score"); e != nil {
			return e
		}
		models := map[string]json.RawMessage{}
		for rule, version := range scope {
			var model json.RawMessage
			if e = tx.QueryRow(ctx, "select model from rule_version where rule_id=$1 and version=$2", rule, version).Scan(&model); e != nil {
				return e
			}
			models[rule] = model
		}
		type scored struct {
			candidate  analytics.Candidate
			evaluation rules.Evaluation
		}
		scoredRows := []scored{}
		counts := map[string]int{}
		for _, candidate := range candidates {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			evaluation, e := rules.Evaluate(models[candidate.RuleID], candidate.Input)
			if e != nil {
				return e
			}
			if evaluation.Result.Flag {
				counts[candidate.RuleID]++
				if counts[candidate.RuleID] > params.ExceptionCeiling {
					return analytics.ErrCeiling
				}
				scoredRows = append(scoredRows, scored{candidate, evaluation})
			}
		}
		if e = stage("reconcile"); e != nil {
			return e
		}
		seen := map[string]bool{}
		at := now().UTC()
		for _, row := range scoredRows {
			key, e := t.observe(ctx, tx, runID, population, parameterHash, scope[row.candidate.RuleID], row.candidate, row.evaluation, at)
			if e != nil {
				return e
			}
			seen[key] = true
		}
		for rule := range scope {
			rows, e := tx.Query(ctx, "select key,state,latest_input_hash,suppression_until from exception where tenant_id=$1 and source_id=$2 and period_id=$3 and rule_id=$4 and state in ('open','in_review','reopened') for update", t.ID, source, population.Period.ID, rule)
			if e != nil {
				return e
			}
			type absent struct {
				key    string
				state  exceptions.State
				hash   string
				expiry *time.Time
			}
			absentRows := []absent{}
			for rows.Next() {
				var r absent
				if e = rows.Scan(&r.key, &r.state, &r.hash, &r.expiry); e != nil {
					rows.Close()
					return e
				}
				if !seen[r.key] {
					absentRows = append(absentRows, r)
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			for _, r := range absentRows {
				state, class := exceptions.Reconcile(r.state, false, r.hash, "", r.expiry, at)
				if _, e = tx.Exec(ctx, "update exception set state=$3,revision=revision+1 where tenant_id=$1 and key=$2", t.ID, r.key, state); e != nil {
					return e
				}
				if e = t.event(ctx, tx, r.key, "system", class, "No longer present in the tested population", r.state, state, at); e != nil {
					return e
				}
			}
		}
		if _, e = tx.Exec(ctx, "update source set watermark=$3 where tenant_id=$1 and id=$2", t.ID, source, runID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "insert into run_stage(tenant_id,id,run_id,stage,status) values($1,$2,$3,'completed','completed')", t.ID, uuid.NewString(), runID); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "update audit_run set status='completed',stage='completed',snapshot_id=$3,exception_count=$4,completed_at=$5,error_code='' where tenant_id=$1 and id=$2", t.ID, runID, runID, len(scoredRows), at)
		return e
	})
	if err != nil && processed && ctx.Err() == nil {
		if errors.Is(err, analytics.ErrCeiling) {
			failure = "ceiling_exceeded"
		}
		if failure == "" {
			failure = "failed"
		}
		diff, _ := json.Marshal(differences)
		if differences == nil {
			diff = []byte("[]")
		}
		recordErr := t.Tx(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "update audit_run set status=$3,error_code=$3,differences=$4,completed_at=$5 where tenant_id=$1 and id=$2", t.ID, runID, failure, diff, now().UTC())
			return e
		})
		if recordErr != nil {
			return processed, recordErr
		}
	}
	return processed, err
}

func (t *Tenant) progress(ctx context.Context, id, stage string) error {
	return t.Tx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "update audit_run set status='running',stage=$3 where tenant_id=$1 and id=$2", t.ID, id, stage); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "insert into run_stage(tenant_id,id,run_id,stage,status) values($1,$2,$3,$4,'started')", t.ID, uuid.NewString(), id, stage)
		return e
	})
}

func (t *Tenant) canonicalize(ctx context.Context, tx pgx.Tx, snapshot string, p domain.Population) error {
	rows := make([][]any, 0, len(p.Records))
	for _, r := range p.Records {
		rows = append(rows, []any{t.ID, p.SourceID, r.ID, snapshot, p.Period.ID, r.Entity, r.Date, r.Amount, r.Currency, r.ReportingAmount, r.ReportingCurrency, r.ExchangeRate, r.RateDate, r.VendorID, r.Debit, r.Credit, r.Limit, r.Reversal, r.Intercompany})
	}
	// Parameterized inserts avoid COPY type ambiguity for date/numeric on drivers.
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue("insert into canonical_record(tenant_id,source_system_id,source_record_id,snapshot_id,period_id,entity_type,posting_date,amount,currency_code,reporting_amount,reporting_currency_code,exchange_rate,rate_date,vendor_id,debit,credit,approval_limit,reversal,intercompany) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)", r...)
	}
	results := tx.SendBatch(ctx, batch)
	if e := results.Close(); e != nil {
		return e
	}
	var count int
	if e := tx.QueryRow(ctx, "select count(*) from canonical_record where tenant_id=$1 and snapshot_id=$2", t.ID, snapshot).Scan(&count); e != nil {
		return e
	}
	if count != len(p.Records) {
		return errors.New("canonical population count mismatch")
	}
	return nil
}

func (t *Tenant) observe(ctx context.Context, tx pgx.Tx, runID string, p domain.Population, paramHash, version string, c analytics.Candidate, evaluation rules.Evaluation, at time.Time) (string, error) {
	key := domain.ExceptionKey(c.RuleID, c.Entity, c.EntityID, p.Period.ID)
	input, _ := json.Marshal(c.Input)
	inputHash := domain.Hash(input)
	// A policy edit is not a source-record change. Suppression reopens only when
	// the consumed source fields change, while the full input remains evidence.
	consumed := make(map[string]any, len(c.Input))
	for k, v := range c.Input {
		if k != "materiality" && k != "qualifies" && k != "weekend" {
			consumed[k] = v
		}
	}
	consumedJSON, _ := json.Marshal(consumed)
	consumedHash := domain.Hash(consumedJSON)
	result, _ := json.Marshal(evaluation.Result)
	observationID := uuid.NewString()
	var oldState exceptions.State
	var oldHash string
	var expiry *time.Time
	err := tx.QueryRow(ctx, "select state,latest_input_hash,suppression_until from exception where tenant_id=$1 and key=$2 for update", t.ID, key).Scan(&oldState, &oldHash, &expiry)
	fresh := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !fresh {
		return "", err
	}
	state, class := exceptions.Reconcile(oldState, true, oldHash, consumedHash, expiry, at)
	if fresh {
		_, err = tx.Exec(ctx, "insert into exception(tenant_id,key,source_id,period_id,rule_id,entity_type,entity_id,state,severity,owner_role,rule_version,parameter_set_hash,snapshot_id,engine_version,input_hash,trace_ref,latest_input_hash,first_seen_at,last_seen_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$18,$17,$17)", t.ID, key, p.SourceID, p.Period.ID, c.RuleID, c.Entity, c.EntityID, state, evaluation.Result.Severity, evaluation.Result.OwnerRole, version, paramHash, runID, rules.EngineVersion, inputHash, observationID, at, consumedHash)
	} else {
		_, err = tx.Exec(ctx, "update exception set state=$3,last_seen_at=$4,latest_input_hash=$5,revision=revision+1,severity=$6,owner_role=$7 where tenant_id=$1 and key=$2", t.ID, key, state, at, consumedHash, evaluation.Result.Severity, evaluation.Result.OwnerRole)
	}
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, "insert into observation(tenant_id,id,exception_key,run_id,rule_id,rule_version,parameter_set_hash,snapshot_id,engine_version,input_hash,trace_ref,input,result,trace,classification) values($1,$2,$3,$4,$5,$6,$7,$4,$8,$9,$14,$10,$11,$12,$13)", t.ID, observationID, key, runID, c.RuleID, version, paramHash, rules.EngineVersion, inputHash, input, result, evaluation.Trace, class, observationID)
	if err != nil {
		return "", err
	}
	if oldState != state {
		if err = t.event(ctx, tx, key, "system", class, "Run reconciliation", oldState, state, at); err != nil {
			return "", err
		}
	}
	if err = t.syncCase(ctx, tx, key, state); err != nil {
		return "", err
	}
	return key, nil
}

func (t *Tenant) event(ctx context.Context, tx pgx.Tx, key, actor, action, reason string, from, to exceptions.State, at time.Time) error {
	at = at.Truncate(time.Microsecond)
	previous := ""
	e := tx.QueryRow(ctx, "select event_hash from exception_event where tenant_id=$1 and exception_key=$2 order by sequence desc limit 1", t.ID, key).Scan(&previous)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	id := uuid.NewString()
	payload, _ := json.Marshal([]string{previous, t.ID, id, key, actor, action, reason, string(from), string(to), at.Format(time.RFC3339Nano)})
	_, e = tx.Exec(ctx, "insert into exception_event(tenant_id,id,exception_key,actor,action,reason,from_state,to_state,previous_hash,event_hash,created_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)", t.ID, id, key, actor, action, reason, from, to, previous, domain.Hash(payload), at)
	return e
}
