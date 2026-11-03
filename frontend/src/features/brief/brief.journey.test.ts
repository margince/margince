// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { BUSY_NIGHT } from "../../testing/magic.fixtures";
import { day } from "../../testing/worklist.testkit";
import { homeJourneyMarks } from "./brief.journey";

const FAILED = { source: "email", reason: "failed" } as const;
const WITHHELD = { source: "deal_suggestion", reason: "withheld" } as const;
const FOCUS = { items: [], total: 0, urgent_remaining: 0 };

describe("homeJourneyMarks", () => {
  it("marks nothing while the day is still loading", () => {
    expect(
      homeJourneyMarks({
        state: "loading",
        day: undefined,
        settled: false,
        receipt: undefined,
      }),
    ).toEqual([]);
  });

  it("marks a day that could not be read as failed, never useful", () => {
    expect(
      homeJourneyMarks({
        state: "failed",
        day: undefined,
        settled: true,
        receipt: BUSY_NIGHT,
      }),
    ).toEqual(["home:failed"]);
  });

  it("marks the day useful before the other sections answer", () => {
    expect(
      homeJourneyMarks({
        state: "ready",
        day: day(),
        settled: false,
        receipt: undefined,
      }),
    ).toEqual(["home:useful"]);
  });

  it("marks a page whose every section answered whole as complete", () => {
    expect(
      homeJourneyMarks({
        state: "ready",
        day: day({ focus: FOCUS, sources_unavailable: [WITHHELD] }),
        settled: true,
        receipt: BUSY_NIGHT,
      }),
    ).toEqual(["home:useful", "home:complete"]);
  });

  it("marks a day with a failed source as degraded, not complete", () => {
    expect(
      homeJourneyMarks({
        state: "ready",
        day: day({ focus: FOCUS, sources_unavailable: [FAILED] }),
        settled: true,
        receipt: BUSY_NIGHT,
      }),
    ).toEqual(["home:useful", "home:degraded"]);
  });

  it("marks a receipt with a failed lane as degraded, not complete", () => {
    expect(
      homeJourneyMarks({
        state: "ready",
        day: day({ focus: FOCUS }),
        settled: true,
        receipt: { ...BUSY_NIGHT, sources_unavailable: [FAILED] },
      }),
    ).toEqual(["home:useful", "home:degraded"]);
  });

  it("marks a day with no focus to lead with as degraded", () => {
    expect(
      homeJourneyMarks({
        state: "ready",
        day: day(),
        settled: true,
        receipt: BUSY_NIGHT,
      }),
    ).toEqual(["home:useful", "home:degraded"]);
  });
});
