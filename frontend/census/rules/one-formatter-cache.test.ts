// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./one-formatter-cache.census";
import { intlFormatters } from "./one-locale.census";

const planted = (path: string, lines: readonly string[]) => ({
  path: join(srcDir, path),
  area: "src" as const,
  text: lines.join("\n"),
});

describe("the one-formatter-cache rule", () => {
  it("refuses a formatter built in a feature, by every spelling", () => {
    const findings = plantedFindings(rule, [
      planted("features/deals/x.ts", [
        'import { INTL_LOCALE } from "../../format/format";',
        "export const a = new Intl.NumberFormat(INTL_LOCALE[locale]);",
        'export const b = new Intl["DateTimeFormat"](INTL_LOCALE[locale], {});',
        "const LF = Intl.ListFormat;",
        "export const c = new LF(INTL_LOCALE[locale]);",
        "export const d = Intl.DateTimeFormat(INTL_LOCALE[locale]);",
      ]),
    ]);
    expect(findings).toEqual([
      "src/features/deals/x.ts:2: Intl.NumberFormat",
      "src/features/deals/x.ts:3: Intl.DateTimeFormat",
      "src/features/deals/x.ts:5: Intl.LF",
      "src/features/deals/x.ts:6: Intl.DateTimeFormat",
      "src/mcp-apps/values.ts: allowed, but builds no formatter",
    ]);
  });

  it("refuses a formatter reached through a name bound to Intl", () => {
    const findings = plantedFindings(rule, [
      planted("features/mail/y.ts", [
        "const platform = Intl;",
        "export const a = new platform.DateTimeFormat(INTL_LOCALE[locale]);",
        "let later;",
        "later = platform;",
        'export const b = new later["NumberFormat"](INTL_LOCALE[locale]);',
        "const { ListFormat } = later;",
        "export const c = new ListFormat(INTL_LOCALE[locale]);",
        "export const d = new globalThis.Intl.Collator(INTL_LOCALE[locale]);",
        "export const z = platform.DateTimeFormat().resolvedOptions();",
      ]),
    ]);
    expect(findings.filter((line) => line.startsWith("src/features"))).toEqual([
      "src/features/mail/y.ts:2: Intl.DateTimeFormat",
      "src/features/mail/y.ts:5: Intl.NumberFormat",
      "src/features/mail/y.ts:7: Intl.ListFormat",
      "src/features/mail/y.ts:8: Intl.Collator",
    ]);
  });

  it("refuses the locale methods that build a formatter per call", () => {
    const findings = plantedFindings(rule, [
      planted("features/deals/z.ts", [
        'export const a = amount.toLocaleString(INTL_LOCALE[locale], { style: "currency", currency });',
        "export const b = at.toLocaleDateString(INTL_LOCALE[locale]);",
        'export const c = at.toLocaleTimeString(INTL_LOCALE[locale], { hour: "2-digit" });',
        'export const d = at["toLocaleString"](INTL_LOCALE[locale]);',
        "export const e = name.toLocaleUpperCase(INTL_LOCALE[locale]);",
      ]),
    ]);
    expect(findings.filter((line) => line.startsWith("src/features"))).toEqual([
      "src/features/deals/z.ts:1: .toLocaleString()",
      "src/features/deals/z.ts:2: .toLocaleDateString()",
      "src/features/deals/z.ts:3: .toLocaleTimeString()",
      "src/features/deals/z.ts:4: .toLocaleString()",
    ]);
  });

  it("covers every formatter the runtime reports", () => {
    const formatters = intlFormatters();
    const findings = plantedFindings(rule, [
      planted(
        "lib/y.ts",
        formatters.map((name) => `export const z = new Intl.${name}("en");`),
      ),
    ]);
    expect(
      findings.filter((line) => line.startsWith("src/lib/y.ts")),
    ).toHaveLength(formatters.length);
  });

  it("passes the cache, the zone lookup, tests, stories and allowed files", () => {
    const build = "export const a = new Intl.NumberFormat(undefined);";
    const findings = plantedFindings(rule, [
      planted("format/formatters.ts", [build]),
      planted("format/format.test.ts", [build]),
      planted("design-system/x.stories.tsx", [build]),
      planted("mcp-apps/values.ts", [build]),
      planted("format/timezone.ts", [
        "export const z = Intl.DateTimeFormat().resolvedOptions().timeZone;",
      ]),
    ]);
    expect(findings).toEqual([]);
  });
});
