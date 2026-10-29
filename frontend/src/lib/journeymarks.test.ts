// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it } from "vitest";
import {
  JOURNEY_START,
  type JourneyMark,
  journeyMarkName,
  journeyOf,
  markJourney,
} from "./journeymarks";

function marksOf(mark: JourneyMark): PerformanceEntry[] {
  return performance.getEntriesByName(journeyMarkName(mark), "mark");
}

describe("markJourney", () => {
  afterEach(() => performance.clearMarks());

  it("sets the mark once, so a remount or refetch keeps the first moment", () => {
    markJourney("home:useful");
    const first = marksOf("home:useful")[0]?.startTime;
    markJourney("home:useful");
    const marks = marksOf("home:useful");
    expect(marks).toHaveLength(1);
    expect(marks[0]?.startTime).toBe(first);
  });

  it("keeps each milestone apart", () => {
    markJourney("session");
    markJourney("gates");
    expect(marksOf("session")).toHaveLength(1);
    expect(marksOf("gates")).toHaveLength(1);
  });

  it("sets the milestone again for a journey opened after the load", () => {
    markJourney("home:useful");
    performance.mark(JOURNEY_START, { detail: "return-1" });
    markJourney("home:useful");
    markJourney("home:useful");
    expect(marksOf("home:useful").map(journeyOf)).toEqual(["", "return-1"]);
  });

  it("belongs each mark to the latest journey only", () => {
    performance.mark(JOURNEY_START, { detail: "return-1" });
    markJourney("home:complete");
    performance.mark(JOURNEY_START, { detail: "return-2" });
    expect(marksOf("home:complete").map(journeyOf)).toEqual(["return-1"]);
    markJourney("home:complete");
    expect(marksOf("home:complete").map(journeyOf)).toEqual([
      "return-1",
      "return-2",
    ]);
  });

  it("reads a mark with no journey as the document's own load", () => {
    performance.mark(journeyMarkName("gates"));
    expect(marksOf("gates").map(journeyOf)).toEqual([""]);
  });
});
