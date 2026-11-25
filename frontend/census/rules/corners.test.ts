// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import {
  aliasOf,
  blocks,
  ladder,
  PHONE,
  phoneRungBlocks,
  radii,
  readsTheLadder,
  rule,
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
    ]);
  });
});
