// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDate, formatMoney, formatTimeOfDay } from "./format";
import {
  dateTimeFormat,
  numberFormat,
  zonelessDateTimeFormat,
} from "./formatters";
import { setDateTimePreferences } from "./preferences";

afterEach(() => {
  vi.restoreAllMocks();
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" });
});

describe("the formatter cache", () => {
  it("builds one money formatter for a page of rows in one currency", () => {
    const built = vi.spyOn(Intl, "NumberFormat");
    for (let row = 0; row < 100; row += 1) {
      formatMoney(row * 100, "CHF", "vi");
    }
    expect(built).toHaveBeenCalledTimes(1);
  });

  it("builds the date formatters for a page of rows in one zone once", () => {
    const built = vi.spyOn(Intl, "DateTimeFormat");
    formatDate("2026-03-04T10:00:00Z", "de", "Asia/Kathmandu");
    const firstRow = built.mock.calls.length;
    expect(firstRow).toBeGreaterThan(0);
    for (let row = 1; row < 100; row += 1) {
      formatDate(
        `2026-03-04T10:${String(row % 60).padStart(2, "0")}:00Z`,
        "de",
        "Asia/Kathmandu",
      );
    }
    expect(built).toHaveBeenCalledTimes(firstRow);
  });

  it("never answers one zone with another zone's formatter", () => {
    // 00:30 on the 16th in Auckland, 05:30 on the 15th in Los Angeles.
    const instant = "2026-06-15T12:30:00Z";
    expect(formatDate(instant, "en", "Pacific/Auckland")).toBe("16/06/2026");
    expect(formatDate(instant, "en", "America/Los_Angeles")).toBe("15/06/2026");
    expect(formatDate(instant, "en", "Pacific/Auckland")).toBe("16/06/2026");
  });

  it("reads a changed clock preference on the next call", () => {
    const evening = "2026-09-13T18:05:00Z";
    setDateTimePreferences({ dateFormat: "locale", timeFormat: "24h" });
    expect(formatTimeOfDay(evening, "en", "UTC")).toBe("18:05");
    setDateTimePreferences({ dateFormat: "locale", timeFormat: "12h" });
    expect(formatTimeOfDay(evening, "en", "UTC")).toMatch(/6:05\s*pm/i);
  });

  it("keys on the options, not on the order they are written in", () => {
    const first = numberFormat("de", {
      style: "percent",
      maximumFractionDigits: 1,
    });
    expect(
      numberFormat("de", { maximumFractionDigits: 1, style: "percent" }),
    ).toBe(first);
    expect(
      numberFormat("de", {
        style: "percent",
        maximumFractionDigits: 1,
        minimumFractionDigits: undefined,
      }),
    ).toBe(first);
    expect(
      numberFormat("en", { style: "percent", maximumFractionDigits: 1 }),
    ).not.toBe(first);
  });

  it("refuses to cache a date formatter that names no zone", () => {
    // @ts-expect-error the type requires a zone; this proves the runtime does too.
    expect(() => dateTimeFormat("en", { day: "numeric" })).toThrow(RangeError);
    expect(() =>
      dateTimeFormat("en", { day: "numeric", timeZone: "" }),
    ).toThrow(RangeError);
  });

  it("builds a zoneless date formatter anew, so it reads the browser zone again", () => {
    expect(zonelessDateTimeFormat("en", { weekday: "narrow" })).not.toBe(
      zonelessDateTimeFormat("en", { weekday: "narrow" }),
    );
  });
});
