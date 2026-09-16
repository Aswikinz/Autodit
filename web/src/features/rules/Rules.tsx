import { lazy, Suspense, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Play, ShieldCheck } from "lucide-react";
import { client, result, message, dateTime } from "../../lib/client";
import type { Rule, JSONObject } from "../../lib/client";
import {
  Badge,
  Drawer,
  ErrorBox,
  SectionTitle,
  Spinner,
} from "../../components/Shared";

const RawEditor = lazy(() => import("./RawEditor"));
type Graph = {
  nodes: {
    id: string;
    type: string;
    content?: { rules: Record<string, string>[] };
  }[];
};
function decisionRows(model: JSONObject) {
  return (
    (model as unknown as Graph).nodes.find(
      (n) => n.type === "decisionTableNode",
    )?.content?.rules ?? []
  );
}
export function Rules({ roles }: { roles: string[] }) {
  const [selected, setSelected] = useState<Rule>();
  const catalog = useQuery({
    queryKey: ["rules"],
    queryFn: () => result<Rule[]>(client.GET("/api/rules")),
  });
  const canRelease = roles.some((r) =>
    ["audit_manager", "rule_engineer"].includes(r),
  );
  return (
    <>
      <SectionTitle
        title="Rule catalog"
        description="One test library. Versioned decisions and policy inputs for your workspace."
      />
      <div className="rule-grid">
        {catalog.isPending ? (
          <Spinner />
        ) : catalog.error ? (
          <ErrorBox>{message(catalog.error)}</ErrorBox>
        ) : (
          catalog.data.map((rule) => (
            <button
              className="rule-card"
              key={rule.rule_id}
              onClick={() => setSelected(rule)}
            >
              <div>
                <span className="rule-code">{rule.rule_id}</span>
                <Badge value={rule.enabled ? "enabled" : "disabled"} />
              </div>
              <h2>{rule.title}</h2>
              <p>
                {rule.rule_id === "AP-01"
                  ? "Same vendor, currency and amount within 30 days."
                  : rule.rule_id === "JE-01"
                    ? "Posting on configured non-working days above materiality."
                    : "Transaction value above the recorded approval limit."}
              </p>
              <div className="rule-stats">
                <span>
                  <strong>{rule.exceptions}</strong>exceptions
                </span>
                <span>
                  <strong>
                    {rule.accepted + rule.dismissed
                      ? `${Math.round((rule.accepted / (rule.accepted + rule.dismissed)) * 100)}%`
                      : "—"}
                  </strong>
                  precision
                </span>
                <ArrowRight size={18} />
              </div>
              <small>
                Version {rule.version.slice(0, 12)} ·{" "}
                {dateTime(rule.released_at)}
              </small>
            </button>
          ))
        )}
      </div>
      <p className="muted">
        Currency thresholds and non-working days are managed in Administration.
      </p>
      {selected && (
        <RuleEditor
          rule={selected}
          canRelease={canRelease}
          rawAllowed={roles.includes("rule_engineer")}
          onClose={() => setSelected(undefined)}
        />
      )}
    </>
  );
}

