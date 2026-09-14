import { expect, it } from "vitest";
import type { MessageKey } from "../../i18n/en";
import { en } from "../../i18n/en";
import { closedDealOf, closeReason, isSavedDeal } from "./closereviewoffer";

// The words the close stated, and whether a result is a saved deal at all.
// Both decide whether a review is offered and what it starts from, so both are
// tested apart from the dialog that calls them.

const t = ((key: MessageKey) => en[key]) as never;

it("passes a lost deal's typed reason through as written", () => {
  expect(
    closeReason({
      wonAsked: false,
      lost: "  Price.  ",
      won: "",
      detail: "",
      t,
    }),
  ).toBe("Price.");
});

it("states a won deal's picked reason by its own label", () => {
  // The reason is a PICK, not prose, so the review starts from the words the
  // reader saw rather than from the enum token underneath them.
  const said = closeReason({
    wonAsked: true,
    lost: "",
    won: "verbal",
    detail: "",
    t,
  });
  expect(said).toBe(en["deals.winReasonVerbal"]);
  // Not empty, and not the enum token: both would pass a looser assertion
  // while stating nothing a reviewer can read.
  expect(said.length).toBeGreaterThan(0);
  expect(said).not.toBe("verbal");
});

it("carries the detail where one was required", () => {
  const said = closeReason({
    wonAsked: true,
    lost: "",
    won: "other",
    detail: "  A framework deal.  ",
    t,
  });
  expect(said).toContain("A framework deal.");
});

it("states nothing for a win that named no reason", () => {
  // The ordinary win: a signed contract behind it, one click, no question
  // asked. There is nothing to hand the review.
  expect(
    closeReason({ wonAsked: true, lost: "", won: "", detail: "", t }),
  ).toBe("");
});

it("reads a saved deal apart from the error returned in its place", () => {
  expect(isSavedDeal({ id: "d-1", version: 2 })).toBe(true);
  expect(isSavedDeal({ status: 409, detail: "version skew" })).toBe(false);
  expect(isSavedDeal(null)).toBe(false);
  expect(isSavedDeal(undefined)).toBe(false);
});

it("offers nothing for a close the server recorded no occurrence for", () => {
  // An imported row, typically. There is no closing to hang answers on, which
  // is the same rule the review panel keeps.
  expect(
    closedDealOf(
      { id: "d-1", name: "D", status: "lost", version: 1 } as never,
      "Price.",
    ),
  ).toBeNull();
});

it("offers nothing for a move that was not a close", () => {
  expect(
    closedDealOf(
      {
        id: "d-1",
        name: "D",
        status: "open",
        version: 1,
        closing_occurrence_id: "c-1",
      } as never,
      "",
    ),
  ).toBeNull();
});

it("names the outcome and the closing the answers belong to", () => {
  const closed = closedDealOf(
    {
      id: "d-1",
      name: "D",
      status: "won",
      version: 1,
      closing_occurrence_id: "c-9",
    } as never,
    "They signed.",
  );
  expect(closed).toEqual({
    dealId: "d-1",
    outcome: "won",
    closingOccurrenceId: "c-9",
    reason: "They signed.",
  });
});
