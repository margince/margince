// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { captionsIn, rule } from "./caption-tables.census";

describe("the caption-table finder, over planted modules", () => {
  it("finds an English caption in a table", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        `export const SAID = { deals: "Loading pipeline", one: "Loading %s deal" };`,
      ),
    ).toEqual(["Loading pipeline", "Loading %s deal"]);
  });

  it("finds one in a typed array", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        `const WROTE = [["Editing {name}", "Editing deal"]] as const;`,
      ),
    ).toEqual(["Editing deal"]);
  });

  it("finds a caption led by a placeholder, a one-word state and a trailing ellipsis", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        `const SAID = { typing: "{name} is typing", busy: "Thinking", wait: "Loading deals…", end: "Failed." };`,
      ),
    ).toEqual(["{name} is typing", "Thinking", "Loading deals…", "Failed."]);
  });

  it("passes a table of type names that only look like states", () => {
    expect(
      captionsIn(
        "/app/src/types.ts",
        `export const TYPES = { string: "String", thing: "Thing", seed: "Seed", embedded: "Embedded" };`,
      ),
    ).toEqual([]);
  });

  it("passes catalog keys, product names and text inside a function", () => {
    expect(
      captionsIn(
        "/app/src/copy.ts",
        [
          `const SAID = { deals: "agent.ticker.read.deals" };`,
          `const NAMES = { meet: "Google Meet", teams: "Microsoft Teams", crm: "Salesforce" };`,
          `const KEYS = { field: "Title", path: "{base}/deals", count: "{count}" };`,
          `function f() { return { x: "Loading pipeline" }; }`,
        ].join("\n"),
      ),
    ).toEqual([]);
  });
});

describe("the caption-tables rule", () => {
  // The real entry, bootstrap.ts, loads one module with a caption table and
  // nothing loads the other.
  const entry = join(srcDir, "bootstrap.ts");
  const planted = [
    {
      path: entry,
      area: "src" as const,
      text: `import { SAID } from "./said";`,
    },
    {
      path: join(srcDir, "said.ts"),
      area: "src" as const,
      text: `export const SAID = { busy: "Loading pipeline" };`,
    },
    {
      path: join(srcDir, "stray.ts"),
      area: "src" as const,
      text: `export const STRAY = { busy: "Loading deals" };`,
    },
  ];

  it("reports a caption the bundle loads, and not one it never loads", () => {
    expect(plantedFindings(rule, planted)).toEqual([
      "said.ts: Loading pipeline",
    ]);
  });
});
