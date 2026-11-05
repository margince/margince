// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  formatAnalyticsHours,
  formatAnalyticsMoney,
  formatAnalyticsNumber,
} from "./format";

describe("Analytics quantities", () => {
  it("writes whole major units with the currency sign, in each currency scale and locale", () => {
    expect(formatAnalyticsMoney(743218965, "EUR", "de")).toBe("≈7.432.190\u00a0€");
    expect(formatAnalyticsMoney(743218965, "EUR", "en")).toBe("≈€7,432,190");
    expect(formatAnalyticsMoney(7432189, "JPY", "vi")).toBe("7.432.189\u00a0¥");
    expect(formatAnalyticsMoney(1234500, "KWD", "en")).toBe("≈KWD\u00a01,235");
    expect(formatAnalyticsMoney(-123450, "EUR", "en")).toBe("≈-€1,235");
  });
  it("keeps missing, zero, and small signed values distinct", () => {
    expect(formatAnalyticsMoney(Number.NaN, "EUR", "en")).toBe("—");
    expect(formatAnalyticsMoney(0, "EUR", "en")).toBe("€0");
    expect(formatAnalyticsMoney(1, "EUR", "en")).toBe("<€1");
    expect(formatAnalyticsMoney(-1, "EUR", "en")).toBe("−(<€1)");
    expect(formatAnalyticsNumber(-0, "en")).toBe("0");
  });
  it("shows durations without decimal hours and retains a small negative sign", () => {
    expect(formatAnalyticsHours(1.7, "en")).toBe("1 hr 42 mins");
    expect(formatAnalyticsHours(-0.001, "en")).toBe("−<1 min");
    expect(formatAnalyticsHours(0, "en")).toBe("0 mins");
  });
});
