import { useState } from "react";
import { client, result, message, requestJSON } from "../../lib/client";
import type { Dataset } from "../analysis/model";
import { ErrorBox } from "../../components/Shared";
export type PreviewData = {
  columns: string[];
  sample: string[][];
  truncated?: boolean;
};
export function TablePreview({ data }: { data: PreviewData }) {
  return (
    <section>
      <h3>Record preview</h3>
      <p className="muted">
        Showing up to 50 records. This sample does not establish completeness.
      </p>
      <div
        className="preview-table"
        tabIndex={0}
        role="region"
        aria-label="Record preview"
      >
        <table>
          <thead>
            <tr>
              {data.columns.map((c, i) => (
                <th key={i}>{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.sample.map((row, i) => (
              <tr key={i}>
                {data.columns.map((_, j) => (
                  <td key={j}>{row[j] ?? ""}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        {!data.sample.length && <p>No records returned.</p>}
      </div>
    </section>
  );
}
type Connection = {
  kind: string;
  host: string;
  port: number;
  database: string;
  username: string;
  password: string;
  allow_plaintext: boolean;
};
type Table = { schema: string; name: string };
export function DatabasePreview({
  onLoadDataset,
}: {
  onLoadDataset?: (
    dataset: Dataset & { name?: string; origin?: string },
  ) => void;
} = {}) {
  const [connection, setConnection] = useState<Connection>({
    kind: "postgres",
    host: "",
    port: 5432,
    database: "",
    username: "",
    password: "",
    allow_plaintext: false,
  });
  const [tables, setTables] = useState<Table[]>();
  const [selected, setSelected] = useState("");
  const [preview, setPreview] = useState<PreviewData>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  function edit(next: Connection) {
    setConnection(next);
    setTables(undefined);
    setPreview(undefined);
    setSelected("");
    setStatus("");
  }
  async function test() {
    setBusy(true);
    setError("");
    setPreview(undefined);
    setTables(undefined);
    try {
      const out = onLoadDataset
        ? await requestJSON<{ tables: Table[]; status: string }>(
            "/api/analyses/connection",
            connection,
          )
        : await result<{ tables: Table[]; status: string }>(
            client.POST("/api/sources/connection", { body: connection }),
          );
      setTables(out.tables);
      setStatus(
        `${out.status}. ${out.tables.length} tables or views available (maximum 500).`,
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function read() {
    const table = tables?.[Number(selected)];
    if (!table) return;
    setBusy(true);
    setError("");
    setPreview(undefined);
    try {
      if (onLoadDataset) {
        const data = await requestJSON<
          Dataset & { name?: string; origin?: string }
        >("/api/analyses/preview", {
          format: "database",
          name: table.name,
          connection,
          table,
        });
        onLoadDataset(data);
        return;
      }
      setPreview(
        await result<PreviewData>(
          client.POST("/api/sources/preview", { body: { connection, table } }),
        ),
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel padded">
      <h2>
        {onLoadDataset
          ? "Connect to your database"
          : "Test a database connection"}
      </h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void test();
        }}
      >
        <fieldset disabled={busy}>
          <div className="parameter-grid">
            <label className="field">
              Database type
              <select
                value={connection.kind}
                onChange={(e) =>
                  edit({
                    ...connection,
                    kind: e.target.value,
                    port:
                      { postgres: 5432, mysql: 3306, sqlserver: 1433 }[
                        e.target.value
                      ] ?? 5432,
                  })
                }
              >
                <option value="postgres">PostgreSQL</option>
                <option value="mysql">MySQL</option>
                <option value="sqlserver">SQL Server</option>
              </select>
            </label>
            <label className="field">
              Host
              <input
                required
                value={connection.host}
                placeholder="Database hostname or IP"
                onChange={(e) => edit({ ...connection, host: e.target.value })}
              />
            </label>
            <label className="field">
              Port
              <input
                required
                type="number"
                min={1}
                max={65535}
                value={connection.port}
                onChange={(e) =>
                  edit({ ...connection, port: Number(e.target.value) })
                }
              />
            </label>
            <label className="field">
              Database
              <input
                required
                value={connection.database}
                onChange={(e) =>
                  edit({ ...connection, database: e.target.value })
                }
              />
            </label>
            <label className="field">
              Database username
              <input
                required
                autoComplete="off"
                value={connection.username}
                onChange={(e) =>
                  edit({ ...connection, username: e.target.value })
                }
              />
            </label>
            <label className="field">
              Database password
              <input
                type="password"
                autoComplete="off"
                value={connection.password}
                onChange={(e) =>
                  edit({ ...connection, password: e.target.value })
                }
              />
            </label>
          </div>
          <label className="toggle">
            <input
              type="checkbox"
              checked={connection.allow_plaintext}
              onChange={(e) =>
                edit({ ...connection, allow_plaintext: e.target.checked })
              }
            />{" "}
            Allow an unencrypted connection on a trusted network
          </label>
          <button type="submit" className="button primary">
            {busy ? "Connecting..." : "Test connection"}
          </button>
        </fieldset>
      </form>
      {status && (
        <p className="notice" role="status">
          {status}
        </p>
      )}
      {tables && (
        <div className="actions">
          <label className="field">
            Table or view
            <select
              value={selected}
              onChange={(e) => {
                setSelected(e.target.value);
                setPreview(undefined);
              }}
            >
              <option value="">Choose a table</option>
              {tables.map((t, i) => (
                <option key={i} value={i}>
                  {t.schema}.{t.name}
                </option>
              ))}
            </select>
          </label>
          <button
            className="button"
            disabled={busy || selected === ""}
            onClick={() => void read()}
          >
            {onLoadDataset ? "Load table" : "Preview table"}
          </button>
        </div>
      )}
      {error && <ErrorBox>{error}</ErrorBox>}
      {preview && <TablePreview data={preview} />}
    </section>
  );
}
