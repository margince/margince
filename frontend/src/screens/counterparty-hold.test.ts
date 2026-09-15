import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// The defect was a box-model one, and jsdom applies no stylesheet — so a render
// test would pass against the exact tree that drew the label outside its own
// button. What CAN be held is the pair that makes the invariant: the label is
// MARKED as data, and a label so marked is allowed to take a second line.
describe("the domain hold's label is marked as data", () => {
  const screen = readFileSync(
    new URL("./counterparty-hold.tsx", import.meta.url),
    "utf8",
  );
  const base = readFileSync(
    new URL("../design-system/base.css", import.meta.url),
    "utf8",
  );

  it("carries the modifier on the button that interpolates the domain", () => {
    const opens = screen.indexOf('setAsking("domain")');
    expect(opens).toBeGreaterThan(-1);
    expect(screen.slice(opens - 400, opens)).toContain("btn-valuelabel");
  });

  it("and the modifier lets such a label take a second line", () => {
    // The SELECTOR, not the first mention: `.btn` names the modifier in its
    // own comment, and matching that would read the wrong declarations.
    const at = base.indexOf("\n.btn-valuelabel,");
    expect(at).toBeGreaterThan(-1);
    const rule = base.slice(at, base.indexOf("}", at));
    expect(rule).toMatch(/white-space:\s*normal/);
    // A domain is one long word to a line breaker, so permission to wrap that
    // it cannot act on is no permission at all.
    expect(rule).toMatch(/overflow-wrap:\s*anywhere/);
  });
});
