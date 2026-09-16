import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  Database,
  Download,
  FileSpreadsheet,
  Maximize2,
  Play,
  Plus,
  Save,
  Trash2,
  Upload,
} from "lucide-react";
import {
  Drawer,
  ErrorBox,
  SectionTitle,
  Spinner,
} from "../../components/Shared";
import {
  dateTime,
  formatNumber,
  message,
  requestJSON,
  type JSONObject,
} from "../../lib/client";
import { DatabasePreview } from "../sources/Preview";
import {
  cell,
  downloadResults,
  inputSchema,
  isFlagged,
  makeModel,
  operators,
  sameGraphLogic,
  simpleConditions,
  type Analysis,
  type Column,
  type Condition,
  type Dataset,
  type Report,
  type ResultRow,
} from "./model";

const RawEditor = lazy(() => import("../rules/RawEditor"));
const steps = ["Load data", "Choose columns", "Configure rules", "Results"];
type Preview = Dataset & { name?: string; origin?: string; sheets?: string[] };
type SavedSummary = {
  id: string;
  name: string;
  revision: number;
  row_count: number;
  updated_at: string;
};

export function AnalyzeData({ roles }: { roles: string[] }) {
  const cache = useQueryClient();
  const canSave = roles.some((role) =>
    ["implementer", "rule_engineer", "audit_manager"].includes(role),
  );
  const [savedPage, setSavedPage] = useState(1);
  const saved = useQuery({
    queryKey: ["analyses", savedPage],
    queryFn: () =>
      requestJSON<SavedSummary[]>(`/api/analyses?page=${savedPage}`),
  });
  const [step, setStep] = useState(0);
  const [source, setSource] = useState<"file" | "database">("file");
  const [name, setName] = useState("");
  const [id, setId] = useState<string>();
  const [revision, setRevision] = useState(0);
  const [dataset, setDataset] = useState<Dataset>();
  const [origin, setOrigin] = useState("");
  const [columns, setColumns] = useState<Column[]>([]);
  const [model, setModel] = useState<JSONObject>();
  const [conditions, setConditions] = useState<Condition[]>([]);
  const [custom, setCustom] = useState(false);
  const [conditionColumn, setConditionColumn] = useState("");
  const [operator, setOperator] = useState("eq");
  const [conditionValue, setConditionValue] = useState("");
  const [report, setReport] = useState<Report>();
  const [filter, setFilter] = useState("all");
  const [page, setPage] = useState(0);
  const [graphOpen, setGraphOpen] = useState(false);
  const [traceRow, setTraceRow] = useState<ResultRow>();
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState("");
  const [pendingWorkbook, setPendingWorkbook] = useState<{
    name: string;
    data: string;
    sheets: string[];
  }>();
  const [sheet, setSheet] = useState("");
  const [dirty, setDirty] = useState(false);
  const inputRevision = useRef(0);

  useEffect(() => {
    if (!dirty || !model || !canSave) return;
    const preserveDraft = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", preserveDraft);
    return () => window.removeEventListener("beforeunload", preserveDraft);
  }, [dirty, model, canSave]);

  function changed() {
    inputRevision.current++;
    setReport(undefined);
    setTraceRow(undefined);
    setStatus("");
    setDirty(true);
  }
  function reset() {
    if (
      dirty &&
      model &&
      !window.confirm("Discard your unsaved analysis and start a new one?")
    )
      return;
    inputRevision.current++;
    setStep(0);
    setId(undefined);
    setRevision(0);
    setName("");
    setDataset(undefined);
    setColumns([]);
    setModel(undefined);
    setConditions([]);
    setCustom(false);
    setReport(undefined);
    setPendingWorkbook(undefined);
    setSheet("");
    setOrigin("");
    setError("");
    setStatus("");
    setDirty(false);
    setTraceRow(undefined);
  }
  function acceptPreview(data: Preview, fallbackName = "Untitled analysis") {
    if (
      dirty &&
      model &&
      !window.confirm("Replace your current data and unsaved rules?")
    )
      return;
    inputRevision.current++;
    setTraceRow(undefined);
    setDataset({
      columns: data.columns,
      rows: data.rows,
      row_count: data.row_count,
    });
    setColumns(data.columns);
    setName(data.name || fallbackName);
    setOrigin(data.origin || "");
    setId(undefined);
    setRevision(0);
    setModel(undefined);
    setConditions([]);
    setCustom(false);
    setReport(undefined);
    setPendingWorkbook(undefined);
    setConditionColumn(data.columns[0]?.name || "");
    setConditionValue("");
    setError("");
    setStatus("");
    setDirty(true);
    setStep(1);
  }
  async function readFile(file?: File) {
    if (!file) return;
    setError("");
    setStatus("");
    setBusy("Reading data");
    setPendingWorkbook(undefined);
    try {
      if (file.size > 16 * 1024 * 1024)
        throw new Error("Choose a file smaller than 16 MB.");
      const extension = file.name.split(".").at(-1)?.toLowerCase();
      if (!["csv", "json", "xlsx"].includes(extension || ""))
        throw new Error("Choose a CSV, Excel (.xlsx), or JSON file.");
      const fileName = file.name.replace(/\.[^.]+$/, "");
      let data: string;
      if (extension === "xlsx") {
        const bytes = new Uint8Array(await file.arrayBuffer());
        const pieces = [];
        for (let at = 0; at < bytes.length; at += 32768)
          pieces.push(String.fromCharCode(...bytes.subarray(at, at + 32768)));
        data = btoa(pieces.join(""));
      } else data = await file.text();
      const preview = await requestJSON<Preview>("/api/analyses/preview", {
        format: extension,
        name: fileName,
        data,
      });
      if (preview.sheets) {
        setPendingWorkbook({ name: fileName, data, sheets: preview.sheets });
        setSheet(preview.sheets[0] || "");
      } else acceptPreview(preview, fileName);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy("");
    }
  }
  async function readSheet() {
    if (!pendingWorkbook || !sheet) return;
    setError("");
    setBusy("Reading worksheet");
    try {
      const data = await requestJSON<Preview>("/api/analyses/preview", {
        format: "xlsx",
        name: pendingWorkbook.name,
        data: pendingWorkbook.data,
        sheet,
      });
      acceptPreview(data, pendingWorkbook.name);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy("");
    }
  }
  function selectColumn(column: Column, selected: boolean) {
    const next = selected
      ? [...columns, column]
      : columns.filter((c) => c.name !== column.name);
    setColumns(
      dataset?.columns.flatMap((c) => next.filter((v) => c.name === v.name)) ||
        [],
    );
    changed();
  }
  function continueToRules() {
    if (!columns.length) return;
    const nextConditions = conditions.filter((c) =>
      columns.some((column) => column.name === c.column),
    );
    setConditions(nextConditions);
    if (!custom || !model) {
      const next = makeModel(columns, nextConditions);
      if (!model || !sameGraphLogic(model, next)) setModel(next);
    } else {
      const next = structuredClone(model) as {
        nodes?: { type: string; content?: JSONObject }[];
      };
      const input = next.nodes?.find((node) => node.type === "inputNode");
      if (input) input.content = { schema: inputSchema(columns) };
      setModel(next);
    }
    if (!columns.some((c) => c.name === conditionColumn))
      setConditionColumn(columns[0]!.name);
    setStep(2);
    setError("");
  }
  function addCondition() {
    setError("");
    const column = columns.find((c) => c.name === conditionColumn);
    if (!column) return;
    if (!["empty", "notempty"].includes(operator)) {
      if (!conditionValue.trim()) {
        setError("Enter a value, or choose Is empty.");
        return;
      }
      if (
        column.type === "number" &&
        (!/^-?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(
          conditionValue.trim(),
        ) ||
          !Number.isFinite(Number(conditionValue)))
      ) {
        setError("Enter a valid number for this condition.");
        return;
      }
      if (
        column.type === "boolean" &&
        !["true", "false"].includes(conditionValue)
      ) {
        setError("Choose true or false for this condition.");
        return;
      }
    }
    const next = [
      ...conditions,
      {
        id: crypto.randomUUID(),
        column: conditionColumn,
        operator,
        value:
          column.type === "string" ? conditionValue : conditionValue.trim(),
      },
    ];
    setConditions(next);
    setModel(makeModel(columns, next));
    setConditionValue("");
    changed();
  }
  function removeCondition(conditionId: string) {
    const next = conditions.filter((c) => c.id !== conditionId);
    setConditions(next);
    setModel(makeModel(columns, next));
    changed();
  }
  async function testRules(stayInGraph = false) {
    if (!dataset || !model) return;
    const requestRevision = inputRevision.current;
    setBusy("Testing rules");
    setError("");
    setStatus("");
    setReport(undefined);
    setTraceRow(undefined);
    try {
      const value = await requestJSON<Report>("/api/analyses/test", {
        dataset,
        model,
        selected_columns: columns,
      });
      if (requestRevision !== inputRevision.current) return;
      setReport(value);
      setFilter("all");
      setPage(0);
      if (!stayInGraph) setStep(3);
      else setTraceRow(value.results.find((r) => !r.error) || value.results[0]);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy("");
    }
  }
  async function save() {
    if (!dataset || !model || !name.trim()) {
      setError("Enter an analysis name before saving.");
      return;
    }
    setBusy("Saving analysis");
    setError("");
    setStatus("");
    const requestRevision = inputRevision.current;
    try {
      const value = await requestJSON<Analysis>("/api/analyses", {
        id,
        name: name.trim(),
        revision,
        dataset,
        model,
        selected_columns: columns,
        origin,
      });
      setId(value.id);
      setRevision(value.revision);
      setDirty(requestRevision !== inputRevision.current);
      setStatus(
        `Saved ${value.name}. Version ${value.revision}.${requestRevision !== inputRevision.current ? " New changes are not saved yet." : ""}`,
      );
      await cache.invalidateQueries({ queryKey: ["analyses"] });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy("");
    }
  }
  async function open(savedId: string) {
    if (
      dirty &&
      model &&
      !window.confirm("Discard your unsaved changes and open this analysis?")
    )
      return;
    inputRevision.current++;
    setBusy("Opening analysis");
    setError("");
    setStatus("");
    try {
      const value = await requestJSON<Analysis>(
        `/api/analyses/${encodeURIComponent(savedId)}`,
      );
      setId(value.id);
      setRevision(value.revision);
      setName(value.name);
      setDataset(value.dataset);
      setColumns(value.selected_columns);
      setModel(value.model);
      setOrigin(value.origin || "");
      const restored = simpleConditions(value.model, value.selected_columns);
      setConditions(restored || []);
      setCustom(restored === undefined);
      setConditionColumn(value.selected_columns[0]?.name || "");
      setReport(undefined);
      setTraceRow(undefined);
      setDirty(false);
      setStep(2);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy("");
    }
  }
  const filtered =
    report?.results.filter((row) =>
      filter === "errors"
        ? !!row.error
        : filter === "flagged"
          ? isFlagged(row.output)
          : true,
    ) || [];
  const selectedType = columns.find((c) => c.name === conditionColumn)?.type;
  const saveControl = canSave && model && (
    <button
      className="button"
      disabled={!!busy || !dirty}
      onClick={() => void save()}
    >
      <Save size={16} /> Save analysis
    </button>
  );

  return (
    <div className="analysis-workspace">
      <SectionTitle
        title="Analyze data"
        description="Load a file, choose your columns, and turn your checks into repeatable rules."
        action={
          dataset && (
            <button className="button" disabled={!!busy} onClick={reset}>
              <Plus size={16} /> New analysis
            </button>
          )
        }
      />
      <nav className="analysis-steps" aria-label="Analysis steps">
        {steps.map((label, index) => (
          <button
            key={label}
            aria-current={step === index ? "step" : undefined}
            disabled={
              !!busy ||
              (index === 1 && !dataset) ||
              (index === 2 && (!dataset || !columns.length)) ||
              (index === 3 && !report)
            }
            onClick={() => {
              if (index === 2) continueToRules();
              else setStep(index);
              setError("");
            }}
          >
            <span>{index < step ? <Check size={16} /> : index + 1}</span>
            {label}
          </button>
        ))}
      </nav>
      {error && !graphOpen && <ErrorBox>{error}</ErrorBox>}
      {status && !graphOpen && (
        <p className="analysis-status" role="status">
          <Check size={16} /> {status}
        </p>
      )}
      {busy && !graphOpen && (
        <p className="muted" role="status">
          {busy}...
        </p>
      )}

      {step === 0 && (
        <>
          <section className="panel padded analysis-load">
            <h2>What would you like to analyze?</h2>
            <div
              className="analysis-source-tabs"
              role="group"
              aria-label="Data source"
            >
              <button
                className={`button ${source === "file" ? "primary" : ""}`}
                onClick={() => setSource("file")}
              >
                <FileSpreadsheet size={17} /> File
              </button>
              <button
                className={`button ${source === "database" ? "primary" : ""}`}
                onClick={() => setSource("database")}
              >
                <Database size={17} /> Database
              </button>
            </div>
            {source === "file" ? (
              <>
                <label className="analysis-upload">
                  <Upload size={32} />
                  <strong>Choose data file</strong>
                  <span>CSV, Excel (.xlsx), or JSON</span>
                  <small>Up to 10,000 rows and 16 MB</small>
                  <input
                    aria-label="Choose data file"
                    type="file"
                    accept=".csv,.xlsx,.json"
                    disabled={!!busy}
                    onChange={(e) => {
                      void readFile(e.target.files?.[0]);
                      e.target.value = "";
                    }}
                  />
                </label>
                {pendingWorkbook && (
                  <div className="actions">
                    <label className="field">
                      Worksheet
                      <select
                        aria-label="Worksheet"
                        value={sheet}
                        onChange={(e) => setSheet(e.target.value)}
                      >
                        {pendingWorkbook.sheets.map((s) => (
                          <option key={s}>{s}</option>
                        ))}
                      </select>
                    </label>
                    <button
                      className="button primary"
                      disabled={!!busy || !sheet}
                      onClick={() => void readSheet()}
                    >
                      Preview worksheet <ArrowRight size={16} />
                    </button>
                  </div>
                )}
              </>
            ) : (
              <DatabasePreview onLoadDataset={acceptPreview} />
            )}
          </section>
          <section className="analysis-saved">
            <h2>Saved analyses</h2>
            {saved.isPending ? (
              <Spinner />
            ) : saved.error ? (
              <ErrorBox>{message(saved.error)}</ErrorBox>
            ) : saved.data?.length ? (
              <div className="analysis-saved-grid">
                {saved.data.map((item) => (
                  <button
                    className="analysis-saved-card"
                    key={item.id}
                    disabled={!!busy}
                    onClick={() => void open(item.id)}
                  >
                    <FileSpreadsheet size={20} />
                    <strong>{item.name}</strong>
                    <span>
                      {formatNumber(item.row_count)} rows · Version{" "}
                      {item.revision}
                    </span>
                    <small>{dateTime(item.updated_at)}</small>
                    <ArrowRight size={16} />
                  </button>
                ))}
              </div>
            ) : (
              <p className="muted">
                Save an analysis to reopen its data and rules here.
              </p>
            )}
            {(savedPage > 1 || (saved.data?.length ?? 0) >= 100) && (
              <div className="analysis-bottom-line">
                <span>Page {savedPage}</span>
                <div className="actions">
                  <button
                    className="button"
                    disabled={savedPage === 1 || saved.isFetching}
                    onClick={() => setSavedPage(savedPage - 1)}
                  >
                    Previous analyses
                  </button>
                  <button
                    className="button"
                    disabled={
                      (saved.data?.length ?? 0) < 100 || saved.isFetching
                    }
                    onClick={() => setSavedPage(savedPage + 1)}
                  >
                    Next analyses
                  </button>
                </div>
              </div>
            )}
          </section>
        </>
      )}

      {step === 1 && dataset && (
        <section className="panel padded">
          <div className="analysis-section-heading">
            <div>
              <h2>Choose columns</h2>
              <p className="muted">
                {formatNumber(dataset.row_count)} rows loaded. Select the
                columns your rules will use.
              </p>
            </div>
            <button
              className="button primary"
              disabled={!columns.length || !!busy}
              onClick={continueToRules}
            >
              Continue to rules <ArrowRight size={16} />
            </button>
          </div>
          <div className="analysis-column-actions">
            <button
              className="text-button"
              onClick={() => {
                setColumns(dataset.columns);
                changed();
              }}
            >
              Select all
            </button>
            <button
              className="text-button"
              onClick={() => {
                setColumns([]);
                changed();
              }}
            >
              Clear selection
            </button>
            <span>
              {columns.length} of {dataset.columns.length} selected
            </span>
          </div>
          <div
            className="analysis-data-table"
            role="region"
            aria-label="Data preview"
            tabIndex={0}
          >
            <table>
              <thead>
                <tr>
                  <th>Row</th>
                  {dataset.columns.map((column) => {
                    const selected = columns.find(
                      (c) => c.name === column.name,
                    );
                    return (
                      <th
                        key={column.name}
                        className={!selected ? "column-unselected" : ""}
                      >
                        <label className="analysis-column-name">
                          <input
                            type="checkbox"
                            aria-label={`Use ${column.name}`}
                            checked={!!selected}
                            onChange={(e) =>
                              selectColumn(column, e.target.checked)
                            }
                          />
                          {column.name}
                        </label>
                        <select
                          aria-label={`${column.name} type`}
                          disabled={!selected}
                          value={selected?.type || column.type}
                          onChange={(e) => {
                            setColumns(
                              columns.map((c) =>
                                c.name === column.name
                                  ? {
                                      ...c,
                                      type: e.target.value as Column["type"],
                                    }
                                  : c,
                              ),
                            );
                            changed();
                          }}
                        >
                          <option value="string">Text</option>
                          <option value="number">Number</option>
                          <option value="boolean">True / false</option>
                        </select>
                      </th>
                    );
                  })}
                </tr>
              </thead>
              <tbody>
                {dataset.rows.slice(0, 50).map((row, index) => (
                  <tr key={index}>
                    <td>{index + 1}</td>
                    {dataset.columns.map((column) => (
                      <td
                        key={column.name}
                        className={
                          !columns.some((c) => c.name === column.name)
                            ? "column-unselected"
                            : ""
                        }
                      >
                        {cell(row[column.name]) || (
                          <span className="muted">Empty</span>
                        )}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="analysis-bottom-line">
            <span>
              Previewing {Math.min(50, dataset.row_count)} of{" "}
              {formatNumber(dataset.row_count)} rows. Rules test every loaded
              row.
            </span>
            <button className="button" onClick={() => setStep(0)}>
              <ArrowLeft size={15} /> Change data
            </button>
          </div>
        </section>
      )}

      {step === 2 && dataset && model && (
        <section className="panel padded">
          <div className="analysis-section-heading">
            <div>
              <h2>Configure rules</h2>
              <p className="muted">
                {columns.length} selected columns ·{" "}
                {formatNumber(dataset.row_count)} rows
              </p>
            </div>
            <button className="button" onClick={() => setStep(1)}>
              <ArrowLeft size={15} /> Choose columns
            </button>
          </div>
          <label className="field analysis-name">
            Analysis name
            <input
              disabled={!!busy}
              value={name}
              maxLength={120}
              onChange={(e) => {
                setName(e.target.value);
                inputRevision.current++;
                setDirty(true);
                setStatus("");
              }}
              placeholder="For example, invoices above approval limit"
            />
          </label>
          {!custom ? (
            <>
              <h3>Flag a row when all conditions match</h3>
              {conditions.length > 0 && (
                <div className="analysis-conditions">
                  {conditions.map((condition, index) => (
                    <div key={condition.id}>
                      <span className="analysis-condition-join">
                        {index ? "AND" : "WHEN"}
                      </span>
                      <strong>{condition.column}</strong>
                      <span>
                        {operators
                          .find((o) => o.value === condition.operator)
                          ?.label.toLowerCase()}
                      </span>
                      {!["empty", "notempty"].includes(condition.operator) && (
                        <code>{condition.value}</code>
                      )}
                      <button
                        className="icon-button"
                        aria-label={`Remove condition ${index + 1}`}
                        onClick={() => removeCondition(condition.id)}
                      >
                        <Trash2 size={16} />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              <form
                className="analysis-condition-form"
                onSubmit={(e) => {
                  e.preventDefault();
                  addCondition();
                }}
              >
                <label className="field">
                  Column
                  <select
                    aria-label="Column"
                    value={conditionColumn}
                    onChange={(e) => {
                      setConditionColumn(e.target.value);
                      setConditionValue("");
                      setOperator("eq");
                    }}
                  >
                    {columns.map((c) => (
                      <option key={c.name}>{c.name}</option>
                    ))}
                  </select>
                </label>
                <label className="field">
                  Condition
                  <select
                    aria-label="Condition"
                    value={operator}
                    onChange={(e) => setOperator(e.target.value)}
                  >
                    {operators
                      .filter(
                        (o) =>
                          selectedType !== "boolean" ||
                          ["eq", "neq", "empty", "notempty"].includes(o.value),
                      )
                      .map((o) => (
                        <option value={o.value} key={o.value}>
                          {o.label}
                        </option>
                      ))}
                  </select>
                </label>
                <label className="field">
                  Value
                  {selectedType === "boolean" ? (
                    <select
                      aria-label="Value"
                      value={conditionValue}
                      disabled={["empty", "notempty"].includes(operator)}
                      onChange={(e) => setConditionValue(e.target.value)}
                    >
                      <option value="">Choose a value</option>
                      <option>true</option>
                      <option>false</option>
                    </select>
                  ) : (
                    <input
                      value={conditionValue}
                      disabled={["empty", "notempty"].includes(operator)}
                      inputMode={
                        selectedType === "number" ? "decimal" : undefined
                      }
                      onChange={(e) => setConditionValue(e.target.value)}
                      placeholder={
                        selectedType === "number"
                          ? "Enter a number"
                          : "Enter text"
                      }
                    />
                  )}
                </label>
                <button type="submit" className="button" disabled={!!busy}>
                  <Plus size={16} /> Add condition
                </button>
              </form>
            </>
          ) : (
            <div className="analysis-custom">
              <div>
                <h3>Your decision graph is ready</h3>
                <p className="muted">
                  Open the graph to edit decision tables, expressions,
                  functions, and branches.
                </p>
              </div>
              <button
                className="text-button"
                onClick={() => {
                  setCustom(false);
                  setConditions([]);
                  setModel(makeModel(columns, []));
                  changed();
                }}
              >
                Replace with simple conditions
              </button>
            </div>
          )}
          <div className="analysis-flow-preview" aria-label="Rule flow">
            <span>
              <Database size={18} /> Your data
            </span>
            <ArrowRight size={18} />
            <button
              onClick={() => {
                setTraceRow(undefined);
                setGraphOpen(true);
              }}
            >
              <Maximize2 size={18} />{" "}
              {custom
                ? "Decision graph"
                : `${conditions.length} condition${conditions.length === 1 ? "" : "s"}`}
            </button>
            <ArrowRight size={18} />
            <span>
              <Check size={18} /> Results
            </span>
          </div>
          <div className="analysis-bottom-line">
            <button
              className="button"
              onClick={() => {
                setTraceRow(undefined);
                setGraphOpen(true);
              }}
            >
              <Maximize2 size={16} /> Open decision graph
            </button>
            <div className="actions">
              {saveControl}
              <button
                className="button primary"
                disabled={!!busy}
                onClick={() => void testRules()}
              >
                <Play size={16} /> Test rules
              </button>
            </div>
          </div>
        </section>
      )}

      {step === 3 && report && dataset && (
        <section className="panel padded">
          <div className="analysis-section-heading">
            <div>
              <h2>Results</h2>
              <p className="muted">
                {name} · {formatNumber(report.total)} rows tested
              </p>
            </div>
            <div className="actions">
              <button className="button" onClick={() => setStep(2)}>
                <ArrowLeft size={15} /> Edit rules
              </button>
              {saveControl}
            </div>
          </div>
          <div className="analysis-result-summary">
            <div>
              <strong>{formatNumber(report.total)}</strong>
              <span>Rows tested</span>
            </div>
            <div>
              <strong>{formatNumber(report.flagged)}</strong>
              <span>Flagged</span>
            </div>
            <div className={report.errors ? "has-errors" : ""}>
              <strong>{formatNumber(report.errors)}</strong>
              <span>Row errors</span>
            </div>
            <p>
              Analysis results use the loaded data. They do not create audit
              cases or certify source completeness.
            </p>
          </div>
          <div className="analysis-bottom-line">
            <div
              className="analysis-result-filters"
              role="group"
              aria-label="Result filter"
            >
              {[
                { value: "all", label: "All rows" },
                { value: "flagged", label: "Flagged" },
                { value: "errors", label: "Errors" },
              ].map((f) => (
                <button
                  className={`button ${filter === f.value ? "primary" : ""}`}
                  key={f.value}
                  onClick={() => {
                    setFilter(f.value);
                    setPage(0);
                  }}
                >
                  {f.label}
                </button>
              ))}
            </div>
            <button
              className="button"
              onClick={() => downloadResults(name, columns, filtered)}
              disabled={!filtered.length}
            >
              <Download size={16} /> Export results
            </button>
          </div>
          <div
            className="analysis-data-table"
            role="region"
            aria-label="Analysis results"
            tabIndex={0}
          >
            <table>
              <thead>
                <tr>
                  <th>Source row</th>
                  <th>Status</th>
                  {columns.map((c) => (
                    <th key={c.name}>{c.name}</th>
                  ))}
                  <th>Result</th>
                  <th>Details</th>
                </tr>
              </thead>
              <tbody>
                {filtered.slice(page * 50, page * 50 + 50).map((row) => (
                  <tr key={row.row}>
                    <td>{row.row}</td>
                    <td>
                      <span
                        className={`badge ${row.error ? "failed" : isFlagged(row.output) ? "high" : "completed"}`}
                      >
                        {row.error
                          ? "Error"
                          : isFlagged(row.output)
                            ? "Flagged"
                            : "Passed"}
                      </span>
                    </td>
                    {columns.map((c) => (
                      <td key={c.name}>{cell(row.input[c.name])}</td>
                    ))}
                    <td>{row.error || cell(row.output)}</td>
                    <td>
                      <button
                        className="text-button"
                        aria-label={`Inspect row ${row.row}`}
                        onClick={() => {
                          setTraceRow(row);
                          setGraphOpen(true);
                        }}
                      >
                        Inspect
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!filtered.length && (
              <p className="analysis-empty">No rows match this filter.</p>
            )}
          </div>
          <div className="analysis-bottom-line">
            <span>
              {filtered.length ? page * 50 + 1 : 0} to{" "}
              {Math.min(page * 50 + 50, filtered.length)} of{" "}
              {formatNumber(filtered.length)} rows
            </span>
            <div className="actions">
              <button
                className="button"
                disabled={!page}
                onClick={() => setPage(page - 1)}
              >
                Previous
              </button>
              <button
                className="button"
                disabled={(page + 1) * 50 >= filtered.length}
                onClick={() => setPage(page + 1)}
              >
                Next
              </button>
            </div>
          </div>
        </section>
      )}

      {graphOpen && model && (
        <Drawer
          fullscreen
          title="Decision graph"
          kicker={name || "New analysis"}
          onClose={() => setGraphOpen(false)}
        >
          <div className="analysis-graph-toolbar">
            <div>
              <strong>{columns.length} input columns</strong>
              <span>
                {formatNumber(dataset?.row_count || 0)} rows available
              </span>
            </div>
            <div className="actions">
              {saveControl}
              <button
                className="button primary"
                disabled={!!busy}
                onClick={() => void testRules(true)}
              >
                <Play size={16} /> Test rules
              </button>
              {report && (
                <button
                  className="button"
                  onClick={() => {
                    setGraphOpen(false);
                    setStep(3);
                  }}
                >
                  View results ({report.flagged} flagged)
                </button>
              )}
            </div>
          </div>
          {error && <ErrorBox>{error}</ErrorBox>}
          {status && (
            <p className="analysis-status" role="status">
              <Check size={16} /> {status}
            </p>
          )}
          {busy && (
            <p className="muted" role="status">
              {busy}...
            </p>
          )}
          {traceRow && (
            <details className="analysis-trace">
              <summary>
                Row {traceRow.row}:{" "}
                {traceRow.error ||
                  (isFlagged(traceRow.output) ? "Flagged" : "Passed")}
              </summary>
              <pre>
                {JSON.stringify(
                  {
                    input: traceRow.input,
                    output: traceRow.output,
                    error: traceRow.error,
                  },
                  null,
                  2,
                )}
              </pre>
            </details>
          )}
          <Suspense fallback={<Spinner />}>
            <RawEditor
              value={model}
              readOnly={!!busy}
              traceRow={traceRow}
              onChange={(value) => {
                const same = sameGraphLogic(model, value);
                const next = { ...value };
                delete next._autodit;
                if (same && model._autodit) next._autodit = model._autodit;
                setModel(next);
                if (!same) {
                  setCustom(true);
                  setConditions([]);
                  changed();
                } else {
                  inputRevision.current++;
                  setDirty(true);
                  setStatus("");
                }
              }}
            />
          </Suspense>
        </Drawer>
      )}
    </div>
  );
}
