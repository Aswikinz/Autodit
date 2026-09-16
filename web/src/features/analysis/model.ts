import type { JSONObject } from "../../lib/client";

export type Column = { name: string; type: "string" | "number" | "boolean" };
export type Dataset = {
  columns: Column[];
  rows: Record<string, unknown>[];
  row_count: number;
};
export type Analysis = {
  id?: string;
  name: string;
  revision: number;
  dataset: Dataset;
  model: JSONObject;
  selected_columns: Column[];
  updated_at?: string;
  origin?: string;
};
export type ResultRow = {
  row: number;
  input: Record<string, unknown>;
  output: unknown;
  error?: string;
  trace?: Record<string, unknown>;
};
export type Report = {
  total: number;
  flagged: number;
  errors: number;
  duration_ms?: number;
  results: ResultRow[];
};
export type Condition = {
  id: string;
  column: string;
  operator: string;
  value: string;
};

export const operators = [
  { value: "eq", label: "Equals" },
  { value: "neq", label: "Does not equal" },
  { value: "gt", label: "Greater than" },
  { value: "gte", label: "At least" },
  { value: "lt", label: "Less than" },
  { value: "lte", label: "At most" },
  { value: "empty", label: "Is empty" },
  { value: "notempty", label: "Is not empty" },
];

function expression(condition: Condition, columns: Column[]) {
  if (condition.operator === "empty") return 'null, ""';
  if (condition.operator === "notempty") return '$ != null and $ != ""';
  const type = columns.find((c) => c.name === condition.column)?.type;
  const value =
    type === "string" ? `(${zenString(condition.value)})` : condition.value;
  const prefix: Record<string, string> = {
    eq: "",
    neq: "!= ",
    gt: "> ",
    gte: ">= ",
    lt: "< ",
    lte: "<= ",
  };
  return `${prefix[condition.operator] ?? ""}${value}`;
}

// ZEN literals preserve backslashes and use either quote delimiter, without
// JavaScript-style quote escapes. Concatenation handles headers containing both.
export function zenString(value: string): string {
  return value
    .split('"')
    .map((part) => `"${part}"`)
    .join(` + '"' + `);
}

export function inputSchema(columns: Column[]) {
  return JSON.stringify(
    {
      type: "object",
      properties: {
        data: {
          type: "object",
          properties: Object.fromEntries(
            columns.map((c) => [c.name, { type: [c.type, "null"] }]),
          ),
        },
      },
    },
    null,
    2,
  );
}

export function makeModel(
  columns: Column[],
  conditions: Condition[],
): JSONObject {
  const inputs = conditions.map((c) => ({
    id: c.id,
    name: c.column,
    field: `data[(${zenString(c.column)})]`,
  }));
  const values = Object.fromEntries(
    conditions.map((c) => [c.id, expression(c, columns)]),
  );
  return {
    _autodit: { mode: "simple", conditions },
    nodes: [
      {
        id: "data-input",
        name: "Your data",
        type: "inputNode",
        position: { x: 80, y: 180 },
        content: { schema: inputSchema(columns) },
      },
      {
        id: "autodit-rules",
        name: "Flag matching rows",
        type: "decisionTableNode",
        position: { x: 380, y: 180 },
        content: {
          hitPolicy: "first",
          inputs,
          outputs: [
            { id: "flag", name: "Flag", field: "flag" },
            { id: "reason", name: "Reason", field: "reason" },
          ],
          rules: [
            ...(conditions.length
              ? [
                  {
                    _id: "match",
                    ...values,
                    flag: "true",
                    reason: '"Matches the configured conditions"',
                  },
                ]
              : []),
            {
              _id: "otherwise",
              ...Object.fromEntries(conditions.map((c) => [c.id, ""])),
              flag: "false",
              reason: '"No matching condition"',
            },
          ],
        },
      },
      {
        id: "data-output",
        name: "Results",
        type: "outputNode",
        position: { x: 710, y: 180 },
        content: { schema: "" },
      },
    ],
    edges: [
      { id: "input-rules", sourceId: "data-input", targetId: "autodit-rules" },
      {
        id: "rules-output",
        sourceId: "autodit-rules",
        targetId: "data-output",
      },
    ],
  };
}

