// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { day as worklistDay } from "../testing/worklist.testkit";
import { outstandingCounts, pillCount } from "./worklistcounts";
import { categoryInView, viewFromAddress, viewOfFilter } from "./worklistviews";

// The client mirrors the server's view split (views.go). These hold the client's
// rules; views_mirror_test.go holds the two sides to each other.
describe("the view a filter belongs to", () => {
  it("files decisions and system under their own views and the rest under Today", () => {
    expect(viewOfFilter("decisions")).toBe("decisions");
    expect(viewOfFilter("system")).toBe("system");
    expect(viewOfFilter("tasks")).toBe("today");
    expect(viewOfFilter("upcoming")).toBe("today");
    expect(viewOfFilter("all")).toBeUndefined();
  });

  it("reads an old filter link as its view when the address names none", () => {
    expect(viewFromAddress(undefined, "decisions")).toBe("decisions");
    expect(viewFromAddress(undefined, "tasks")).toBe("today");
    expect(viewFromAddress("system", "all")).toBe("system");
    expect(viewFromAddress("nonsense", "all")).toBe("today");
  });

  it("puts every category but decisions and system in Today", () => {
    expect(categoryInView("meetings", "today")).toBe(true);
    expect(categoryInView("decisions", "today")).toBe(false);
    expect(categoryInView("decisions", "decisions")).toBe(true);
    expect(categoryInView("tasks", "system")).toBe(false);
  });
});

describe("a view's own count", () => {
  const day = worklistDay({
    counts: [
      { category: "tasks", considered: 3, shown: 1, more_available: false },
      { category: "decisions", considered: 5, shown: 0, more_available: false },
      { category: "upcoming", considered: 4, shown: 0, more_available: false },
    ],
  });

  it("never adds upcoming work to Today's outstanding total", () => {
    expect(pillCount(day, "all", "today")).toBe(3);
    expect(outstandingCounts(day).map((count) => count.category)).toEqual([
      "tasks",
      "decisions",
    ]);
  });

  it("counts only the view's own categories", () => {
    expect(pillCount(day, "all", "decisions")).toBe(5);
    expect(pillCount(day, "all", "system")).toBe(0);
  });
});
