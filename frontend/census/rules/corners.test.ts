// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import {
  aliasOf,
  blocks,
  corneredPseudoElements,
  isRound,
  ladder,
  PHONE,
  phoneRungBlocks,
  radii,
  readsTheLadder,
  rule,
  squircleLadder,
  squircleSelectors,
} from "./corners.census";

describe("the ladder itself", () => {
  const tokens = readFileSync(
    join(srcDir, "design-system", "tokens.css"),
    "utf8",
  );

  it("runs in fours", () => {
    for (const [name, value] of ladder(tokens)) {
      if (name === "--r-full" || aliasOf(value)) {
        continue;
      }
      const px = Number(value.replace("px", ""));
      expect(
        Number.isInteger(px) && px % 4 === 0,
        `${name} is ${value}. The ladder runs in fours, ` +
          `so that a corner is a rung a reader can name rather than a number ` +
          `somebody liked.`,
      ).toBe(true);
    }
  });

  it("aliases a rung only to a rung the ladder sizes itself", () => {
    const base = ladder(tokens);
    for (const [name, value] of base) {
      const target = aliasOf(value);
      if (!target) {
        continue;
      }
      expect(
        base.get(target),
        `${name} reads ${target}, which the ladder does not size.`,
      ).toMatch(/^\d+px$/);
    }
  });

  it("doubles every finite rung under corner-shape, and the pill not at all", () => {
    const base = ladder(tokens);
    const smooth = squircleLadder(tokens);
    expect(smooth.size).toBeGreaterThan(0);
    for (const [name, value] of base) {
      if (aliasOf(value)) {
        expect(
          smooth.has(name),
          `${name} reads ${value} and is redeclared in the squircle block. ` +
            `An alias follows its rung there by itself; a second number is a ` +
            `second answer that drifts from the first.`,
        ).toBe(false);
        continue;
      }
      if (name === "--r-full") {
        expect(
          smooth.has(name),
          `--r-full is redeclared in the squircle block. It is already larger ` +
            `than any box it lands on, and doubling it says nothing; the pill ` +
            `stays round by cancelling the shape, not by growing.`,
        ).toBe(false);
        continue;
      }
      const doubled = smooth.get(name);
      expect(
        doubled,
        `${name} has no twin in the @supports (corner-shape: squircle) block. ` +
          `A rung that is not doubled there tightens the day a browser ships the ` +
          `property, and only there — the two states never appear side by side, ` +
          `so nothing would look wrong here.`,
      ).toBeDefined();
      expect(doubled).toBe(`${Number(value.replace("px", "")) * 2}px`);
    }
  });

  it("keeps one ladder at every width", () => {
    const phone = phoneRungBlocks(tokens);
    expect(
      phone.flatMap((block) => [...block.rungs.keys()]),
      `a ${PHONE} block redeclares a rung. A phone and a desktop round alike: ` +
        `change the rung itself, or alias it to a smaller one.`,
    ).toEqual([]);
  });
});

