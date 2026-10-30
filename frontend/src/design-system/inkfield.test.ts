// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  appStylesheets,
  declaredValues,
  rulesOf,
  selectorList,
} from "../../scripts/lib/css-rules";
import { contrastOf, themes } from "./tokens-testing";

// The ink field is one dark plate in both themes, and the page's inks are
// tuned for a light page. inkfield.css lists every component that paints the
// plate and hands it the plate's inks; these gates derive the plate painters
// from the app's sheets, so a new one cannot land unlisted.

const here = dirname(fileURLToPath(import.meta.url));
const frontendRoot = join(here, "..", "..");

const squash = (selector: string) => selector.replace(/\s+/g, " ").trim();
const scopeOf = (selector: string) =>
  selectorList(/^:(?:where|is)\(([\s\S]*)\)/.exec(selector)?.[1] ?? "").map(
    squash,
  );

const [tokenRule, linkRule] = rulesOf([join(here, "inkfield.css")]);
const scope = scopeOf(tokenRule.selector);

const painters = rulesOf(appStylesheets(frontendRoot)).filter((rule) =>
  ["background", "background-color"].some((property) =>
    declaredValues(rule.body, property).some((value) =>
      value.includes("--bgInkField"),
    ),
  ),
);

// Each page token the scope re-points, and the plate ink it now reads.
const repointed = [
  ...tokenRule.body.matchAll(/(--[\w-]+)\s*:[^;]*?var\((--[\w-]+)\)/g),
].map(([, token, ink]) => ({ token, ink }));

describe("the ink field's scope", () => {
  it("finds the components that paint the plate", () => {
    expect(painters.length).toBeGreaterThanOrEqual(6);
  });

  it("lists every rule that paints the plate", () => {
    const unlisted = painters.flatMap((rule) =>
      selectorList(rule.selector)
        .map(squash)
        .filter((selector) => !scope.includes(selector))
        .map((selector) => `${rule.file}: ${selector}`),
    );
    expect(unlisted, "add the selector to inkfield.css").toEqual([]);
  });

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
