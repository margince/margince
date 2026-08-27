import { describe, expect, it } from "vitest";
import { recordRoute } from "./approvalrow";

// Where the product offers to put an approved change back.
//
// The undo is the RECORD's: an effect that changes a field writes an ordinary
// update audit row, and the record's history panel reverses one of those. So
// the offer is only honest for an approval naming a record with a page — and
// this function is the whole of that rule.
describe("recordRoute", () => {
  it("routes each record kind to the screen that holds its history", () => {
    expect(recordRoute("deal", "d1")).toEqual({ screen: "deals", id: "d1" });
    expect(recordRoute("organization", "o1")).toEqual({
      screen: "companies",
      id: "o1",
    });
    expect(recordRoute("person", "p1")).toEqual({
      screen: "contacts",
      id: "p1",
    });
    expect(recordRoute("lead", "l1")).toEqual({ screen: "leads", id: "l1" });
    expect(recordRoute("project", "pr1")).toEqual({
      screen: "projects",
      id: "pr1",
    });
  });

  // An approval that names no record — a step-up, a held scheduled send —
  // changed nothing a history panel can show, so there is nothing to offer.
  it("offers nothing when the approval names no target", () => {
    expect(recordRoute(null, null)).toBeUndefined();
    expect(recordRoute(undefined, undefined)).toBeUndefined();
    expect(recordRoute("deal", null)).toBeUndefined();
    expect(recordRoute(null, "d1")).toBeUndefined();
  });

  // activity is served by the history engine and has no record page. Offering
  // it would send a reader to a screen that cannot answer, which is worse than
  // not offering: the button would promise a reversal the product cannot reach.
  it("offers nothing for a kind with no record page", () => {
    expect(recordRoute("activity", "a1")).toBeUndefined();
    expect(recordRoute("approval", "ap1")).toBeUndefined();
    expect(recordRoute("", "x1")).toBeUndefined();
  });
});