describe("the corner detector sees what it claims to", () => {
  const rungs = new Set([
    "--r-xs",
    "--r-sm",
    "--r-control",
    "--r-md",
    "--r-lg",
    "--r-full",
  ]);

  const cases: { name: string; css: string; strays: number }[] = [
    {
      name: "a plain token",
      css: ".a { border-radius: var(--r-md); }",
      strays: 0,
    },
    { name: "a raw px", css: ".a { border-radius: 11px; }", strays: 1 },
    {
      name: "a raw px inside a media query",
      css: "@media (max-width: 40rem) { .a { border-radius: 11px; } }",
      strays: 1,
    },
    {
      name: "a raw px on one corner only",
      css: ".a { border-bottom-right-radius: 3px; }",
      strays: 1,
    },
    {
      name: "a token nested by a calc",
      css: ".a { border-radius: calc(var(--r-sm) - 2px); }",
      strays: 0,
    },
    {
      name: "an elliptical corner whose two radii adjust a rung",
      css: ".a { border-radius: calc(var(--r-sm) + 2px) / calc(var(--r-sm) + 6px); }",
      strays: 0,
    },
    {
      name: "an elliptical corner with a raw down radius",
      css: ".a { border-radius: var(--r-sm) / 6px; }",
      strays: 1,
    },
    {
      name: "an elliptical corner written with no spaces round its slash",
      css: ".a { border-radius: var(--r-sm)/var(--r-md); }",
      strays: 0,
    },
    {
      name: "a raw down radius with no spaces round its slash",
      css: ".a { border-radius: var(--r-sm)/6px; }",
      strays: 1,
    },
    {
      name: "a mixed shorthand with one raw corner",
      css: ".a { border-radius: 4px var(--r-md) var(--r-md) var(--r-md); }",
      strays: 1,
    },
    {
      name: "a zero and a keyword",
      css: ".a { border-radius: 0; } .b { border-radius: inherit; }",
      strays: 0,
    },
    { name: "a circle", css: ".a { border-radius: 50%; }", strays: 0 },
    {
      name: "a waived one-off",
      css: ".a { border-radius: 3px; /* ds:ignore the notch's own tip */ }",
      strays: 0,
    },
    {
      name: "a waiver with no reason",
      css: ".a { border-radius: 3px; /* ds:ignore */ }",
      strays: 1,
    },
    {
      // The name is assembled, so check-space-tokens.sh does not read it as a
      // use of a custom property nothing declares.
      name: "a token that is not on the ladder",
      css: `.a { border-radius: var(${"--radius-lg"}); }`,
      strays: 1,
    },
  ];

  for (const probe of cases) {
    it(`${probe.strays ? "reports" : "ignores"} ${probe.name}`, () => {
      const strays = blocks(probe.css)
        .flatMap((block) => radii(block.body))
        .filter(
          (radius) => !radius.waived && !readsTheLadder(radius.value, rungs),
        );
      expect(strays).toHaveLength(probe.strays);
    });
  }

  it("sees a cornered pseudo-element, and ignores one with no corner", () => {
    expect(
      corneredPseudoElements(".a::after { border-radius: var(--r-sm); }"),
    ).toEqual(["::after"]);
    expect(
      corneredPseudoElements(".a::before { /* border-radius: 4px; */ }"),
      "a commented-out radius draws nothing, so it corners nothing",
    ).toEqual([]);
    expect(
      corneredPseudoElements('.a::before { content: ""; opacity: 0.5; }'),
    ).toEqual([]);
    expect(
      corneredPseudoElements(".a::marker { border-radius: 50%; }"),
    ).toEqual(["::marker"]);
  });

  it("reads the squircle rule's own selector list, not the file's text", () => {
    const real =
      "@supports (corner-shape: squircle) {\n" +
      "  :root { --r-sm: 16px; }\n" +
      "  :where(*),\n  :where(*)::before {\n    corner-shape: squircle;\n  }\n}";
    expect(squircleSelectors(real)).toEqual([":where(*)", ":where(*)::before"]);
    const decoy =
      "@supports (corner-shape: squircle) {\n" +
      "  .card::before { border-radius: var(--r-sm); }\n" +
      "  :where(*) { corner-shape: squircle; }\n}";
    expect(
      squircleSelectors(decoy).includes(":where(*)::before"),
      "a pseudo-element named on an unrelated rule under the query covers nothing",
    ).toBe(false);
    const descendant =
      "@supports (corner-shape: squircle) {\n" +
      "  :where(*) ::before { corner-shape: squircle; }\n}";
    expect(
      squircleSelectors(descendant),
      "the space is the descendant combinator: that rule paints every " +
        "descendant's generated box and not the subject's own, so it must not " +
        "read as the exact form",
    ).toEqual([":where(*) ::before"]);
    const commented =
      "@supports (corner-shape: squircle) {\n" +
      "  :where(*)::before { /* corner-shape: squircle; */ }\n}";
    expect(
      squircleSelectors(commented),
      "a commented-out declaration declares nothing",
    ).toEqual([]);
    const customProperty =
      "@supports (corner-shape: squircle) {\n" +
      "  :where(*)::before { --corner-shape: squircle; }\n}";
    expect(
      squircleSelectors(customProperty),
      "a custom property of a similar name paints no corner",
    ).toEqual([]);
    const otherValue =
      "@supports (corner-shape: squircle) {\n" +
      "  :where(*)::before { corner-shape: squircle-variant; }\n}";
    expect(
      squircleSelectors(otherValue),
      "a different value is a different shape",
    ).toEqual([]);
    const important =
      "@supports (corner-shape: squircle) {\n" +
      "  :where(*) { corner-shape: squircle !important; }\n}";
    expect(squircleSelectors(important)).toEqual([":where(*)"]);
  });

  it("reads a round corner in every spelling it is written in", () => {
    expect(isRound("var(--r-full)")).toBe(true);
    expect(isRound("50%")).toBe(true);
    expect(isRound("var(--r-md)")).toBe(false);
  });

  it("sees a block that opts out, and one that forgot to", () => {
    const kept = blocks(".a { border-radius: 50%; corner-shape: round; }")[0];
    const missed = blocks(".a { border-radius: 50%; }")[0];
    expect(/corner-shape\s*:\s*round/.test(kept.body)).toBe(true);
    expect(/corner-shape\s*:\s*round/.test(missed.body)).toBe(false);
  });
});

describe("the corners rule", () => {
  it("reports a planted raw radius through its own corpus and across", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "design-system", "tokens.css"),
        area: "src",
        text: ":root { --r-sm: 8px; }\n",
      },
      {
        path: join(srcDir, "features", "p.css"),
        area: "src",
        text: ".a { border-radius: 11px; }\n.b { border-radius: 50%; }\n",
      },
    ]);
    expect(findings).toEqual([
      "src/features/p.css: `border-radius: 11px` names a length, not a rung (--r-sm)",
      "src/features/p.css: `border-radius: 50%` is round and does not cancel the squircle; add `corner-shape: round;`",
    ]);
  });
});
