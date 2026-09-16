// JDM 1.52.0 delays Function, DecisionTable, and DecisionGraph onChange by
// 100 ms each. Closing a tab can discard edits before either callback runs.
// Keep those three controlled values synchronous, using the bundle's existing
// React useCallback alias. There are no added or conditional hook calls.
export function synchronousJdmChanges(source: string, version: string): string {
  if (version !== "1.52.0")
    throw new Error(
      "Review the JDM controlled-value adapter before upgrading @gorules/jdm-editor.",
    );
  let count = 0;
  const output = source.replace(
    /Mt\(\((\w+)\) => \{\s*(\w+)\?\.\(\1\);\s*\}, 100\)/g,
    (_match, argument: string, callback: string) => {
      count++;
      return `re((${argument}) => { ${callback}?.(${argument}); }, [${callback}])`;
    },
  );
  if (count !== 3)
    throw new Error(
      `JDM controlled-value adapter expected 3 callback sites, found ${count}. Review the installed bundle.`,
    );
  return output;
}
