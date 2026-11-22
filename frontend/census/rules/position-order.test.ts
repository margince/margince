// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { rule } from "./position-order.census";

const OWNER_TEXT = [
  "export function inPositionOrder<T extends { position: number }>(stages: readonly T[]): T[] {",
  "  return [...stages].sort((a, b) => a.position - b.position);",
  "}",
];

const planted = (path: string, lines: readonly string[]) => ({
  path: join(srcDir, path),
  area: "src" as const,
  text: lines.join("\n"),
});

const findingsIn = (lines: readonly string[]) =>
  plantedFindings(rule, [
    planted("lib/pipelines.queries.ts", OWNER_TEXT),
    planted("features/deals/probe.ts", lines),
  ]);

describe("the position-order rule", () => {
  it("fails a position comparator outside the lib, by every spelling", () => {
    expect(
      findingsIn([
        "stages.sort((a, b) => a.position - b.position);",
        "const desc = stages.toSorted((a, b) => b.position - a.position);",
        "const byPosition = (left: Stage, right: Stage) =>",
        "  (left.position ?? 0) - (right.position ?? 0);",
        "stages.sort(function (a, b) {",
        "  return a.position - b.position;",
        "});",
        "stages.sort(({ position: a }, { position: b }) => a - b);",
        "stages.sort(({ position }, other) => position - other.position);",
      ]),
    ).toEqual([
      "src/features/deals/probe.ts:1: compares two rows by position",
      "src/features/deals/probe.ts:2: compares two rows by position",
      "src/features/deals/probe.ts:3: compares two rows by position",
      "src/features/deals/probe.ts:5: compares two rows by position",
      "src/features/deals/probe.ts:8: compares two rows by position",
      "src/features/deals/probe.ts:9: compares two rows by position",
    ]);
  });

  it("spares the lib's own comparator, other keys, a distance, and a mention", () => {
    expect(
      findingsIn([
        "rows.sort((a, b) => a.stagePosition - b.stagePosition);",
        "deals.sort((a, b) => a.stage.position - b.stage.position);",
        "const gap = here.position - start.position;",
        "const shift = (stage: Stage) => stage.position - 1;",
        "// stages.sort((a, b) => a.position - b.position)",
        'const advice = "never write (a, b) => a.position - b.position";',
      ]),
    ).toEqual([]);
  });

  it("states a floor when the lib's comparator is missing or unrecognised", () => {
    const floorsOf = (owner: readonly string[] | undefined) => {
      const tree = treeOf(
        owner === undefined ? [] : [planted("lib/pipelines.queries.ts", owner)],
      );
      return rule.floors?.(rule.corpus(tree), tree);
    };
    expect(floorsOf(OWNER_TEXT)).toEqual([]);
    expect(floorsOf(["export const inPositionOrder = sortBy;"])).toEqual([
      "src/lib/pipelines.queries.ts holds no comparator this rule recognises",
    ]);
    expect(floorsOf(undefined)).toEqual([
      "src/lib/pipelines.queries.ts was not read",
    ]);
  });
});
