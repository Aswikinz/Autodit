import { useQuery } from "@tanstack/react-query";
import { client, result, message, label } from "../../lib/client";
import type { Rule } from "../../lib/client";
import {
  ErrorBox,
  Metric,
  SectionTitle,
  Spinner,
} from "../../components/Shared";

type Summary = {
  states: Record<string, number>;
  severities: Record<string, number>;
  completed_runs: number;
  failed_runs: number;
  tested_records: number;
  overdue_exceptions: number;
  trend: { day: string; runs: number; exceptions: number }[];
};
export function Assurance() {
  const q = useQuery({
    queryKey: ["assurance"],
    queryFn: () => result<Summary>(client.GET("/api/assurance")),
  });
  const catalog = useQuery({
    queryKey: ["rules"],
    queryFn: () => result<Rule[]>(client.GET("/api/rules")),
  });
  const data = q.data;
  const disposed = (data?.states.accepted ?? 0) + (data?.states.dismissed ?? 0);
  return (
    <>
      <SectionTitle
        title="Assurance overview"
        description="Review executed coverage and the outcomes of auditor decisions."
      />
      {q.isPending ? (
        <Spinner />
      ) : q.error ? (
        <ErrorBox>{message(q.error)}</ErrorBox>
      ) : data ? (
        <>
          <div className="metrics">
            <Metric
              label="Completed runs"
              value={data.completed_runs}
              note="Tie-out passed and tests executed"
            />
            <Metric
              label="Records tested"
              value={data.tested_records.toLocaleString()}
              note="Cumulative; includes repeat runs"
            />
            <Metric
              label="Runs requiring attention"
              value={data.failed_runs}
              note="Failed, halted or ceiling exceeded"
            />
            <Metric
              label="Decision precision"
              value={
                disposed
                  ? `${Math.round((100 * (data.states.accepted ?? 0)) / disposed)}%`
                  : "—"
              }
              note="Accepted ÷ (accepted + dismissed)"
            />
          </div>
          <div className="two-column">
            <section className="panel padded">
              <h2>Disposition mix</h2>
              <p className="muted">
                Current states across the exception population
              </p>
              <div className="bar-chart">
                {[
                  "open",
                  "in_review",
                  "accepted",
                  "dismissed",
                  "suppressed",
                  "reopened",
                  "resolved_in_source",
                ].map((state) => (
                  <div key={state}>
                    <span>{label(state)}</span>
                    <div className="bar-track">
                      <div
                        className={`bar ${state}`}
                        style={{
                          width: `${Math.max(0, ((data.states[state] ?? 0) / Math.max(1, ...Object.values(data.states))) * 100)}%`,
                        }}
                      />
                    </div>
                    <strong>{data.states[state] ?? 0}</strong>
                  </div>
                ))}
              </div>
            </section>
            <section className="panel padded">
              <h2>Test coverage</h2>
              <p className="muted">Shipped tests and current decisions</p>
              {catalog.data?.map((rule) => (
                <div className="coverage-row" key={rule.rule_id}>
                  <span className="rule-code">{rule.rule_id}</span>
                  <div>
                    <strong>{rule.title}</strong>
                    <small>
                      {rule.enabled ? "Enabled" : "Disabled"} ·{" "}
                      {rule.exceptions} exceptions
                    </small>
                  </div>
                  <strong>
                    {rule.accepted + rule.dismissed
                      ? `${Math.round((rule.accepted / (rule.accepted + rule.dismissed)) * 100)}%`
                      : "—"}
                  </strong>
                </div>
              ))}
              <p className="small muted">
                Coverage is limited to the declared test populations. These
                metrics do not assert that every business control was tested.
              </p>
            </section>
          </div>
          <section className="panel">
            <div className="panel-top">
              <h2>Completed runs by day</h2>
            </div>
            <table>
              <thead>
                <tr>
                  <th>Accounting date of run</th>
                  <th>Completed runs</th>
                  <th>Exceptions observed</th>
                </tr>
              </thead>
              <tbody>
                {data.trend.map((row) => (
                  <tr key={row.day}>
                    <td>{row.day}</td>
                    <td>{row.runs}</td>
                    <td>{row.exceptions}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {data.trend.length === 0 && (
              <p className="padded muted">
                Run history will appear after the first completed audit.
              </p>
            )}
          </section>
        </>
      ) : null}
    </>
  );
}
