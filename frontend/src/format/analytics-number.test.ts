// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  formatAnalyticsHours,
  formatAnalyticsMoney,
  formatAnalyticsNumber,
} from "./format";

describe("Analytics quantities", () => {
  it("groups whole major units using each currency scale and locale", () => {
    expect(formatAnalyticsMoney(743218965, "EUR", "de")).toBe("≈7.432.190 EUR");
    expect(formatAnalyticsMoney(743218965, "EUR", "en")).toBe("≈7,432,190 EUR");
    expect(formatAnalyticsMoney(7432189, "JPY", "vi")).toBe("7.432.189 JPY");
    expect(formatAnalyticsMoney(1234500, "KWD", "en")).toBe("≈1,235 KWD");
    expect(formatAnalyticsMoney(-123450, "EUR", "en")).toBe("≈-1,235 EUR");
  });
  it("keeps missing, zero, and small signed values distinct", () => {
    expect(formatAnalyticsMoney(Number.NaN, "EUR", "en")).toBe("—");
    expect(formatAnalyticsMoney(0, "EUR", "en")).toBe("0 EUR");
    expect(formatAnalyticsMoney(1, "EUR", "en")).toBe("<1 EUR");
    expect(formatAnalyticsMoney(-1, "EUR", "en")).toBe("−(<1) EUR");
    expect(formatAnalyticsNumber(-0, "en")).toBe("0");
  });
  it("shows durations without decimal hours and retains a small negative sign", () => {
    expect(formatAnalyticsHours(1.7, "en")).toBe("1 hr 42 mins");
    expect(formatAnalyticsHours(-0.001, "en")).toBe("−<1 min");
    expect(formatAnalyticsHours(0, "en")).toBe("0 mins");
  });
});
