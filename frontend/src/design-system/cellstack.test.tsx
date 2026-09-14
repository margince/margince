/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CellStack } from "./cellstack";

const here = dirname(fileURLToPath(import.meta.url));

describe("CellStack", () => {
  it("holds both facts in one cell", () => {
    render(
      <CellStack>
        <span>04/09/2026, 15:29</span>
        <span>provider_quota</span>
      </CellStack>,
    );
    const stacked = screen.getByText("04/09/2026, 15:29").parentElement;
    expect(stacked).toHaveClass("cell-stack");
    expect(stacked).toHaveTextContent("provider_quota");
  });

  // The defect this closed was not the markup — the markup was already there,
  // as `<span className="cell-stack">`. It was that no rule anywhere in the
  // tree matched the name, so the two spans stayed inline and an operator
  // reading `04/09/2026, 15:29provider_quota` had the timestamp welded to the
  // sentinel explaining it. jsdom applies no stylesheet, so the case above
  // passes against that tree too; only reading the sheet can tell.
  //
  // The general form — every class a component renders resolves to a rule — is
  // a gate this suite does not have yet, and eighty more classes are orphaned
  // the same way.
  it("is styled by a rule that actually stacks", () => {
    const sheet = readFileSync(join(here, "cellstack.css"), "utf8");
    const at = sheet.indexOf(".cell-stack");
    expect(at).toBeGreaterThan(-1);
    const rule = sheet.slice(at, sheet.indexOf("}", at));
    expect(rule).toMatch(/display:\s*flex/);
    expect(rule).toMatch(/flex-direction:\s*column/);
  });
});
