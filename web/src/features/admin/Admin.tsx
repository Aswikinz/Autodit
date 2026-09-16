import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { client, result, message, label } from "../../lib/client";
import { ErrorBox, Help, SectionTitle, Spinner } from "../../components/Shared";
import { SettingsPanel } from "./Settings";

type User = {
  username: string;
  display_name: string;
  roles: string[];
  enabled: boolean;
  must_change_password?: boolean;
  revision: number;
};
const blank: User = {
  username: "",
  display_name: "",
  roles: ["auditor"],
  enabled: true,
  revision: 0,
};
export function Admin() {
  const cache = useQueryClient();
  const q = useQuery({
    queryKey: ["users"],
    queryFn: () =>
      result<{ users: User[]; roles: Record<string, string> }>(
        client.GET("/api/admin/users"),
      ),
  });
  const [draft, setDraft] = useState<User>(blank);
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  async function save() {
    setError("");
    setBusy(true);
    try {
      await result(
        client.POST("/api/admin/users", { body: { ...draft, password } }),
      );
      setDraft(blank);
      setPassword("");
      setNotice(
        "Account saved. Changed accounts must sign in again. New and reset passwords must be changed at first login.",
      );
      await cache.invalidateQueries({ queryKey: ["users"] });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <SectionTitle
        title="Administration"
        description="Manage access and the settings used throughout your workspace."
      />
      <Help title="Manage users">
        <ol>
          <li>Create a named account for each colleague.</li>
          <li>
            Choose roles by responsibility. You can select more than one.
            Administrators manage access; add an operational role if they also
            perform audit work.
          </li>
          <li>
            Give the initial password to the account holder privately. They must
            replace it on first login.
          </li>
          <li>
            Disable access when someone leaves. You must keep at least one
            enabled administrator.
          </li>
        </ol>
      </Help>
      {q.isPending ? (
        <Spinner />
      ) : q.error ? (
        <ErrorBox>{message(q.error)}</ErrorBox>
      ) : (
        <div className="admin-grid">
          <section className="panel padded">
            <h2>People and access</h2>
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Roles</th>
                  <th>Access</th>
                  <th>Action</th>
                </tr>
              </thead>
              <tbody>
                {q.data.users.map((u) => (
                  <tr key={u.username}>
                    <td>
                      {u.display_name}
                      <small className="muted"> ({u.username})</small>
                    </td>
                    <td>{u.roles.map(label).join(", ")}</td>
                    <td>{u.enabled ? "Enabled" : "Disabled"}</td>
                    <td>
                      <button
                        className="button"
                        onClick={() => {
                          setDraft(u);
                          setPassword("");
                          setNotice("");
                        }}
                      >
                        Edit {u.username}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
          <form
            className="panel padded"
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <h2>{draft.revision ? "Edit account" : "Create account"}</h2>
            <label className="field">
              Username
              <input
                required
                pattern="[a-z0-9][a-z0-9._@-]{1,79}"
                autoComplete="off"
                disabled={draft.revision > 0}
                value={draft.username}
                onChange={(e) =>
                  setDraft({ ...draft, username: e.target.value.toLowerCase() })
                }
              />
            </label>
            <label className="field">
              Display name
              <input
                required
                maxLength={100}
                value={draft.display_name}
                onChange={(e) =>
                  setDraft({ ...draft, display_name: e.target.value })
                }
              />
            </label>
            <label className="field">
              {draft.revision
                ? "Reset password (leave blank to keep current)"
                : "Initial password"}
              <input
                type="password"
                autoComplete="new-password"
                required={!draft.revision}
                minLength={12}
                maxLength={256}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              <small>Use at least 12 characters.</small>
            </label>
            <fieldset>
              <legend>What can this person do?</legend>
              {Object.entries(q.data.roles).map(([role, description]) => (
                <label className="role-option" key={role}>
                  <input
                    type="checkbox"
                    checked={draft.roles.includes(role)}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        roles: e.target.checked
                          ? [...draft.roles, role]
                          : draft.roles.filter((r) => r !== role),
                      })
                    }
                  />
                  <span>
                    <strong>{label(role)}</strong>
                    <small>{description}</small>
                  </span>
                </label>
              ))}
            </fieldset>
            <label className="toggle">
              <input
                type="checkbox"
                checked={draft.enabled}
                onChange={(e) =>
                  setDraft({ ...draft, enabled: e.target.checked })
                }
              />{" "}
              Account enabled
            </label>
            <div className="actions">
              <button
                type="submit"
                className="button primary"
                disabled={busy || !draft.roles.length}
              >
                Save account
              </button>
              <button
                type="button"
                className="button"
                onClick={() => {
                  setDraft(blank);
                  setPassword("");
                }}
              >
                New account
              </button>
            </div>
          </form>
        </div>
      )}
      {error && <ErrorBox>{error}</ErrorBox>}
      {notice && (
        <div className="notice" role="status">
          {notice}
        </div>
      )}
      <SettingsPanel />
    </>
  );
}

export function ChangePassword({
  required = false,
  onDone,
}: {
  required?: boolean;
  onDone: () => void;
}) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <form
      className="panel padded password-form"
      onSubmit={async (e) => {
        e.preventDefault();
        setError("");
        if (next !== confirm) {
          setError("The new passwords do not match.");
          return;
        }
        setBusy(true);
        try {
          await result(
            client.POST("/api/password", {
              body: { current_password: current, new_password: next },
            }),
          );
          onDone();
        } catch (e) {
          setError(message(e));
        } finally {
          setBusy(false);
        }
      }}
    >
      <h2>{required ? "Choose your own password" : "Change password"}</h2>
      <p>
        {required
          ? "Replace your initial password before opening the workspace."
          : "You will sign in again after changing your password."}
      </p>
      <label className="field">
        Current password
        <input
          type="password"
          required
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
        />
      </label>
      <label className="field">
        New password
        <input
          type="password"
          required
          minLength={12}
          maxLength={256}
          autoComplete="new-password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
        />
      </label>
      <label className="field">
        Confirm new password
        <input
          type="password"
          required
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
      </label>
      {error && <ErrorBox>{error}</ErrorBox>}
      <button className="button primary" type="submit" disabled={busy}>
        Save password and sign in again
      </button>
    </form>
  );
}
