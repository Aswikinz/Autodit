import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { synchronousJdmChanges } from "./jdmTransform";

describe("pinned JDM controlled-value adapter", () => {
  const source = readFileSync(
    new URL(
      "../../node_modules/@gorules/jdm-editor/dist/index.js",
      import.meta.url,
    ),
    "utf8",
  );

  it("updates the three installed callback sites and leaves throttled layout callbacks intact", () => {
    const result = synchronousJdmChanges(source, "1.52.0");
    expect(result).not.toEqual(source);
    expect(
      result.match(/re\(\(\w+\) => \{ \w+\?\.\(\w+\); \}, \[\w+\]\)/g),
    ).toHaveLength(3);
    expect(result).toContain("kn(() => z?.layout(), 100");
  });

  it("requires review when the dependency version or callback structure changes", () => {
    expect(() => synchronousJdmChanges(source, "1.53.0")).toThrow(
      "before upgrading",
    );
    expect(() => synchronousJdmChanges("different bundle", "1.52.0")).toThrow(
      "found 0",
    );
  });
});
