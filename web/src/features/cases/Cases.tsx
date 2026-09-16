import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { client, result, message, label, dateTime } from "../../lib/client";
import {
  Badge,
  Drawer,
  ErrorBox,
  Help,
  SectionTitle,
  Spinner,
} from "../../components/Shared";
import { ExceptionDetail } from "../queue/Queue";
import type { Workflow } from "../admin/WorkflowSettings";
type Case = {
  exception_key: string;
  workflow_revision: number;
  workflow: Workflow;
  step_index: number;
  status: string;
  assignee: string;
  last_actor: string;
  revision: number;
  rule_id: string;
  entity_id: string;
  severity: string;
  finding_state: string;
};
export function Cases({
  subject,
  roles,
}: {
  subject: string;
  roles: string[];
}) {
  const [status, setStatus] = useState("active");
  const [page,setPage]=useState(1);
  const [selected, setSelected] = useState<string>();
  const q = useQuery({
    queryKey: ["cases", status,page],
    queryFn: () =>
      result<Case[]>(
        client.GET("/api/cases", { params: { query: { status,page } } }),
      ),
    refetchInterval: 10000,
  });
  const current = q.data?.find((c) => c.exception_key === selected);
  return (
    <>
      <SectionTitle
        title="Review cases"
        description="Follow each finding through review, remediation and approval."
      />
      <Help title="Work on a case">
        <ol>
          <li>Open a case and inspect the finding evidence.</li>
          <li>
            Check the current step and responsible role. Claim unassigned work
            or ask a manager to assign it.
          </li>
          <li>
            Record your review or remediation note, then complete the step. Send
            it back when corrections are needed.
          </li>
          <li>
            The final approver confirms the outcome and closes the case.
            Managers can reopen it with a reason.
          </li>
        </ol>
      </Help>
      <label className="field">
        Show cases
        <select
          value={status}
          onChange={(e) => {
            setStatus(e.target.value);setPage(1);
            setSelected(undefined);
          }}
        >
          <option value="active">Active</option>
          <option value="closed">Closed</option>
        </select>
      </label>
      {q.isPending ? (
        <Spinner />
      ) : q.error ? (
        <ErrorBox>{message(q.error)}</ErrorBox>
      ) : (
        <section className="panel padded">
          <p className="muted">
            Page {page}. Showing up to 50 cases, oldest updated first.
          </p>
          <table>
            <thead>
              <tr>
                <th>Finding</th>
                <th>Priority</th>
                <th>Current step</th>
                <th>Assigned to</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((c) => (
                <tr key={c.exception_key}>
                  <td>
                    {c.rule_id}: {c.entity_id}
                  </td>
                  <td>
                    <Badge value={c.severity} />
                  </td>
                  <td>
                    {c.status === "closed"
                      ? "Closed"
                      : c.workflow.steps[c.step_index]?.name}
                  </td>
                  <td>{c.assignee || "Unassigned"}</td>
                  <td>
                    <button
                      className="button"
                      onClick={() => setSelected(c.exception_key)}
                    >
                      Open case
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="actions"><button className="button" disabled={page===1} onClick={()=>{setPage(page-1);setSelected(undefined)}}>Previous page</button><button className="button" disabled={q.data.length<50} onClick={()=>{setPage(page+1);setSelected(undefined)}}>Next page</button></div>
          {!q.data.length && (
            <p>
              No {status} cases. An administrator can publish a workflow to
              enroll active findings.
            </p>
          )}
        </section>
      )}
      {current && (
        <CaseDetail
          key={current.exception_key}
          value={current}
          subject={subject}
          roles={roles}
          onClose={() => setSelected(undefined)}
        />
      )}
    </>
  );
}
function CaseDetail({
  value: c,
  subject,
  roles,
  onClose,
}: {
  value: Case;
  subject: string;
  roles: string[];
  onClose: () => void;
}) {
  const cache = useQueryClient();
  const [note, setNote] = useState("");
  const [assignee, setAssignee] = useState("");
  const [resolution, setResolution] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [evidence, setEvidence] = useState(false);
  const events = useQuery({
    queryKey: ["case-events", c.exception_key, c.revision],
    queryFn: () =>
      result<
        {
          id: string;
          actor: string;
          action: string;
          note: string;
          created_at: string;
        }[]
      >(
        client.GET("/api/cases/{key}/events", {
          params: { path: { key: c.exception_key } },
        }),
      ),
  });
  const people = useQuery({
    queryKey: ["case-people"],
    queryFn: () =>
      result<{ username: string; display_name: string; roles: string[] }[]>(
        client.GET("/api/cases/people"),
      ),
  });
  const step = c.workflow.steps[c.step_index]!;
  const manager = roles.includes("audit_manager");
  const permitted =
    roles.includes(step.role) && (!c.assignee || c.assignee === subject);
  const separation = step.different_actor && c.last_actor === subject;
  const final = c.step_index === c.workflow.steps.length - 1;
  async function act(action: string, target = "") {
    setBusy(true);
    setError("");
    try {
      await result(
        client.POST("/api/cases/{key}/actions", {
          params: { path: { key: c.exception_key } },
          body: {
            action,
            note,
            assignee: target,
            resolution,
            revision: c.revision,
          },
        }),
      );
      setNote("");
      await cache.invalidateQueries({ queryKey: ["cases"] });
      await cache.invalidateQueries({ queryKey: ["queue"] });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  if (evidence)
    return (
      <ExceptionDetail
        id={c.exception_key}
        onClose={() => setEvidence(false)}
      />
    );
  return (
    <Drawer
      title={`${c.rule_id}: ${c.entity_id}`}
      kicker={`${c.workflow.name} / version ${c.workflow_revision}`}
      onClose={onClose}
    >
      <ol className="workflow-timeline">
        {c.workflow.steps.map((s, i) => (
          <li
            key={i}
            className={
              i === c.step_index && c.status === "active" ? "current" : ""
            }
          >
            <strong>
              {i + 1}. {s.name}
            </strong>
            <span>
              {label(s.role)}{" "}
              {i < c.step_index || c.status === "closed"
                ? "(completed)"
                : i === c.step_index
                  ? "(current)"
                  : ""}
            </span>
          </li>
        ))}
      </ol>
      <p>
        Finding state: {label(c.finding_state)}. Case: {c.status}.
      </p>
      {roles.some((r) => ["auditor", "audit_manager"].includes(r)) && (
        <button className="button" onClick={() => setEvidence(true)}>
          View finding evidence
        </button>
      )}
      {separation && c.status === "active" && (
        <div className="notice">
          A different person must complete this step because you completed the
          previous step.
        </div>
      )}
      <label className="field">
        Review note or reason
        <textarea
          required
          minLength={3}
          maxLength={4000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Describe the evidence reviewed, correction made or decision reached."
        />
      </label>
      {c.status === "active" && (
        <>
          {final && (
            <label className="field">
              Closure outcome
              <select
                value={resolution}
                onChange={(e) => setResolution(e.target.value)}
              >
                <option value="">Choose an outcome</option>
                <option value="accepted">Confirmed audit issue</option>
                <option value="dismissed">Dismissed after review</option>
              </select>
            </label>
          )}
          <div className="actions">
            <button
              className="button primary"
              disabled={
                busy ||
                note.trim().length < 3 ||
                !permitted ||
                separation ||
                (final && !resolution)
              }
              onClick={() => void act("advance")}
            >
              {final ? "Approve and close case" : "Complete step"}
            </button>
            <button
              className="button"
              disabled={
                busy ||
                note.trim().length < 3 ||
                !permitted ||
                c.step_index === 0
              }
              onClick={() => void act("return")}
            >
              Send back one step
            </button>
            {!c.assignee && roles.includes(step.role) && (
              <button
                className="button"
                disabled={busy || note.trim().length < 3}
                onClick={() => void act("assign", subject)}
              >
                Assign to me
              </button>
            )}
          </div>
          {manager && (
            <>
              <label className="field">
                Assign current step
                <select
                  value={assignee}
                  onChange={(e) => setAssignee(e.target.value)}
                >
                  <option value="">Unassigned</option>
                  {people.data
                    ?.filter((p) => p.roles.includes(step.role))
                    .map((p) => (
                      <option key={p.username} value={p.username}>
                        {p.display_name} ({p.username})
                      </option>
                    ))}
                </select>
              </label>
              <button
                className="button"
                disabled={busy || note.trim().length < 3}
                onClick={() => void act("assign", assignee)}
              >
                Save assignment
              </button>
            </>
          )}
        </>
      )}
      {c.status === "closed" && manager && (
        <button
          className="button"
          disabled={busy || note.trim().length < 3}
          onClick={() => void act("reopen")}
        >
          Reopen case
        </button>
      )}
      {error && <ErrorBox>{error}</ErrorBox>}
      <h3>Case history</h3>
      {events.error && <ErrorBox>{message(events.error)}</ErrorBox>}
      {events.data?.map((e) => (
        <article className="notice" key={e.id}>
          <strong>
            {label(e.action)} by {e.actor}
          </strong>
          <p>{e.note}</p>
          <small>{dateTime(e.created_at)}</small>
        </article>
      ))}
    </Drawer>
  );
}
