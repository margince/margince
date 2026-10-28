// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it } from "vitest";
import { journeyMarkName, markJourney } from "./journeymarks";

describe("markJourney", () => {
  afterEach(() => performance.clearMarks());

  it("sets the mark once, so a remount or refetch keeps the first moment", () => {
    markJourney("home:useful");
    const first = performance.getEntriesByName(
      journeyMarkName("home:useful"),
      "mark",
    )[0]?.startTime;
    markJourney("home:useful");
    const marks = performance.getEntriesByName(
      journeyMarkName("home:useful"),
      "mark",
    );
    expect(marks).toHaveLength(1);
    expect(marks[0]?.startTime).toBe(first);
  });

  it("keeps each milestone apart", () => {
    markJourney("session");
    markJourney("gates");
    expect(
      performance.getEntriesByName(journeyMarkName("session"), "mark"),
    ).toHaveLength(1);
    expect(
      performance.getEntriesByName(journeyMarkName("gates"), "mark"),
    ).toHaveLength(1);
  });
});
