// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import type { Rule } from "../../scripts/lib/css-rules";
import { here, membersOf, probe, ref } from "../../scripts/lib/surface-sheets";
import { stable } from "../../src/format/collate";
import { srcDir, treeOf } from "../tree";
import {
  cssSpellings,
  memberPaints,
  RUNGS,
  rule,
  rungTokens,
  scriptSpellings,
  THEME_ROOTS,
  tokenDefinitions,
  tokensPath,
  unstyledLadderClasses,
} from "./surfaces.census";

describe("the nesting ladder has one painter", () => {
  it("reads the theme roots and the rungs off the token sheets", () => {
    expect([...THEME_ROOTS].sort(stable)).toEqual([
      ":root",
      ':root:not([data-theme="light"])',
      '[data-theme="dark"]',
    ]);
    expect([...RUNGS].sort(stable)).toEqual(
      expect.arrayContaining([
        "--bgElevated",
        "--pane",
        "--paneEdge",
        "--ladder2",
        "--ladder4Edge",
      ]),
    );
  });

  it("finds a rung however it is spelled", () => {
    const planted: Rule[] = [
      ["background-image", "linear-gradient(var(--pane), var(--pane))"],
      ["background", 'url("a.svg") no-repeat, var(--ladder2)'],
      ["--plate", "var(--bgElevated)"],
      ["box-shadow", "0 0 0 100vw var(--ladder4)"],
      ["background-color", "var(--ladder3)"],
      ["border", "1px solid var(--paneEdge)"],
      ["border-color", "var(--ladder3Edge)"],
    ].map(([property, value]) => probe(".probe", `${property}: ${value}`));
    expect(cssSpellings(planted)).toHaveLength(planted.length);
    expect(
      scriptSpellings(
        join(here, "probe.tsx"),
        `const a = <div style={{ background: "var(--ladder2)" }} />; const b = \`var(--pane)\`; const c = \`var(\${token})\`; const d = "var(--" + name + ")";`,
      ),
    ).toHaveLength(4);
    const dynamic = 'const d = "var(--" + name;';
    expect(scriptSpellings(join(here, "probe.ts"), dynamic)).toHaveLength(1);
    expect(
      scriptSpellings(join(here, "probe.tsx"), 'const e = "\\u002d-pane";'),
    ).toHaveLength(1);
    expect(
      cssSpellings([
        probe(".probe", `border: 1px solid ${ref("--paneEdgeStrong")}`),
      ]),
    ).toEqual([]);
  });

  it("takes a token's definition only from a theme root of the token sheets", () => {
    const at = (file: string, selector: string): Rule => ({
      ...probe(selector, "--plate: var(--ladder3)"),
      file,
    });
    expect(
      cssSpellings([
        at(tokensPath, '[data-theme="dark"] .x'),
        at(tokensPath, ':root:not([data-theme="light"]) .x'),
        at(tokensPath, ":root .x"),
        at(join(here, "probe.css"), ":root"),
      ]),
    ).toHaveLength(4);
    expect(
      cssSpellings([
        at(tokensPath, ":root"),
        at(tokensPath, '[data-theme="dark"]'),
      ]),
    ).toEqual([]);
  });

  it("closes the rungs over every alias, either way and in either theme", () => {
    expect(
      rungTokens([
        ...tokenDefinitions,
        { property: "--cardGround", value: "var(--ladder2)" },
        { property: "--ladder3", value: ref("--darkStep") },
        { property: "--stepAlias", value: ref("--darkStep") },
      ]),
    ).toEqual(new Set([...RUNGS, "--cardGround", "--darkStep", "--stepAlias"]));
  });

  it("finds a member painted by a rule that outranks the ladder", () => {
    const members = membersOf([
      ".pane",
      ".host .dialog",
      ".alert.is-quiet",
      ".rows > li",
    ]);
    const flat = (token: string) =>
      `background-image: linear-gradient(var(${token}), var(${token}))`;
    expect(
      memberPaints(
        [
          probe(".pane", "background: var(--dangerBg)"),
          probe(".pane.is-picked", flat("--bgInset")),
          probe(
            ".dialog",
            "background: color-mix(in srgb, red 68%, transparent)",
          ),
          probe(".alert", "background: var(--dangerBg)"),
          probe(".rows > li.is-on", "background-color: var(--accentLight)"),
          probe(".pane::before", "background: var(--danger)"),
          probe(".pane:hover", "background: var(--groundHover)"),
          probe(".pane.is-ai", flat("--aiLight")),
          probe(
            ".pane.is-wash",
            "background-image: radial-gradient(red, blue)",
          ),
          probe(".pane.is-flat", "background: none"),
        ],
        members,
      ),
    ).toHaveLength(5);
    expect(
      memberPaints(
        [
          probe(".dialog", "background: var(--danger)"),
          probe(".host .dialog", "background: var(--ground)"),
          probe(".alert", "background: var(--dangerBg)"),
          probe(".alert.is-quiet", "background-color: var(--ground)"),
        ],
        members,
      ),
    ).toEqual([]);
  });

  it("finds a ladder class no other sheet styles", () => {
    expect(unstyledLadderClasses([])).not.toEqual([]);
  });
});

describe("the surfaces rule", () => {
  it("reports a planted rung spelling through its own corpus and across", () => {
    const tree = treeOf([
      {
        path: join(srcDir, "features", "planted.tsx"),
        area: "src",
        text: 'const a = <div style={{ background: "var(--ladder2)" }} />;',
      },
    ]);
    const findings =
      "across" in rule ? rule.across(rule.corpus(tree), tree) : [];
    expect(findings).toContain(
      'src/features/planted.tsx: "var(--ladder2)": not in the ledger',
    );
  });
});
