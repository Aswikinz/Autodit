package storage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/exceptions"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

type WorkflowStep struct {
	Name           string `json:"name"`
	Role           string `json:"role"`
	DifferentActor bool   `json:"different_actor"`
}
type Workflow struct {
	Name  string         `json:"name"`
	Steps []WorkflowStep `json:"steps"`
}

func (w Workflow) Validate() error {
	if len(strings.TrimSpace(w.Name)) < 1 || len(w.Name) > 100 || len(w.Steps) < 2 || len(w.Steps) > 20 {
		return domain.ErrInvalid
	}
	names := map[string]bool{}
	for i, s := range w.Steps {
		if len(strings.TrimSpace(s.Name)) < 1 || len(s.Name) > 80 || names[s.Name] {
			return domain.ErrInvalid
		}
		names[s.Name] = true
		if _, ok := RoleDescriptions[s.Role]; !ok || s.Role == "admin" {
			return domain.ErrInvalid
		}
		if i == 0 && s.DifferentActor {
			return domain.ErrInvalid
		}
	}
	return nil
}
func (t *Tenant) Workflow(ctx context.Context) (json.RawMessage, error) {
	out := json.RawMessage(`{"revision":0,"workflow":{"name":"","steps":[]}}`)
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `select jsonb_build_object('revision',revision,'workflow',content) from workflow_definition where tenant_id=$1 order by revision desc limit 1`, t.ID).Scan(&out)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		return e
	})
	return out, e
}
func (t *Tenant) SaveWorkflow(ctx context.Context, w Workflow, revision int, actor string) error {
	if e := w.Validate(); e != nil {
		return e
	}
	content, _ := json.Marshal(w)
	return t.Tx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `select id from tenant where id=$1 for update`, t.ID); e != nil {
			return e
		}
		var current int
		if e := tx.QueryRow(ctx, `select coalesce(max(revision),0) from workflow_definition where tenant_id=$1`, t.ID).Scan(&current); e != nil {
			return e
		}
		if current != revision {
			return ErrConflict
		}
		if _, e := tx.Exec(ctx, `insert into workflow_definition(tenant_id,revision,content) values($1,$2,$3)`, t.ID, current+1, content); e != nil {
			return e
		}
		// Existing cases retain their workflow version. Unmanaged active findings enter this workflow.
		if _, e := tx.Exec(ctx, `insert into review_case(tenant_id,exception_key,workflow_revision) select tenant_id,key,$2 from exception where tenant_id=$1 and state in ('open','reopened','in_review') on conflict do nothing`, t.ID, current+1); e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "workflow.publish", domain.Hash(content))
	})
}
func (t *Tenant) syncCase(ctx context.Context, tx pgx.Tx, key string, state exceptions.State) error {
	if _, e := tx.Exec(ctx, `insert into review_case(tenant_id,exception_key,workflow_revision) select $1,$2,revision from workflow_definition where tenant_id=$1 order by revision desc limit 1 on conflict do nothing`, t.ID, key); e != nil {
		return e
	}
	if state == exceptions.Reopened {
		tag, e := tx.Exec(ctx, `update review_case set status='active',step_index=0,assignee='',last_actor='',revision=revision+1,updated_at=now() where tenant_id=$1 and exception_key=$2 and status='closed'`, t.ID, key)
		if e != nil {
			return e
		}
		if tag.RowsAffected() > 0 {
			return t.caseEvent(ctx, tx, key, "system", "reopen", "New evidence requires another review.", 0, "")
		}
	}
	return nil
}
func (t *Tenant) caseEvent(ctx context.Context, tx pgx.Tx, key, actor, action, note string, step int, assignee string) error {
	_, e := tx.Exec(ctx, `insert into case_event(tenant_id,id,exception_key,actor,action,note,step_index,assignee) values($1,$2,$3,$4,$5,$6,$7,$8)`, t.ID, uuid.NewString(), key, actor, action, note, step, assignee)
	return e
}
func (t *Tenant) Cases(ctx context.Context, status string, pages ...int) ([]json.RawMessage, error) {
	page := 1
	if len(pages) > 0 {
		page = pages[0]
	}
	if page < 1 || page > 100000 {
		return nil, domain.ErrInvalid
	}
	if status != "active" && status != "closed" {
		return nil, domain.ErrInvalid
	}
	var out []json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `select to_jsonb(c)||jsonb_build_object('workflow',d.content,'rule_id',e.rule_id,'entity_id',e.entity_id,'severity',e.severity,'finding_state',e.state) from review_case c join workflow_definition d on d.tenant_id=c.tenant_id and d.revision=c.workflow_revision join exception e on e.tenant_id=c.tenant_id and e.key=c.exception_key where c.tenant_id=$1 and c.status=$2 order by c.updated_at,c.exception_key limit 50 offset $3`, t.ID, status, (page-1)*50)
		if e != nil {
			return e
		}
		out, e = JSONRows(rows)
		return e
	})
	return out, e
}
func (t *Tenant) CaseEvents(ctx context.Context, key string) ([]json.RawMessage, error) {
	var out []json.RawMessage
	e := t.Tx(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `select to_jsonb(v) from case_event v where tenant_id=$1 and exception_key=$2 order by sequence`, t.ID, key)
		if e != nil {
			return e
		}
		out, e = JSONRows(rows)
		return e
	})
	return out, e
}

