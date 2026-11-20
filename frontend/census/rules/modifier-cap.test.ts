// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// biome-ignore-all lint/suspicious/noTemplateCurlyInString: these strings are source fixtures, and a template literal in them is the subject.

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { ctrlSpellingsIn, maySpellCtrl, rule } from "./modifier-cap.census";

// One spelling per line, in each place shown text can carry it.
const SPELLED = [
  'const a = "Ctrl";',
  "const b = `Ctrl ${key}`;",
  "const c = `${mod} or Ctrl+${key}`;",
  "const d = <kbd>Ctrl</kbd>;",
  "const e = 'Ctrl+Enter';",
] as const;

const CLEAN = [
  "// Ctrl is named in a comment",
  'const f = "Control+K";',
  'const g = { ctrlKey: true, label: "CtrlLeft" };',
] as const;

describe("the Ctrl cap is copy", () => {
  it("finds the word in every kind of shown text", () => {
    const planted = parseSource("planted.tsx", SPELLED.join("\n"));
    expect(ctrlSpellingsIn(planted)).toEqual(
      SPELLED.map((_, index) => index + 1),
    );
  });

  it("leaves comments, the ARIA name and longer words alone", () => {
    const planted = parseSource("planted.tsx", CLEAN.join("\n"));
    expect(ctrlSpellingsIn(planted)).toEqual([]);
  });

  it("lets through every shape the matcher can see", () => {
    for (const spelling of SPELLED) {
      expect(maySpellCtrl(spelling)).toBe(true);
    }
  });

  it("reports shipped code and spares a test that drives the key", () => {
    const text = 'export const cap = "Ctrl";\n';
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "app", "p.ts"), area: "src", text },
        { path: join(srcDir, "app", "p.test.ts"), area: "src", text },
      ]),
    ).toEqual(["src/app/p.ts:1"]);
  });
});
