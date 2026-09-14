import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// The defect this component closed was not the markup — the markup was already
// there, as `<span className="cell-stack">`. It was that no rule anywhere in
// the tree matched the name, so two spans stayed inline and an operator read
// `04/09/2026, 15:29provider_quota` as one word on the page they open to find
// out why a model lane stopped answering.
//
// jsdom applies no stylesheet, so a render test cannot see this: it would pass
// against the exact tree that shipped the bug. Reading the sheet can. The
// general form — every class a component renders has a rule somewhere — is a
// gate this suite does not have yet, and three more design-system classes are
// orphaned the same way.
describe("the cell stack's class is styled", () => {
  const sheet = readFileSync(
    new URL("./cellstack.css", import.meta.url),
    "utf8",
  );

  it("declares the column layout the component's name promises", () => {
    expect(sheet).toContain(".cell-stack");
    expect(sheet).toMatch(/display:\s*flex/);
    expect(sheet).toMatch(/flex-direction:\s*column/);
  });
});
