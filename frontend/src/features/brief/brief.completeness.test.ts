// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { BUSY_NIGHT } from "../../testing/magic.fixtures";
import { day } from "../../testing/worklist.testkit";
import { receiptGaps, worklistGaps } from "./brief.completeness";

const FOCUS = { items: [], total: 0, urgent_remaining: 0 };

describe("worklistGaps", () => {
  it("finds no gap in a day read whole", () => {
    expect(worklistGaps(day({ focus: FOCUS }))).toEqual([]);
  });

  it("names each source that failed", () => {
    const failed = day({
      focus: FOCUS,
      sources_unavailable: [
        { source: "email", reason: "failed" },
        { source: "calendar", reason: "failed", contributes_rows: false },
      ],
    });
    expect(worklistGaps(failed)).toEqual([
      "worklist: source email failed",
      "worklist: source calendar failed",
    ]);
  });

  it("leaves a withheld source out, because it is the reader's grant", () => {
    const withheld = day({
      focus: FOCUS,
      sources_unavailable: [{ source: "deal_suggestion", reason: "withheld" }],
    });
    expect(worklistGaps(withheld)).toEqual([]);
  });

  it("names a day with no focus, because the lead card is drawn from it", () => {
    expect(worklistGaps(day())).toEqual(["worklist: no focus to lead with"]);
  });

  it("reads a bounded day as whole: more is a floor, not a gap", () => {
    const bounded = day({
      focus: FOCUS,
      scope_truncated: true,
      counts: [
        { category: "tasks", considered: 50, shown: 25, more_available: true },
      ],
      readings: { ...day().readings, more_available: true },
    });
    expect(worklistGaps(bounded)).toEqual([]);
  });
});

describe("receiptGaps", () => {
  it("names a lane that failed and leaves a whole receipt alone", () => {
    expect(receiptGaps(BUSY_NIGHT)).toEqual([]);
    expect(
      receiptGaps({
        ...BUSY_NIGHT,
        sources_unavailable: [{ source: "automation", reason: "failed" }],
      }),
    ).toEqual(["magic: source automation failed"]);
  });
});
