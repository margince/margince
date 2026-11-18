// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  ambiguousReadings,
  groupingSign,
  parseTypedDecimal,
  writtenDecimal,
} from "./decimalinput";

describe("parseTypedDecimal", () => {
  it.each([
    ["33,33", 33.33],
    ["33.33", 33.33],
    ["0,5", 0.5],
    [",5", 0.5],
    ["12.", 12],
    [" 1250 ", 1250],
    ["-100", -100],
    ["9.9999", 9.9999],
  ])("reads %s as %d in every locale", (typed, want) => {
    for (const locale of ["en", "de", "vi"] as const) {
      expect(parseTypedDecimal(typed, locale)).toEqual({ value: want });
    }
  });

  // One table per locale over the five forms the decision names. The reader's
  // own grouping sign before three digits is refused; the other sign is the
  // decimal one.
  it.each([
    ["en", "1.500", { value: 1.5 }],
    ["en", "12,5", { value: 12.5 }],
    ["en", "1.234,56", { refused: "grouping" }],
    ["en", "1,500", { refused: "ambiguousGrouping" }],
    ["en", "12.5", { value: 12.5 }],
    ["de", "1.500", { refused: "ambiguousGrouping" }],
    ["de", "12,5", { value: 12.5 }],
    ["de", "1.234,56", { refused: "grouping" }],
    ["de", "1,500", { value: 1.5 }],
    ["de", "12.5", { value: 12.5 }],
    ["vi", "1.500", { refused: "ambiguousGrouping" }],
    ["vi", "12,5", { value: 12.5 }],
    ["vi", "1.234,56", { refused: "grouping" }],
    ["vi", "1,500", { value: 1.5 }],
    ["vi", "12.5", { value: 12.5 }],
  ] as const)("%s reads %s as %j", (locale, typed, want) => {
    expect(parseTypedDecimal(typed, locale)).toEqual(want);
  });

  // Only three digits after the grouping sign look grouped.
  it.each(["1.50", "1.5000", "-1.5", ".500", "1500.5"])(
    "de reads %s by the decimal rule",
    (typed) => {
      expect(parseTypedDecimal(typed, "de")).toHaveProperty("value");
    },
  );

  it("refuses a signed grouped form too", () => {
    expect(parseTypedDecimal("-1.500", "de")).toEqual({
      refused: "ambiguousGrouping",
    });
  });

  it.each(["1,234.56", "1 234", "1.234.567", "1'234"])(
    "refuses %s as a grouped number instead of guessing its separator",
    (typed) => {
      expect(parseTypedDecimal(typed, "en")).toEqual({ refused: "grouping" });
    },
  );

  it.each(["", "-", "abc", "15e-1", "12€", "Infinity"])(
    "refuses %s as not a number",
    (typed) => {
      expect(parseTypedDecimal(typed, "en")).toEqual({ refused: "notNumber" });
    },
  );
});

describe("the reader's signs", () => {
  it.each([
    ["en", ","],
    ["de", "."],
    ["vi", "."],
  ] as const)("%s groups with %s", (locale, sign) => {
    expect(groupingSign(locale)).toBe(sign);
  });

  it("writes a plain decimal with the reader's sign", () => {
    expect(writtenDecimal("1.234", "de")).toBe("1,234");
    expect(writtenDecimal("1.234", "en")).toBe("1.234");
  });

  it("names both readings of an ambiguous text", () => {
    expect(ambiguousReadings("1.500", "de")).toEqual({
      whole: "1500",
      fraction: "1,5",
    });
  });
});
