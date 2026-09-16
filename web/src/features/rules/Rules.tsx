import { lazy, Suspense, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Play, ShieldCheck, SlidersHorizontal } from "lucide-react";
import { client, result, message, dateTime } from "../../lib/client";
import type { Rule, Parameters, JSONObject } from "../../lib/client";
import {
  Badge,
  Drawer,
  Help,
  ErrorBox,
  SectionTitle,
  Spinner,
} from "../../components/Shared";

const RawEditor = lazy(() => import("./RawEditor"));
function CurrencyPolicies({
  parameters,
  disabled,
  onChange,
}: {
  parameters: Parameters;
  disabled: boolean;
  onChange: (p: Parameters) => void;
}) {
  const [currency, setCurrency] = useState("");
  return (
    <div className="currency-policies">
      <h3>Additional transaction currencies</h3>
      <p className="small muted">
        A run using an unconfigured currency halts before scoring.
      </p>
      <div className="parameter-grid">
        {Object.entries(parameters.currency_thresholds ?? {}).map(
          ([code, threshold]) => (
            <label className="field" key={code}>
              {code} materiality
              <input
                disabled={disabled}
                value={threshold}
                onChange={(e) =>
                  onChange({
                    ...parameters,
                    currency_thresholds: {
                      ...parameters.currency_thresholds,
                      [code]: e.target.value,
                    },
                  })
                }
              />
            </label>
          ),
        )}
      </div>
      <div className="actions">
        <input
          aria-label="Additional currency code"
          placeholder="EUR"
          maxLength={3}
          value={currency}
          disabled={disabled}
          onChange={(e) => setCurrency(e.target.value.toUpperCase())}
        />
        <button
          className="button"
          disabled={
            disabled || !/^[A-Z]{3}$/.test(currency) || currency === "USD"
          }
          onClick={() => {
            onChange({
              ...parameters,
              currency_thresholds: {
                ...parameters.currency_thresholds,
                [currency]: "1000.0000",
              },
            });
            setCurrency("");
          }}
        >
          Add currency policy
        </button>
      </div>
    </div>
  );
}
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
  const cache = useQueryClient();
  const catalog = useQuery({
    queryKey: ["rules"],
    queryFn: () => result<Rule[]>(client.GET("/api/rules")),
  });
  const settings = useQuery({
    queryKey: ["parameters"],
    queryFn: () =>
      result<{ parameters: Parameters; revision: number; hash: string }>(
        client.GET("/api/parameters"),
      ),
  });
  const [draft, setDraft] = useState<Parameters>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const canRelease = roles.some((r) =>
    ["audit_manager", "rule_engineer"].includes(r),
  );
  const parameters = draft ?? settings.data?.parameters;
  async function save() {
    if (!parameters || !settings.data) return;
    setBusy(true);
    setError("");
    try {
      await result(
        client.PUT("/api/parameters", {
          body: { parameters, revision: settings.data.revision },
        }),
      );
      setDraft(undefined);
      setNotice(
        "Parameters released. New runs use this version; historical evidence is unchanged.",
      );
      await cache.invalidateQueries({ queryKey: ["parameters"] });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
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
      <section className="panel padded">
        <div className="panel-title">
          <SlidersHorizontal size={20} />
          <div>
            <h2>Tenant policy inputs</h2>
            <p className="muted">
              Changes apply to future runs and are stored as immutable parameter
              sets.
            </p>
          </div>
        </div>
        {parameters && (
          <>
            <div className="parameter-grid">
              <label className="field">
                USD materiality (transaction currency)
                <input
                  value={parameters.materiality}
                  disabled={!canRelease}
                  onChange={(e) =>
                    setDraft({ ...parameters, materiality: e.target.value })
                  }
                />
                <small>
                  Exact decimal threshold; qualifying values include the
                  boundary.
                </small>
              </label>
              <label className="field">
                Maximum exceptions per test
                <input
                  type="number"
                  min={1}
                  max={100000}
                  value={parameters.exception_ceiling}
                  disabled={!canRelease}
                  onChange={(e) =>
                    setDraft({
                      ...parameters,
                      exception_ceiling: Number(e.target.value),
                    })
                  }
                />
                <small>
                  The whole run halts if a rule exceeds this ceiling.
                </small>
              </label>
            </div>
            <CurrencyPolicies
              parameters={parameters}
              disabled={!canRelease}
              onChange={setDraft}
            />
            <fieldset className="days">
              <legend>Non-working days</legend>
              {["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map(
                (day, i) => (
                  <label key={day}>
                    <input
                      type="checkbox"
                      checked={parameters.weekend_days.includes(i)}
                      disabled={!canRelease}
                      onChange={(e) =>
                        setDraft({
                          ...parameters,
                          weekend_days: e.target.checked
                            ? [...parameters.weekend_days, i].sort((a, b) => a - b)
                            : parameters.weekend_days.filter((d) => d !== i),
                        })
                      }
                    />
                    {day}
                  </label>
                ),
              )}
            </fieldset>
            <button
              className="button primary"
              disabled={!canRelease || busy || !draft}
              onClick={() => void save()}
            >
              Release parameters <ShieldCheck size={16} />
            </button>
          </>
        )}
        {error && <ErrorBox>{error}</ErrorBox>}
        {notice && (
          <div className="notice" role="status">
            {notice}
          </div>
        )}
      </section>
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
      <Help title="Configure a rule">
        <ol>
          <li>Choose the severity and the team responsible for reviewing a finding.</li>
          <li>Use the decision graph to inspect the rule flow. Select the decision table to edit its rows. Scroll to zoom and drag the canvas to move around.</li>
          <li>Select Simulate draft to check matching, non-matching and missing data examples.</li>
          <li>Review the changes, then release a version. Future runs use it; earlier evidence keeps its original version.</li>
        </ol>
      </Help>
      <div className="notice">
        Engineering owns the tested population and decimal qualification. This
        model decides whether to flag, how severe, and who should review.
      </div>
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
