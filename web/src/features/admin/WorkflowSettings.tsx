import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { client, result, message, label } from "../../lib/client";
import { ErrorBox, Help } from "../../components/Shared";
export type Workflow = {
  name: string;
  steps: { name: string; role: string; different_actor: boolean }[];
};
const proposed: Workflow = {
  name: "Review and approval",
  steps: [
    { name: "Review finding", role: "auditor", different_actor: false },
    { name: "Approve and close", role: "audit_manager", different_actor: true },
  ],
};
export function WorkflowSettings() {
  const cache = useQueryClient();
  const q = useQuery({
    queryKey: ["workflow"],
    queryFn: () =>
      result<{ revision: number; workflow: Workflow }>(
        client.GET("/api/admin/workflow"),
      ),
  });
  const [draft, setDraft] = useState<Workflow>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const workflow = draft ?? (q.data?.revision ? q.data.workflow : proposed);
  function step(index: number, change: Partial<Workflow["steps"][number]>) {
    setDraft({
      ...workflow,
      steps: workflow.steps.map((s, i) =>
        i === index ? { ...s, ...change } : s,
      ),
    });
  }
  async function publish() {
    if (!q.data) return;
    setBusy(true);
    setError("");
    try {
      await result(
        client.PUT("/api/admin/workflow", {
          body: { workflow, revision: q.data.revision },
        }),
      );
      setDraft(undefined);
      await cache.invalidateQueries({ queryKey: ["workflow"] });
      setNotice(
        "Workflow published. Active findings without a case now enter this workflow. Existing cases keep their original steps.",
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel padded">
      <h2>Review and approval workflow</h2>
      <Help title="Build a review workflow">
        <ol>
          <li>
            Name the workflow and add steps in the order people should complete
            them.
          </li>
          <li>
            Choose the role responsible for each step. Create accounts with
            those roles in People and access.
          </li>
          <li>
            Require a different person at approval steps when reviewers must not
            approve their own work.
          </li>
          <li>
            The last step closes the case as a confirmed issue or a dismissed
            finding. A step can send work back for corrections.
          </li>
          <li>
            Publish to apply the workflow to future cases. Open cases retain
            their original version.
          </li>
        </ol>
      </Help>
      {q.error && <ErrorBox>{message(q.error)}</ErrorBox>}
      <label className="field">
        Workflow name
        <input
          maxLength={100}
          value={workflow.name}
          onChange={(e) => setDraft({ ...workflow, name: e.target.value })}
        />
      </label>
      <ol className="workflow-steps">
        {workflow.steps.map((s, i) => (
          <li key={i}>
            <div className="parameter-grid">
              <label className="field">
                Step {i + 1} name
                <input
                  value={s.name}
                  maxLength={80}
                  onChange={(e) => step(i, { name: e.target.value })}
                />
              </label>
              <label className="field">
                Responsible role
                <select
                  value={s.role}
                  onChange={(e) => step(i, { role: e.target.value })}
                >
                  {[
                    "auditor",
                    "audit_manager",
                    "implementer",
                    "rule_engineer",
                  ].map((r) => (
                    <option key={r} value={r}>
                      {label(r)}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            {i > 0 && (
              <label className="toggle">
                <input
                  type="checkbox"
                  checked={s.different_actor}
                  onChange={(e) =>
                    step(i, { different_actor: e.target.checked })
                  }
                />{" "}
                Must be completed by someone other than the previous step's
                author
              </label>
            )}
            <div className="actions">
              <button
                className="button"
                disabled={i === 0}
                onClick={() => {
                  const next = [...workflow.steps];
                  [next[i - 1], next[i]] = [next[i]!, next[i - 1]!];
                  next[0] = { ...next[0]!, different_actor: false };
                  setDraft({ ...workflow, steps: next });
                }}
              >
                Move up
              </button>
              <button
                className="button"
                disabled={workflow.steps.length <= 2}
                onClick={() => {
                  const next = workflow.steps.filter((_, j) => i !== j);
                  next[0] = { ...next[0]!, different_actor: false };
                  setDraft({ ...workflow, steps: next });
                }}
              >
                Remove step
              </button>
            </div>
          </li>
        ))}
      </ol>
      <div className="actions">
        <button
          className="button"
          disabled={workflow.steps.length >= 20}
          onClick={() =>
            setDraft({
              ...workflow,
              steps: [
                ...workflow.steps,
                { name: "", role: "auditor", different_actor: false },
              ],
            })
          }
        >
          Add step
        </button>
        <button
          className="button primary"
          disabled={
            busy ||
            !q.data ||
            !workflow.name ||
            workflow.steps.some((s) => !s.name)
          }
          onClick={() => void publish()}
        >
          Publish workflow
        </button>
      </div>
      {error && <ErrorBox>{error}</ErrorBox>}
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
    </section>
  );
}
