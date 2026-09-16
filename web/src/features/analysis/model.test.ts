import { describe, expect, it } from "vitest";
import {
  makeModel,
  sameGraphLogic,
  simpleConditions,
  zenString,
} from "./model";
import type { JSONObject } from "../../lib/client";

describe("saved analysis graph editing", () => {
  const columns = [{ name: "Invoice amount", type: "number" as const }];
  const conditions = [
    {
      id: "threshold",
      column: "Invoice amount",
      operator: "gt",
      value: "1000",
    },
  ];

  it("preserves the simple builder across canvas moves and editor normalization", () => {
    const original = makeModel(columns, conditions);
    const moved = structuredClone(original);
    const nodes = moved.nodes as JSONObject[];
    nodes[1]!.position = { x: 530, y: 225 };
    const content = nodes[1]!.content as JSONObject;
    nodes[1]!.content = {
      executionMode: "single",
      outputPath: null,
      passThrough: false,
      inputField: null,
      ...Object.fromEntries(Object.entries(content).reverse()),
    };
    expect(sameGraphLogic(original, moved)).toBe(true);
    expect(simpleConditions(original, columns)).toEqual(conditions);
  });

  it("recognizes changed table rules and connections as actual logic edits", () => {
    const original = makeModel(columns, conditions);
    const edited = structuredClone(original);
    const content = (edited.nodes as JSONObject[])[1]!.content as JSONObject;
    (content.rules as JSONObject[])[0]!.threshold = "> 500";
    expect(sameGraphLogic(original, edited)).toBe(false);
    expect(simpleConditions(edited, columns)).toBeUndefined();
    const rewired = structuredClone(original);
    (rewired.edges as JSONObject[])[0]!.targetId = "data-output";
    expect(sameGraphLogic(original, rewired)).toBe(false);
  });

  it("does not restore conditions that refer to removed columns", () => {
    expect(
      simpleConditions(makeModel(columns, conditions), []),
    ).toBeUndefined();
  });

  it("preserves literal backslashes and splits quote delimiters for ZEN", () => {
    expect(zenString('Invoice "A"\\path')).toBe(
      '"Invoice " + \'"\' + "A" + \'"\' + "\\path"',
    );
    const model = makeModel(
      [{ name: 'Total "net"', type: "string" }],
      [
        {
          id: "match",
          column: 'Total "net"',
          operator: "eq",
          value: 'O\'Brien "verified"',
        },
      ],
    );
    const table = (model.nodes as JSONObject[])[1]!.content as JSONObject;
    expect((table.inputs as JSONObject[])[0]!.field).toBe(
      'data[("Total " + \'"\' + "net" + \'"\' + "")]',
    );
    expect((table.rules as JSONObject[])[0]!.match as string).toContain(
      "O'Brien",
    );
  });
});
