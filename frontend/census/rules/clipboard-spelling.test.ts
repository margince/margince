// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import {
  clipboardAccessesIn,
  mayReachForTheClipboard,
  rule,
} from "./clipboard-spelling.census";

// One spelling per line: the shapes hand-rolled sites use, and the bracketed
// forms of the same reach. The matcher and the prefilter both read this list.
const PLANTED_SPELLINGS = [
  "await navigator.clipboard.writeText(a);",
  "const writer = navigator.clipboard;",
  "if (!navigator.clipboard) return;",
  "window.navigator.clipboard.writeText(b).then(ok, no);",
  'await navigator["clipboard"].writeText(c);',
  'const bracketed = window.navigator["clipboard"];',
  'window["navigator"].clipboard.writeText(d).then(ok, no);',
  "const { clipboard } = navigator;",
  "const { clipboard: writer } = window.navigator;",
  'const { "clipboard": quoted } = navigator;',
  'const { ["clipboard"]: computed } = navigator;',
] as const;

describe("one clipboard spelling", () => {
  it("finds the access however the caller spells it", () => {
    const planted = parseSource("planted.tsx", PLANTED_SPELLINGS.join("\n"));
    expect(clipboardAccessesIn(planted)).toEqual(
      PLANTED_SPELLINGS.map((_, index) => index + 1),
    );
  });

  it("lets through every shape the matcher can see", () => {
    // The prefilter is where this gate could read a smaller tree and pass.
    for (const spelling of PLANTED_SPELLINGS) {
      expect(mayReachForTheClipboard(spelling)).toBe(true);
    }
  });

  it("reports a planted access through its own corpus, and spares the owner", () => {
    const text =
      "export const copy = () => navigator.clipboard.writeText(x);\n";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "p.ts"), area: "src", text },
        {
          path: join(srcDir, "design-system", "clipboardcopy.tsx"),
          area: "src",
          text,
        },
      ]),
    ).toEqual(["src/features/p.ts:1"]);
  });
});
