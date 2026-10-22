import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { DISPLAY_FIELDS } from "./approvaldisplay";

it("retains a declared display for every existing typed producer", () => {
  const source = readFileSync(
    new URL(
      "../../../../backend/internal/compose/approvalpreviewcoverage.go",
      import.meta.url,
    ),
    "utf8",
  );
  const declaration = source.match(
    /func hasTypedProposalDisplay\(kind string\) bool \{([\s\S]*?)\n\}/,
  )?.[1];
  expect(declaration).toBeDefined();
  const cases = [...(declaration ?? "").matchAll(/case ([^:]+):/g)];
  expect(cases.length).toBeGreaterThan(0);
  const kinds = cases.flatMap((entry) =>
    [...entry[1].matchAll(/"([a-z_]+)"/g)].map((kind) => kind[1]),
  );
  expect(kinds.length).toBeGreaterThan(0);
  for (const kind of kinds)
    expect(Object.hasOwn(DISPLAY_FIELDS, kind), kind).toBe(true);
});
