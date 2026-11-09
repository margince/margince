// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// biome-ignore-all lint/suspicious/noTemplateCurlyInString: these strings are source fixtures, and a template literal in them is the subject.

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import {
  colourLines,
  familiesIn,
  handRolledClasses,
  rule,
  scannableSource,
} from "./conformance.census";

describe("the type-family reader", () => {
  it("reads a family in every spelling a source can carry", () => {
    for (const spelling of [
      'font-family: "Comic Sans MS";',
      "font-family: Comic Sans MS;",
      'fontFamily: "Comic Sans MS"',
      "fontFamily: 'Comic Sans MS'",
      "fontFamily: `Comic Sans MS`",
    ]) {
      expect(familiesIn(spelling)).toEqual(["Comic Sans MS"]);
    }
    // Every family of a stack is held to the rule.
    expect(familiesIn('font-family: Outfit, "DM Sans", sans-serif;')).toEqual([
      "Outfit",
      "DM Sans",
      "sans-serif",
    ]);
  });

  it("reads nothing from a token, a run-time family or inherit", () => {
    expect(familiesIn("font-family: var(--fontFamilyBody);")).toEqual([]);
    expect(familiesIn("fontFamily: `${chosenFamily}`")).toEqual([]);
    expect(familiesIn("font-family: inherit;")).toEqual([]);
    expect(familiesIn("fontFamily: `inherit`")).toEqual([]);
  });
});

describe("the comment blanking", () => {
  it("reads a colour beside a comment, and not an issue reference inside one", () => {
    const sample = [
      'import "./x.css";',
      "// the app-wide version lands with #2455",
      "/* and #2456 with it */",
      "export function Swatch() {",
      "  return (",
      '    <div className="t-caption">',
      "      {/* ... see #2463 for the app-wide version. */}",
      '      <span style={{ color: "#ff0000" }} />',
      "    </div>",
      "  );",
      "}",
    ].join("\n");
    expect(colourLines("sample.tsx", sample)).toEqual([8]);
  });

  it("reads past a CSS comment without going blind to the rule beside it", () => {
    const sample = [
      '/* see #2463, and font-family: "Comic Sans MS" is discussed there */',
      ".swatch {",
      "  color: #ff0000;",
      "}",
    ].join("\n");
    expect(colourLines("sample.css", sample)).toEqual([3]);
    expect(familiesIn(scannableSource("sample.css", sample))).toEqual([]);
  });

  it("reads past an HTML comment the same way", () => {
    const sample = [
      "<!-- #2463: the meta colour cannot read a custom property -->",
      '<meta name="theme-color" content="#0b1f17" />',
    ].join("\n");
    expect(colourLines("sample.html", sample)).toEqual([2]);
  });
});

describe("the button and card spellings", () => {
  const found = (markup: string) =>
    handRolledClasses(
      "src/x.tsx",
      parseSource("x.tsx", `export const F = () => (${markup});\n`),
    ).map((line) => line.replace(/ — .*/, ""));

  it("finds a hand-rolled btn or card on any element but an anchor", () => {
    expect(found('<div className={ok ? "btn primary" : "x"} />')).toEqual([
      "src/x.tsx:1 <div> spells the btn class by hand",
    ]);
    expect(found('<a className="btn" href="/" />')).toEqual([]);
    expect(found('<span className="iconbtn" />')).toEqual([]);
    expect(found('<section className="card-inset" />')).toEqual([
      "src/x.tsx:1 <section> spells the card class by hand",
    ]);
  });

  it("spares a card that declares a literal role other than status", () => {
    expect(found('<div className="card" role="dialog" />')).toEqual([]);
    expect(found('<div className="card" role="status" />')).toHaveLength(1);
    expect(found('<div className="card" role={role} />')).toHaveLength(1);
  });
});

describe("the conformance rule", () => {
  it("reports planted defects through its own corpus and across", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text = [
      "export const F = () => (",
      '  <p style={{ fontFamily: "Comic Sans MS", color: "#ff0000" }}>',
      "    Hello there 🟢",
      "  </p>",
      ");",
    ].join("\n");
    const findings = plantedFindings(rule, [{ path, area: "src", text }]);
    expect(findings).toContain(
      'src/features/planted.tsx: font-family "Comic Sans MS" is outside the three-family rule',
    );
    expect(findings).toContain(
      "src/features/planted.tsx:2 hard-codes a colour — read it from a token",
    );
    expect(findings).toContainEqual(
      expect.stringMatching(
        /^src\/features\/planted\.tsx:\d JSX text "Hello there/,
      ),
    );
    expect(findings).toContain(
      'src/features/planted.tsx: emoji in "Hello there 🟢"',
    );
  });
});
