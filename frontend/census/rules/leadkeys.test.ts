// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule, spelledIn } from "./leadkeys.census";

// Each case is keyed on how a real site spelled it.
const planted: ReadonlyArray<readonly [string, string, number]> = [
  ["the list", 'q({ queryKey: ["leads"] });', 1],
  ["the detail page", 'q({ queryKey: ["lead", id] });', 1],
  ["a child of the detail page", 'q({ queryKey: ["lead", id, "score"] });', 1],
  [
    "the promote preview's sibling root",
    'q({ queryKey: ["lead-promote-preview", id] });',
    1,
  ],
  [
    "the shared history key, narrowed to a lead",
    'q({ queryKey: ["record-history", "lead", id] });',
    1,
  ],
  [
    "two in one call, which a per-file flag would count once",
    'x({ queryKey: ["leads"] }); y({ queryKey: ["lead", id] });',
    2,
  ],
  [
    "the module's own name in an import",
    'import { leadWriteKeys } from "./leadkeys";',
    0,
  ],
  [
    "the shared history key for another record kind",
    'q({ queryKey: ["record-history", "deal", id] });',
    0,
  ],
  ["a word that starts with lead", 'q({ queryKey: ["leadership", id] });', 0],
  [
    "a record-kind list, which import.tsx passes",
    'p({ options: ["lead", "company"] as const });',
    0,
  ],
  ["a key wearing `as const`", 'q({ queryKey: ["lead", id] as const });', 1],
  [
    "a key in parentheses, and one behind a non-null assertion",
    'a({ queryKey: (["leads"]) }); b({ queryKey: ["lead", id]! });',
    2,
  ],
  [
    "a key whose head is a template literal",
    "q({ queryKey: [`lead`, id] });",
    1,
  ],
  [
    "the shared history key with a templated record kind",
    "q({ queryKey: [`record-history`, `lead`, id] });",
    1,
  ],
  [
    "the positional spelling setQueryData calls use",
    'queryClient.setQueryData(["lead", id], next);',
    1,
  ],
  [
    "the adopted spelling",
    "for (const key of leadWriteKeys(id)) q({ queryKey: key });",
    0,
  ],
];

describe("the leadkeys rule", () => {
  for (const [name, source, expected] of planted) {
    it(`sees ${name}`, () => {
      expect(spelledIn("planted.tsx", source)).toHaveLength(expected);
    });
  }

  it("fails a planted screen and spares the owner", () => {
    const text = 'q({ queryKey: ["lead", id] });\n';
    const findings = plantedFindings(rule, [
      { path: join(srcDir, "features", "planted.tsx"), area: "src", text },
      { path: join(srcDir, "lib", "leadkeys.ts"), area: "src", text },
    ]);
    expect(findings).toEqual(['src/features/planted.tsx:1 ["lead", id]']);
  });
});
