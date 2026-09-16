import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
} from "@tanstack/react-table";
import type { ColumnDef } from "@tanstack/react-table";
import {
  ArrowRight,
  ChevronLeft,
  ChevronRight,
  Search,
  ShieldCheck,
} from "lucide-react";
import {
  formatNumber,
  client,
  result,
  message,
  dateTime,
  label,
} from "../../lib/client";
import type {
  QueuePage,
  ExceptionRow,
  Detail,
  JSONObject,
} from "../../lib/client";
import {
  Badge,
  Drawer,
  Empty,
  ErrorBox,
  Metric,
  SectionTitle,
  Spinner,
} from "../../components/Shared";

export function Queue({ onImport }: { onImport: () => void }) {
  const cache = useQueryClient();
  const [state, setState] = useState("active");
  const [rule, setRule] = useState("");
  const [severity, setSeverity] = useState("");
  const [sort, setSort] = useState("severity");
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<string>();
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [bulkError, setBulkError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => {
      setDebounced(search);
      setPage(1);
    }, 250);
    return () => clearTimeout(t);
  }, [search]);
  const query = useQuery({
    queryKey: ["queue", state, rule, severity, sort, debounced, page],
    queryFn: ({ signal }) =>
      result<QueuePage>(
        client.GET("/api/exceptions", {
          signal,
          params: {
            query: {
              state,
              rule,
              severity,
              sort,
              search: debounced,
              page,
              size: 30,
            },
          },
        }),
      ),
    refetchInterval: 10000,
  });
  const stats = useQuery({
    queryKey: ["assurance"],
    queryFn: () =>
      result<{
        states: Record<string, number>;
        severities: Record<string, number>;
        overdue_exceptions: number;
      }>(client.GET("/api/assurance")),
    refetchInterval: 10000,
  });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const columns = useMemo<ColumnDef<ExceptionRow>[]>(
    () => [
      {
        id: "select",
        header: "",
        cell: ({ row }) => (
          <input
            type="checkbox"
            aria-label={`Select ${row.original.entity_id}`}
            checked={!!checked[row.original.key]}
            onChange={(e) =>
              setChecked({ ...checked, [row.original.key]: e.target.checked })
            }
          />
        ),
      },
      {
        accessorKey: "entity_id",
        header: "Exception / source record",
        cell: ({ row }) => (
          <button
            className="record-link"
            onClick={() => setSelected(row.original.key)}
          >
            <span>
              {row.original.entity_id.split("|").slice(1).join(" + ")}
            </span>
            <small>
              {label(row.original.entity_type)} · {row.original.period_id}
            </small>
          </button>
        ),
      },
      {
        accessorKey: "rule_id",
        header: "Audit test",
        cell: ({ getValue }) => (
          <span className="rule-code">{String(getValue())}</span>
        ),
      },
      {
        accessorKey: "severity",
        header: "Priority",
        cell: ({ getValue }) => <Badge value={String(getValue())} />,
      },
      {
        accessorKey: "state",
        header: "Status",
        cell: ({ getValue }) => <Badge value={String(getValue())} />,
      },
      {
        accessorKey: "first_seen_at",
        header: "First detected",
        cell: ({ getValue }) => (
          <span className="muted">{dateTime(String(getValue()))}</span>
        ),
      },
      {
        id: "open",
        header: "",
        cell: ({ row }) => (
          <button
            className="icon-button"
            aria-label={`Open ${row.original.entity_id}`}
            onClick={() => setSelected(row.original.key)}
          >
            <ArrowRight size={17} />
          </button>
        ),
      },
    ],
    [checked],
  );
  const table = useReactTable({
    data: items,
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  });
  async function startSelected() {
    setBusy(true);
    setBulkError("");
    let done = 0;
    try {
      for (const r of items.filter(
        (r) => checked[r.key] && ["open", "reopened"].includes(r.state),
      )) {
        await result(
          client.POST("/api/exceptions/{key}/disposition", {
            params: { path: { key: r.key } },
            body: {
              state: "in_review",
              reason: "Started review from work queue",
              revision: r.revision,
              suppression_until: null,
            },
          }),
        );
        done++;
      }
      setChecked({});
    } catch (e) {
      setBulkError(`${done} items updated. ${message(e)}`);
    } finally {
      setBusy(false);
      void cache.invalidateQueries({ queryKey: ["queue"] });
    }
  }
  return (
    <>
      <SectionTitle
        title="Exception queue"
        description="Review what matters. Every exception has a traceable evidence chain."
        action={
          <button className="button primary" onClick={onImport}>
            Import population <ArrowRight size={16} />
          </button>
        }
      />
      <div className="metrics">
        <Metric
          label="Awaiting review"
          value={
            (stats.data?.states.open ?? 0) + (stats.data?.states.reopened ?? 0)
          }
          note="New and reopened exceptions"
        />
        <Metric
          label="In review"
          value={stats.data?.states.in_review ?? 0}
          note="Investigations in progress"
        />
        <Metric
          label="High priority"
          value={
            (stats.data?.severities.high ?? 0) +
            (stats.data?.severities.critical ?? 0)
          }
          note="Active high and critical items"
        />
        <Metric
          label="Older than 30 days"
          value={stats.data?.overdue_exceptions ?? 0}
          note="Active exceptions needing attention"
        />
      </div>
      <section className="panel">
        <div className="panel-top">
          <div className="tabs">
            {[
              ["active", "Work queue"],
              ["accepted", "Accepted"],
              ["dismissed", "Dismissed"],
              ["suppressed", "Suppressed"],
              ["", "All exceptions"],
            ].map(([v, l]) => (
              <button
                key={v}
                className={state === v ? "active" : ""}
                onClick={() => {
                  setState(v ?? "");
                  setPage(1);
                  setChecked({});
                }}
              >
                {l}
              </button>
            ))}
          </div>
          <span className="small muted">
            {formatNumber(total)} matching exceptions
          </span>
        </div>
        <div className="filters">
          <label className="search">
            <Search size={17} />
            <input
              placeholder="Search record or exception ID"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </label>
          <select
            aria-label="Filter audit test"
            value={rule}
            onChange={(e) => {
              setRule(e.target.value);
              setPage(1);
            }}
          >
            <option value="">All tests</option>
            {["AP-01", "AP-02", "JE-01"].map((v) => (
              <option key={v}>{v}</option>
            ))}
          </select>
          <select
            aria-label="Filter priority"
            value={severity}
            onChange={(e) => {
              setSeverity(e.target.value);
              setPage(1);
            }}
          >
            <option value="">All priorities</option>
            {["critical", "high", "medium", "low"].map((v) => (
              <option key={v} value={v}>
                {label(v)}
              </option>
            ))}
          </select>
          <select
            aria-label="Sort exceptions"
            value={sort}
            onChange={(e) => {
              setSort(e.target.value);
              setPage(1);
            }}
          >
            <option value="severity">Priority first</option>
            <option value="oldest">Oldest first</option>
            <option value="newest">Newest first</option>
          </select>
        </div>
        {Object.values(checked).some(Boolean) && (
          <div className="bulk">
            <span>
              {Object.values(checked).filter(Boolean).length} selected on this
              page
            </span>
            <button
              className="button"
              disabled={busy}
              onClick={() => void startSelected()}
            >
              Start review
            </button>
          </div>
        )}
        {bulkError && <ErrorBox>{bulkError}</ErrorBox>}
        {query.isPending ? (
          <Spinner />
        ) : query.error ? (
          <ErrorBox>{message(query.error)}</ErrorBox>
        ) : items.length === 0 ? (
          <Empty
            title={
              state === "active"
                ? "Your work queue is clear"
                : "No matching exceptions"
            }
            action={
              <button className="button" onClick={onImport}>
                Connect your first population
              </button>
            }
          >
            Completed runs and data freshness appear in Run monitor. An empty
            queue alone does not prove a control ran.
          </Empty>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                {table.getHeaderGroups().map((g) => (
                  <tr key={g.id}>
                    {g.headers.map((h) => (
                      <th key={h.id}>
                        {flexRender(h.column.columnDef.header, h.getContext())}
                      </th>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody>
                {table.getRowModel().rows.map((r) => (
                  <tr key={r.id}>
                    {r.getVisibleCells().map((c) => (
                      <td key={c.id}>
                        {flexRender(c.column.columnDef.cell, c.getContext())}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <footer className="table-footer">
          <span>
            Showing {total ? (page - 1) * 30 + 1 : 0}–
            {Math.min(page * 30, total)} of {total}
          </span>
          <div>
            <button
              className="icon-button"
              disabled={page <= 1}
              aria-label="Previous page"
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft size={18} />
            </button>
            <span>Page {page}</span>
            <button
              className="icon-button"
              disabled={page * 30 >= total}
              aria-label="Next page"
              onClick={() => setPage(page + 1)}
            >
              <ChevronRight size={18} />
            </button>
          </div>
        </footer>
      </section>
      <div className="assurance-note">
        <ShieldCheck size={17} /> Dispositions persist across runs. Evidence
        remains bound to the rule and population that produced it.
      </div>
      {selected && (
        <ExceptionDetail id={selected} onClose={() => setSelected(undefined)} />
      )}
    </>
  );
}

export function ExceptionDetail({
  id,
  onClose,
}: {
  id: string;
  onClose: () => void;
}) {
  const cache = useQueryClient();
  const query = useQuery({
    queryKey: ["exception", id],
    queryFn: () =>
      result<Detail>(
        client.GET("/api/exceptions/{key}", { params: { path: { key: id } } }),
      ),
  });
  const [reason, setReason] = useState("");
  const [expiry, setExpiry] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const data = query.data;
  const item = data?.exception;
  async function dispose(state: string) {
    if (!item) return;
    setBusy(true);
    setError("");
    try {
      await result(
        client.POST("/api/exceptions/{key}/disposition", {
          params: { path: { key: id } },
          body: {
            state,
            reason,
            revision: item.revision,
            suppression_until:
              state === "suppressed" && expiry
                ? new Date(expiry).toISOString()
                : null,
          },
        }),
      );
      setReason("");
      setNotice("Disposition saved to the audit trail.");
      await cache.invalidateQueries();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function comment() {
    setBusy(true);
    setError("");
    try {
      await result(
        client.POST("/api/exceptions/{key}/comments", {
          params: { path: { key: id } },
          body: { body: reason },
        }),
      );
      setReason("");
      await cache.invalidateQueries({ queryKey: ["exception", id] });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function replay(observation: string) {
    setBusy(true);
    setError("");
    try {
      await result<JSONObject>(
        client.POST("/api/observations/{id}/replay", {
          params: { path: { id: observation } },
        }),
      );
      setNotice(
        "Replay verified: snapshot checksum, rule version and decision result match.",
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={
        item?.entity_id.split("|").slice(1).join(" + ") ?? "Exception detail"
      }
      kicker={`${item?.rule_id ?? "Evidence"} / Investigation`}
      onClose={onClose}
    >
      {query.isPending ? (
        <Spinner />
      ) : query.error ? (
        <ErrorBox>{message(query.error)}</ErrorBox>
      ) : data && item ? (
        <>
          <div className="detail-badges">
            <Badge value={item.severity} />
            <Badge value={item.state} />
            <span>{item.period_id}</span>
          </div>
          {data.newer_snapshot_exists && (
            <div className="notice">
              A newer population exists. Original evidence below remains
              unchanged.
            </div>
          )}
          <h3>Evidence chain</h3>
          <dl className="evidence-grid">
            <dt>Rule version</dt>
            <dd>{item.rule_version}</dd>
            <dt>Original snapshot</dt>
            <dd>{item.snapshot_id}</dd>
            <dt>Parameters</dt>
            <dd>{item.parameter_set_hash}</dd>
            <dt>Engine</dt>
            <dd>{item.engine_version}</dd>
          </dl>
          <h3>
            Evaluations{" "}
            <span className="count">{data.observations.length}</span>
          </h3>
          {data.observations.map((o, index) => (
            <details key={o.id} open={index === 0}>
              <summary>
                {dateTime(o.created_at)} <Badge value={o.classification} />
              </summary>
              <div className="evidence-content">
                <dl className="input-grid">
                  {Object.entries(o.input).map(([k, v]) => (
                    <div key={k}>
                      <dt>{label(k)}</dt>
                      <dd>{String(v)}</dd>
                    </div>
                  ))}
                </dl>
                <button
                  className="button"
                  disabled={busy}
                  onClick={() => void replay(o.id)}
                >
                  <ShieldCheck size={15} /> Verify replay
                </button>
                <details>
                  <summary>Decision trace</summary>
                  <pre>{JSON.stringify(o.trace, null, 2)}</pre>
                </details>
                <details>
                  <summary>Version references</summary>
                  <pre>
                    {JSON.stringify(
                      {
                        run: o.run_id,
                        snapshot: o.snapshot_id,
                        rule: o.rule_version,
                        parameters: o.parameter_set_hash,
                        input_hash: o.input_hash,
                      },
                      null,
                      2,
                    )}
                  </pre>
                </details>
              </div>
            </details>
          ))}
          <h3>Investigation</h3>
          {data.has_review_case && (
            <div className="notice">
              This finding is managed through Review cases. Complete its
              required workflow steps there. You can add evidence notes here.
            </div>
          )}
          <label className="field">
            Reason or investigation note
            <textarea
              rows={3}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Record your assessment and supporting context…"
            />
          </label>
          {!data.has_review_case && item.state === "in_review" && (
            <label className="field">
              Suppression expiry (required to suppress)
              <input
                type="datetime-local"
                value={expiry}
                onChange={(e) => setExpiry(e.target.value)}
              />
            </label>
          )}
          <div className="actions">
            {!data.has_review_case &&
              ["open", "reopened"].includes(item.state) && (
                <button
                  disabled={busy || !reason.trim()}
                  className="button primary"
                  onClick={() => void dispose("in_review")}
                >
                  Start review
                </button>
              )}
            {!data.has_review_case && item.state === "in_review" && (
              <>
                <button
                  disabled={busy || !reason.trim()}
                  className="button primary"
                  onClick={() => void dispose("accepted")}
                >
                  Accept finding
                </button>
                <button
                  disabled={busy || !reason.trim()}
                  className="button"
                  onClick={() => void dispose("dismissed")}
                >
                  Dismiss
                </button>
                <button
                  disabled={busy || !reason.trim() || !expiry}
                  className="button"
                  onClick={() => void dispose("suppressed")}
                >
                  Suppress
                </button>
              </>
            )}
            <button
              disabled={busy || !reason.trim()}
              className="button"
              onClick={() => void comment()}
            >
              Add note
            </button>
          </div>
          {error && <ErrorBox>{error}</ErrorBox>}
          {notice && (
            <div role="status" className="notice">
              {notice}
            </div>
          )}
          <h3>Activity trail</h3>
          <ol className="timeline">
            {data.events.map((e) => (
              <li key={e.id}>
                <span className="timeline-dot" />
                <strong>
                  {label(e.action)} · {label(e.to_state)}
                </strong>
                <p>{e.reason}</p>
                <small>
                  {e.actor} · {dateTime(e.created_at)}
                </small>
              </li>
            ))}
          </ol>
        </>
      ) : null}
    </Drawer>
  );
}
