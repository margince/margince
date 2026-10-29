// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { day } from "../../testing/worklist.testkit";
import { BUSY_NIGHT } from "../sign-in/magic.fixtures";
import { homeJourneyMarks } from "./brief.journey";

const FAILED = { source: "email", reason: "failed" } as const;
const WITHHELD = { source: "deal_suggestion", reason: "withheld" } as const;

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
        day: day({ sources_unavailable: [WITHHELD] }),
        settled: true,
        receipt: BUSY_NIGHT,
      }),
    ).toEqual(["home:useful", "home:complete"]);
  });

  it("marks a day with a failed source as degraded, not complete", () => {
    expect(
      homeJourneyMarks({
        state: "ready",
        day: day({ sources_unavailable: [FAILED] }),
        settled: true,
        receipt: BUSY_NIGHT,
      }),
    ).toEqual(["home:useful", "home:degraded"]);
  });

  it("marks a receipt with a failed lane as degraded, not complete", () => {
    expect(
      homeJourneyMarks({
        state: "ready",
        day: day(),
        settled: true,
        receipt: { ...BUSY_NIGHT, sources_unavailable: [FAILED] },
      }),
    ).toEqual(["home:useful", "home:degraded"]);
  });
});
