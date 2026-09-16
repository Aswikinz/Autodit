import createClient from "openapi-fetch";
import type { paths } from "../api/schema";

export const client = createClient<paths>({
  baseUrl: "",
  credentials: "same-origin",
});
let csrf = "";
export function setCSRF(value: string) {
  csrf = value;
}
client.use({
  onRequest({ request }) {
    if (request.method !== "GET") request.headers.set("X-CSRF-Token", csrf);
    return request;
  },
});
export async function result<T>(
  promise: Promise<{ data?: unknown; error?: unknown; response: Response }>,
): Promise<T> {
  const r = await promise;
  if (!r.response.ok) {
    const e = r.error;
    throw new Error(
      typeof e === "object" && e !== null && "error" in e
        ? String(e.error)
        : `Request failed (${r.response.status})`,
    );
  }
  return r.data as T;
}
export function message(e: unknown) {
  return e instanceof Error
    ? e.message
    : "The operation could not be completed.";
}
let displaySettings = { locale: "", timezone: "" };
export function configureDisplay(settings: {
  locale: string;
  timezone: string;
}) {
  displaySettings = settings;
}
export function number(value: number) {
  return new Intl.NumberFormat(displaySettings.locale || undefined).format(
    value,
  );
}
export function dateTime(value: string | null | undefined) {
  return value
    ? new Intl.DateTimeFormat(displaySettings.locale || undefined, {
        timeZone: displaySettings.timezone || undefined,
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(value))
    : "Not yet";
}
export function label(value: string) {
  return value.replaceAll("_", " ").replace(/\b\w/g, (c) => c.toUpperCase());
}
export type JSONObject = Record<string, unknown>;
export type Session = {
  authenticated: boolean;
  mode: string;
  login_url: string;
  identity: {
    subject: string;
    tenant_id: string;
    roles: string[];
    csrf_token: string;
    must_change_password?: boolean;
  };
};
export type ExceptionRow = {
  key: string;
  rule_id: string;
  entity_type: string;
  entity_id: string;
  period_id: string;
  state: string;
  severity: string;
  owner_role: string;
  revision: number;
  first_seen_at: string;
  last_seen_at: string;
  suppression_until: string | null;
  rule_version?: string;
  snapshot_id?: string;
  parameter_set_hash?: string;
  engine_version?: string;
  trace_ref?: string;
};
export type QueuePage = {
  items: ExceptionRow[];
  total: number;
  page: number;
  size: number;
};
export type Observation = {
  id: string;
  run_id: string;
  rule_id: string;
  rule_version: string;
  parameter_set_hash: string;
  snapshot_id: string;
  engine_version: string;
  input_hash: string;
  input: JSONObject;
  result: JSONObject;
  trace: unknown;
  classification: string;
  created_at: string;
};
export type Detail = {
  has_review_case?: boolean;
  exception: ExceptionRow;
  observations: Observation[];
  events: {
    id: string;
    actor: string;
    action: string;
    reason: string;
    to_state: string;
    created_at: string;
    event_hash: string;
  }[];
  newer_snapshot_exists: boolean;
};
export type Run = {
  id: string;
  source_id: string;
  period_id: string;
  status: string;
  stage: string;
  record_count: number;
  exception_count: number;
  snapshot_id: string | null;
  created_at: string;
  completed_at: string | null;
  error_code: string;
  differences: {
    expected: Record<string, string | number>;
    actual: Record<string, string | number>;
  }[];
  stages?: { stage: string; status: string; at: string }[];
};
export type Source = {
  id: string;
  name: string;
  kind: string;
  mapping: Record<string, string>;
  interval_minutes: number;
  last_landed_at: string | null;
  last_completed_at: string | null;
  overdue: boolean;
  watermark: string;
};
export type Rule = {
  rule_id: string;
  title: string;
  version: string;
  revision: number;
  enabled: boolean;
  model: JSONObject;
  accepted: number;
  dismissed: number;
  exceptions: number;
  released_at: string;
};
export type Parameters = {
  explicit_currencies?: boolean;
  materiality: string;
  currency_thresholds?: Record<string, string>;
  weekend_days: number[];
  exception_ceiling: number;
};
