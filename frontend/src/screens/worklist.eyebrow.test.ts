import { describe, expect, it } from "vitest";
import { eyebrowKeyFor } from "./worklist.eyebrow";
import type { WorklistItem } from "./worklist.queries";

function row(over: Partial<WorklistItem>): WorklistItem {
  return {
    id: "i1",
    source: "deal_at_risk",
    category: "deals_at_risk",
    actions: [],
    ...over,
  } as WorklistItem;
}

describe("the word above a queue row", () => {
  // Every overnight row sits in deals_at_risk, because that is where the
  // worklist folds it against the sibling risk row for the same deal. The night
  // knows why it picked each one, and a deal it ranked as winnable announced as
  // "Deal at risk" is the label a reader learns to distrust.
  it("names the night's own reason on a brief row", () => {
    expect(
      eyebrowKeyFor(row({ source: "brief_item", kind: "opportunity" })),
    ).toBe("worklist.signal.opportunity");
    expect(
      eyebrowKeyFor(row({ source: "brief_item", kind: "closing_soon" })),
    ).toBe("worklist.signal.closing_soon");
  });

  it("keeps the category on every other row", () => {
    expect(eyebrowKeyFor(row({ category: "customer_waiting" }))).toBe(
      "worklist.category.customer_waiting",
    );
  });

  // A signal from a newer build must not reach `t` with a key nothing
  // translates, which renders the key itself on the page.
  it("falls back to the category for a signal it does not know", () => {
    expect(
      eyebrowKeyFor(row({ source: "brief_item", kind: "invented_later" })),
    ).toBe("worklist.category.deals_at_risk");
  });

  it("falls back when the night named nothing", () => {
    expect(eyebrowKeyFor(row({ source: "brief_item" }))).toBe(
      "worklist.category.deals_at_risk",
    );
  });
});
