import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, FileUp, FlaskConical, Plus, Database } from "lucide-react";
import { client, result, message, dateTime } from "../../lib/client";
import type { JSONObject, Source } from "../../lib/client";
import { Badge, ErrorBox, SectionTitle } from "../../components/Shared";
import example from "../../../../test/fixtures/population.json";

const fields = [
  "entity",
  "id",
  "date",
  "amount",
  "currency",
  "reporting_amount",
  "reporting_currency",
  "exchange_rate",
  "rate_date",
  "vendor_id",
  "debit",
  "credit",
  "limit",
  "reversal",
  "intercompany",
];
export function Sources({ onRun }: { onRun: () => void }) {
  const cache = useQueryClient();
  const sources = useQuery({
    queryKey: ["sources"],
    queryFn: () => result<Source[]>(client.GET("/api/sources")),
  });
  const [source, setSource] = useState("test-source");
  const [name, setName] = useState("Finance file extract");
  const [interval, setInterval] = useState(1440);
  const [csv, setCSV] = useState("");
  const [fileName, setFileName] = useState("");
  const [population, setPopulation] = useState<JSONObject>();
  const [metadata, setMetadata] = useState<JSONObject>();
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [profile, setProfile] = useState<{
    columns: string[];
    rows: number;
    nulls: Record<string, number>;
  }>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  async function action(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
      await cache.invalidateQueries();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    await result(
      client.POST("/api/sources", {
        body: { id: source, name, interval_minutes: interval, mapping },
      }),
    );
  }
  async function readFile(file: File | undefined) {
    if (!file) return;
    if (file.size > 32 * 1024 * 1024) {
      setError("Maximum file size is 32 MB.");
      return;
    }
    setError("");
    setNotice("");
    setFileName(file.name);
    setPopulation(undefined);
    setCSV("");
    setProfile(undefined);
    try {
      const text = await file.text();
      if (file.name.toLowerCase().endsWith(".json")) {
        const parsed: unknown = JSON.parse(text);
        if (
          typeof parsed !== "object" ||
          parsed === null ||
          !("records" in parsed)
        )
          throw new Error(
            "Choose a population JSON file with records and independent controls.",
          );
        setPopulation(parsed as JSONObject);
        if ("source_id" in parsed) setSource(String(parsed.source_id));
      } else {
        setCSV(text);
        const p = await result<{
          columns: string[];
          rows: number;
          nulls: Record<string, number>;
        }>(client.POST("/api/sources/profile", { body: { csv: text } }));
        setProfile(p);
        setMapping(
          Object.fromEntries(
            fields.map((field) => [
              field,
              p.columns.find((c) => c.toLowerCase() === field) ?? "",
            ]),
          ),
        );
      }
    } catch (e) {
      setError(message(e));
    }
  }
  async function readControls(file: File | undefined) {
    if (!file) return;
    try {
      const p: unknown = JSON.parse(await file.text());
      if (
        typeof p !== "object" ||
        p === null ||
        !("controls" in p) ||
        !("period" in p)
      )
        throw new Error("Control report must contain period and controls.");
      setMetadata(p as JSONObject);
    } catch (e) {
      setError(message(e));
    }
  }
  async function submit() {
    await save();
    if (population) {
      await result(
        client.POST("/api/runs", {
          body: { ...population, source_id: source },
        }),
      );
    } else {
      if (!metadata)
        throw new Error("Attach the independently supplied control report.");
      await result(
        client.POST("/api/imports", {
          body: {
            csv,
            mapping,
            population: { ...metadata, source_id: source, records: [] },
          },
        }),
      );
    }
    onRun();
  }
  return (
    <>
      <SectionTitle
        title="Sources & mapping"
        description="Connect a complete population to the canonical audit model."
      />
      <div className="two-column">
        <section className="panel padded">
          <div className="panel-title">
            <Database size={20} />
            <h2>Source configuration</h2>
          </div>
          <label className="field">
            Source identifier
            <input
              value={source}
              onChange={(e) => setSource(e.target.value)}
              placeholder="finance-erp"
            />
          </label>
          <label className="field">
            Display name
            <input value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="field">
            Expected interval (minutes)
            <input
              type="number"
              min={1}
              max={525600}
              value={interval}
              onChange={(e) => setInterval(Number(e.target.value))}
            />
            <small>
              A missing successful run after this interval is flagged as
              overdue.
            </small>
          </label>
          <button
            className="button"
            disabled={busy}
            onClick={() =>
              void action(async () => {
                await save();
                setNotice("Source configuration saved.");
              })
            }
          >
            <Plus size={16} /> Save source
          </button>
          <hr />
          <h3>Configured sources</h3>
          {sources.data?.map((s) => (
            <button
              key={s.id}
              className="source-choice"
              onClick={() => {
                setSource(s.id);
                setName(s.name);
                setInterval(s.interval_minutes);
                setMapping(s.mapping ?? {});
              }}
            >
              <span>
                <strong>{s.name}</strong>
                <small>
                  {s.id} · {dateTime(s.last_landed_at)}
                </small>
              </span>
              <Badge value={s.overdue ? "overdue" : s.kind} />
            </button>
          ))}
          {sources.data?.length === 0 && (
            <p className="muted">No sources configured yet.</p>
          )}
        </section>
        <section className="panel padded">
          <div className="panel-title">
            <FileUp size={20} />
            <h2>Import a full population</h2>
          </div>
          <p className="muted">
            Upload a CSV plus an independent control report, or a population
            JSON containing both. Each import covers one complete source and
            fiscal period.
          </p>
          <label className="upload">
            <FileUp size={30} />
            <strong>{fileName || "Choose an extract"}</strong>
            <span>CSV or JSON · maximum 32 MB</span>
            <input
              aria-label="Choose extract file"
              type="file"
              accept=".csv,.json"
              onChange={(e) => void readFile(e.target.files?.[0])}
            />
          </label>
          {csv && (
            <>
              <label className="field">
                Independent control report (JSON)
                <input
                  type="file"
                  accept=".json"
                  onChange={(e) => void readControls(e.target.files?.[0])}
                />
              </label>
              {metadata && (
                <div className="notice">
                  Control report attached. Totals will be checked before any
                  test executes.
                </div>
              )}
            </>
          )}
          {population && (
            <div className="notice">
              Population loaded.{" "}
              {Array.isArray(population.records)
                ? population.records.length
                : 0}{" "}
              records. Controls will be verified by the worker.
            </div>
          )}
          {profile && (
            <>
              <h3>
                Column mapping{" "}
                <span className="count">{profile.rows} rows</span>
              </h3>
              <div className="mapping-grid">
                {fields.map((field) => (
                  <label className="field" key={field}>
                    {field}
                    <select
                      value={mapping[field] ?? ""}
                      onChange={(e) =>
                        setMapping({ ...mapping, [field]: e.target.value })
                      }
                    >
                      <option value="">Select source column</option>
                      {profile.columns.map((c) => (
                        <option key={c} value={c}>
                          {c}
                          {profile.nulls[c]
                            ? ` (${profile.nulls[c]} empty)`
                            : ""}
                        </option>
                      ))}
                    </select>
                  </label>
                ))}
              </div>
            </>
          )}
          <button
            className="button primary"
            disabled={busy || (!population && (!csv || !metadata))}
            onClick={() => void action(submit)}
          >
            Validate & queue run <ArrowRight size={16} />
          </button>
          <div className="contract-note">
            Control totals must come from an independent source report. The
            extract's own row count is a diagnostic, never proof of
            completeness.
          </div>
        </section>
      </div>
      <section className="demo-panel">
        <FlaskConical size={26} />
        <div>
          <h3>Explore with a synthetic population</h3>
          <p>
            Five fictional records exercise duplicate payments, weekend postings
            and an approval breach. No client data is included.
          </p>
        </div>
        <button
          className="button"
          disabled={busy}
          onClick={() =>
            void action(async () => {
              await result(
                client.POST("/api/sources", {
                  body: {
                    id: example.source_id,
                    name: "Synthetic finance extract",
                    interval_minutes: 1440,
                    mapping: {},
                  },
                }),
              );
              await result(client.POST("/api/runs", { body: example }));
              onRun();
            })
          }
        >
          Run example <ArrowRight size={16} />
        </button>
      </section>
      {error && <ErrorBox>{error}</ErrorBox>}
      {notice && (
        <div className="notice" role="status">
          {notice}
        </div>
      )}
    </>
  );
}
