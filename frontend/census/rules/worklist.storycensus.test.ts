// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { KNOWN_SOURCES } from "../../src/lib/worklistcopy";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./worklist.storycensus.census";

const storiesDir = join(srcDir, "features", "worklist");
const [drawn, missed] = Object.keys(KNOWN_SOURCES).filter(
  (source) => source !== "batch",
);

describe("the worklist.storycensus rule", () => {
  it("counts a source set beside a category, and fails one never drawn", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(storiesDir, "worklist.planted.stories.tsx"),
        area: "src",
        text:
          `export const a = { source: "${drawn}", category: "task" };\n` +
          `export const b = { source: "retired_source", category: "task" };\n` +
          `export const c = { source: "${missed}" };\n`,
      },
      {
        path: join(storiesDir, "planted.stories.tsx"),
        area: "src",
        text: `export const d = { source: "${missed}", category: "task" };\n`,
      },
    ]);
    expect(findings).toContain(`${missed}: no worklist story draws it`);
    expect(findings).not.toContain(`${drawn}: no worklist story draws it`);
    expect(findings).toContain(
      "retired_source: a story draws it, the queue does not know it",
    );
  });
});