export function simpleConditions(
  model: JSONObject,
  columns: Column[],
): Condition[] | undefined {
  const metadata = model._autodit as
    { mode?: string; conditions?: unknown } | undefined;
  if (metadata?.mode !== "simple" || !Array.isArray(metadata.conditions))
    return undefined;
  const valid = metadata.conditions.every((condition: unknown) => {
    if (!condition || typeof condition !== "object") return false;
    const c = condition as Condition;
    return (
      typeof c.id === "string" &&
      typeof c.value === "string" &&
      columns.some((column) => column.name === c.column) &&
      operators.some((operator) => operator.value === c.operator)
    );
  });
  if (!valid) return undefined;
  const conditions = metadata.conditions as Condition[];
  return sameGraphLogic(model, makeModel(columns, conditions))
    ? conditions
    : undefined;
}

export function sameGraphLogic(left: JSONObject, right: JSONObject): boolean {
  const canonical = (value: unknown): unknown => {
    if (Array.isArray(value)) return value.map(canonical);
    if (value && typeof value === "object")
      return Object.fromEntries(
        Object.entries(value)
          .filter(([key, value]) => value !== undefined && key !== "_diff")
          .sort(([a], [b]) => a.localeCompare(b))
          .map(([key, value]) => [key, canonical(value)]),
      );
    return value;
  };
  const normalize = (model: JSONObject) => {
    const nodes = model.nodes as {
      id: string;
      type: string;
      content: unknown;
    }[];
    const edges = model.edges as {
      sourceId: string;
      targetId: string;
      sourceHandle?: string | null;
      targetHandle?: string | null;
    }[];
    return JSON.stringify({
      nodes: nodes
        .map((n) => {
          let content = n.content;
          if (
            n.type === "decisionTableNode" &&
            content &&
            typeof content === "object"
          ) {
            const table = content as JSONObject;
            content = {
              ...table,
              passThrough: table.passThrough ?? false,
              executionMode: table.executionMode ?? "single",
              inputField: table.inputField ?? null,
              outputPath: table.outputPath ?? null,
            };
          }
          return { id: n.id, type: n.type, content: canonical(content) };
        })
        .sort((a, b) => a.id.localeCompare(b.id)),
      edges: edges
        .map((e) => ({
          sourceId: e.sourceId,
          targetId: e.targetId,
          sourceHandle: e.sourceHandle || null,
          targetHandle: e.targetHandle || null,
        }))
        .sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b))),
    });
  };
  return normalize(left) === normalize(right);
}

export function isFlagged(output: unknown): boolean {
  if (Array.isArray(output)) return output.some(isFlagged);
  return (
    !!output &&
    typeof output === "object" &&
    (output as Record<string, unknown>).flag === true
  );
}

export function cell(value: unknown): string {
  if (value === null || value === undefined) return "";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

export function downloadResults(
  name: string,
  columns: Column[],
  rows: ResultRow[],
) {
  const escape = (value: unknown) => {
    let text = cell(value);
    if (typeof value === "string" && /^[=+\-@\t\r]/.test(text))
      text = `'${text}`;
    return `"${text.replaceAll('"', '""')}"`;
  };
  const data = [
    ["Source row", ...columns.map((c) => c.name), "Flagged", "Result", "Error"],
    ...rows.map((r) => [
      r.row,
      ...columns.map((c) => r.input[c.name]),
      isFlagged(r.output),
      cell(r.output),
      r.error ?? "",
    ]),
  ]
    .map((row) => row.map(escape).join(","))
    .join("\r\n");
  const url = URL.createObjectURL(
    new Blob(["\ufeff", data], { type: "text/csv;charset=utf-8" }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${name.replaceAll(/[^a-zA-Z0-9_-]/g, "_") || "analysis"}-results.csv`;
  anchor.click();
  URL.revokeObjectURL(url);
}
