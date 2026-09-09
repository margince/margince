import { describe, expect, it } from "vitest";
import {
  isLinkedOnlyFilter,
  WORKLIST_FILTER_PARAM,
  worklistFilterFrom,
  worklistLaneHref,
} from "./worklist.header";

// The narrowings a link asks for and a pill cannot.
//
// Both exist so that a count on Brief and the door beneath it read ONE filter.
// The failure they replace was silent in both directions: an unknown filter
// deliberately falls back to `all`, so a value the address could not spell
// opened the whole queue under a number describing part of it, with nothing on
// screen to say the narrowing had been dropped.

describe("the filters that arrive by link", () => {
  // The one that would have shipped the bug back. worklistFilterFrom used to
  // read the PILL list; a link-only value checked against that list is not
  // found, falls through to `all`, and the door opens everything.
  it("reads a link-only value off the address instead of falling back", () => {
    for (const filter of ["except_decisions", "changed_since_brief"] as const) {
      const params = new Map([[WORKLIST_FILTER_PARAM, filter]]);
      expect(worklistFilterFrom(params)).toBe(filter);
    }
  });

  // The behaviour that makes the above a real risk rather than a theoretical
  // one: an unknown lane is not an error, so a typo cannot be seen.
  it("still falls back to all for a lane this build does not know", () => {
    const params = new Map([[WORKLIST_FILTER_PARAM, "not_a_lane"]]);
    expect(worklistFilterFrom(params)).toBe("all");
  });

  it("keeps reading the ordinary pill lanes", () => {
    const params = new Map([[WORKLIST_FILTER_PARAM, "meetings"]]);
    expect(worklistFilterFrom(params)).toBe("meetings");
  });

  // The href and the reader are one rule: whatever the builder writes, the
  // screen must read back. Asserting the string alone would pass against a
  // builder that spelled a value the reader then discards.
  it("writes an address its own reader resolves to the same filter", () => {
    for (const filter of ["except_decisions", "changed_since_brief"] as const) {
      const href = worklistLaneHref(filter);
      const query = new URLSearchParams(href.slice(href.indexOf("?") + 1));
      const params = new Map(query.entries());

      expect(href.startsWith("#/worklist?")).toBe(true);
      expect(worklistFilterFrom(params)).toBe(filter);
    }
  });

  // Only the two link-only values name themselves in the caption, because only
  // they leave the pill row with nothing pressed. A pill lane doing so would
  // print a sentence beside the pressed pill that already says it.
  it("marks the link-only values and no others", () => {
    expect(isLinkedOnlyFilter("except_decisions")).toBe(true);
    expect(isLinkedOnlyFilter("changed_since_brief")).toBe(true);
    for (const pill of ["all", "meetings", "decisions", "system"] as const) {
      expect(isLinkedOnlyFilter(pill)).toBe(false);
    }
  });
});
