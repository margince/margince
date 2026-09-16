// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { normalize, parseBlock, tokenDecls, tokensCss } from "./tokens-testing";

// The TYPE half of tokens.css: the three weights, the ten size shorthands that
// read them, and the geometry of the one control size. Split from
// tokens.test.ts, which pins the Ledger-Green palette — two subjects that share
// a sheet and nothing else, and one file holding both had grown past the point
// where a reader could hold it.
//
// This is the inside of the rule type-source.test.ts holds everywhere else.
// That gate refuses a size, leading, weight or tracking spelled by value in any
// file but this one; something still has to say what tokens.css itself may say,
// and a sheet nobody checks is the one place a second spelling can hide in
// plain sight — it would be the SOURCE of the drift rather than a copy of it.

describe("the type scale and the control geometry", () => {
  const light = parseBlock(tokenDecls, ":root");

  // Declared in the block every document gets, not in one a device may not
  // match. tokens.test.ts holds the layout half of the same obligation.
  it("declares every level unconditionally", () => {
    for (const name of [
      "--fontWeightRegular",
      "--fontWeightMedium",
      "--fontWeightBold",
      "--fontBodyLarge",
      "--paragraphSpacingLarge",
      "--fontBody",
      "--paragraphSpacing",
      "--fontBodySmall",
      "--paragraphSpacingSmall",
      "--fontHeadingXXLarge",
      "--fontHeadingXLarge",
      "--fontHeadingLarge",
      "--fontHeadingMedium",
      "--fontHeadingSmall",
      "--fontHeadingXSmall",
      "--fontHeadingXXSmall",
      "--fontFamilyBody",
      "--controlHeight",
      "--controlPaddingX",
      "--controlGap",
      "--controlIcon",
      "--inputHeight",
      "--inputPaddingX",
      "--inputPaddingY",
    ]) {
      expect(light[name], `${name} missing from :root`).toBeTruthy();
    }
  });

  // The weights and the control geometry, by value. A role token whose name
  // survives a retune that silently changed what it means is worse than no
  // token: every call site keeps reading it and the product moves under them.
  it("declares the three weights and the one control geometry", () => {
    const want: Readonly<Record<string, string>> = {
      "--fontWeightRegular": "400",
      "--fontWeightMedium": "500",
      "--fontWeightBold": "700",
      "--controlHeight": "32px",
      "--controlPaddingX": "var(--space-3)",
      "--controlGap": "6px",
      "--controlIcon": "16px",
      // A text input is the one control that is not --controlHeight tall, and
      // its height is arithmetic rather than taste: 20px of line, 8px of
      // padding top and bottom, 1px of edge on each side.
      "--inputHeight": "40px",
      "--inputPaddingX": "6px",
      "--inputPaddingY": "var(--space-2)",
    };
    for (const [name, value] of Object.entries(want)) {
      expect(normalize(light[name] ?? ""), name).toBe(normalize(value));
    }
  });

  // One control size. The small rung was a second answer to "how tall is a
  // control", and the two drifted on every screen that mixed them; an icon
  // button now takes --controlHeight like everything else. Asserted on the
  // whole sheet rather than on :root, so the touch arm cannot bring it back.
  it("carries no second control height", () => {
    expect(tokensCss).not.toMatch(/--control-h\b/);
  });

  // 32px under a finger as much as under a mouse. The height used to rise to 44
  // for a coarse pointer, which made the one control size two again — one per
  // input device — and left every surface owing the same question a second
  // answer. Asserted on the sheet, because the arm that did it was a media
  // query and a :root list cannot see one.
  it("raises the control height for no input device", () => {
    expect(tokensCss).not.toMatch(/pointer:\s*coarse/);
  });

  // The input's parts have to FIT the height it declares. Asserted as the
  // inequality rather than as a sum, because the sum is not what breaks: a
  // padding raised past the height is an input that clips the text being typed
  // into it, and that holds whatever slack the height was given. (It has 2px
  // today — 20px of line, 2x8 of padding and 2x1 of edge is 38 in a 40px box.)
  it("fits the input's parts inside its height", () => {
    // One level of var() is resolved, because the padding names a rung of the
    // spacing scale rather than restating its value — which is the point of it.
    const px = (name: string): number => {
      const value = light[name] ?? "";
      const alias = /^var\((--[\w-]+)\)$/.exec(value.trim());
      return alias ? px(alias[1]) : Number.parseInt(value, 10);
    };
    const line = 20;
    const edges = 2;
    expect(px("--inputPaddingY")).toBe(8);
    expect(line + 2 * px("--inputPaddingY") + edges).toBeLessThanOrEqual(
      px("--inputHeight"),
    );
  });

  // Every size in the type scale reads a weight token. A spelled 400 or 700
  // here is the second spelling type-source.test.ts refuses everywhere else,
  // and this file is the one place that gate cannot speak for.
  it("spells no weight by value in the type scale", () => {
    const spelled = Object.entries(light)
      .filter(([name]) => /^--font(Body|Heading)/.test(name))
      .filter(([, value]) => !value.startsWith("var(--fontWeight"))
      .map(([name, value]) => `${name}: ${value}`);
    expect(spelled, spelled.join("\n")).toEqual([]);
  });
});
