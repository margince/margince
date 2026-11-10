// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { rawFoldMatchesIn, rule } from "./search-fold-spelling.census";

// One substring match per line, in every shape the tree has spelled one.
const PLANTED_MATCHES = [
  "{ const needle = q.trim().toLowerCase(); name.toLowerCase().includes(needle); }",
  "{ const typed = q.toLocaleLowerCase(); row.label.startsWith(typed); }",
  "haystack.includes(query.toLowerCase());",
  "foldForMatch(deal.name).includes(needle);",
  "{ const term = q.normalize('NFKC').toLowerCase(); [a, b].join(' ').includes(term); }",
  "(label ?? '').indexOf(foldForMatch(q));",
  "{ const folded = q.toLowerCase(); name.includes(folded); }",
  "function f(q: string) { const n = q.toLowerCase(); return x.includes(n); }",
  "{ const deal = foldForMatch(name); deal.includes(needle); }",
] as const;

// Not a reader's text matched by a part of it: equality, the shared fold, a
// needle the code wrote, a list.
const PLANTED_INNOCENTS = [
  "foldForMatch(a) === foldForMatch(b);",
  "foldForSearch(name).includes(word);",
  "holdsEveryWord(searchWords(query), [name]);",
  "file.name.toLowerCase().endsWith('.csv');",
  "host.toLowerCase().endsWith('.' + allowed);",
  "picked.includes(name);",
  "const n = q.trim(); x.includes(n);",
] as const;

describe("one fold for a substring match", () => {
  it("finds the match however the caller folds it", () => {
    const planted = parseSource(
      "planted.ts",
      [...PLANTED_MATCHES, ...PLANTED_INNOCENTS].join("\n"),
    );

    expect(rawFoldMatchesIn(planted)).toEqual(
      PLANTED_MATCHES.map((_, index) => index + 1),
    );
  });

  it("reports a screen through the rule and skips a test", () => {
    const text =
      "export const hit = (q: string) => name.includes(q.toLowerCase());\n";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "x.ts"), area: "src", text },
        { path: join(srcDir, "features", "x.test.ts"), area: "src", text },
      ]),
    ).toEqual(["src/features/x.ts:1"]);
  });
});
