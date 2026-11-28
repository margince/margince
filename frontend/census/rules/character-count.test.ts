// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { characterCountsIn, rule } from "./character-count.census";

// One offence per line, in each shape a hand-rolled count takes.
const PLANTED = [
  "const a = <input maxLength={120} />;",
  "const b = <textarea maxLength={TEXT_LIMITS.note}></textarea>;",
  "const c = [...name].length;",
  "const d = Array.from(name.trim()).length;",
  "const e = name.length > TEXT_LIMITS.label;",
  "const f = TEXT_LIMITS.note >= body.text.length;",
] as const;

// What the rule must leave alone on the same lines.
const SPARED = [
  "const a = <TextInput maxLength={120} />;",
  "const b = <Textarea maxLength={TEXT_LIMITS.note} />;",
  "const c = [first, ...rest].length;",
  "const d = Array.from(rows, toCell).length;",
  "const e = rows.length > MAX_ROWS;",
  "const f = characterCount(name) > TEXT_LIMITS.label;",
] as const;

describe("one count for a text limit", () => {
  it("finds each hand-rolled count", () => {
    const planted = parseSource("planted.tsx", PLANTED.join("\n"));
    expect(characterCountsIn(planted)).toEqual(
      PLANTED.map((_, index) => index + 1),
    );
  });

  it("leaves the shared count and other lengths alone", () => {
    const spared = parseSource("spared.tsx", SPARED.join("\n"));
    expect(characterCountsIn(spared)).toEqual([]);
  });

  it("reports through its own corpus, and spares the owner and tests", () => {
    const text = "export const n = (s: string) => [...s].length;\n";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "p.ts"), area: "src", text },
        { path: join(srcDir, "lib", "charactercount.ts"), area: "src", text },
        { path: join(srcDir, "features", "p.test.ts"), area: "src", text },
      ]),
    ).toEqual(["src/features/p.ts:1"]);
  });
});

describe("the length prefilter", () => {
  it("reads a unit count however length is spelled", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "plant", "a.ts"),
          area: "src",
          text: "if (name.length > TEXT_LIMITS.name) save();\n",
        },
      ]),
    ).toHaveLength(1);
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "plant", "b.ts"),
          area: "src",
          text: "if (name.l\\u0065ngth > TEXT_LIMITS.name) save();\n",
        },
      ]),
    ).toHaveLength(1);
  });
});