function RuleEditor({
  rule,
  canRelease,
  rawAllowed,
  onClose,
}: {
  rule: Rule;
  canRelease: boolean;
  rawAllowed: boolean;
  onClose: () => void;
}) {
  const cache = useQueryClient();
  const [model, setModel] = useState(rule.model);
  const [enabled, setEnabled] = useState(rule.enabled);
  const [raw, setRaw] = useState(false);
  const [error, setError] = useState("");
  const [simulation, setSimulation] = useState<JSONObject>();
  const [busy, setBusy] = useState(false);
  const rows = decisionRows(model);
  const first = rows[0];
  const previous = decisionRows(rule.model);
  const changes = rows.flatMap((row) => {
    const before = previous.find((p) => p._id === row._id);
    return ["severity", "owner_role"]
      .filter((field) => before?.[field] !== row[field])
      .map((field) => ({
        id: row._id ?? "",
        field,
        before: before?.[field] ?? "—",
        after: row[field] ?? "",
      }));
  });
  function edit(field: string, value: string) {
    const next = structuredClone(model);
    const row = decisionRows(next)[0];
    if (row) row[field] = JSON.stringify(value);
    setModel(next);
    setSimulation(undefined);
  }
  async function simulate() {
    setBusy(true);
    setError("");
    try {
      setSimulation(
        await result<JSONObject>(
          client.POST("/api/rules/simulate", { body: { model } }),
        ),
      );
    } catch (e) {
      setError(message(e));
      setSimulation(undefined);
    } finally {
      setBusy(false);
    }
  }
  async function release() {
    setBusy(true);
    setError("");
    try {
      await result(
        client.PUT("/api/rules/{id}", {
          params: { path: { id: rule.rule_id } },
          body: { model, revision: rule.revision, enabled },
        }),
      );
      await cache.invalidateQueries({ queryKey: ["rules"] });
      onClose();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Drawer
      fullscreen
      title={rule.title}
      kicker={`${rule.rule_id} / Versioned authoring`}
      onClose={onClose}
    >
      <p className="muted">
        Set the severity and review team for this built-in audit test. Use
        Analyze data to create rules for your own columns.
      </p>
      <label className="toggle">
        <input
          type="checkbox"
          checked={enabled}
          disabled={!canRelease}
          onChange={(e) => setEnabled(e.target.checked)}
        />{" "}
        Enable for future runs
      </label>
      <h3>When the candidate qualifies</h3>
      <div className="parameter-grid">
        <label className="field">
          Severity
          <select
            value={String(first?.severity ?? '"high"').replaceAll('"', "")}
            disabled={!canRelease}
            onChange={(e) => edit("severity", e.target.value)}
          >
            {["low", "medium", "high", "critical"].map((v) => (
              <option key={v}>{v}</option>
            ))}
          </select>
        </label>
        <label className="field">
          Route to
          <select
            value={String(first?.owner_role ?? '"auditor"').replaceAll('"', "")}
            disabled={!canRelease}
            onChange={(e) => edit("owner_role", e.target.value)}
          >
            <option value="auditor">Auditor</option>
            <option value="audit_manager">Audit manager</option>
          </select>
        </label>
      </div>
      {rawAllowed && (
        <>
          <button className="button" onClick={() => setRaw(!raw)}>
            {raw ? "Hide" : "Open"} decision graph
          </button>
          {raw && (
            <Suspense fallback={<Spinner />}>
              <RawEditor
                readOnly
                value={model}
                onChange={(value) => {
                  setModel(value);
                  setSimulation(undefined);
                }}
              />
            </Suspense>
          )}
        </>
      )}
      <h3>Changes by decision row</h3>
      {changes.length ? (
        <table>
          <thead>
            <tr>
              <th>Row</th>
              <th>Field</th>
              <th>Previous</th>
              <th>Draft</th>
            </tr>
          </thead>
          <tbody>
            {changes.map((c) => (
              <tr key={c.id + c.field}>
                <td>{c.id}</td>
                <td>{c.field}</td>
                <td>{c.before}</td>
                <td>{c.after}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="muted">No decision row changes.</p>
      )}
      <h3>Required simulation</h3>
      <p className="muted">
        The server checks a qualifying candidate, a non-qualifying candidate,
        and a missing qualification. Release rechecks these fixtures.
      </p>
      <div className="actions">
        <button
          className="button"
          disabled={busy}
          onClick={() => void simulate()}
        >
          <Play size={16} /> Simulate draft
        </button>
        <button
          className="button primary"
          disabled={!canRelease || busy || !simulation}
          onClick={() => void release()}
        >
          <ShieldCheck size={16} /> Release version
        </button>
      </div>
      {simulation && (
        <div className="notice" role="status">
          All 3 required fixtures passed.
          <details>
            <summary>Evaluation traces</summary>
            <pre>{JSON.stringify(simulation, null, 2)}</pre>
          </details>
        </div>
      )}
      {error && <ErrorBox>{error}</ErrorBox>}
    </Drawer>
  );
}
