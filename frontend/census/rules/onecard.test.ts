// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Area, plantedFindings, srcDir } from "../tree";
import { CARD_SOURCE, cardChrome, rule } from "./onecard.census";

const area: Area = "src";
const atoms =
  ".card { background: var(--bgElevated); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: var(--shadow-card); }";

const findings = (screen: string) =>
  plantedFindings(rule, [
    { path: CARD_SOURCE, area, text: atoms },
    { path: join(srcDir, "features", "planted.css"), area, text: screen },
  ]);

describe("one card surface", () => {
  it("reports a copy of the card chrome, however it is spelled", () => {
    expect(
      findings(
        ".tile {\n  background-color:var(--bgElevated );\n  border-width: 1px; border-style: solid; border-color: var(--border);\n  border-radius: var(--radius); box-shadow: var(--shadow-card); padding: 8px;\n}",
      ),
    ).toEqual(["src/features/planted.css: .tile"]);
  });

  it("passes a rule that carries only part of the chrome", () => {
    expect(
      findings(
        ".chip { background: var(--bgElevated); border-radius: var(--radius); }",
      ),
    ).toEqual([]);
  });

  it("reads the subject from one .card rule and refuses two", () => {
    expect(cardChrome(atoms)).toContain("box-shadow: var(--shadow-card)");
    expect(cardChrome(`${atoms}\n.card { padding: 0; }`)).toBe(
      "atoms.css declares .card 2 times; this rule needs one to read its subject from",
    );
  });
});
