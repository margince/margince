// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { findingsIn, mayHoldFinding, rule } from "./calendar-days-zone.census";

describe("a day count names its zone", () => {
  it.each([
    ["hours floor", "Math.floor(hours / 24);"],
    ["named hours floor", "Math.floor(hours / HOURS_PER_DAY);"],
    ["no zone", "calendarDaysBetween(a, b);"],
    ["a UTC literal", 'calendarDaysBetween(a, b, "UTC");'],
    ["a hand floor", "Math.floor(a.getTime() / 86_400_000);"],
    ["a named hand floor", "Math.floor(a / DAY_MS);"],
    ["a separated hand floor", "Math.floor(a / 86_400_000);"],
    ["an exponent floor", "Math.floor(a / 8.64e7);"],
    ["a hex floor", "Math.floor(a / 0x5265C00);"],
    ["a binary floor", "Math.floor(a / 0b101001001100101110000000000);"],
    ["an escaped call name", "calendar\\u0044aysBetween(a, b);"],
    ["an escaped day constant", "Math.floor(a / DAY\\u005fMS);"],
  ])("sees %s", (_name, body) => {
    expect(findingsIn("seed.ts", body)).toHaveLength(1);
  });

  it("skips text that holds no place for a finding", () => {
    expect(mayHoldFinding("const days = 7;")).toBe(false);
  });

  it("passes a zoned call", () => {
    expect(findingsIn("seed.ts", "calendarDaysBetween(a, b, zone);")).toEqual(
      [],
    );
  });
});

describe("the calendar-days-zone rule", () => {
  it("reports a screen, honours a live exemption and reports the idle ones", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "x.ts"),
        area: "src",
        text: 'export const d = calendarDaysBetween(a, b, "UTC");\n',
      },
      {
        path: join(srcDir, "format", "now.ts"),
        area: "src",
        text: [
          "export function formatSpan(ms: number) { return Math.floor(ms / 86_400_000); }",
          "export function formatElapsed(ms: number) { return ms; }",
        ].join("\n"),
      },
    ]);
    expect(findings).toEqual([
      "src/features/x.ts:1: calendarDaysBetween without a real zone",
      "format/format.ts:formatDuration: the elapsed-span exemption covers no floor any more",
      "format/now.ts:formatElapsed: the elapsed-span exemption covers no floor any more",
    ]);
  });
});
