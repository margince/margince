// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// happy-dom lays nothing out, so the row's two width rules are read off the
// sheets that hold them.
const here = dirname(fileURLToPath(import.meta.url));
const sheet = (name: string) => readFileSync(join(here, name), "utf8");

// The container width from which a sheet stands the verbs beside the work.
function verbsBesideFrom(css: string): string[] {
  const widths: string[] = [];
  for (const block of css.matchAll(
    /@container \(min-width: ([\d.]+rem)\) \{([\s\S]*?)\n\}/g,
  )) {
    if (/\.worklist-row-acts\s*\{[^}]*grid-column:\s*[34];/.test(block[2])) {
      widths.push(block[1]);
    }
  }
  return widths;
}

describe("a row's verbs leave the title its width", () => {
  it("stand beside the work in the drawer only from the queue page's width", () => {
    const page = verbsBesideFrom(sheet("worklist.row.css"));
    const drawer = verbsBesideFrom(sheet("worklist.embedded.css"));
    expect(page).toHaveLength(1);
    expect(drawer).toEqual(page);
  });

  it("never opens a wrapped caption line on its separator", () => {
    const css = sheet("worklist.row.css");
    expect(css).not.toMatch(/\.worklist-row-facts-line > \* \+ \*::before/);
    expect(css).toMatch(
      /\.worklist-row-facts-line > :has\(\+ \*\)::after\s*\{\s*content: "·";/,
    );
  });
});
