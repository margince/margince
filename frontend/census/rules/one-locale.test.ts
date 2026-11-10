// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { INTL_LOCALE } from "../../src/format/intllocale";
import { plantedFindings, srcDir } from "../tree";
import {
  findingsIn,
  intlFormatters,
  localeMethods,
  rule,
} from "./one-locale.census";

const formatters = intlFormatters();
const methods = localeMethods();
const linesIn = (name: string, lines: readonly string[]) =>
  findingsIn(name, lines.join("\n"), formatters, methods).map(
    ({ line }) => line,
  );

describe("one locale for every rendered value", () => {
  it("derives the Intl formatters it gates, reaching the two it is about", () => {
    expect(formatters).toContain("NumberFormat");
    expect(formatters).toContain("DateTimeFormat");
    expect(formatters).toContain("RelativeTimeFormat");
    expect(formatters).toContain("PluralRules");
    expect(formatters.length).toBeGreaterThanOrEqual(8);
    // Intl.Locale is a locale, not a consumer of one.
    expect(formatters).not.toContain("Locale");
  });

  it("derives the locale-sensitive methods and where each takes its locale", () => {
    // Nothing derived is filtered back out, and localeCompare takes its locale
    // second, which is why the position is derived.
    expect([...methods.entries()].sort()).toEqual([
      ["localeCompare", 1],
      ["toLocaleDateString", 0],
      ["toLocaleLowerCase", 0],
      ["toLocaleString", 0],
      ["toLocaleTimeString", 0],
      ["toLocaleUpperCase", 0],
    ]);
    expect(methods.has("supportedLocalesOf")).toBe(false);
  });

  it("sees each shape of the defect, including the ones a grep cannot", () => {
    // The fixture imports the mapping, so the argument check is what refuses
    // each line rather than the import check.
    const planted: { code: string; finding: boolean }[] = [
      {
        code: 'import { INTL_LOCALE } from "../format/format";',
        finding: false,
      },
      { code: "const a = (1234).toLocaleString();", finding: true },
      { code: "const b = new Date(x).toLocaleDateString();", finding: true },
      {
        code: "const c = new Date(x).toLocaleTimeString(undefined, {});",
        finding: true,
      },
      {
        code: "const d = new Intl.NumberFormat(locale).format(1);",
        finding: true,
      },
      {
        code: 'const e = new Intl.NumberFormat("en-US").format(1);',
        finding: true,
      },
      {
        code: "const f = Intl.DateTimeFormat(locale).format(now);",
        finding: true,
      },
      {
        code: "const h = new Intl.NumberFormat(tag).format(1);",
        finding: true,
      },
      {
        code: 'const i = new Intl.NumberFormat(INTL_LOCALE["en"]).format(1);',
        finding: true,
      },
      {
        code: 'const j = new Intl["NumberFormat"](locale).format(1);',
        finding: true,
      },
      { code: 'const m = (1234)["toLocaleString"]();', finding: true },
      { code: "const { DateTimeFormat } = Intl;", finding: false },
      {
        code: "const k = new DateTimeFormat(locale).format(now);",
        finding: true,
      },
      { code: "const NF = Intl.NumberFormat;", finding: false },
      { code: "const l = new NF(locale).format(1);", finding: true },
      { code: "let LF;", finding: false },
      { code: "LF = Intl.NumberFormat;", finding: false },
      { code: "const n = new LF(locale).format(1);", finding: true },
      { code: 'const o = a.localeCompare(b, "en");', finding: true },
      { code: "const p = a.localeCompare(b);", finding: true },
      { code: "const q = name.toLocaleLowerCase();", finding: true },
      { code: "const r = name.toLocaleUpperCase();", finding: true },
    ];
    // One line per formatter the runtime reports, so the set is covered and
    // not one name typed here.
    const beforeDerived = planted.length;
    for (const formatter of formatters) {
      planted.push({
        code: `const z = new Intl.${formatter}(locale);`,
        finding: true,
      });
    }
    const expected = planted
      .map(({ finding }, index) => (finding ? index + 1 : 0))
      .filter((line) => line > 0);
    expect(
      linesIn(
        "planted.ts",
        planted.map(({ code }) => code),
      ),
    ).toEqual(expected);
    expect(planted.length).toBeGreaterThan(beforeDerived);
  });

  it("passes the one permitted spelling, and the zone lookup next door", () => {
    expect(
      linesIn("clean.ts", [
        'import { INTL_LOCALE } from "../format/format";',
        "const a = new Intl.NumberFormat(INTL_LOCALE[locale]).format(1);",
        "const b = new Intl.DateTimeFormat(INTL_LOCALE[locale], {}).format(d);",
        "const z = Intl.DateTimeFormat().resolvedOptions().timeZone;",
        "const NumberFormat = ourOwnThing;",
        "const u = new NumberFormat(whatever);",
      ]),
    ).toEqual([]);
  });

  it("refuses a mapping the file never imported or shadows", () => {
    const read =
      "const a = new Intl.NumberFormat(INTL_LOCALE[locale]).format(1);";
    const imported = 'import { INTL_LOCALE } from "../format/format";';
    expect(
      linesIn("shadowed.ts", ['const INTL_LOCALE = { en: "en-US" };', read]),
    ).toEqual([2]);
    expect(
      linesIn("shadowedlocal.ts", [
        imported,
        'const INTL_LOCALE = { en: "en-US" };',
        read,
      ]),
    ).toEqual([3]);
    expect(
      linesIn("shadowedparam.ts", [
        imported,
        "function render(INTL_LOCALE) {",
        "  return new Intl.NumberFormat(INTL_LOCALE[locale]).format(1);",
        "}",
      ]),
    ).toEqual([3]);
    // A default import of the module does not carry the binding.
    expect(
      linesIn("wrong.ts", ['import format from "../format/format";', read]),
    ).toEqual([2]);
  });

  it("pins the mapping the whole rule is written against", () => {
    // A100: unconfigured English is en-GB, which formats unlike en-US.
    expect(INTL_LOCALE).toEqual({ de: "de-DE", en: "en-GB", vi: "vi-VN" });
  });
});

describe("the one-locale rule", () => {
  it("reports a screen, passes the collation owner and reports idle pins", () => {
    const bare = "export const n = (1234).toLocaleString();\n";
    const findings = plantedFindings(rule, [
      { path: join(srcDir, "features", "x.ts"), area: "src", text: bare },
      { path: join(srcDir, "format", "collate.ts"), area: "src", text: bare },
      { path: join(srcDir, "format", "address.ts"), area: "src", text: bare },
    ]);
    expect(findings[0]).toBe("features/x.ts:1: .toLocaleString()");
    expect(findings).not.toContain("format/collate.ts:1: .toLocaleString()");
    expect(findings).toContain(
      "format/timezone.ts: pinned, but reaches no formatter",
    );
    expect(findings).not.toContain(
      "format/address.ts: pinned, but reaches no formatter",
    );
  });
});
