import { describe, expect, it } from "vitest";
import { NO_URL_PARAMS, type UrlParams } from "../../lib/urlstate";
import { addressFrom, DEFAULT_ADDRESS, paramsFor, VIEWS } from "./brief.view";

// The Brief's address: which view, and which week.

function params(entries: Record<string, string>): UrlParams {
  return new Map(Object.entries(entries));
}

describe("the Brief's address", () => {
  it("opens on the morning", () => {
    expect(addressFrom(NO_URL_PARAMS)).toEqual(DEFAULT_ADDRESS);
  });

  it("round-trips every view it offers", () => {
    for (const view of VIEWS) {
      expect(addressFrom(paramsFor({ view }))).toEqual({ view });
    }
    const week = { view: "weekly", week: "2026-09-07" } as const;
    expect(addressFrom(paramsFor(week))).toEqual(week);
  });

  // A link can carry an address from an older build. A value this build does
  // not know opens the default rather than a broken page.
  it("falls back rather than breaking on a value it does not know", () => {
    expect(addressFrom(params({ view: "quarterly" })).view).toBe("morning");
  });

  // Older builds had a team view. Its link opens the reader's own Brief, and
  // the next write clears what it carried.
  it("opens an old team link as the reader's own Brief, and clears it", () => {
    const old = params({ view: "weekly", scope: "team", team: "t-1" });
    expect(addressFrom(old)).toEqual({ view: "weekly" });
    expect([...paramsFor({ view: "weekly" }, old)]).toEqual([
      ["view", "weekly"],
    ]);
  });

  // The default is absent from the address, so a reader who changed nothing can
  // copy the short link and get the same page.
  it("writes only what differs from the default", () => {
    expect([...paramsFor(DEFAULT_ADDRESS)]).toEqual([]);
    expect([...paramsFor({ view: "weekly" })]).toEqual([["view", "weekly"]]);
  });
});

it("changing Brief views preserves the queue's independent address", () => {
  const queue = new Map([
    ["queue_scope", "unassigned"],
    ["owner", "colleague"],
    ["filter", "tasks"],
    ["selected", "task-123"],
  ]);
  const weekly = paramsFor({ view: "weekly", week: "2026-09-07" }, queue);
  const morning = paramsFor(DEFAULT_ADDRESS, weekly);
  expect(morning).toEqual(queue);
  expect(weekly.get("view")).toBe("weekly");
});
