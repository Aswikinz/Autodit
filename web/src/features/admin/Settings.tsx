import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { client, result, message, type Parameters } from "../../lib/client";
import { ErrorBox, Help } from "../../components/Shared";
type Settings = {
  name: string;
  timezone: string;
  locale: string;
  reporting_currency: string;
  fiscal_start_month: number;
};
export function SettingsPanel() {
  const cache = useQueryClient();
  const workspace = useQuery({
    queryKey: ["workspace"],
    queryFn: () =>
      result<{ settings: Settings; revision: number }>(
        client.GET("/api/workspace"),
      ),
  });
  const policies = useQuery({
    queryKey: ["parameters"],
    queryFn: () =>
      result<{ parameters: Parameters; revision: number }>(
        client.GET("/api/parameters"),
      ),
  });
  const [settings, setSettings] = useState<Settings>();
  const [parameters, setParameters] = useState<Parameters>();
  const [code, setCode] = useState("");
  const [threshold, setThreshold] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const s = settings ?? workspace.data?.settings;
  const original = policies.data?.parameters;
  const p =
    parameters ??
    (original
      ? {
          ...original,
          explicit_currencies: true,
          currency_thresholds: original.explicit_currencies
            ? (original.currency_thresholds ?? {})
            : { USD: original.materiality, ...original.currency_thresholds },
        }
      : undefined);
  async function saveWorkspace() {
    if (!s || !workspace.data) return;
    setBusy(true);
    setError("");
    try {
      await result(
        client.PUT("/api/admin/settings", {
          body: { settings: s, revision: workspace.data.revision },
        }),
      );
      setSettings(undefined);
      await cache.invalidateQueries({ queryKey: ["workspace"] });
      setNotice("Workspace settings saved.");
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function savePolicy() {
    if (!p || !policies.data) return;
    setBusy(true);
    setError("");
    try {
      await result(
        client.PUT("/api/parameters", {
          body: {
            parameters: { ...p, explicit_currencies: true },
            revision: policies.data.revision,
          },
        }),
      );
      setParameters(undefined);
      await cache.invalidateQueries({ queryKey: ["parameters"] });
      setNotice("Audit policy released for future runs.");
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel padded">
      <h2>Workspace settings</h2>
      <Help title="Set up policies">
        <p>
          Set each transaction currency and its materiality threshold
          explicitly. The reporting currency labels your workspace; it does not
          convert amounts or supply an exchange rate. Imports must supply their
          own reporting amounts and controls.
        </p>
        <p>
          Dates use the selected timezone and locale. The fiscal start month
          records your reporting convention; every extraction still declares its
          own period.
        </p>
      </Help>
      {workspace.error && <ErrorBox>{message(workspace.error)}</ErrorBox>}
      {policies.error && <ErrorBox>{message(policies.error)}</ErrorBox>}
      {s && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void saveWorkspace();
          }}
        >
          <div className="parameter-grid">
            <label className="field">
              Workspace name
              <input
                required
                value={s.name}
                onChange={(e) => setSettings({ ...s, name: e.target.value })}
              />
            </label>
            <label className="field">
              Reporting currency
              <input
                required
                placeholder="Choose a three-letter code"
                pattern="[A-Z]{3}"
                maxLength={3}
                value={s.reporting_currency}
                onChange={(e) =>
                  setSettings({
                    ...s,
                    reporting_currency: e.target.value.toUpperCase(),
                  })
                }
              />
            </label>
            <label className="field">
              Timezone
              <input
                required
                placeholder="e.g. Asia/Colombo"
                value={s.timezone}
                onChange={(e) =>
                  setSettings({ ...s, timezone: e.target.value })
                }
              />
            </label>
            <label className="field">
              Date and number locale
              <input
                required
                placeholder="e.g. en-LK"
                value={s.locale}
                onChange={(e) => setSettings({ ...s, locale: e.target.value })}
              />
            </label>
            <label className="field">
              Fiscal year starts
              <select
                required
                value={s.fiscal_start_month}
                onChange={(e) =>
                  setSettings({
                    ...s,
                    fiscal_start_month: Number(e.target.value),
                  })
                }
              >
                <option value={0}>Choose a month</option>
                {[
                  "January",
                  "February",
                  "March",
                  "April",
                  "May",
                  "June",
                  "July",
                  "August",
                  "September",
                  "October",
                  "November",
                  "December",
                ].map((m, i) => (
                  <option key={m} value={i + 1}>
                    {m}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <button
            type="submit"
            className="button primary"
            disabled={busy || !settings}
          >
            Save workspace settings
          </button>
        </form>
      )}
      {p && (
        <>
          <h3>Transaction currencies and materiality</h3>
          {!original?.explicit_currencies && (
            <p className="notice">
              This workspace has a legacy policy. Review the existing currency
              below before releasing an explicit policy.
            </p>
          )}
          <div className="parameter-grid">
            {Object.entries(p.currency_thresholds ?? {}).map(
              ([currency, value]) => (
                <label className="field" key={currency}>
                  {currency} threshold
                  <input
                    value={value}
                    onChange={(e) =>
                      setParameters({
                        ...p,
                        currency_thresholds: {
                          ...p.currency_thresholds,
                          [currency]: e.target.value,
                        },
                      })
                    }
                  />
                  <button
                    type="button"
                    className="button"
                    onClick={() => {
                      const next = { ...p.currency_thresholds };
                      delete next[currency];
                      setParameters({ ...p, currency_thresholds: next });
                    }}
                  >
                    Remove {currency}
                  </button>
                </label>
              ),
            )}
          </div>
          <div className="actions">
            <input
              aria-label="Currency code"
              placeholder="Currency code"
              maxLength={3}
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            <input
              aria-label="Materiality threshold"
              placeholder="Materiality threshold"
              value={threshold}
              onChange={(e) => setThreshold(e.target.value)}
            />
            <button
              type="button"
              className="button"
              disabled={
                !/^[A-Z]{3}$/.test(code) || !/^\d+(\.\d{1,4})?$/.test(threshold)
              }
              onClick={() => {
                setParameters({
                  ...p,
                  currency_thresholds: {
                    ...p.currency_thresholds,
                    [code]: threshold,
                  },
                });
                setCode("");
                setThreshold("");
              }}
            >
              Add currency
            </button>
          </div>
          <label className="field">
            Maximum exceptions per test
            <input
              type="number"
              min={1}
              max={100000}
              value={p.exception_ceiling}
              onChange={(e) =>
                setParameters({
                  ...p,
                  exception_ceiling: Number(e.target.value),
                })
              }
            />
          </label>
          <fieldset className="days">
            <legend>Non-working days</legend>
            {["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map((d, i) => (
              <label key={d}>
                <input
                  type="checkbox"
                  checked={p.weekend_days.includes(i)}
                  onChange={(e) =>
                    setParameters({
                      ...p,
                      weekend_days: e.target.checked
                        ? [...p.weekend_days, i].sort((a, b) => a - b)
                        : p.weekend_days.filter((v) => v !== i),
                    })
                  }
                />
                {d}
              </label>
            ))}
          </fieldset>
          <button
            type="button"
            className="button primary"
            disabled={
              busy ||
              !parameters ||
              !Object.keys(p.currency_thresholds ?? {}).length
            }
            onClick={() => void savePolicy()}
          >
            Release audit policy
          </button>
        </>
      )}
      {error && <ErrorBox>{error}</ErrorBox>}
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
    </section>
  );
}
