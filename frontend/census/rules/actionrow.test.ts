// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { actionRows, ROLE_VALUE, rule } from "./actionrow.census";

describe("what counts as an action row", () => {
  const gaps = new Map<string, Set<string>>([
    ["verbs", new Set([ROLE_VALUE])],
    ["loose-verbs", new Set(["var(--space-3)"])],
  ]);
  const rowsIn = (markup: string) =>
    actionRows("fixture.tsx", `export const F = () => (${markup});\n`, gaps);

  it("passes a row whose class carries the action gap", () => {
    expect(
      rowsIn('<div className="verbs"><Button /><Button /></div>').map(
        (r) => r.ok,
      ),
    ).toEqual([true]);
  });

  it("passes a row that spells the gap inline", () => {
    expect(
      rowsIn(
        '<div style={{ display: "flex", gap: "var(--gapActions)" }}><Button /><Button /></div>',
      ).map((r) => r.ok),
    ).toEqual([true]);
  });

  it("reports a row that carries some other rung", () => {
    expect(
      rowsIn('<div className="loose-verbs"><Button /><Button /></div>').map(
        (r) => r.ok,
      ),
    ).toEqual([false]);
  });

  it("reports a row nothing in the tree styles at all", () => {
    const [row] = rowsIn('<div className="nowhere"><Button /><Button /></div>');
    expect(row?.ok).toBe(false);
    expect(row?.found).toContain("nothing in the tree gives it a gap");
  });

  it("is not a row when the container holds anything else", () => {
    // A label and its verbs is a layout row: its gap governs the label too.
    expect(
      rowsIn(
        '<div className="nowhere"><span>Owner</span><Button /><Button /></div>',
      ),
    ).toEqual([]);
  });

  it("reads a conditional child by what it renders, not by its text", () => {
    expect(
      rowsIn(
        '<div className="nowhere"><Button />{expanded && <Card><Button /></Card>}</div>',
      ),
    ).toEqual([]);
    expect(
      rowsIn(
        '<div className="verbs"><Button />{canEdit && <Button />}</div>',
      ).map((r) => r.ok),
    ).toEqual([true]);
  });

  it("counts buttons through a fragment, which draws no box of its own", () => {
    expect(
      rowsIn('<div className="verbs"><><Button /><Button /></></div>').map(
        (row) => row.ok,
      ),
    ).toEqual([true]);
    expect(
      rowsIn('<div className="nowhere"><><Button /><Button /></></div>').map(
        (row) => row.ok,
      ),
    ).toEqual([false]);
  });

  it("counts a conditional fragment's buttons, all of which render together", () => {
    // A guarded fragment draws both its buttons or none.
    expect(
      rowsIn(
        '<div className="nowhere"><>{canEdit && <><Button /><Button /></>}</></div>',
      ).map((row) => row.ok),
    ).toEqual([false]);
    // Branches are alternatives, not children: one button renders either way.
    expect(
      rowsIn(
        '<div className="nowhere"><Button />{canEdit ? <Button /> : <IconAction />}</div>',
      ).map((row) => row.ok),
    ).toEqual([false]);
    // A conditional fragment holding content is still content.
    expect(
      rowsIn(
        '<div className="nowhere"><Button />{canEdit && <><Button /><span>or</span></>}</div>',
      ),
    ).toEqual([]);
  });

  it("is not a row when the fragment's element also holds something else", () => {
    expect(
      rowsIn(
        '<div className="nowhere"><span>Owner</span><><Button /><Button /></></div>',
      ),
    ).toEqual([]);
  });

  it("leaves a bare fragment to whoever receives it", () => {
    expect(rowsIn("<><Button /><Button /></>")).toEqual([]);
  });

  it("leaves a component container to the component", () => {
    expect(rowsIn("<OverflowMenu><Button /><Button /></OverflowMenu>")).toEqual(
      [],
    );
  });

  it("honours a waiver on the row or the line above it", () => {
    expect(
      rowsIn(
        '<div className="nowhere" /* ds:ignore a reason */><Button /><Button /></div>',
      ),
    ).toEqual([]);
  });
});

describe("the actionrow rule", () => {
  it("reports a planted unstyled row through its own corpus and across", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text =
      'export const F = () => <div className="nowhere"><Button /><Button /></div>;\n';
    const css = join(srcDir, "features", "planted.css");
    expect(
      plantedFindings(rule, [
        { path, area: "src", text },
        { path: css, area: "src", text: ".verbs { gap: var(--gapActions); }" },
      ]),
    ).toEqual([
      'src/features/planted.tsx:1: "nowhere" — nothing in the tree gives it a gap',
    ]);
  });
});
