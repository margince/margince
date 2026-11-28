// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// biome-ignore-all lint/suspicious/noTemplateCurlyInString: these strings are source fixtures, and a template literal in them is the subject.

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { gluedPercentsIn, mayGlueAPercent, rule } from "./percent-sign.census";

// One glued sign per line, in each shape a hand-rolled percent takes.
const GLUED = [
  "const a = `${formatNumber(x, locale)}%`;",
  "const b = `${formatAnalyticsNumber(x, locale)} %`;",
  "const c = <span>{formatNumber(x, locale)}%</span>;",
  'const d = formatNumber(x, locale) + "%";',
  "const e = `${fmt.formatNumber(x, locale)}%`;",
  'const f = `${formatNumber(x, locale)}${unit === "percent" ? "%" : ""}`;',
  'const g = `${formatNumber(x, locale)}${"%"}`;',
] as const;

// What the gate leaves alone: a style length, and a formatter that sets the
// sign itself.
const CLEAN = [
  "const f = { inlineSize: `${Math.round(share * 100)}%` };",
  "const g = <span>{formatPercent(x, locale)}</span>;",
  "const h = `${formatNumber(x, locale)} deals`;",
] as const;

describe("one percent spelling", () => {
  it("finds a sign glued to a formatter however the caller writes it", () => {
    const planted = parseSource("planted.tsx", GLUED.join("\n"));
    expect(gluedPercentsIn(planted)).toEqual(
      GLUED.map((_, index) => index + 1),
    );
  });

  it("leaves a style length and a percent formatter alone", () => {
    const planted = parseSource("planted.tsx", CLEAN.join("\n"));
    expect(gluedPercentsIn(planted)).toEqual([]);
  });

  it("lets through every shape the matcher can see", () => {
    for (const spelling of GLUED) {
      expect(mayGlueAPercent(spelling)).toBe(true);
    }
  });

  it("reports a planted site through its own corpus", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "p.tsx"),
          area: "src",
          text: "export const p = (n: number) => `${formatNumber(n, 'en')}%`;\n",
        },
      ]),
    ).toEqual(["src/features/p.tsx:1"]);
  });
});

describe("the percent prefilter", () => {
  it("reads a glued sign however it is spelled", () => {
    const at = (sign: string) =>
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "plant", "p.tsx"),
          area: "src",
          text: `export const p = (n: number) => \`\${formatNumber(n, "en")}${sign}\`;\n`,
        },
      ]);
    expect(at("%")).toHaveLength(1);
    expect(at("\\x25")).toHaveLength(1);
  });
});
