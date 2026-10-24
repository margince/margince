// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { closeDayOf } from "./closeday";

describe("closeDayOf", () => {
  const dates = {
    closed_at: "2026-08-20T12:00:00Z",
    expected_close_date: "2026-03-02",
  };

  it("is the day it closed for a won or lost deal", () => {
    expect(closeDayOf({ ...dates, status: "won" })).toBe(dates.closed_at);
    expect(closeDayOf({ ...dates, status: "lost" })).toBe(dates.closed_at);
  });

  it("is the forecast for an open deal", () => {
    expect(closeDayOf({ ...dates, status: "open" })).toBe(
      dates.expected_close_date,
    );
  });

  it("does not fall back to the forecast for a closed deal with no close", () => {
    expect(
      closeDayOf({ status: "won", expected_close_date: "2026-03-02" }),
    ).toBeUndefined();
  });
});
