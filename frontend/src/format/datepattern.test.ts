// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it } from "vitest";
import { LOCALES } from "../i18n/locale";
import {
  datePattern,
  formatISODate,
  isoOrBlank,
  parseTypedDate,
  patternPlaceholder,
} from "./datepattern";
import { formatDate } from "./format";
import {
  type DateFormat,
  FIXED_DATE_PATTERNS,
  setDateTimePreferences,
} from "./preferences";
import { viewerZone } from "./timezone";

const LETTERS = { day: "dd", month: "mm", year: "yyyy" };

describe("the reader's pattern", () => {
  it.each([
    ["en", "locale", "06/05/2026"],
    ["de", "locale", "06.05.2026"],
    ["vi", "locale", "06/05/2026"],
    ["en", "dmy", "06.05.2026"],
    ["vi", "mdy", "05/06/2026"],
    ["de", "ymd", "2026-05-06"],
  ] as const)("%s with %s writes 6 May as %s", (locale, format, expected) => {
    expect(formatISODate("2026-05-06", datePattern(locale, format))).toBe(
      expected,
    );
  });

  it("spells the placeholder in the pattern's order", () => {
    expect(patternPlaceholder(datePattern("vi", "locale"), LETTERS)).toBe(
      "dd/mm/yyyy",
    );
    expect(patternPlaceholder(datePattern("en", "mdy"), LETTERS)).toBe(
      "mm/dd/yyyy",
    );
  });
});

describe("a typed day", () => {
  const dayFirst = datePattern("de", "locale");
  const monthFirst = datePattern("en", "mdy");

  it.each([
    ["6.5.2026", "2026-05-06"],
    ["06/05/2026", "2026-05-06"],
    [" 6-5-2026 ", "2026-05-06"],
    ["2026-05-06", "2026-05-06"],
    ["29.2.2028", "2028-02-29"],
    ["1.1.1000", "1000-01-01"],
  ])("reads %s day first as %s", (typed, expected) => {
    expect(parseTypedDate(typed, dayFirst)).toBe(expected);
  });

  it("reads the same digits month first under mdy", () => {
    expect(parseTypedDate("6.5.2026", monthFirst)).toBe("2026-06-05");
  });

  it.each([
    "",
    "6.5.26",
    "6.5/2026",
    "31.02.2026",
    "13/13/2026",
    "006.5.2026",
    "6 May 2026",
    "2026-5",
    "6.5.0026",
    "0099-09-09",
    "1.1.0999",
  ])("names no day in %j", (typed) => {
    expect(parseTypedDate(typed, dayFirst)).toBeNull();
  });
});

// The box a reader types into and the date every page prints must agree, or a
// day typed as 06/05 reads back as 05/06 on the record.
describe("the field and the page", () => {
  afterEach(() =>
    setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" }),
  );
  // The record type makes the table name every fixed value the schema has.
  const formats = ["locale", ...Object.keys(FIXED_DATE_PATTERNS)].filter(
    (key): key is DateFormat =>
      key === "locale" || Object.hasOwn(FIXED_DATE_PATTERNS, key),
  );

  it.each(formats)(
    "write a day alike under %s in every UI language",
    (format) => {
      setDateTimePreferences({ dateFormat: format, timeFormat: "locale" });
      for (const locale of LOCALES) {
        expect(formatISODate("2026-05-06", datePattern(locale, format))).toBe(
          formatDate("2026-05-06", locale, viewerZone()),
        );
      }
    },
  );
});

it.each([
  ["2026-05-06", "2026-05-06"],
  ["", ""],
  [null, ""],
  [undefined, ""],
  ["2026-05-06T09:00:00Z", ""],
  ["06/05/2026", ""],
])("takes %j into the date field as %j", (raw, expected) => {
  expect(isoOrBlank(raw)).toBe(expected);
});
