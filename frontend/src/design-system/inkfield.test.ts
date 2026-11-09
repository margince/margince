// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { scopeOf } from "../../census/rules/inkfield.census";
import { rulesOf } from "../../scripts/lib/css-rules";
import { contrastOf, themes } from "./tokens-testing";

// The ink field is one dark plate in both themes, and the page's inks are
// tuned for a light page. inkfield.css hands every plate the plate's inks;
// census/rules/inkfield.census.ts holds that it lists every painter.

const here = dirname(fileURLToPath(import.meta.url));

const [tokenRule, linkRule] = rulesOf([join(here, "inkfield.css")]);
const scope = scopeOf(tokenRule.selector);

// Each page token the scope re-points, and the plate ink it now reads.
const repointed = [
  ...tokenRule.body.matchAll(/(--[\w-]+)\s*:[^;]*?var\((--[\w-]+)\)/g),
].map(([, token, ink]) => ({ token, ink }));

describe("the ink field's scope", () => {
  it("underlines links on the same plates it re-inks", () => {
    expect(scopeOf(`${linkRule.selector.split(/\)\s*:is/)[0]})`)).toEqual(
      scope,
    );
  });

  it("re-points the text, link and focus inks", () => {
    expect(repointed.map(({ token }) => token)).toEqual([
      "--textPrimary",
      "--textSecondary",
      "--accentText",
      "--focus-ring",
    ]);
  });

  it("gives every re-pointed ink AA on the plate, both themes", () => {
    const failures = Object.entries(themes).flatMap(([theme, pal]) =>
      repointed.flatMap(({ token, ink }) => {
        const ratio = contrastOf(pal[ink], pal["--bgInkField"]);
        return ratio < 4.5
          ? [`${theme}: ${token} → ${ink} = ${ratio.toFixed(2)}:1`]
          : [];
      }),
    );
    expect(failures).toEqual([]);
  });
});
