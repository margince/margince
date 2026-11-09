// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { Rule } from "../../scripts/lib/css-rules";
import { parseSource } from "../../scripts/lib/source-tree";
import {
  extensionsDir,
  plantedFindings,
  sheetRules,
  srcDir,
  treeOf,
} from "../tree";
import {
  cutsOfWholeText,
  figureInks,
  openLabels,
  rule,
} from "./statcard.census";

const here = join(srcDir, "design-system");
const at = (file: string, selector: string, body: string): Rule[] => [
  { file: join(here, file), selector, body },
];

describe("a toned figure takes its ink only from atoms.css", () => {
  it.each([
    [
      "atoms.css",
      ".stat-card-warning.stat-card-value",
      "color: var(--warning);",
    ],
    ["atoms.css", ".stat-card-danger:hover", "color: var(--danger);"],
    ["atoms.css", ":where(.stat-card-info)", "color: var(--successText);"],
    [
      "atoms.css",
      ".stat-card-success",
      "color: var(--successText); color: var(--success);",
    ],
    ["brief.css", ".brief .stat-card-value", "color: var(--textPrimary);"],
    ["brief.css", ".brief .stat-card-warning", "color: var(--warningText);"],
  ])("refuses %s %s { %s }", (file, selector, body) => {
    expect(figureInks(at(file, selector, body))).toHaveLength(1);
  });

  it("lets atoms.css spell the untoned figure and each toned one once", () => {
    expect(
      figureInks([
        ...at("atoms.css", ".stat-card-value", "color: var(--textPrimary);"),
        ...at("atoms.css", ".stat-card-warning", "color: var(--warningText);"),
        ...at(
          "brief.css",
          ".stat-card-warning ~ .stat-card-meter",
          "color: red;",
        ),
      ]),
    ).toEqual([]);
  });
});

describe("a reading's name and figure are never cut", () => {
  it.each([
    ["atoms.css", ".stat-card-value", "white-space: nowrap;"],
    ["atoms.css", ".stat-card-label-text", "-webkit-line-clamp: 1;"],
    ["atoms.css", ".stat-card-label-text", "text-overflow: ellipsis;"],
    ["atoms.css", ".stat-card-value", "overflow-wrap: anywhere;"],
    [
      "../../../extensions/demo/frontend/atoms.css",
      ".stat-card-value",
      "text-overflow: ellipsis;",
    ],
    [
      "atoms.css",
      '[data-density="compact"] .stat-card-value',
      "overflow: hidden;",
    ],
    ["brief.css", ".brief .stat-card-value", "text-overflow: ellipsis;"],
    ["statstrip.css", ".stat-strip > * > .stat-card", "overflow: clip;"],
  ])("refuses %s %s { %s }", (file, selector, body) => {
    expect(cutsOfWholeText(at(file, selector, body))).toHaveLength(1);
  });

  it("lets atoms.css end an over-wide figure in an ellipsis", () => {
    expect(
      cutsOfWholeText(
        at("atoms.css", ".stat-card-value", "text-overflow: ellipsis;"),
      ),
    ).toEqual([]);
  });
});

describe("no reading names its own door", () => {
  it("finds an openLabel attribute wherever it is written", () => {
    const source = parseSource(
      "planted.tsx",
      'const a = <StatCard label="x" value="1" />;\nconst b = <StatCard openLabel="Go" />;',
    );
    expect(openLabels(source)).toEqual([2]);
  });
});

describe("the statcard rule", () => {
  it("reads a sheet's rules from the tree's text, comments left out", () => {
    const sheet = treeOf([
      {
        path: join(here, "planted.css"),
        area: "src",
        text: "/* .x { color: red } */\n.a, .b { color: blue; }",
      },
    ]).files;
    expect(sheetRules(sheet).map((one) => one.selector)).toEqual([".a", ".b"]);
  });

  it("reports planted findings through its own corpus and across", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.tsx"),
          area: "src",
          text: 'const a = <StatCard openLabel="Go" />;',
        },
        {
          path: join(srcDir, "features", "planted.stories.tsx"),
          area: "src",
          text: 'const a = <StatCard openLabel="Go" />;',
        },
        {
          path: join(extensionsDir, "demo", "frontend", "planted.css"),
          area: "extensions",
          text: ".stat-card-value { white-space: nowrap; color: red; }",
        },
      ]),
    ).toEqual([
      "src/features/planted.tsx:1: openLabel",
      "planted.css .stat-card-value { color: red }",
      "../../extensions/demo/frontend/planted.css .stat-card-value cuts the reading",
    ]);
  });
});
