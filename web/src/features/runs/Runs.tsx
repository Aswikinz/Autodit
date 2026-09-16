import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, CheckCircle2, Clock3 } from "lucide-react";
import {
  formatNumber,
  client,
  result,
  dateTime,
  label,
  message,
} from "../../lib/client";
import type { Run, Source } from "../../lib/client";
import {
  Badge,
  Drawer,
  Empty,
  ErrorBox,
  SectionTitle,
  Spinner,
} from "../../components/Shared";

export function Runs({ onImport }: { onImport: () => void }) {
  const [selected, setSelected] = useState<string>();
  const query = useQuery({
    queryKey: ["runs"],
    queryFn: () => result<Run[]>(client.GET("/api/runs")),
    refetchInterval: 3000,
  });
  const sources = useQuery({
    queryKey: ["sources"],
    queryFn: () => result<Source[]>(client.GET("/api/sources")),
    refetchInterval: 10000,
  });
  return (
    <>
      <SectionTitle
        title="Run monitor"
        description="Know which controls ran, which failed, and which are waiting for data."
        action={
          <button className="button primary" onClick={onImport}>
            Import population <ArrowRight size={16} />
          </button>
        }
      />
      {sources.data?.some((s) => s.overdue) && (
        <div className="error">
          Expected successful run missing for{" "}
          {sources.data
            .filter((s) => s.overdue)
            .map((s) => s.name)
            .join(", ")}
          . This is an assurance gap.
        </div>
      )}
      <div className="source-strip">
        {sources.data?.map((s) => (
          <div key={s.id}>
            <span className={`status-dot ${s.overdue ? "warning" : "good"}`} />
            <div>
              <strong>{s.name}</strong>
              <small>Last complete: {dateTime(s.last_completed_at)}</small>
            </div>
            <Badge
              value={
                s.overdue
                  ? "overdue"
                  : s.last_completed_at
                    ? "current"
                    : "awaiting_data"
              }
            />
          </div>
        ))}
      </div>
      <section className="panel">
        <div className="panel-top">
          <h2>Execution history</h2>
          <span className="small muted">
            Latest 100 runs · updates every 3 seconds
          </span>
        </div>
        {query.isPending ? (
          <Spinner />
        ) : query.error ? (
          <ErrorBox>{message(query.error)}</ErrorBox>
        ) : query.data.length === 0 ? (
          <Empty title="No runs yet">
            Import a full population and its independent control report to
            begin.
          </Empty>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Run / source</th>
                  <th>Fiscal period</th>
                  <th>Status</th>
                  <th>Stage</th>
                  <th>Records</th>
                  <th>Exceptions</th>
                  <th>Submitted</th>
                </tr>
              </thead>
              <tbody>
                {query.data.map((r) => (
                  <tr key={r.id}>
                    <td>
                      <button
                        className="record-link"
                        onClick={() => setSelected(r.id)}
                      >
                        <span>{r.source_id}</span>
                        <small>{r.id.slice(0, 8)}</small>
                      </button>
                    </td>
                    <td>{r.period_id}</td>
                    <td>
                      <Badge value={r.status} />
                    </td>
                    <td>{label(r.stage)}</td>
                    <td>{formatNumber(r.record_count)}</td>
                    <td>
                      {r.status === "completed"
                        ? formatNumber(r.exception_count)
                        : "—"}
                    </td>
                    <td className="muted">{dateTime(r.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      <div className="assurance-note">
        <CheckCircle2 size={17} /> Zero exceptions is a valid result only when
        the run completed and tie-out passed.
      </div>
      {selected && (
        <RunDetail id={selected} onClose={() => setSelected(undefined)} />
      )}
    </>
  );
}
function RunDetail({ id, onClose }: { id: string; onClose: () => void }) {
  const query = useQuery({
    queryKey: ["run", id],
    queryFn: () =>
      result<Run>(client.GET("/api/runs/{id}", { params: { path: { id } } })),
    refetchInterval: 3000,
  });
  const r = query.data;
  return (
    <Drawer
      title={r?.source_id ?? "Run"}
      kicker="Execution detail"
      onClose={onClose}
    >
      {query.isPending ? (
        <Spinner />
      ) : query.error ? (
        <ErrorBox>{message(query.error)}</ErrorBox>
      ) : r ? (
        <>
          <Badge value={r.status} />
          <dl className="evidence-grid">
            <dt>Run</dt>
            <dd>{r.id}</dd>
            <dt>Period</dt>
            <dd>{r.period_id}</dd>
            <dt>Snapshot</dt>
            <dd>{r.snapshot_id ?? "Not committed"}</dd>
            <dt>Submitted</dt>
            <dd>{dateTime(r.created_at)}</dd>
          </dl>
          {r.status === "tieout_failed" && (
            <ErrorBox>
              Independent control totals do not match. No exceptions were
              created or resolved by this run.
            </ErrorBox>
          )}
          {r.differences.map((d, i) => (
            <div className="panel compact" key={i}>
              <h3>
                {String(d.actual.entity)} · {String(d.actual.currency)}
              </h3>
              <table>
                <thead>
                  <tr>
                    <th>Measure</th>
                    <th>Control report</th>
                    <th>Extract</th>
                  </tr>
                </thead>
                <tbody>
                  {["count", "amount", "debit", "credit"].map((k) => (
                    <tr key={k}>
                      <td>{label(k)}</td>
                      <td>{String(d.expected[k] ?? "Missing")}</td>
                      <td>{String(d.actual[k])}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
          {r.error_code && r.status !== "tieout_failed" && (
            <ErrorBox>
              {label(r.error_code)}. Check the source contract, rule policy,
              storage and worker health before submitting a corrected run.
            </ErrorBox>
          )}
          <h3>Pipeline history</h3>
          <ol className="timeline">
            {r.stages?.map((s, i) => (
              <li key={i}>
                <Clock3 size={16} />
                <strong>{label(s.stage)}</strong>
                <small>{dateTime(s.at)}</small>
              </li>
            ))}
          </ol>
        </>
      ) : null}
    </Drawer>
  );
}
