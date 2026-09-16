import { StrictMode, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Activity,
  ArrowRight,
  Blocks,
  CheckCheck,
  ChevronRight,
  Database,
  Files,
  LogOut,
  ShieldCheck,
} from "lucide-react";
import { Queue } from "./features/queue/Queue";
import { Runs } from "./features/runs/Runs";
import { Sources } from "./features/sources/Sources";
import { Rules } from "./features/rules/Rules";
import { Assurance } from "./features/assurance/Assurance";
import { ErrorBox, Spinner } from "./components/Shared";
import { client, result, message, setCSRF } from "./lib/client";
import type { Session } from "./lib/client";
import "./style.css";

const qc = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 5000, refetchOnWindowFocus: true },
  },
});
const tabs = [
  {
    id: "queue",
    name: "Exception queue",
    icon: Files,
    roles: ["auditor", "audit_manager"],
  },
  {
    id: "runs",
    name: "Run monitor",
    icon: Activity,
    roles: ["auditor", "audit_manager", "implementer", "admin"],
  },
  {
    id: "rules",
    name: "Rule catalog",
    icon: Blocks,
    roles: ["auditor", "audit_manager", "rule_engineer"],
  },
  {
    id: "sources",
    name: "Sources & mapping",
    icon: Database,
    roles: ["implementer"],
  },
  {
    id: "assurance",
    name: "Assurance",
    icon: CheckCheck,
    roles: ["auditor", "audit_manager"],
  },
];
function App() {
  const cache = useQueryClient();
  const q = useQuery({
    queryKey: ["session"],
    queryFn: async () => {
      const s = await result<Session>(client.GET("/api/session"));
      setCSRF(s.identity.csrf_token);
      return s;
    },
    retry: false,
  });
  const [active, setActive] = useState("queue");
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function login() {
    setBusy(true);
    setError("");
    try {
      await result(client.POST("/api/login", { body: { token } }));
      setToken("");
      await cache.invalidateQueries({ queryKey: ["session"] });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function logout() {
    await result(client.POST("/api/logout", {}));
    cache.clear();
    location.reload();
  }
  if (q.isPending) return <Spinner />;
  if (q.error)
    return (
      <div className="login">
        <ErrorBox>{message(q.error)}</ErrorBox>
        <button className="button" onClick={() => void q.refetch()}>
          Retry connection
        </button>
      </div>
    );
  const session = q.data;
  if (!session.authenticated)
    return (
      <div className="login">
        <div className="login-art">
          <div className="brand">
            <span className="brand-mark">
              <Blocks />
            </span>
            autodit<span className="brand-dot">.</span>
          </div>
          <div>
            <span className="eyebrow">CONTINUOUS AUDIT WORKSPACE</span>
            <h1>
              Every population.
              <br />
              Every exception.
              <br />A clear trail.
            </h1>
            <p>
              Independent tie-out, versioned decisions, and evidence you can
              replay.
            </p>
          </div>
          <small>Private by design. Deployed in your environment.</small>
        </div>
        <main className="login-form">
          <span className="eyebrow">WELCOME TO AUTODIT</span>
          <h2>Sign in to your workspace</h2>
          <p className="muted">
            Access your audit queue and connected populations.
          </p>
          {session.mode === "demo" ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void login();
              }}
            >
              <div className="notice">
                Local evaluation mode. Use the access token generated during
                installation.
              </div>
              <label className="field">
                Workspace access token
                <input
                  type="password"
                  autoComplete="current-password"
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  required
                />
              </label>
              <button type="submit" className="button primary" disabled={busy}>
                Open workspace <ArrowRight size={16} />
              </button>
            </form>
          ) : (
            <a className="button primary" href={session.login_url}>
              Continue with your identity provider <ArrowRight size={16} />
            </a>
          )}
          {error && <ErrorBox>{error}</ErrorBox>}
          <div className="login-foot">
            <ShieldCheck size={16} /> Your data stays within this deployment.
          </div>
        </main>
      </div>
    );
  const allowed = tabs.filter((t) =>
    t.roles.some((r) => session.identity.roles.includes(r)),
  );
  const current = allowed.find((t) => t.id === active) ?? allowed[0];
  const page = current?.id;
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="/">
          <span className="brand-mark">
            <Blocks />
          </span>
          autodit<span className="brand-dot">.</span>
        </a>
        <div className="workspace">
          <div className="workspace-avatar">A</div>
          <div>
            <strong>Audit workspace</strong>
            <small>
              {session.mode === "demo"
                ? "Local evaluation"
                : "Private deployment"}
            </small>
          </div>
        </div>
        <span className="nav-label">WORKSPACE</span>
        <nav>
          {allowed.map((t) => (
            <button
              key={t.id}
              className={page === t.id ? "active" : ""}
              onClick={() => setActive(t.id)}
            >
              <t.icon size={18} />
              <span>{t.name}</span>
              {page === t.id && <ChevronRight size={14} />}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="system-status">
            <span className="status-dot good" /> Evidence retention enabled
          </div>
          <div className="profile">
            <span className="avatar">
              {session.identity.subject.charAt(0).toUpperCase()}
            </span>
            <div>
              <strong>
                {session.mode === "demo"
                  ? "Demo operator"
                  : session.identity.subject}
              </strong>
              <small>
                {session.mode === "demo"
                  ? "Evaluation access"
                  : "Federated identity"}
              </small>
            </div>
            <button
              className="icon-button"
              aria-label="Sign out"
              onClick={() => void logout()}
            >
              <LogOut size={17} />
            </button>
          </div>
        </div>
      </aside>
      <div className="main-shell">
        <header className="topbar">
          <span>
            Workspace <ChevronRight size={13} />{" "}
            <strong>{current?.name ?? "Access not assigned"}</strong>
          </span>
          <div>
            <span className="status-dot good" />{" "}
            {session.mode === "demo"
              ? "LOCAL EVALUATION"
              : "PRIVATE ENVIRONMENT"}{" "}
            <span className="version">v0.1.0</span>
          </div>
        </header>
        <main className="content">
          {page === "queue" && <Queue onImport={() => setActive("sources")} />}{" "}
          {page === "runs" && <Runs onImport={() => setActive("sources")} />}{" "}
          {page === "sources" && <Sources onRun={() => setActive("runs")} />}{" "}
          {page === "rules" && <Rules roles={session.identity.roles} />}{" "}
          {page === "assurance" && <Assurance />}
          {!page && (
            <ErrorBox>
              No workspace permissions are assigned to this identity.
            </ErrorBox>
          )}
        </main>
        <footer className="app-footer">
          AUTODIT <span>Continuous auditing · Evidence you can follow</span>
        </footer>
      </div>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={qc}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