type CaseAction struct {
	Action     string `json:"action"`
	Note       string `json:"note"`
	Assignee   string `json:"assignee"`
	Resolution string `json:"resolution"`
	Revision   int    `json:"revision"`
}

func (t *Tenant) ActOnCase(ctx context.Context, key, actor string, roles []string, a CaseAction) error {
	if len(strings.TrimSpace(a.Note)) < 3 || len(a.Note) > 4000 {
		return domain.ErrInvalid
	}
	return t.Tx(ctx, func(tx pgx.Tx) error {
		// Lock in the same order as the worker and disposition endpoint.
		var findingState exceptions.State
		if e := tx.QueryRow(ctx, `select state from exception where tenant_id=$1 and key=$2 for update`, t.ID, key).Scan(&findingState); e != nil {
			return e
		}
		var content []byte
		var index, revision int
		var status, assignee, lastActor string
		e := tx.QueryRow(ctx, `select d.content,c.step_index,c.revision,c.status,c.assignee,c.last_actor from review_case c join workflow_definition d on d.tenant_id=c.tenant_id and d.revision=c.workflow_revision where c.tenant_id=$1 and c.exception_key=$2 for update of c`, t.ID, key).Scan(&content, &index, &revision, &status, &assignee, &lastActor)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if revision != a.Revision {
			return ErrConflict
		}
		var w Workflow
		if json.Unmarshal(content, &w) != nil || index < 0 || index >= len(w.Steps) {
			return domain.ErrInvalid
		}
		step := w.Steps[index]
		manager := slices.Contains(roles, "audit_manager")
		if a.Action == "reopen" {
			if status != "closed" || !manager {
				return domain.ErrInvalid
			}
			index = 0
			status = "active"
			assignee = ""
			lastActor = ""
		} else {
			if status != "active" {
				return domain.ErrInvalid
			}
			switch a.Action {
			case "assign":
				if !manager && !slices.Contains(roles, step.Role) {
					return domain.ErrInvalid
				}
				if !manager && (a.Assignee != actor || (assignee != "" && assignee != actor)) {
					return domain.ErrInvalid
				}
				if a.Assignee != "" {
					var userRoles []string
					var enabled bool
					if e = tx.QueryRow(ctx, `select roles,enabled from local_user where tenant_id=$1 and username=$2`, t.ID, a.Assignee).Scan(&userRoles, &enabled); e != nil || !enabled || !slices.Contains(userRoles, step.Role) {
						return domain.ErrInvalid
					}
				}
				assignee = a.Assignee
			case "advance", "return":
				if !slices.Contains(roles, step.Role) || (assignee != "" && assignee != actor) {
					return domain.ErrInvalid
				}
				if a.Action == "return" {
					if index == 0 {
						return domain.ErrInvalid
					}
					index--
					assignee = ""
				} else {
					if step.DifferentActor && lastActor == actor {
						return domain.ErrInvalid
					}
					lastActor = actor
					assignee = ""
					if index == len(w.Steps)-1 {
						if a.Resolution != "accepted" && a.Resolution != "dismissed" {
							return domain.ErrInvalid
						}
						status = "closed"
						to := exceptions.State(a.Resolution)
						if _, e = tx.Exec(ctx, `update exception set state=$3,suppression_until=null,revision=revision+1 where tenant_id=$1 and key=$2`, t.ID, key, to); e != nil {
							return e
						}
						if e = t.event(ctx, tx, key, actor, "workflow.close", a.Note, findingState, to, time.Now().UTC()); e != nil {
							return e
						}
					} else {
						index++
					}
				}
			default:
				return domain.ErrInvalid
			}
		}
		if a.Action == "reopen" {
			if _, e = tx.Exec(ctx, `update exception set state='in_review',revision=revision+1 where tenant_id=$1 and key=$2`, t.ID, key); e != nil {
				return e
			}
			if e = t.event(ctx, tx, key, actor, "workflow.reopen", a.Note, findingState, exceptions.InReview, time.Now().UTC()); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `update review_case set step_index=$3,status=$4,assignee=$5,last_actor=$6,revision=revision+1,updated_at=now() where tenant_id=$1 and exception_key=$2`, t.ID, key, index, status, assignee, lastActor); e != nil {
			return e
		}
		if e = t.caseEvent(ctx, tx, key, actor, a.Action, a.Note, index, assignee); e != nil {
			return e
		}
		return t.Audit(ctx, tx, actor, "case."+a.Action, key)
	})
}
